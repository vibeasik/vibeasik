package routemap

import "go/ast"

// Pemicu = kapan sebuah fungsi dijalankan. Rumus biaya yang sama berarti beban yang sangat berbeda: fungsi per
// permintaan dikalikan jumlah pengguna aktif, pekerjaan latar belakang dikalikan jumlah peristiwa/jadwal, dan
// fungsi yang dipanggil langsung dari main/init hanya berjalan sekali saat proses dinyalakan (memuat konfigurasi,
// mendaftarkan rute, mengisi data contoh) — tidak ikut tumbuh bersama pengguna.
const (
	TriggerRequest    = "request"    // dijangkau rute HTTP: per permintaan
	TriggerBackground = "background" // goroutine, worker antrean, callback terdaftar, loop peristiwa
	TriggerStartup    = "startup"    // dipanggil langsung dari main/init: sekali per proses
	TriggerTool       = "tool"       // skrip sekali jalan (cmd alat bantu, seed, migrasi)
)

// triggerReach mengelompokkan fungsi di luar rute. Dari main/init: pemanggilan langsung = startup; pemanggilan
// di dalam `go`, loop peristiwa (`for {}`), atau fungsi yang diteruskan sebagai nilai (callback) = background,
// beserta semua yang dijangkau dari sana.
func (g *Graph) triggerReach(m *module) map[string]string {
	out := map[string]string{}
	async := map[string]map[string]bool{}
	asyncOf := func(k string) map[string]bool {
		if a, ok := async[k]; ok {
			return a
		}
		a := asyncCallees(m, m.funcs[k])
		async[k] = a
		return a
	}
	var startup, background, tools []string
	seenStart := map[string]bool{}
	for k, f := range m.funcs {
		if f.Recv != "" || (f.Name != "main" && f.Name != "init") {
			continue
		}
		if oneOffFile(f.file) || (f.Name == "main" && g.toolMain(f)) {
			tools = append(tools, k)
			continue
		}
		seenStart[k] = true
		startup = append(startup, k)
	}
	// 1. startup: jalur pemanggilan langsung; simpan akar background yang ditemui.
	for len(startup) > 0 {
		k := startup[len(startup)-1]
		startup = startup[:len(startup)-1]
		a := asyncOf(k)
		for _, c := range g.callees[k] {
			if m.funcs[c] == nil {
				continue
			}
			if a[c] {
				background = append(background, c)
				continue
			}
			if !seenStart[c] {
				seenStart[c] = true
				startup = append(startup, c)
			}
		}
		for c := range a {
			background = append(background, c)
		}
	}
	// 2. background: semua yang dijangkau dari akar background.
	seenBg := map[string]bool{}
	for len(background) > 0 {
		k := background[len(background)-1]
		background = background[:len(background)-1]
		if seenBg[k] || m.funcs[k] == nil || oneOffFile(m.funcs[k].file) {
			continue
		}
		seenBg[k] = true
		background = append(background, g.callees[k]...)
		for c := range asyncOf(k) {
			background = append(background, c)
		}
	}
	// 3. tool: dijangkau dari main alat bantu.
	seenTool := map[string]bool{}
	for len(tools) > 0 {
		k := tools[len(tools)-1]
		tools = tools[:len(tools)-1]
		if seenTool[k] {
			continue
		}
		seenTool[k] = true
		tools = append(tools, g.callees[k]...)
	}
	for k := range seenTool {
		out[k] = TriggerTool
	}
	for k := range seenStart {
		out[k] = TriggerStartup
	}
	for k := range seenBg {
		out[k] = TriggerBackground
	}
	return out
}

// triggerOf: pemicu fungsi k; fungsi yang dijangkau rute selalu "request" (pemicu tersering menang).
func (g *Graph) triggerOf(k string, routes int) string {
	if routes > 0 {
		return TriggerRequest
	}
	return g.triggers[k]
}

// asyncCallees: fungsi lokal yang TIDAK dijalankan sebagai bagian jalur langsung f — dipanggil di dalam `go`,
// di dalam loop peristiwa tanpa syarat berhenti (`for {}`), atau hanya dirujuk sebagai nilai (callback).
func asyncCallees(m *module, f *funcInfo) map[string]bool {
	out := map[string]bool{}
	if f == nil || f.Decl == nil || f.Decl.Body == nil {
		return out
	}
	mark := func(n ast.Node) {
		for _, k := range directCallees(m, f.file, n) {
			out[k] = true
		}
	}
	ast.Inspect(f.Decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GoStmt:
			mark(x)
			return false
		case *ast.ForStmt:
			if x.Cond == nil {
				mark(x.Body)
				return false
			}
		}
		return true
	})
	for _, k := range valueRefs(m, f) {
		out[k] = true
	}
	return out
}
