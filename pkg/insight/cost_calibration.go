package insight

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Kalibrasi: harga satu query, satu tick, dan ongkos dasar per permintaan dicocokkan dengan latensi yang benar-benar
// terukur (data trafik, atau probe HTTP Deep Check), supaya proyeksi tidak bergantung pada angka bawaan.
//
// Hanya rute yang biayanya TIDAK bergantung pada jumlah baris data yang dipakai (jumlah baris nyata saat diukur tidak
// diketahui). Modelnya latensi = dasar + query × Q + tick × T dengan koefisien tak negatif. Hasilnya dipakai hanya
// bila cukup rute terukur dan modelnya memang menjelaskan pengukuran (R²); selain itu angka bawaan tetap berlaku dan
// alasannya ditampilkan.

// Calibration = hasil pencocokan dengan latensi terukur.
type Calibration struct {
	Source   string  `json:"source"`   // traffic | deep_check | "" (tidak ada pengukuran)
	Measured int     `json:"measured"` // rute kode yang punya latensi terukur
	Samples  int     `json:"samples"`  // di antaranya yang layak dicocokkan (biaya tidak bergantung baris data)
	Used     int     `json:"used"`     // setelah pencilan dibuang
	BaseMs   float64 `json:"base_ms"`
	QueryMs  float64 `json:"query_ms"`
	TickNs   float64 `json:"tick_ns"`
	R2       float64 `json:"r2"`
	Applied  bool    `json:"applied"`
	Note     string  `json:"note"`
}

const (
	minCalSamples = 6
	minCalR2      = 0.5
)

type calSample struct{ ms, ticks, queries float64 }

// measuredLatency = latensi terukur per rute (kunci routeKey) dari satu sumber.
type measuredLatency struct {
	source string
	ms     map[string]float64
}

// loadDeepCheckLatency: latensi probe HTTP nyata dari laporan Deep Check terbaru proyek.
func loadDeepCheckLatency(db *gorm.DB, projectID uint) *measuredLatency {
	var rep models.DeepCheckReport
	if db.Where("project_id = ?", projectID).Order("id DESC").First(&rep).Error != nil {
		return nil
	}
	var sims []models.RouteSimulation
	if json.Unmarshal([]byte(rep.RouteSimulationsJSON), &sims) != nil {
		return nil
	}
	out := &measuredLatency{source: "deep_check", ms: map[string]float64{}}
	for _, s := range sims {
		if s.RealLatencyMs > 0 {
			out.ms[routeKey(s.Method, s.Path)] = s.RealLatencyMs
		}
	}
	if len(out.ms) == 0 {
		return nil
	}
	return out
}

// solve menyelesaikan sistem linear kecil A·x = b (eliminasi Gauss dengan pivot); false bila singular.
func solve(a [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	for i := 0; i < n; i++ {
		p := i
		for r := i + 1; r < n; r++ {
			if math.Abs(a[r][i]) > math.Abs(a[p][i]) {
				p = r
			}
		}
		if math.Abs(a[p][i]) < 1e-12 {
			return nil, false
		}
		a[i], a[p], b[i], b[p] = a[p], a[i], b[p], b[i]
		for r := i + 1; r < n; r++ {
			f := a[r][i] / a[i][i]
			for c := i; c < n; c++ {
				a[r][c] -= f * a[i][c]
			}
			b[r] -= f * b[i]
		}
	}
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		x[i] = b[i]
		for c := i + 1; c < n; c++ {
			x[i] -= a[i][c] * x[c]
		}
		x[i] /= a[i][i]
	}
	return x, true
}

// leastSquares mencocokkan ms = Σ koef × kolom untuk kolom terpilih (0 = dasar, 1 = query, 2 = tick). Kolom
// dinormalkan ke nilai terbesarnya agar tick (jutaan) dan query (satuan) tidak merusak ketelitian.
func leastSquares(s []calSample, cols []int) (coef [3]float64, sse float64, ok bool) {
	scale := [3]float64{1, 0, 0}
	for _, x := range s {
		scale[1], scale[2] = math.Max(scale[1], x.queries), math.Max(scale[2], x.ticks)
	}
	row := func(x calSample) [3]float64 { return [3]float64{1, x.queries, x.ticks} }
	n := len(cols)
	a := make([][]float64, n)
	b := make([]float64, n)
	for i := range a {
		a[i] = make([]float64, n)
	}
	for _, x := range s {
		r := row(x)
		for i, ci := range cols {
			if scale[ci] == 0 {
				return coef, 0, false
			}
			vi := r[ci] / scale[ci]
			b[i] += vi * x.ms
			for j, cj := range cols {
				a[i][j] += vi * r[cj] / scale[cj]
			}
		}
	}
	sol, good := solve(a, b)
	if !good {
		return coef, 0, false
	}
	for i, ci := range cols {
		if sol[i] < 0 {
			return coef, 0, false
		}
		coef[ci] = sol[i] / scale[ci]
	}
	for _, x := range s {
		r := row(x)
		d := coef[0] + coef[1]*r[1] + coef[2]*r[2] - x.ms
		sse += d * d
	}
	return coef, sse, true
}

// bestFit: gabungan kolom berkoefisien tak negatif dengan galat terkecil.
func bestFit(s []calSample) (coef [3]float64, sse float64, ok bool) {
	for _, cols := range [][]int{{0}, {0, 1}, {0, 2}, {0, 1, 2}} {
		c, e, good := leastSquares(s, cols)
		if good && (!ok || e < sse) {
			coef, sse, ok = c, e, true
		}
	}
	return coef, sse, ok
}

// fitCalibration: cocokkan, buang pencilan (sisa > 3× median sisa mutlak) sekali, cocokkan lagi, lalu hitung R².
func fitCalibration(s []calSample) (coef [3]float64, r2 float64, used int, ok bool) {
	coef, _, ok = bestFit(s)
	if !ok {
		return coef, 0, 0, false
	}
	res := make([]float64, len(s))
	for i, x := range s {
		res[i] = math.Abs(coef[0] + coef[1]*x.queries + coef[2]*x.ticks - x.ms)
	}
	sorted := append([]float64(nil), res...)
	sort.Float64s(sorted)
	cut := 3 * math.Max(sorted[len(sorted)/2], 0.05)
	kept := make([]calSample, 0, len(s))
	for i, x := range s {
		if res[i] <= cut {
			kept = append(kept, x)
		}
	}
	if len(kept) >= minCalSamples && len(kept) < len(s) {
		if c, _, good := bestFit(kept); good {
			coef, s = c, kept
		}
	}
	var mean, sse, sst float64
	for _, x := range s {
		mean += x.ms
	}
	mean /= float64(len(s))
	for _, x := range s {
		d := coef[0] + coef[1]*x.queries + coef[2]*x.ticks - x.ms
		sse += d * d
		sst += (x.ms - mean) * (x.ms - mean)
	}
	if sst > 0 {
		r2 = 1 - sse/sst
	}
	return coef, r2, len(s), true
}

// calibrate menyusun hasil kalibrasi dari sampel; applied = layak menggantikan angka bawaan.
func calibrate(source string, measured int, s []calSample) Calibration {
	c := Calibration{Source: source, Measured: measured, Samples: len(s)}
	label := map[string]string{"traffic": "data trafik", "deep_check": "probe Deep Check"}[source]
	switch {
	case source == "":
		c.Note = "Belum ada latensi terukur (data trafik atau Deep Check dengan aplikasi berjalan): harga tick dan query memakai angka bawaan."
		return c
	case len(s) < minCalSamples:
		c.Note = fmt.Sprintf("%d rute terukur dari %s, tetapi hanya %d yang biayanya tidak bergantung jumlah baris data (butuh %d): angka bawaan tetap dipakai.", measured, label, len(s), minCalSamples)
		return c
	}
	coef, r2, used, ok := fitCalibration(s)
	c.BaseMs, c.QueryMs, c.TickNs, c.R2, c.Used = coef[0], coef[1], coef[2]*1e6, r2, used
	switch {
	case !ok:
		c.Note = "Pengukuran tidak bisa dicocokkan dengan rumus: angka bawaan tetap dipakai."
	case r2 < minCalR2 || (c.QueryMs == 0 && c.TickNs == 0):
		c.Note = fmt.Sprintf("Rumus hanya menjelaskan %.0f%% keragaman latensi %d rute terukur (%s); di bawah %.0f%% dianggap tidak cocok, jadi angka bawaan tetap dipakai. Latensinya ditentukan hal yang tidak terlihat di rumus (ukuran respons, layanan luar) atau pengukurannya tidak menjalankan handler.", math.Max(r2, 0)*100, used, label, minCalR2*100)
	default:
		c.Applied = true
		c.Note = fmt.Sprintf("Dicocokkan dengan %d rute terukur (%s): rumus menjelaskan %.0f%% keragaman latensinya. Harga yang tidak bisa ditentukan dari pengukuran tetap memakai bawaan.", used, label, r2*100)
	}
	return c
}
