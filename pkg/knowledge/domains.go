package knowledge

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Domain node ada dua macam:
//   - ekosistem (bahasa/framework: go, php, web, flutter, …) — eksklusif: patokan Go tidak dibawa ke todo PHP
//     (Aturan 16). Ekosistem yang belum punya node sama sekali = celah pengetahuan (teknologi baru).
//   - area (security, testing, devops, …) — berlaku lintas ekosistem.

// extDomains memetakan ekstensi berkas ke ekosistem.
var extDomains = map[string]string{
	".go": "go", ".php": "php", ".py": "python", ".rb": "ruby", ".rs": "rust", ".java": "jvm", ".scala": "scala",
	".kt": "android", ".kts": "android", ".dart": "flutter", ".swift": "ios", ".m": "ios",
	".vue": "web", ".tsx": "web", ".jsx": "web", ".svelte": "web", ".html": "web", ".css": "web", ".scss": "web",
	".cs": "csharp", ".c": "cpp", ".cpp": "cpp", ".h": "cpp", ".hpp": "cpp",
	".ex": "elixir", ".exs": "elixir", ".erl": "erlang", ".zig": "zig", ".nim": "nim", ".lua": "lua",
	".hs": "haskell", ".clj": "clojure", ".ml": "ocaml", ".jl": "julia", ".sol": "solidity", ".gleam": "gleam",
	".r": "r", ".pl": "perl", ".cr": "crystal", ".v": "vlang", ".odin": "odin", ".mojo": "mojo",
	".astro": "web", ".less": "web", ".sass": "web", ".ejs": "web", ".hbs": "web", ".tf": "terraform",
}

// nonCodeExt: ekstensi yang bukan bahasa pemrograman (tidak pernah jadi celah).
var nonCodeExt = map[string]bool{
	".md": true, ".txt": true, ".json": true, ".lock": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".svg": true, ".ico": true, ".webp": true, ".pdf": true, ".toml": true, ".ini": true, ".conf": true, ".env": true,
	".xml": true, ".csv": true, ".log": true, ".mod": true, ".sum": true, ".map": true, ".ts": true, ".js": true,
	".mjs": true, ".cjs": true, ".yml": true, ".yaml": true, ".sh": true, ".sql": true, ".com": true, ".id": true,
	".io": true, ".app": true, ".dev": true, ".net": true, ".org": true, ".gz": true, ".zip": true,
	".exe": true, ".so": true, ".dll": true, ".bin": true, ".db": true, ".sqlite": true, ".woff": true, ".woff2": true,
	".ttf": true, ".glb": true, ".gltf": true, ".mp4": true, ".mp3": true, ".wav": true, ".bak": true, ".tmp": true,
	".gitignore": true, ".dockerignore": true, ".editorconfig": true, ".prettierrc": true, ".eslintrc": true,
	".npmrc": true, ".nvmrc": true, ".gitkeep": true, ".keep": true, ".example": true, ".local": true, ".sample": true,
	".dist": true, ".orig": true, ".patch": true, ".diff": true, ".pem": true, ".key": true, ".crt": true, ".pub": true,
	".jar": true, ".apk": true, ".aab": true, ".ipa": true, ".deb": true, ".rpm": true, ".tar": true, ".tgz": true,
	".pid": true, ".cache": true, ".out": true, ".old": true, ".jsonl": true, ".mdx": true, ".mts": true, ".cts": true,
	".bat": true, ".cmd": true, ".tsbuildinfo": true, ".html5": true, ".htm": true,
}

// keywordDomains: kata kunci teks → ekosistem/area.
var keywordDomains = []struct {
	re     *regexp.Regexp
	domain string
}{
	{regexp.MustCompile(`(?i)\b(golang|gorm|gofiber|goroutine)\b`), "go"},
	{regexp.MustCompile(`(?i)\b(php|laravel|codeigniter|ci4|symfony|eloquent)\b`), "php"},
	{regexp.MustCompile(`(?i)\b(python|django|fastapi|flask|pydantic)\b`), "python"},
	{regexp.MustCompile(`(?i)\b(node\.?js|express|nestjs|deno)\b`), "node"},
	{regexp.MustCompile(`(?i)\b(vue|nuxt|react|next\.?js|svelte|sveltekit|tailwind|frontend)\b`), "web"},
	{regexp.MustCompile(`(?i)\b(flutter|dart)\b`), "flutter"},
	{regexp.MustCompile(`(?i)\b(android|kotlin|jetpack compose)\b`), "android"},
	{regexp.MustCompile(`(?i)\b(swiftui|swift|ios)\b`), "ios"},
	{regexp.MustCompile(`(?i)\b(java|spring ?boot|spring)\b`), "jvm"},
	{regexp.MustCompile(`(?i)\b(rust|cargo|tokio|axum)\b`), "rust"},
	{regexp.MustCompile(`(?i)\b(ruby|rails)\b`), "ruby"},
	{regexp.MustCompile(`(?i)\b(elixir|phoenix)\b`), "elixir"},
	{regexp.MustCompile(`(?i)\b(keamanan|security|xss|csrf|injection|injeksi|idor|kredensial|secret)\b`), "security"},
	{regexp.MustCompile(`(?i)\b(test|testing|pengujian|unit test|regresi|regression)\b`), "testing"},
	{regexp.MustCompile(`(?i)\b(performa|performance|latency|lambat|cache|n\+1)\b`), "performance"},
	{regexp.MustCompile(`(?i)\b(docker|deploy|kubernetes|k8s|nginx|caddy|ci/cd|pipeline)\b`), "devops"},
	{regexp.MustCompile(`(?i)\b(sql|database|basis data|query|kueri|migrasi|migration|indeks)\b`), "database"},
	{regexp.MustCompile(`(?i)\b(ui|ux|tampilan|desain|design|layout|tombol|button|navbar|sidebar|tipografi|typography|responsif|responsive|aksesibilitas|accessibility|dashboard|landing|warna|color|colour|palet|palette|kontras|contrast|harmoni|harmony|gestalt|hierarki|hierarchy|whitespace)\b`), "ui_ux"},
	{regexp.MustCompile(`(?i)\b(roadmap|milestone|perencanaan|prioritas|backlog)\b`), "planning"},
}

// ecosystems: domain yang eksklusif (bahasa/framework/platform).
var ecosystems = map[string]bool{}

// areaDomains: domain lintas ekosistem.
var areaDomains = map[string]bool{
	"security": true, "testing": true, "performance": true, "devops": true, "database": true, "planning": true,
	"architecture": true, "backend": true, "mobile": true, "ai": true, "observability": true, "ui_ux": true,
	"process": true, "business": true, "documentation": true, "code_quality": true,
}

// domainAliases memetakan label domain yang pernah tersimpan ke domain tetap (kunci sudah melewati domainKey):
// ejaan ganda, sub-bidang yang terlalu sempit untuk berdiri sendiri, dan nama sampah hasil deteksi ekstensi
// berkas. Tujuan sampah dipilih dari isi node-nya; bila ragu, area lintas ekosistem lebih aman daripada
// ekosistem karena node di ekosistem yang salah tersaring dari todo yang semestinya (lihat Recall).
var domainAliases = map[string]string{
	// area
	"ui": "ui_ux", "ux": "ui_ux", "design": "ui_ux", "accessibility": "ui_ux", "a11y": "ui_ux",
	"test": "testing", "tests": "testing", "qa": "testing", "audit": "testing", "resolve": "testing",
	"sql": "database", "db": "database", "migration": "database", "data": "database",
	"deployment": "devops", "deploy": "devops", "release": "devops", "release_engineering": "devops", "ops": "devops",
	"rollback": "devops", "configuration": "devops", "git": "devops", "shell": "devops", "linux": "devops",
	"windows": "devops", "mji": "devops", "current": "devops",
	"api": "backend", "server": "backend", "event_driven_backend": "backend", "reliability": "backend",
	"billing": "backend", "sheet": "backend", "xlsx": "backend",
	"run":   "mobile",
	"other": "observability", "debugging": "observability", "error_handling": "observability", "logging": "observability",
	"docs": "documentation", "doc": "documentation", "msi": "documentation",
	"clean_code": "code_quality", "refactoring": "code_quality", "refactor": "code_quality",
	"software_engineering": "code_quality", "tooling": "code_quality", "typescript": "code_quality",
	"engineering": "process", "quality": "process", "general": "process", "workflow": "process",
	"software": "architecture", "frontend_architecture": "architecture",
	"product": "planning", "project_management": "planning",
	"marketing": "business", "accounting": "business",
	// ekosistem
	"frontend": "web", "vue": "web", "nuxt": "web", "avif": "web",
	"golang": "go",
	"gradle": "android", "pro": "android", "kotlin": "android",
	"mounted": "flutter", "builder": "flutter",
	"resolved": "ios", "swift": "ios",
	"java": "jvm",
}

// domainKey: bentuk baku sebuah label — huruf kecil, spasi & tanda hubung menjadi garis bawah.
func domainKey(d string) string {
	d = strings.ToLower(strings.Join(strings.Fields(d), "_"))
	return strings.ReplaceAll(d, "-", "_")
}

// CanonicalDomain mengembalikan domain tetap untuk sebuah label. Label yang tidak dikenal dikembalikan dalam
// bentuk baku (domainKey) tanpa dipetakan, karena bisa jadi ekosistem baru yang belum punya node (celah).
func CanonicalDomain(d string) string {
	k := domainKey(d)
	if c, ok := domainAliases[k]; ok {
		return c
	}
	return k
}

// KnownDomain: label adalah domain tetap (area, atau ekosistem yang dikenali dari ekstensi/kata kunci).
func KnownDomain(d string) bool { return areaDomains[d] || ecosystems[d] }

// CanonicalDomains: daftar domain tetap, urut abjad.
func CanonicalDomains() []string {
	out := make([]string, 0, len(areaDomains)+len(ecosystems))
	for d := range areaDomains {
		out = append(out, d)
	}
	for d := range ecosystems {
		if !areaDomains[d] {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

func init() {
	for _, d := range extDomains {
		ecosystems[d] = true
	}
	for _, k := range keywordDomains {
		if !areaDomains[k.domain] {
			ecosystems[k.domain] = true
		}
	}
	ecosystems["dart"] = true // checklist audit Dart non-Flutter
}

// IsEcosystem: domain bahasa/framework (eksklusif), bukan area lintas ekosistem. Alias dinilai lewat domain tetapnya.
func IsEcosystem(d string) bool {
	d = CanonicalDomain(d)
	return ecosystems[d] || (d != "" && !areaDomains[d])
}

var pathLike = regexp.MustCompile(`[\w./\[\]-]*[/\\][\w.\[\]-]+\.[a-z][a-z0-9]{0,7}\b|\b[\w-]+\.(?:vue|tsx|jsx|svelte|go|php|py|rb|rs|java|kt|kts|dart|swift|cs|cpp|ex|exs|zig|gleam|sol|lua|hs|scala|clj)\b`)

// newLangExt: ekstensi yang layak dianggap bahasa baru — 2–8 huruf saja (bukan v1, 2024, dsb.).
var newLangExt = regexp.MustCompile(`^\.[a-z]{2,8}$`)

// DetectDomains mengenali ekosistem & area dari teks (judul, keterangan, path berkas). Ekosistem di depan.
// Ekstensi berkas yang tidak dikenal (mis. src/main.mojo) dianggap ekosistem baru bernama ekstensinya.
func DetectDomains(text string) []string {
	seen := map[string]bool{}
	var eco, area []string
	add := func(d string) {
		d = CanonicalDomain(d)
		if d == "" || seen[d] {
			return
		}
		seen[d] = true
		if IsEcosystem(d) {
			eco = append(eco, d)
		} else {
			area = append(area, d)
		}
	}
	for _, p := range pathLike.FindAllString(text, 60) {
		ext := strings.ToLower(filepath.Ext(p))
		if d, ok := extDomains[ext]; ok {
			add(d)
		} else if !nonCodeExt[ext] && strings.ContainsAny(p, "/\\") && newLangExt.MatchString(ext) {
			add(strings.TrimPrefix(ext, "."))
		}
	}
	for _, k := range keywordDomains {
		if k.re.MatchString(text) {
			add(k.domain)
		}
	}
	return append(eco, area...)
}

// StackDomain: ekosistem dari bahasa & framework komponen (models.Component.Language/Framework).
func StackDomain(language, framework string) string {
	lang, fw := strings.ToLower(strings.TrimSpace(language)), strings.ToLower(strings.TrimSpace(framework))
	if d := primaryDomain(fw); d != "" && IsEcosystem(d) {
		return d
	}
	switch lang {
	case "", "auto", "unknown":
		return ""
	case "go", "golang":
		return "go"
	case "typescript", "javascript":
		return "node"
	case "dart":
		return "flutter"
	case "kotlin":
		return "android"
	case "java":
		return "jvm"
	case "swift":
		return "ios"
	}
	if d, ok := extDomains["."+lang]; ok {
		return d
	}
	if d := primaryDomain(lang); d != "" {
		return d
	}
	return CanonicalDomain(lang)
}

// primaryDomain: domain utama sebuah teks (ekosistem pertama, lalu area pertama).
func primaryDomain(text string) string {
	if ds := DetectDomains(text); len(ds) > 0 {
		return ds[0]
	}
	return ""
}

// guidelineDomain memetakan kategori berkas guidelines ke domain.
func guidelineDomain(category string) string {
	c := strings.ToLower(category)
	for _, m := range []struct{ key, domain string }{
		{"perencanaan", "planning"}, {"keamanan", "security"}, {"frontend", "web"}, {"mobile", "mobile"},
		{"testing", "testing"}, {"error_handling", "observability"}, {"devops", "devops"}, {"performa", "performance"},
		{"ai_dan_mcp", "ai"}, {"backend", "backend"}, {"arsitektur", "architecture"},
	} {
		if strings.Contains(c, m.key) {
			return m.domain
		}
	}
	return "architecture"
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return m[out[i]] > m[out[j]] || m[out[i]] == m[out[j]] && out[i] < out[j] })
	return out
}
