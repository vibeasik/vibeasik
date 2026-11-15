package insight

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vibeasik/vibeasik/pkg/routemap"
)

const loadFixture = `package main

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

var db *gorm.DB

type Item struct{ ID uint }

func main() {
	app := fiber.New()
	app.Get("/items", listAll)
	app.Get("/items/:id", detail)
	app.Post("/items", create)
}

// listAll: N+1 atas seluruh baris — ikut tumbuh bersama data.
func listAll(c *fiber.Ctx) error {
	var rows []Item
	db.Find(&rows)
	for _, r := range rows {
		db.First(&r, r.ID)
	}
	return c.JSON(rows)
}

func detail(c *fiber.Ctx) error {
	var it Item
	db.First(&it, c.Params("id"))
	return c.JSON(it)
}

func create(c *fiber.Ctx) error {
	var it Item
	if err := c.BodyParser(&it); err != nil {
		return err
	}
	return db.Create(&it).Error
}
`

func loadReport(t *testing.T) *routemap.CostReport {
	t.Helper()
	root := t.TempDir()
	for name, src := range map[string]string{"go.mod": "module demo\n\ngo 1.22\n", "main.go": loadFixture} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return routemap.Costs(routemap.Analyze(root))
}

// Rute tulis & detail biayanya tetap berapa pun datanya; daftar ber-N+1 ikut tumbuh dan menentukan titik jenuh.
func TestProjectionGrowsOnlyWhereDataDrivesN(t *testing.T) {
	out := buildProjection(loadReport(t), LoadParams{Users: []int64{10, 1000}, Rows: []int64{100, 100000}}, nil, nil, 0)
	if !out.Supported || out.Mix != "assumed" || len(out.Scenarios) != 4 || len(out.Limits) != 2 {
		t.Fatalf("bentuk proyeksi: %+v", out)
	}
	by := map[string]RouteLoad{}
	share := 0.0
	for _, r := range out.Routes {
		by[r.Method+" "+r.Path] = r
		share += r.Share
	}
	if share < 0.999 || share > 1.001 {
		t.Errorf("porsi trafik harus berjumlah 1, dapat %.3f", share)
	}
	for _, k := range []string{"GET /items/:id", "POST /items"} {
		r := by[k]
		if r.Points[0].LatencyMs != r.Points[1].LatencyMs || r.BreakRows != 0 {
			t.Errorf("%s: biaya harus tetap terhadap data: %+v", k, r)
		}
	}
	list := by["GET /items"]
	if list.Scale != routemap.ScaleData || list.Points[0].Queries != 101 || list.Points[1].Status != LoadTimeout {
		t.Errorf("daftar ber-N+1 harus tumbuh bersama data sampai diputus batas waktu: %+v", list)
	}
	// 300 ms / 2 ms per query ≈ 150 baris: titik patahnya di antara dua skenario.
	if list.BreakRows < 100 || list.BreakRows > 200 {
		t.Errorf("titik patah daftar = %d baris", list.BreakRows)
	}
	has := map[string]bool{}
	for _, s := range list.Signals {
		has[s] = true
	}
	for _, s := range []string{"cache", "paginate", "nplus1", "db_bound", "slow_at_scale"} {
		if !has[s] {
			t.Errorf("ciri %q hilang dari daftar: %v", s, list.Signals)
		}
	}
	if out.Limits[1].MaxUsers >= out.Limits[0].MaxUsers || out.Limits[1].Bottleneck != "db" {
		t.Errorf("data lebih besar harus menurunkan kapasitas pengguna dan dibatasi database: %+v", out.Limits)
	}
	// Beban naik sebanding pengguna aktif pada data yang sama.
	a, b := out.Scenarios[0], out.Scenarios[1]
	if a.Rows != b.Rows || b.RPS != a.RPS*100 || b.DBConns <= a.DBConns {
		t.Errorf("skenario pengguna: %+v vs %+v", a, b)
	}
}

func TestProjectionUsesRealTrafficShare(t *testing.T) {
	mix := &trafficMix{requests: 1000, share: map[string]float64{routeKey("GET", "/items/{id}"): 0.9, routeKey("POST", "/items"): 0.1},
		avgMs: map[string]float64{routeKey("GET", "/items/{id}"): 12}}
	out := buildProjection(loadReport(t), LoadParams{}, mix, &measuredLatency{source: "traffic", ms: mix.avgMs}, 0)
	if out.Mix != "traffic" {
		t.Fatalf("trafik nyata harus dipakai: %s", out.Mix)
	}
	for _, r := range out.Routes {
		switch r.Method + " " + r.Path {
		case "GET /items/:id":
			if r.Share != 0.9 || r.MeasuredMs != 12 {
				t.Errorf("detail: porsi %.2f, terukur %.1f", r.Share, r.MeasuredMs)
			}
		case "GET /items":
			if r.Share != 0 {
				t.Errorf("rute tanpa trafik harus berporsi 0: %.2f", r.Share)
			}
		}
	}
}

func TestParseScenarioList(t *testing.T) {
	if got := ParseScenarioList("10, 100,abc,-5,0,1000"); len(got) != 3 || got[0] != 10 || got[2] != 1000 {
		t.Errorf("daftar skenario: %v", got)
	}
}

// Kalibrasi menemukan kembali harga yang membentuk pengukuran, membuang pencilan, dan menolak pengukuran yang
// tidak mengikuti rumus (mis. probe yang ditolak sebelum handler: semua rute sama cepatnya).
func TestCalibrationRecoversPricesAndRejectsNoise(t *testing.T) {
	var s []calSample
	for i := 0; i < 12; i++ {
		q, ticks := float64(i%5+1), float64(1000*(i+1))
		s = append(s, calSample{ms: 0.4 + 0.8*q + ticks*30e-6, ticks: ticks, queries: q})
	}
	s = append(s, calSample{ms: 2500, ticks: 500, queries: 1}) // pencilan: memanggil layanan luar
	c := calibrate("traffic", len(s), s)
	near := func(got, want float64) bool { return got > want*0.9 && got < want*1.1 }
	if !c.Applied || c.Used != 12 || !near(c.BaseMs, 0.4) || !near(c.QueryMs, 0.8) || !near(c.TickNs, 30) || c.R2 < 0.99 {
		t.Errorf("kalibrasi: %+v", c)
	}
	var flat []calSample
	for i := 0; i < 12; i++ {
		flat = append(flat, calSample{ms: 0.6 + float64(i%3)*0.05, ticks: float64(100 * (12 - i)), queries: float64(12 - i)})
	}
	if c := calibrate("deep_check", len(flat), flat); c.Applied {
		t.Errorf("pengukuran yang tidak mengikuti rumus tidak boleh dipakai: %+v", c)
	}
	if c := calibrate("deep_check", 9, flat[:3]); c.Applied || c.Note == "" {
		t.Errorf("sampel terlalu sedikit: %+v", c)
	}
}

// Harga hasil kalibrasi menggantikan bawaan, kecuali yang diisi pengguna sendiri.
func TestProjectionAppliesCalibration(t *testing.T) {
	rep := loadReport(t)
	ms := map[string]float64{}
	p := LoadParams{}.withDefaults()
	// Fixture hanya punya dua rute berbiaya tetap; cukupkan sampel dengan rute tiruan lewat buildProjection langsung.
	for _, r := range rep.Routes {
		if r.DataDegree == 0 {
			tk, q := r.Estimate(1, p.SmallItems)
			ms[routeKey(r.Method, r.Path)] = 0.2 + 1.5*q + tk*40e-6
		}
	}
	out := buildProjection(rep, LoadParams{}, nil, &measuredLatency{source: "deep_check", ms: ms}, 0)
	if out.Calibration.Applied || out.Calibration.Samples != len(ms) || out.Params.QueryMs != 2 {
		t.Errorf("dua rute terukur belum cukup untuk kalibrasi: %+v", out.Calibration)
	}
	for _, r := range out.Routes {
		if want := ms[routeKey(r.Method, r.Path)]; want > 0 && r.MeasuredMs == 0 {
			t.Errorf("%s %s: latensi terukur tidak ditampilkan", r.Method, r.Path)
		}
	}
}

// Rute terukur dijangkarkan: pada CurrentRows perkiraan sama dengan pengukuran, dan pertumbuhannya mengikuti rumus.
func TestMeasuredRouteIsAnchored(t *testing.T) {
	rep := loadReport(t)
	ms := map[string]float64{routeKey("GET", "/items"): 40, routeKey("GET", "/items/:id"): 3}
	p := LoadParams{Rows: []int64{100, 1000}, CurrentRows: 100}
	out := buildProjection(rep, p, nil, &measuredLatency{source: "deep_check", ms: ms}, 0)
	if out.Anchored != 2 {
		t.Fatalf("rute dijangkarkan: %d", out.Anchored)
	}
	for _, r := range out.Routes {
		switch r.Method + " " + r.Path {
		case "GET /items": // N+1: 10× baris → ±10× waktu, berangkat dari 40 ms terukur
			if a, b := r.Points[0].LatencyMs, r.Points[1].LatencyMs; a < 39.9 || a > 40.1 || b < 350 || b > 450 || r.Anchor >= 1 {
				t.Errorf("daftar: %.1f → %.1f ms, jangkar %.3f", a, b, r.Anchor)
			}
		case "GET /items/:id":
			if a, b := r.Points[0].LatencyMs, r.Points[1].LatencyMs; a < 2.99 || a > 3.01 || a != b {
				t.Errorf("detail: %.2f → %.2f ms", a, b)
			}
		case "POST /items":
			if r.Anchor != 0 {
				t.Errorf("rute tanpa pengukuran tidak boleh dijangkarkan: %.2f", r.Anchor)
			}
		}
	}
}
