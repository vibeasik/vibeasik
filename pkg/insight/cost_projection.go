package insight

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
	"github.com/vibeasik/vibeasik/pkg/routemap"
	"github.com/vibeasik/vibeasik/pkg/traffic"
)

// Proyeksi beban menerjemahkan rumus biaya menjadi beban server, lapis demi lapis:
//
//	1. kode      — T(n) tick & Q(n) query tiap rute (routemap, statis dari AST);
//	2. fungsi    — pemicunya: per permintaan, latar belakang, atau sekali saat proses dinyalakan;
//	3. guna      — arti n: baris data tersimpan, satu halaman, isi permintaan, berkas, koleksi di memori;
//	4. pemakaian — jenis operasi (daftar/detail/tulis) dan porsi trafiknya (asumsi, atau trafik nyata bila ada);
//	5. proyeksi  — pengguna aktif × permintaan per menit × biaya per permintaan pada ukuran data tertentu.
//
// Hasilnya perkiraan orde besaran untuk membandingkan skenario dan menemukan titik jenuh, bukan pengukuran: satu
// tick dan satu query diberi harga rata-rata (LoadParams), dan porsi trafik tiap rute diasumsikan kecuali proyek
// punya data trafik nyata.

// LoadParams = asumsi proyeksi; nilai 0 diganti bawaan.
type LoadParams struct {
	Users         []int64 `json:"users"`            // skenario pengguna aktif bersamaan
	Rows          []int64 `json:"rows"`             // skenario jumlah baris data tersimpan
	ReqPerUserMin float64 `json:"req_per_user_min"` // permintaan per pengguna aktif per menit
	SmallItems    float64 `json:"small_items"`      // ukuran koleksi selain baris data (halaman, isi permintaan, memori)
	BaseMs        float64 `json:"base_ms"`          // ongkos dasar per permintaan (router, middleware, serialisasi)
	TickNs        float64 `json:"tick_ns"`          // harga satu tick (nanodetik CPU)
	QueryMs       float64 `json:"query_ms"`         // harga satu query berindeks (milidetik)
	RowUs         float64 `json:"row_us"`           // harga membaca & mengirim satu baris data (mikrodetik)
	FileMs        float64 `json:"file_ms"`          // harga satu akses berkas (milidetik)
	NetMs         float64 `json:"net_ms"`           // harga satu panggilan keluar: HTTP, email, antrean (milidetik)
	ExecMs        float64 `json:"exec_ms"`          // harga menjalankan satu proses lain (milidetik)
	CurrentRows   float64 `json:"current_rows"`     // baris data saat latensi diukur (jangkar rute terukur)
	Cores         float64 `json:"cores"`            // inti CPU server aplikasi
	DBConns       float64 `json:"db_conns"`         // koneksi database yang tersedia (pool)
	BudgetMs      float64 `json:"budget_ms"`        // batas waktu respons yang masih nyaman
	TimeoutMs     float64 `json:"timeout_ms"`       // batas waktu permintaan diputus
}

func (p LoadParams) withDefaults() LoadParams {
	def := func(v *float64, d float64) {
		if *v <= 0 {
			*v = d
		}
	}
	if len(p.Users) == 0 {
		p.Users = []int64{10, 100, 1000, 10000}
	}
	if len(p.Rows) == 0 {
		p.Rows = []int64{1000, 100000, 1000000}
	}
	def(&p.ReqPerUserMin, 6)
	def(&p.SmallItems, 20)
	def(&p.BaseMs, 0.5)
	def(&p.TickNs, 50)
	def(&p.QueryMs, 2)
	def(&p.RowUs, 20)
	def(&p.FileMs, 0.05)
	def(&p.NetMs, 150)
	def(&p.ExecMs, 40)
	def(&p.CurrentRows, 1000)
	def(&p.Cores, 2)
	def(&p.DBConns, 20)
	def(&p.BudgetMs, 300)
	def(&p.TimeoutMs, 30000)
	sort.Slice(p.Users, func(i, j int) bool { return p.Users[i] < p.Users[j] })
	sort.Slice(p.Rows, func(i, j int) bool { return p.Rows[i] < p.Rows[j] })
	return p
}

const maxScenarios = 6

// ParseScenarioList membaca daftar skenario "10,100,1000" (paling banyak 6 angka positif; yang tidak sah dilewati).
func ParseScenarioList(s string) []int64 {
	var out []int64
	for _, part := range strings.Split(s, ",") {
		var v int64
		if _, err := fmt.Sscan(strings.TrimSpace(part), &v); err == nil && v > 0 && v <= maxBreakRows && len(out) < maxScenarios {
			out = append(out, v)
		}
	}
	return out
}

// Status beban.
const (
	LoadOK      = "ok"      // di bawah 60% kapasitas / di bawah batas nyaman
	LoadTight   = "tight"   // 60–100% kapasitas
	LoadOver    = "over"    // melewati kapasitas
	LoadSlow    = "slow"    // respons melewati batas nyaman
	LoadTimeout = "timeout" // respons melewati batas waktu permintaan
)

// LoadPoint = biaya satu permintaan sebuah rute pada jumlah baris data tertentu.
type LoadPoint struct {
	Rows      int64   `json:"rows"`
	Queries   float64 `json:"queries"`
	CPUMs     float64 `json:"cpu_ms"`
	DBMs      float64 `json:"db_ms"`
	IOMs      float64 `json:"io_ms"` // menunggu berkas, panggilan keluar, proses lain (tidak memakai CPU/koneksi DB)
	LatencyMs float64 `json:"latency_ms"`
	Status    string  `json:"status"` // ok | slow | timeout
}

// RouteLoad = satu rute pada semua lapisan.
type RouteLoad struct {
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	Handler      string   `json:"handler"`
	Op           string   `json:"op"`
	Scale        string   `json:"scale"`
	Drivers      []string `json:"drivers,omitempty"`
	BigO         string   `json:"big_o"`
	QueryFormula string   `json:"query_formula"`
	// DataBigO: pertumbuhan terhadap baris data saja (yang dipakai proyeksi); BigO = kasus terburuk seragam.
	DataBigO string `json:"data_big_o"`
	NPlusOne bool   `json:"n_plus_one"`
	Composed bool   `json:"composed,omitempty"`
	// Loose: perkiraan tidak bisa dipercaya dari kode saja (lebih dari dua tingkat koleksi kecil bersarang);
	// rute ini tidak dihitung ke beban & kapasitas server.
	Loose bool    `json:"loose,omitempty"`
	Share float64 `json:"share"` // porsi trafik (0–1)
	// MeasuredMs: latensi rata-rata nyata dari data trafik (0 = belum ada), pembanding perkiraan.
	MeasuredMs float64 `json:"measured_ms,omitempty"`
	// Anchor: rute terukur dijangkarkan — perkiraan dikalikan Anchor agar sama dengan latensi terukur pada
	// CurrentRows; rumus hanya menentukan pertumbuhannya. 0 = tidak terukur, memakai harga perkiraan.
	Anchor float64     `json:"anchor,omitempty"`
	Points []LoadPoint `json:"points"`
	// BreakRows: jumlah baris data saat respons mulai melewati batas nyaman (0 = tidak dalam rentang 1 miliar
	// baris; 1 = sudah lambat tanpa data).
	BreakRows int64 `json:"break_rows"`
	// Signals: ciri yang relevan untuk saran arsitektur — cache (bacaan ber-query), paginate (daftar yang ikut
	// tumbuh bersama data), nplus1, db_bound / cpu_bound (penentu waktu respons), slow_at_scale.
	Signals []string `json:"signals,omitempty"`
}

// LoadScenario = beban seluruh server pada satu pasangan (pengguna aktif, baris data).
type LoadScenario struct {
	Users      int64   `json:"users"`
	Rows       int64   `json:"rows"`
	RPS        float64 `json:"rps"`
	QPS        float64 `json:"qps"`
	CPUCores   float64 `json:"cpu_cores"` // inti CPU yang terpakai rata-rata
	DBConns    float64 `json:"db_conns"`  // koneksi database yang sibuk rata-rata
	CPUUse     float64 `json:"cpu_use"`   // CPUCores / Cores
	DBUse      float64 `json:"db_use"`    // DBConns / pool
	AvgMs      float64 `json:"avg_ms"`    // latensi rata-rata tertimbang porsi trafik
	SlowRoutes int     `json:"slow_routes"`
	Status     string  `json:"status"`     // ok | tight | over
	Bottleneck string  `json:"bottleneck"` // cpu | db | "" (belum ada)
}

// LoadLimit = pengguna aktif yang masih tertampung satu server pada jumlah baris data tertentu.
type LoadLimit struct {
	Rows       int64  `json:"rows"`
	MaxUsers   int64  `json:"max_users"`
	ByCPU      int64  `json:"by_cpu"`
	ByDB       int64  `json:"by_db"`
	Bottleneck string `json:"bottleneck"`
}

// StartupCost = fungsi berpangkat tinggi yang TIDAK menambah beban per pengguna: hanya berjalan saat proses
// dinyalakan atau sebagai skrip sekali jalan.
type StartupCost struct {
	Name    string `json:"name"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	BigO    string `json:"big_o"`
	Trigger string `json:"trigger"`
	Scale   string `json:"scale"`
}

// LoadProjection = jawaban endpoint proyeksi beban.
type LoadProjection struct {
	Supported bool       `json:"supported"`
	Params    LoadParams `json:"params"`
	// Mix: assumed = porsi trafik dibagi menurut jenis operasi; traffic = dari data trafik nyata proyek.
	Mix         string `json:"mix"`
	MixRequests int64  `json:"mix_requests,omitempty"`
	// Calibration: pencocokan harga tick/query dengan latensi terukur, dan apakah hasilnya dipakai.
	Calibration  Calibration    `json:"calibration"`
	Scenarios    []LoadScenario `json:"scenarios"`
	Limits       []LoadLimit    `json:"limits"`
	Routes       []RouteLoad    `json:"routes"`
	TotalRoutes  int            `json:"total_routes"`
	FuncTriggers map[string]int `json:"func_triggers"` // lapisan fungsi: jumlah fungsi per pemicu
	RouteScales  map[string]int `json:"route_scales"`  // lapisan guna: jumlah rute per arti n
	RouteOps     map[string]int `json:"route_ops"`     // lapisan pemakaian: jumlah rute per jenis operasi
	LooseRoutes  int            `json:"loose_routes"`  // rute yang tidak bisa diperkirakan dari kode saja
	Anchored     int            `json:"anchored"`      // rute yang dijangkarkan ke latensi terukur
	Startup      []StartupCost  `json:"startup"`
	Assumptions  []string       `json:"assumptions"`
}

const (
	defaultLoadRoutes = 30
	maxLoadRoutes     = 200
	maxStartupCosts   = 10
	maxBreakRows      = int64(1e9)
)

// opWeights: porsi trafik bawaan per jenis operasi — aplikasi CRUD umumnya ±2/3 baca, 1/3 tulis.
var opWeights = map[string]float64{
	routemap.OpList: 0.35, routemap.OpDetail: 0.30, routemap.OpCreate: 0.12, routemap.OpUpdate: 0.15, routemap.OpDelete: 0.05, routemap.OpOther: 0.03,
}

// rawPoint: biaya satu permintaan rute pada `rows` baris data, sebelum jangkar & batas waktu.
func (p LoadParams) rawPoint(c routemap.Cost, rows int64) LoadPoint {
	ticks, queries := c.Estimate(float64(rows), p.SmallItems)
	io := c.EstimateIO(float64(rows), p.SmallItems)
	pt := LoadPoint{Rows: rows, Queries: queries, CPUMs: p.BaseMs + ticks*p.TickNs/1e6, DBMs: io.DB * p.QueryMs,
		IOMs: io.File*p.FileMs + io.Net*p.NetMs + io.Exec*p.ExecMs}
	switch c.Scale {
	case routemap.ScaleData: // membaca & mengirim baris yang ikut tumbuh bersama data
		pt.DBMs += float64(rows) * p.RowUs / 1e3
	case routemap.ScalePage:
		pt.DBMs += math.Min(float64(rows), p.SmallItems) * p.RowUs / 1e3
	}
	pt.LatencyMs = pt.CPUMs + pt.DBMs + pt.IOMs
	return pt
}

const minAnchor, maxAnchor = 0.01, 100

// anchorFor: pengali agar perkiraan pada CurrentRows sama dengan latensi terukur (dibatasi 0,01–100×).
func (p LoadParams) anchorFor(c routemap.Cost, measuredMs float64) float64 {
	est := p.rawPoint(c, int64(p.CurrentRows)).LatencyMs
	if measuredMs <= 0 || est <= 0 {
		return 0
	}
	return math.Min(maxAnchor, math.Max(minAnchor, measuredMs/est))
}

// point: biaya satu permintaan rute pada `rows` baris data; anchor > 0 menskalakan ke latensi terukur.
func (p LoadParams) point(c routemap.Cost, rows int64, anchor float64) LoadPoint {
	pt := p.rawPoint(c, rows)
	if anchor > 0 {
		pt.CPUMs, pt.DBMs, pt.IOMs, pt.LatencyMs = pt.CPUMs*anchor, pt.DBMs*anchor, pt.IOMs*anchor, pt.LatencyMs*anchor
	}
	switch {
	case pt.LatencyMs >= p.TimeoutMs:
		pt.Status = LoadTimeout
		// Permintaan diputus di batas waktu: beban yang ditanggung server berhenti di sana.
		f := p.TimeoutMs / pt.LatencyMs
		pt.CPUMs, pt.DBMs, pt.IOMs, pt.Queries, pt.LatencyMs = pt.CPUMs*f, pt.DBMs*f, pt.IOMs*f, pt.Queries*f, p.TimeoutMs
	case pt.LatencyMs > p.BudgetMs:
		pt.Status = LoadSlow
	default:
		pt.Status = LoadOK
	}
	return pt
}

// breakRows: baris data terkecil saat respons melewati batas nyaman (pencarian biner; biaya naik monoton).
func (p LoadParams) breakRows(c routemap.Cost, anchor float64) int64 {
	if p.point(c, 1, anchor).Status != LoadOK {
		return 1
	}
	if p.point(c, maxBreakRows, anchor).Status == LoadOK {
		return 0
	}
	lo, hi := int64(1), maxBreakRows
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		if p.point(c, mid, anchor).Status == LoadOK {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi
}

var routeParamRe = regexp.MustCompile(`(^|/)(?::[^/]+|\{[^/]+\}|<[^/]+>|\*[^/]*)`)

// routeKey menyamakan bentuk parameter path (":id", "{id}", "<id>") agar rute kode cocok dengan rute trafik.
func routeKey(method, path string) string {
	return strings.ToUpper(method) + " " + routeParamRe.ReplaceAllString(strings.TrimRight(path, "/"), "$1:p")
}

type trafficMix struct {
	share    map[string]float64
	avgMs    map[string]float64
	requests int64
}

// loadTrafficMix: porsi & latensi nyata tiap rute dari sumber trafik proyek (7 hari terakhir). nil bila proyek
// belum punya data trafik yang cukup.
func loadTrafficMix(db *gorm.DB, projectID uint) *trafficMix {
	var srcs []models.TrafficSource
	if db.Where("project_id = ? AND total_events > 0", projectID).Find(&srcs).Error != nil || len(srcs) == 0 {
		return nil
	}
	mix := &trafficMix{share: map[string]float64{}, avgMs: map[string]float64{}}
	count := map[string]float64{}
	for _, src := range srcs {
		rep, err := traffic.BuildReport(db, src, traffic.ParseWindow("7d"))
		if err != nil || rep == nil {
			continue
		}
		mix.requests += rep.Totals.Requests
		for _, r := range rep.Routes {
			k := routeKey(r.Method, r.Route)
			mix.avgMs[k] = (mix.avgMs[k]*count[k] + r.AvgMS*float64(r.Count)) / math.Max(1, count[k]+float64(r.Count))
			count[k] += float64(r.Count)
		}
	}
	if mix.requests < 100 {
		return nil
	}
	for k, c := range count {
		mix.share[k] = c / float64(mix.requests)
	}
	return mix
}

// assumedShares membagi porsi trafik menurut jenis operasi, rata di antara rute sejenis.
func assumedShares(routes []routemap.RouteCost) []float64 {
	perOp := map[string]int{}
	for _, r := range routes {
		perOp[r.Op]++
	}
	total := 0.0
	for op := range perOp {
		total += opWeights[op]
	}
	out := make([]float64, len(routes))
	if total == 0 {
		return out
	}
	for i, r := range routes {
		out[i] = opWeights[r.Op] / total / float64(perOp[r.Op])
	}
	return out
}

func routeSignals(r routemap.RouteCost, pts []LoadPoint, breakRows int64) []string {
	var out []string
	read := r.Op == routemap.OpList || r.Op == routemap.OpDetail
	if read && r.Queries1 > 0 {
		out = append(out, "cache")
	}
	if r.Op == routemap.OpList && r.Scale == routemap.ScaleData {
		out = append(out, "paginate")
	}
	if r.Loose {
		out = append(out, "measure")
	}
	if r.NPlusOne {
		out = append(out, "nplus1")
	}
	if len(pts) > 0 {
		last := pts[len(pts)-1]
		switch {
		case last.IOMs > last.DBMs && last.IOMs > last.CPUMs:
			out = append(out, "external")
		case last.DBMs > last.CPUMs:
			out = append(out, "db_bound")
		case last.CPUMs > 1:
			out = append(out, "cpu_bound")
		}
	}
	if breakRows > 0 {
		out = append(out, "slow_at_scale")
	}
	return out
}

func capStatus(use float64) string {
	switch {
	case use >= 1:
		return LoadOver
	case use >= 0.6:
		return LoadTight
	}
	return LoadOK
}

// ProjectLoad menghitung proyeksi beban proyek. limit = jumlah rute yang dikembalikan (terberat dulu).
func ProjectLoad(db *gorm.DB, projectID uint, p LoadParams, limit int) (*LoadProjection, error) {
	a, err := analyze(db, projectID)
	if err != nil {
		return nil, err
	}
	mix := loadTrafficMix(db, projectID)
	var measured *measuredLatency
	if mix != nil {
		measured = &measuredLatency{source: "traffic", ms: mix.avgMs}
	} else {
		measured = loadDeepCheckLatency(db, projectID)
	}
	return buildProjection(a.rep, p, mix, measured, limit), nil
}

func buildProjection(rep *routemap.CostReport, p LoadParams, mix *trafficMix, measured *measuredLatency, limit int) *LoadProjection {
	asked := p // harga yang diisi pengguna tidak ditimpa kalibrasi
	p = p.withDefaults()
	cal := calibrate("", 0, nil)
	if measured != nil {
		var samples []calSample
		hit := 0
		for _, r := range rep.Routes {
			ms := measured.ms[routeKey(r.Method, r.Path)]
			if ms <= 0 {
				continue
			}
			hit++
			// Layak dicocokkan: biaya tidak bergantung baris data dan tanpa I/O luar (harganya tidak diketahui).
			if io := r.EstimateIO(1, p.SmallItems); r.DataDegree == 0 && !r.Loose && io.Net == 0 && io.Exec == 0 {
				t, _ := r.Estimate(1, p.SmallItems)
				samples = append(samples, calSample{ms: ms, ticks: t, queries: io.DB})
			}
		}
		cal = calibrate(measured.source, hit, samples)
		if cal.Applied {
			if asked.BaseMs <= 0 {
				p.BaseMs = cal.BaseMs
			}
			if asked.QueryMs <= 0 && cal.QueryMs > 0 {
				p.QueryMs = cal.QueryMs
			}
			if asked.TickNs <= 0 && cal.TickNs > 0 {
				p.TickNs = cal.TickNs
			}
		}
	}
	if limit <= 0 {
		limit = defaultLoadRoutes
	}
	limit = min(limit, maxLoadRoutes)
	out := &LoadProjection{Supported: len(rep.Routes) > 0, Params: p, Mix: "assumed", Scenarios: []LoadScenario{}, Limits: []LoadLimit{},
		Routes: []RouteLoad{}, TotalRoutes: len(rep.Routes), FuncTriggers: map[string]int{}, RouteScales: map[string]int{}, RouteOps: map[string]int{},
		Startup: []StartupCost{}, Calibration: cal}

	for _, f := range rep.Funcs {
		trig := f.Trigger
		if trig == "" {
			trig = "unreached"
		}
		out.FuncTriggers[trig]++
		if (f.Trigger == routemap.TriggerStartup || f.Trigger == routemap.TriggerTool) && f.Degree >= 2 && len(out.Startup) < maxStartupCosts {
			out.Startup = append(out.Startup, StartupCost{Name: f.Name, File: f.File, Line: f.Line, BigO: f.BigO, Trigger: f.Trigger, Scale: f.Scale})
		}
	}

	shares := assumedShares(rep.Routes)
	if mix != nil {
		matched := 0.0
		real := make([]float64, len(rep.Routes))
		for i, r := range rep.Routes {
			real[i] = mix.share[routeKey(r.Method, r.Path)]
			matched += real[i]
		}
		// Trafik nyata dipakai bila sebagian besar permintaannya mengenai rute yang dikenali dari kode.
		if matched >= 0.5 {
			for i := range real {
				real[i] /= matched
			}
			shares, out.Mix, out.MixRequests = real, "traffic", mix.requests
		}
	}

	loads := make([]RouteLoad, len(rep.Routes))
	for i, r := range rep.Routes {
		out.RouteScales[r.Scale]++
		out.RouteOps[r.Op]++
		rl := RouteLoad{Method: r.Method, Path: r.Path, Handler: r.Handler, Op: r.Op, Scale: r.Scale, Drivers: r.Drivers, BigO: r.BigO,
			QueryFormula: r.QueryFormula, DataBigO: r.DataBigO, NPlusOne: r.NPlusOne, Composed: r.Composed, Loose: r.Loose, Share: shares[i],
			Points: make([]LoadPoint, 0, len(p.Rows))}
		if r.Loose {
			out.LooseRoutes++
		}
		if measured != nil {
			rl.MeasuredMs = math.Round(measured.ms[routeKey(r.Method, r.Path)]*100) / 100
		}
		rl.Anchor = p.anchorFor(r.Cost, rl.MeasuredMs)
		if rl.Anchor > 0 {
			out.Anchored++
		}
		for _, rows := range p.Rows {
			rl.Points = append(rl.Points, p.point(r.Cost, rows, rl.Anchor))
		}
		rl.BreakRows = p.breakRows(r.Cost, rl.Anchor)
		rl.Signals = routeSignals(r, rl.Points, rl.BreakRows)
		loads[i] = rl
	}

	perUserRPS := p.ReqPerUserMin / 60
	for ri, rows := range p.Rows {
		var cpuSec, dbSec, queries, lat float64 // per permintaan rata-rata tertimbang
		slow := 0
		for _, rl := range loads {
			if rl.Loose {
				continue // perkiraannya tidak bisa dipercaya: tidak dihitung ke beban server
			}
			pt := rl.Points[ri]
			cpuSec += rl.Share * pt.CPUMs / 1e3
			dbSec += rl.Share * pt.DBMs / 1e3
			queries += rl.Share * pt.Queries
			lat += rl.Share * pt.LatencyMs
			if pt.Status != LoadOK {
				slow++
			}
		}
		for _, users := range p.Users {
			rps := float64(users) * perUserRPS
			sc := LoadScenario{Users: users, Rows: rows, RPS: rps, QPS: rps * queries, CPUCores: rps * cpuSec, DBConns: rps * dbSec, AvgMs: lat, SlowRoutes: slow}
			sc.CPUUse, sc.DBUse = sc.CPUCores/p.Cores, sc.DBConns/p.DBConns
			sc.Status = capStatus(math.Max(sc.CPUUse, sc.DBUse))
			if sc.Status != LoadOK {
				sc.Bottleneck = "cpu"
				if sc.DBUse > sc.CPUUse {
					sc.Bottleneck = "db"
				}
			}
			out.Scenarios = append(out.Scenarios, sc)
		}
		lim := LoadLimit{Rows: rows, ByCPU: maxBreakRows, ByDB: maxBreakRows}
		if cpuSec > 0 {
			lim.ByCPU = int64(math.Min(float64(maxBreakRows), p.Cores/cpuSec/perUserRPS))
		}
		if dbSec > 0 {
			lim.ByDB = int64(math.Min(float64(maxBreakRows), p.DBConns/dbSec/perUserRPS))
		}
		lim.MaxUsers, lim.Bottleneck = lim.ByCPU, "cpu"
		if lim.ByDB < lim.ByCPU {
			lim.MaxUsers, lim.Bottleneck = lim.ByDB, "db"
		}
		out.Limits = append(out.Limits, lim)
	}

	// Terberat dulu: sumbangan beban (porsi × waktu) pada data terbesar, lalu waktu per permintaan. Rute yang
	// perkiraannya tidak bisa dipercaya ditaruh di belakang.
	last := len(p.Rows) - 1
	sort.SliceStable(loads, func(i, j int) bool {
		if loads[i].Loose != loads[j].Loose {
			return !loads[i].Loose
		}
		a, b := loads[i].Share*loads[i].Points[last].LatencyMs, loads[j].Share*loads[j].Points[last].LatencyMs
		if a != b {
			return a > b
		}
		return loads[i].Points[last].LatencyMs > loads[j].Points[last].LatencyMs
	})
	out.Routes = loads[:min(limit, len(loads))]

	mixNote := "Porsi trafik tiap rute DIASUMSIKAN: dibagi menurut jenis operasi (daftar 35%, detail 30%, buat 12%, ubah 15%, hapus 5%, lainnya 3%) lalu rata di antara rute sejenis. Pasang SDK trafik agar porsi nyata dipakai."
	if out.Mix == "traffic" {
		mixNote = fmt.Sprintf("Porsi trafik tiap rute diambil dari data trafik nyata 7 hari terakhir (%d permintaan).", out.MixRequests)
	}
	out.Assumptions = []string{
		mixNote,
		fmt.Sprintf("Tiap pengguna aktif mengirim %.3g permintaan per menit.", p.ReqPerUserMin),
		fmt.Sprintf("Ongkos dasar per permintaan = %.3g ms, satu tick = %.3g ns CPU, satu query = %.3g ms, satu baris data terbaca & terkirim = %.3g µs, satu akses berkas = %.3g ms, satu panggilan keluar (HTTP, email, antrean) = %.3g ms, satu proses lain = %.3g ms.", p.BaseMs, p.TickNs, p.QueryMs, p.RowUs, p.FileMs, p.NetMs, p.ExecMs),
		fmt.Sprintf("%d rute punya latensi terukur dan dijangkarkan: perkiraannya dikalikan agar sama dengan pengukuran pada %.0f baris data (anggapan jumlah baris saat diukur); rumus hanya menentukan pertumbuhannya.", out.Anchored, p.CurrentRows),
		"Kalibrasi: " + cal.Note,
		fmt.Sprintf("Hanya tingkat loop yang memutari hasil query tanpa Limit yang dihitung sebanyak baris data; tingkat lain (satu halaman, isi permintaan, koleksi di memori) dihitung %.3g putaran, paling banyak dua tingkat. Rumus Big-O di tabel tetap kasus terburuk (semua tingkat n).", p.SmallItems),
		fmt.Sprintf("%d rute punya lebih dari dua tingkat koleksi kecil bersarang (pangkat hasil perkalian lintas fungsi): perkiraannya tidak bisa dipercaya dari kode saja, jadi tidak dihitung ke beban & kapasitas. Ukur rute itu dengan trafik nyata.", out.LooseRoutes),
		fmt.Sprintf("Kapasitas satu server: %.3g inti CPU dan %.3g koneksi database; permintaan diputus setelah %.3g detik, dianggap lambat di atas %.3g ms.", p.Cores, p.DBConns, p.TimeoutMs/1e3, p.BudgetMs),
		"Fungsi yang hanya berjalan saat proses dinyalakan atau sebagai skrip sekali jalan tidak dihitung ke beban per pengguna.",
	}
	return out
}
