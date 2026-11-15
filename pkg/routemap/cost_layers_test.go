package routemap

import "testing"

// Lapisan di atas rumus: pemicu fungsi, arti n, jenis operasi rute, dan callback yang hanya didaftarkan.
const layersFixture = `package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

var db *gorm.DB

type Item struct {
	ID   uint
	Tags []string
}

type payload struct{ Items []Item }

func main() {
	seed()
	for o := range strings.SplitSeq(os.Getenv("ORIGINS"), ",") {
		_ = o
	}
	app := fiber.New()
	app.Get("/items", listAll)
	app.Get("/items/page", listPage)
	app.Get("/items/:id", detail)
	app.Post("/items", create)
	app.Delete("/items/:id", remove)
	mux := http.NewServeMux()
	mux.HandleFunc("/raw", func(w http.ResponseWriter, r *http.Request) {
		var rows []Item
		db.Find(&rows)
		for _, a := range rows {
			for _, b := range rows {
				if a.ID == b.ID {
					_ = b
				}
			}
		}
	})
	go sweep()
}

// seed: O(n³) tetapi hanya dipanggil langsung dari main — sekali saat proses dinyalakan.
func seed() {
	var rows []Item
	db.Find(&rows)
	for _, a := range rows {
		for _, b := range rows {
			for _, c := range rows {
				if a.ID == b.ID && b.ID == c.ID {
					db.Save(&c)
				}
			}
		}
	}
}

func sweep() {
	var rows []Item
	db.Find(&rows)
	for _, r := range rows {
		db.Delete(&r)
	}
}

func listAll(c *fiber.Ctx) error {
	var rows []Item
	db.Find(&rows)
	for _, r := range rows {
		_ = r
	}
	return c.JSON(rows)
}

func listPage(c *fiber.Ctx) error {
	var rows []Item
	db.Limit(c.QueryInt("limit")).Find(&rows)
	for _, r := range rows {
		_ = r
	}
	return c.JSON(rows)
}

func detail(c *fiber.Ctx) error {
	var it Item
	db.First(&it, c.Params("id"))
	return c.JSON(it)
}

func create(c *fiber.Ctx) error {
	var p payload
	if err := c.BodyParser(&p); err != nil {
		return err
	}
	for _, it := range p.Items {
		db.Create(&it)
	}
	return nil
}

func remove(c *fiber.Ctx) error {
	return db.Delete(&Item{}, c.Params("id")).Error
}
`

func layersReport(t *testing.T) (*CostReport, []*Graph) {
	t.Helper()
	root := writeTree(t, map[string]string{"go.mod": "module demo\n\ngo 1.22\n", "main.go": layersFixture})
	graphs := Analyze(root)
	return Costs(graphs), graphs
}

func TestRouteLayersSayWhatNIs(t *testing.T) {
	rep, _ := layersReport(t)
	want := map[string][2]string{ // "METHOD path" → {op, scale}
		"GET /items":        {OpList, ScaleData},
		"GET /items/page":   {OpList, ScalePage},
		"GET /items/:id":    {OpDetail, ScaleFixed},
		"POST /items":       {OpCreate, ScaleInput},
		"DELETE /items/:id": {OpDelete, ScaleFixed},
	}
	seen := 0
	for _, r := range rep.Routes {
		w, ok := want[r.Method+" "+r.Path]
		if !ok {
			continue
		}
		seen++
		if r.Op != w[0] || r.Scale != w[1] {
			t.Errorf("%s %s: op=%s scale=%s drivers=%v, ingin op=%s scale=%s", r.Method, r.Path, r.Op, r.Scale, r.Drivers, w[0], w[1])
		}
	}
	if seen != len(want) {
		t.Fatalf("hanya %d dari %d rute ditemukan", seen, len(want))
	}
}

func TestStartupFunctionsAreNotCostSources(t *testing.T) {
	rep, graphs := layersReport(t)
	trig := map[string]FuncCost{}
	for _, f := range rep.Funcs {
		trig[f.Name] = f
	}
	for name, want := range map[string]string{"main.main": TriggerStartup, "main.seed": TriggerStartup, "main.sweep": TriggerBackground,
		"main.listAll": TriggerRequest} {
		if got := trig[name].Trigger; got != want {
			t.Errorf("%s: pemicu %q, ingin %q", name, got, want)
		}
	}
	if s := trig["main.seed"]; s.Degree != 3 || s.OwnDepth != 3 || s.Composed {
		t.Errorf("seed: loop tiga tingkat ditulis di fungsinya sendiri: degree=%d own=%d composed=%v", s.Degree, s.OwnDepth, s.Composed)
	}
	// main memanggil seed: pangkatnya warisan, bukan loop yang ditulis di main.
	if m := trig["main.main"]; m.Degree != 3 || m.OwnDepth != 0 || !m.Composed {
		t.Errorf("main: degree=%d own=%d composed=%v", m.Degree, m.OwnDepth, m.Composed)
	}
	for _, h := range Hotspots(graphs) {
		switch h.Func.Name {
		case "main.seed", "main.main":
			t.Errorf("fungsi sekali saat start tidak boleh menjadi sumber biaya: %s (%s)", h.Func.Name, h.Kind)
		}
	}
	found := false
	for _, h := range Hotspots(graphs) {
		found = found || (h.Func.Name == "main.sweep" && h.Background)
	}
	if !found {
		t.Error("goroutine dari main tetap harus dilaporkan sebagai sumber biaya latar belakang")
	}
}

// Handler yang didaftarkan sebagai func literal tidak dijalankan oleh main: biayanya milik rute, bukan main.
func TestRegisteredCallbackIsNotCostOfRegistrar(t *testing.T) {
	root := writeTree(t, map[string]string{"go.mod": "module demo\n\ngo 1.22\n", "main.go": `package main

import (
	"net/http"

	"gorm.io/gorm"
)

var db *gorm.DB

type Row struct{ ID uint }

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/rows", func(w http.ResponseWriter, r *http.Request) {
		var rows []Row
		db.Find(&rows)
		for _, x := range rows {
			db.First(&x, x.ID)
		}
	})
	_ = http.ListenAndServe(":0", mux)
}
`})
	rep := Costs(Analyze(root))
	for _, f := range rep.Funcs {
		if f.Name == "main.main" && (f.Degree != 0 || f.Queries1 != 0 || f.NPlusOne) {
			t.Errorf("main ikut menanggung biaya handler yang hanya didaftarkan: %s, Q(n) = %s", f.BigO, f.QueryFormula)
		}
	}
	ok := false
	for _, r := range rep.Routes {
		if r.Path == "/rows" {
			ok = r.NPlusOne && r.Scale == ScaleData
		}
	}
	if !ok {
		t.Errorf("rute /rows harus tetap menanggung N+1 handlernya: %+v", rep.Routes)
	}
}

// Perkiraan beban hanya menaikkan pangkat untuk tingkat loop yang memutari baris data.
func TestEstimateCountsOnlyDataLevels(t *testing.T) {
	rep, _ := layersReport(t)
	by := map[string]FuncCost{}
	for _, f := range rep.Funcs {
		by[f.Name] = f
	}
	sweep := by["main.sweep"]
	if _, q := sweep.Estimate(1000, 20); q < 1000 || q > 1010 || sweep.DataDegree != 1 || sweep.DataBigO != "O(n)" {
		t.Errorf("sweep: N+1 atas 1000 baris harus ±1001 query (data O(n)), dapat %.0f, %s", q, sweep.DataBigO)
	}
	// create memutari isi permintaan: O(n) menurut rumus, tetapi tetap terhadap baris data.
	create := by["main.create"]
	t1, _ := create.Estimate(100, 20)
	t2, _ := create.Estimate(1000000, 20)
	if create.Degree != 1 || create.DataDegree != 0 || t1 != t2 {
		t.Errorf("create: degree=%d data=%d, perkiraan %.0f vs %.0f", create.Degree, create.DataDegree, t1, t2)
	}
	if seed := by["main.seed"]; seed.DataDegree != 3 {
		t.Errorf("seed: tiga loop atas hasil query yang sama = pangkat data 3, dapat %d", seed.DataDegree)
	}
	// n³ dengan satu tingkat data: n · m²; tingkat koleksi kecil di atas dua diabaikan dan ditandai longgar.
	if got := evalMixed(poly{term{N: 3, D: 1}: 1}, 1000, 10); got != 1000*10*10 {
		t.Errorf("evalMixed satu tingkat data = %.0f", got)
	}
	if got := evalMixed(poly{term{N: 6}: 1}, 1000, 10); got != 100 {
		t.Errorf("evalMixed tanpa tingkat data = %.0f", got)
	}
	if _, loose := (poly{term{N: 6}: 1}).dataDegree(); !loose {
		t.Error("enam tingkat koleksi kecil harus ditandai longgar")
	}
	// Rumus yang ditampilkan tidak membedakan asal data.
	if got := (poly{term{N: 1, D: 1}: 2, term{N: 1}: 3, term{}: 4}).String(); got != "5n + 4" {
		t.Errorf("rumus gabungan = %q", got)
	}
}

// Fungsi bantu yang memutari parameternya ikut tumbuh hanya bila pemanggil memberinya baris data.
func TestParamLoopFollowsCaller(t *testing.T) {
	root := writeTree(t, map[string]string{"go.mod": "module demo\n\ngo 1.22\n", "main.go": `package main

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

var db *gorm.DB

type Row struct{ ID uint }

func main() {
	app := fiber.New()
	app.Get("/all", all)
	app.Post("/some", some)
}

func total(rows []Row) uint {
	var sum uint
	for _, r := range rows {
		sum += r.ID
	}
	return sum
}

func all(c *fiber.Ctx) error {
	var rows []Row
	db.Find(&rows)
	return c.JSON(total(rows))
}

func some(c *fiber.Ctx) error {
	var rows []Row
	if err := c.BodyParser(&rows); err != nil {
		return err
	}
	return c.JSON(total(rows))
}
`})
	for _, r := range Costs(Analyze(root)).Routes {
		want := map[string]int{"/all": 1, "/some": 0}[r.Path]
		if r.Degree != 1 || r.DataDegree != want {
			t.Errorf("%s: degree=%d data_degree=%d, ingin 1 dan %d", r.Path, r.Degree, r.DataDegree, want)
		}
	}
}

func TestRouteOp(t *testing.T) {
	for in, want := range map[string]string{"GET /api/todos": OpList, "GET /api/todos/:id": OpDetail, "GET /x/{id}": OpDetail,
		"POST /api/todos": OpCreate, "PATCH /api/todos/:id": OpUpdate, "DELETE /api/todos/:id": OpDelete, "ALL /ws": OpOther} {
		var m, p string
		for i := range in {
			if in[i] == ' ' {
				m, p = in[:i], in[i+1:]
			}
		}
		if got := routeOp(m, p); got != want {
			t.Errorf("%s: %s, ingin %s", in, got, want)
		}
	}
}
