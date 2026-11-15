package routemap

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
)

// Lapisan di atas rumus tick: APA yang diputari sebuah loop atas data. Rumus T(n) menyamakan semua koleksi
// menjadi n, padahal bebannya berbeda jauh: baris tabel tumbuh bersama data tersimpan, satu halaman dibatasi
// Limit, isi permintaan dibatasi klien, dan koleksi di memori biasanya kecil. Asal data ini yang menentukan nilai n
// saat rumus diterjemahkan menjadi beban server (lihat paket insight, proyeksi beban).

type driver uint8

const (
	drvDB    driver = 1 << iota // baris hasil query tanpa Limit: tumbuh bersama data tersimpan
	drvPage                     // baris hasil query ber-Limit/Offset: paling banyak satu halaman
	drvInput                    // isi permintaan (body, form, query string)
	drvFile                     // berkas, isi folder, baris teks berkas
	drvParam                    // parameter fungsi: ukurannya ditentukan pemanggil
	drvMem                      // koleksi di memori / asal tidak diketahui
)

var driverNames = []struct {
	d    driver
	name string
}{{drvDB, "db"}, {drvPage, "page"}, {drvInput, "input"}, {drvFile, "file"}, {drvParam, "param"}, {drvMem, "memory"}}

func (d driver) names() []string {
	var out []string
	for _, n := range driverNames {
		if d&n.d != 0 {
			out = append(out, n.name)
		}
	}
	return out
}

// Arti n untuk proyeksi beban, dari asal data terberat yang ditemukan.
const (
	ScaleData   = "data"   // baris tersimpan: n ikut tumbuh bersama data
	ScalePage   = "page"   // n paling banyak ukuran halaman
	ScaleInput  = "input"  // n = banyaknya isi permintaan
	ScaleFile   = "file"   // n = banyaknya berkas / baris berkas
	ScaleMemory = "memory" // koleksi di memori atau asal tidak diketahui
	ScaleFixed  = "fixed"  // tidak ada yang tumbuh: biaya tetap per pemanggilan
)

// scale: growing = rumusnya memang punya suku n (loop atas data atau I/O per item).
func (d driver) scale(growing bool) string {
	switch {
	case d&drvDB != 0:
		return ScaleData
	case !growing:
		return ScaleFixed
	case d&drvPage != 0:
		return ScalePage
	case d&drvInput != 0:
		return ScaleInput
	case d&drvFile != 0:
		return ScaleFile
	}
	return ScaleMemory
}

var (
	// multiRowTerminals: method pengirim query yang mengembalikan banyak baris.
	multiRowTerminals = map[string]bool{"Find": true, "Scan": true, "Rows": true, "Query": true, "QueryContext": true, "Queryx": true,
		"NamedQuery": true, "Select": true, "SelectContext": true, "Pluck": true, "Aggregate": true}
	// limitBuilders: penyusun query yang membatasi jumlah baris.
	limitBuilders = map[string]bool{"Limit": true, "Offset": true, "Paginate": true, "Page": true}
	sqlLimitRe    = regexp.MustCompile(`(?i)\blimit\b`)
	// inputBinders: pengisi struct dari isi permintaan (argumen &tujuan).
	inputBinders = map[string]bool{"BodyParser": true, "QueryParser": true, "ParamsParser": true, "Bind": true, "BindJSON": true,
		"ShouldBind": true, "ShouldBindJSON": true, "ShouldBindQuery": true, "Decode": true, "ReadJSON": true}
	// inputReaders: pembaca isi permintaan yang mengembalikan nilainya.
	inputReaders = map[string]bool{"Body": true, "FormValue": true, "Query": true, "Queries": true, "MultipartForm": true, "PostForm": true,
		"FormFile": true, "QueryArray": true}
	rowIterRe = regexp.MustCompile(`(?i)rows?$|cursor|cur$|iter|result`)
)

// fetchDriver: query banyak-baris → drvPage bila rantainya dibatasi (Limit/Offset atau SQL ber-LIMIT), selain itu
// drvDB. Query satu baris / tulis (First, Create, Count …) → 0.
func fetchDriver(sel *ast.SelectorExpr) driver {
	if !multiRowTerminals[sel.Sel.Name] {
		return 0
	}
	if chainLimited(sel.X) {
		return drvPage
	}
	return drvDB
}

// chainLimited: rantai penyusun query memuat Limit/Offset/Paginate, atau teks SQL-nya memuat LIMIT.
func chainLimited(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.SelectorExpr:
			if limitBuilders[x.Sel.Name] {
				found = true
			}
		case *ast.BasicLit:
			if x.Kind == token.STRING && sqlLimitRe.MatchString(x.Value) {
				found = true
			}
		}
		return !found
	})
	return found
}

// scopeVars mencatat asal variabel lokal sebuah badan fungsi: defs = ekspresi yang mengisinya, fill = diisi lewat
// &variabel oleh query (db/page) atau pembaca isi permintaan (input).
func scopeVars(body []ast.Stmt) (defs map[string]ast.Expr, fill map[string]driver) {
	defs, fill = map[string]ast.Expr{}, map[string]driver{}
	def := func(l, r ast.Expr) {
		if id, ok := l.(*ast.Ident); ok && id.Name != "_" && r != nil {
			if _, seen := defs[id.Name]; !seen {
				defs[id.Name] = r
			}
		}
	}
	for _, s := range body {
		ast.Inspect(s, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				for i, l := range x.Lhs {
					switch {
					case len(x.Rhs) == len(x.Lhs):
						def(l, x.Rhs[i])
					case len(x.Rhs) == 1 && i == 0: // v, err := f()
						def(l, x.Rhs[0])
					}
				}
			case *ast.ValueSpec:
				for i, name := range x.Names {
					if i < len(x.Values) {
						def(name, x.Values[i])
					}
				}
			case *ast.RangeStmt: // elemen mewarisi asal koleksinya
				def(x.Key, x.X)
				def(x.Value, x.X)
			case *ast.CallExpr:
				sel, ok := x.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				var d driver
				switch {
				case inputBinders[sel.Sel.Name]:
					d = drvInput
				case isQueryCall(sel, len(x.Args)):
					d = fetchDriver(sel)
				}
				if d == 0 {
					return true
				}
				for _, a := range x.Args {
					if u, ok := a.(*ast.UnaryExpr); ok && u.Op == token.AND {
						if id, ok := u.X.(*ast.Ident); ok {
							fill[id.Name] |= d
						}
					}
				}
			}
			return true
		})
	}
	return defs, fill
}

// paramSet: nama parameter (dan penerima) sebuah fungsi.
func paramSet(decl *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	if decl == nil || decl.Type == nil || decl.Type.Params == nil {
		return out
	}
	for _, f := range decl.Type.Params.List {
		for _, n := range f.Names {
			out[n.Name] = true
		}
	}
	return out
}

const maxDriverDepth = 6

// forDriver: asal data sebuah `for` atas data, dari syarat berhentinya.
func (c *costCtx) forDriver(sc *fnScope, x *ast.ForStmt) driver {
	switch cond := x.Cond.(type) {
	case *ast.CallExpr: // for rows.Next() / for scanner.Scan()
		if sel, ok := cond.Fun.(*ast.SelectorExpr); ok {
			switch {
			case sel.Sel.Name == "Next" && rowIterRe.MatchString(chainRoot(sel.X)):
				return drvDB
			case sel.Sel.Name == "Scan":
				return drvFile
			}
		}
	case *ast.BinaryExpr:
		for _, side := range []ast.Expr{cond.Y, cond.X} {
			if _, lit := intLit(side); lit {
				continue
			}
			if id, ok := side.(*ast.Ident); ok && id.Name == forIndex(x) {
				continue
			}
			return c.exprDriver(sc, side, 0)
		}
	}
	return drvMem
}

// exprDriver: asal data sebuah ekspresi koleksi (yang di-range atau diukur len()-nya).
func (c *costCtx) exprDriver(sc *fnScope, e ast.Expr, depth int) driver {
	if e == nil || depth > maxDriverDepth {
		return drvMem
	}
	switch x := e.(type) {
	case *ast.ParenExpr:
		return c.exprDriver(sc, x.X, depth+1)
	case *ast.StarExpr:
		return c.exprDriver(sc, x.X, depth+1)
	case *ast.UnaryExpr:
		return c.exprDriver(sc, x.X, depth+1)
	case *ast.SliceExpr:
		return c.exprDriver(sc, x.X, depth+1)
	case *ast.IndexExpr:
		return c.exprDriver(sc, x.X, depth+1)
	case *ast.Ident:
		return c.varDriver(sc, x.Name, depth)
	case *ast.SelectorExpr: // req.Items, user.Orders → asal akar rantainya
		return c.exprDriver(sc, x.X, depth+1)
	case *ast.CallExpr:
		return c.callDriver(sc, x, depth)
	}
	return drvMem
}

func (c *costCtx) varDriver(sc *fnScope, name string, depth int) driver {
	if sc == nil {
		return drvMem
	}
	if d := sc.fill[name]; d != 0 {
		return d
	}
	if sc.params[name] {
		return drvParam
	}
	if def, ok := sc.defs[name]; ok {
		return c.exprDriver(sc, def, depth+1)
	}
	return drvMem
}

func (c *costCtx) callDriver(sc *fnScope, ce *ast.CallExpr, depth int) driver {
	if id, ok := ce.Fun.(*ast.Ident); ok && len(ce.Args) > 0 {
		switch id.Name {
		case "len", "append", "string", "min", "max":
			return c.exprDriver(sc, ce.Args[0], depth+1)
		}
		if len(ce.Args) == 1 && id.Obj == nil && strings.HasPrefix(id.Name, "[]") {
			return c.exprDriver(sc, ce.Args[0], depth+1)
		}
	}
	if fs := c.m.resolveFuncs(sc.fi, ce.Fun); len(fs) > 0 {
		var d driver
		for _, f := range fs {
			if f == nil {
				continue
			}
			fc := c.funcCost(f.Key)
			switch {
			case fc.drv&(drvDB|drvPage) != 0:
				d |= fc.drv & (drvDB | drvPage)
			case fc.kinds&ioQuery != 0:
				d |= drvDB
			case fc.kinds&ioFile != 0:
				d |= drvFile
			}
		}
		if d != 0 {
			return d
		}
		// Fungsi murni: hasilnya diturunkan dari argumennya.
		for _, a := range ce.Args {
			if ad := c.exprDriver(sc, a, depth+1); ad != drvMem {
				return ad
			}
		}
		return drvMem
	}
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return drvMem
	}
	switch {
	case isQueryCall(sel, len(ce.Args)):
		if d := fetchDriver(sel); d != 0 {
			return d
		}
		return drvDB
	case ioCallKind(sel) == ioFile:
		return drvFile
	case inputReaders[sel.Sel.Name]:
		return drvInput
	}
	// Method pada sebuah nilai (m.Keys(), strings.Fields(x)): asal penerimanya, lalu argumennya.
	if d := c.exprDriver(sc, sel.X, depth+1); d != drvMem {
		return d
	}
	for _, a := range ce.Args {
		if d := c.exprDriver(sc, a, depth+1); d != drvMem {
			return d
		}
	}
	return drvMem
}

// argsDriver: asal data koleksi yang diproses sebuah pemanggilan pustaka (sort.Slice(rows, …), lo.Map(rows, …)).
func (c *costCtx) argsDriver(sc *fnScope, ce *ast.CallExpr) driver {
	var d driver
	for _, a := range ce.Args {
		if _, ok := a.(*ast.FuncLit); !ok {
			d |= c.exprDriver(sc, a, 0)
		}
	}
	if sel, ok := ce.Fun.(*ast.SelectorExpr); ok {
		if id, isPkg := sel.X.(*ast.Ident); !isPkg || id.Obj != nil {
			d |= c.exprDriver(sc, sel.X, 0)
		}
	}
	return d
}

// bindParams: fungsi terpanggil yang memutari parameternya mengikuti apa yang diberikan pemanggil — baris data
// (tingkat itu menjadi tingkat data), parameter pemanggil sendiri (tetap terbuka), atau koleksi kecil.
func (c *costCtx) bindParams(sc *fnScope, ce *ast.CallExpr, callee cst) cst {
	if !callee.ticks.hasParamLevels() && !callee.queries.hasParamLevels() {
		return callee
	}
	d := c.argsDriver(sc, ce)
	switch {
	case d&drvDB != 0:
		callee.ticks, callee.queries = callee.ticks.bindParams(true), callee.queries.bindParams(true)
		callee.drv |= drvDB
	case d&drvParam != 0: // diteruskan dari parameter pemanggil: baru diketahui di pemanggil berikutnya
	default:
		callee.ticks, callee.queries = callee.ticks.bindParams(false), callee.queries.bindParams(false)
	}
	return callee
}

// registerCallRe: pemanggilan yang MENDAFTARKAN callback untuk dijalankan nanti (handler rute, middleware, job
// terjadwal, pendengar peristiwa). Func literal di argumennya tidak dijalankan oleh fungsi pendaftar, jadi bukan
// biaya fungsi itu: handler rute dihitung sebagai rute, job dihitung saat dijalankan. Group/Route/Mount tidak
// termasuk — callback-nya dijalankan langsung untuk menyusun rute.
var registerCallRe = regexp.MustCompile(`^(?:Handle|HandleFunc|Get|Post|Put|Patch|Delete|Head|Options|Connect|Trace|Any|All|Add|Use|` +
	`GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|Method|MethodFunc|NotFound|NoRoute|NoMethod|AddFunc|AddJob|AfterFunc|Every|Schedule|Cron|` +
	`Subscribe\w*|Register\w*|Listen\w*|On[A-Z]\w*|On)$`)

func registersCallback(ce *ast.CallExpr) bool {
	if !hasFuncLit(ce) {
		return false
	}
	switch f := ce.Fun.(type) {
	case *ast.SelectorExpr:
		return registerCallRe.MatchString(f.Sel.Name)
	case *ast.Ident:
		return registerCallRe.MatchString(f.Name)
	}
	return false
}

// registeredArgs: untuk pemanggilan pendaftar, hitung argumen selain func literal dan penerimanya di sini;
// false = argumen sudah dihitung (func literal dilewati). Pemanggilan lain → true (argumen ditelusuri pemanggil).
func (c *costCtx) registeredArgs(sc *fnScope, ce *ast.CallExpr, out *cst) bool {
	if !registersCallback(ce) {
		return true
	}
	for _, a := range ce.Args {
		if _, ok := a.(*ast.FuncLit); !ok {
			out.add(c.expr(sc, a))
		}
	}
	if sel, ok := ce.Fun.(*ast.SelectorExpr); ok {
		out.add(c.expr(sc, sel.X))
	}
	return false
}

// Jenis operasi sebuah rute.
const (
	OpList   = "list"   // GET tanpa parameter di ujung path: daftar / agregat
	OpDetail = "detail" // GET satu sumber daya (/x/:id)
	OpCreate = "create"
	OpUpdate = "update"
	OpDelete = "delete"
	OpOther  = "other"
)

var pathParamRe = regexp.MustCompile(`^(?::|\{|<|\*)`)

// routeOp menebak jenis operasi dari method + path. Tulis (create/update/delete) umumnya menyentuh satu baris;
// daftar membaca banyak baris.
func routeOp(method, path string) string {
	switch strings.ToUpper(method) {
	case "GET", "HEAD":
		segs := strings.Split(strings.Trim(path, "/"), "/")
		if last := segs[len(segs)-1]; pathParamRe.MatchString(last) {
			return OpDetail
		}
		return OpList
	case "POST":
		return OpCreate
	case "PUT", "PATCH":
		return OpUpdate
	case "DELETE":
		return OpDelete
	}
	return OpOther
}
