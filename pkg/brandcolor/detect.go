package brandcolor

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Colors adalah warna merek sebuah proyek: dipakai tema dokumen (sampul, header, footer) dan bisa diedit pemilik.
type Colors struct {
	Primary   string   `json:"primary"`             // warna utama (navbar, tombol utama)
	Secondary string   `json:"secondary,omitempty"` // warna pendamping (footer, latar gelap)
	Accent    string   `json:"accent,omitempty"`    // aksen kecil (tautan, sorotan)
	Source    string   `json:"source,omitempty"`    // manual | detected
	Evidence  []string `json:"evidence,omitempty"`  // asal tiap warna hasil deteksi, mis. "app/app.config.ts: primary 'emerald'"
}

// Normalize merapikan hex, membuang nilai yang bukan warna, dan memotong bukti yang terlalu panjang.
func (c *Colors) Normalize() {
	c.Primary, c.Secondary, c.Accent = NormalizeHex(c.Primary), NormalizeHex(c.Secondary), NormalizeHex(c.Accent)
	if len(c.Evidence) > 12 {
		c.Evidence = c.Evidence[:12]
	}
	for i, e := range c.Evidence {
		if r := []rune(e); len(r) > 200 {
			c.Evidence[i] = string(r[:200])
		}
	}
}

// Empty: belum ada warna utama.
func (c Colors) Empty() bool { return c.Primary == "" }

// Palet Tailwind (tingkat 500) untuk nama warna di konfigurasi Nuxt UI/Tailwind dan kelas utilitas.
var tailwind500 = map[string]string{
	"red": "#ef4444", "orange": "#f97316", "amber": "#f59e0b", "yellow": "#eab308", "lime": "#84cc16",
	"green": "#22c55e", "emerald": "#10b981", "teal": "#14b8a6", "cyan": "#06b6d4", "sky": "#0ea5e9",
	"blue": "#3b82f6", "indigo": "#6366f1", "violet": "#8b5cf6", "purple": "#a855f7", "fuchsia": "#d946ef",
	"pink": "#ec4899", "rose": "#f43f5e",
}

// Warna Material (tingkat 500) untuk Colors.<nama> di tema Flutter.
var material500 = map[string]string{
	"red": "#f44336", "pink": "#e91e63", "purple": "#9c27b0", "deepPurple": "#673ab7", "indigo": "#3f51b5",
	"blue": "#2196f3", "lightBlue": "#03a9f4", "cyan": "#00bcd4", "teal": "#009688", "green": "#4caf50",
	"lightGreen": "#8bc34a", "lime": "#cddc39", "yellow": "#ffeb3b", "amber": "#ffc107", "orange": "#ff9800",
	"deepOrange": "#ff5722", "brown": "#795548", "blueGrey": "#607d8b",
}

const colorValue = `(#[0-9a-fA-F]{3,8}\b|(?:rgba?|hsla?|oklch)\([^)]*\)|[\d.]+\s+[\d.]+%\s+[\d.]+%)`

var (
	// --primary / --color-primary-500 / --brand / --ui-primary … di CSS, SCSS, atau @theme Tailwind v4.
	cssVarRe = regexp.MustCompile(`(?i)--(?:color-|ui-|app-|theme-)?(primary|brand|accent|secondary)(?:-color)?(?:-(?:500|600|default|base))?\s*:\s*` + colorValue)
	// $primary: #… (SCSS/Bootstrap).
	scssVarRe = regexp.MustCompile(`(?i)\$(primary|brand|accent|secondary)(?:-color)?\s*:\s*` + colorValue)
	// tailwind.config: primary: '#…' atau primary: { 500: '#…' }.
	twFlatRe   = regexp.MustCompile(`(?i)['"]?(primary|brand|accent|secondary)['"]?\s*:\s*['"](#[0-9a-fA-F]{3,8})['"]`)
	twNestedRe = regexp.MustCompile(`(?is)['"]?(primary|brand|accent|secondary)['"]?\s*:\s*\{[^{}]*?(?:'?500'?|"500"|DEFAULT)\s*:\s*['"](#[0-9a-fA-F]{3,8})['"]`)
	// Nuxt UI app.config: colors: { primary: 'emerald' }.
	nuxtUIRe = regexp.MustCompile(`(?i)\b(primary|secondary)\s*:\s*['"]([a-z]+)['"]`)
	// Flutter: seedColor/primary/secondary: Color(0xFF…) atau Colors.teal.
	flutterHexRe  = regexp.MustCompile(`\b(seedColor|colorSchemeSeed|primary|primaryColor|secondary|tertiary|accentColor)\s*:\s*(?:const\s+)?Color\(\s*0x([0-9a-fA-F]{8})\s*\)`)
	flutterNameRe = regexp.MustCompile(`\b(seedColor|colorSchemeSeed|primary|primaryColor|primarySwatch|secondary)\s*:\s*Colors\.([a-zA-Z]+)\b`)
	// Android res/values/colors.xml.
	androidRe = regexp.MustCompile(`(?i)<color\s+name="(colorPrimary|primary|brand|colorAccent|accent|colorSecondary|secondary)\w*"\s*>\s*(#[0-9a-fA-F]{6,8})\s*<`)
	// manifest web & <meta name="theme-color">.
	themeColorRe = regexp.MustCompile(`(?i)(?:"theme_color"\s*:\s*"|name=["']theme-color["']\s+content=["']|themeColor\s*:\s*['"])(#[0-9a-fA-F]{3,8})`)
	// Kelas utilitas Tailwind yang menyebut keluarga warna, mis. bg-emerald-600, text-sky-500.
	twClassRe = regexp.MustCompile(`\b(?:bg|text|border|from|to|via|ring|fill|stroke|accent|decoration|outline)-(red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-(\d{2,3})\b`)
	// Warna hex bebas (latar/teks arbitrer) untuk cadangan bila tak ada nama peran.
	anyHexRe = regexp.MustCompile(`#[0-9a-fA-F]{6}\b`)
)

var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, ".nuxt": true, ".output": true, "dist": true, "build": true, "vendor": true,
	".dart_tool": true, "Pods": true, ".gradle": true, "coverage": true, ".next": true, ".svelte-kit": true, "tmp": true,
	"storage": true, "ios": true, "windows": true, "linux": true, "macos": true,
}

var scanExt = map[string]bool{
	".css": true, ".scss": true, ".sass": true, ".less": true, ".vue": true, ".html": true, ".ts": true, ".js": true,
	".mjs": true, ".cjs": true, ".tsx": true, ".jsx": true, ".svelte": true, ".dart": true, ".xml": true, ".json": true,
	".webmanifest": true,
}

const (
	maxFiles    = 4000
	maxFileSize = 400 * 1024
)

type vote struct {
	hex    string
	weight int
	why    string
}

// Detect membaca warna merek dari folder aplikasi (frontend/mobile lebih dulu). Hasil kosong bila tidak ada tanda.
func Detect(dirs []string) Colors {
	roles := map[string][]vote{} // primary | secondary | accent
	familyCount := map[string]int{}
	hexCount := map[string]int{}
	seen := 0
	for _, root := range dirs {
		if root == "" {
			continue
		}
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			if seen >= maxFiles {
				return filepath.SkipAll
			}
			name := d.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if !scanExt[ext] || strings.Contains(name, ".min.") || strings.HasSuffix(name, ".lock") {
				return nil
			}
			if ext == ".json" && name != "manifest.json" && name != "site.webmanifest" {
				return nil
			}
			if ext == ".xml" && name != "colors.xml" {
				return nil
			}
			info, err := d.Info()
			if err != nil || info.Size() > maxFileSize {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			seen++
			rel, _ := filepath.Rel(root, path)
			scanFile(filepath.ToSlash(filepath.Join(filepath.Base(root), rel)), name, string(raw), roles, familyCount, hexCount)
			return nil
		})
	}
	return decide(roles, familyCount, hexCount)
}

func roleOf(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "secondary"), strings.Contains(n, "tertiary"):
		return "secondary"
	case strings.Contains(n, "accent"):
		return "accent"
	}
	return "primary" // primary, brand, seedColor, primaryColor, theme-color
}

func scanFile(rel, name, text string, roles map[string][]vote, family, hexes map[string]int) {
	add := func(role, hex string, w int, why string) {
		if h := NormalizeHex(hex); h != "" {
			roles[role] = append(roles[role], vote{h, w, fmt.Sprintf("%s: %s", rel, why)})
		}
	}
	addCSS := func(role, val string, w int, why string) {
		if c, ok := ParseCSS(val); ok {
			roles[role] = append(roles[role], vote{c.Hex(), w, fmt.Sprintf("%s: %s", rel, why)})
		}
	}
	for _, m := range cssVarRe.FindAllStringSubmatch(text, -1) {
		addCSS(roleOf(m[1]), m[2], 10, strings.TrimSpace(m[0]))
	}
	for _, m := range scssVarRe.FindAllStringSubmatch(text, -1) {
		addCSS(roleOf(m[1]), m[2], 9, strings.TrimSpace(m[0]))
	}
	if strings.HasPrefix(name, "tailwind.config") || strings.HasPrefix(name, "uno.config") || strings.HasPrefix(name, "app.config") || strings.HasPrefix(name, "nuxt.config") {
		for _, m := range twNestedRe.FindAllStringSubmatch(text, -1) {
			add(roleOf(m[1]), m[2], 10, m[1]+" 500 "+m[2])
		}
		for _, m := range twFlatRe.FindAllStringSubmatch(text, -1) {
			add(roleOf(m[1]), m[2], 9, m[1]+": "+m[2])
		}
	}
	if strings.HasPrefix(name, "app.config") {
		for _, m := range nuxtUIRe.FindAllStringSubmatch(text, -1) {
			if hex, ok := tailwind500[strings.ToLower(m[2])]; ok {
				add(roleOf(m[1]), hex, 11, fmt.Sprintf("%s '%s' (Nuxt UI)", m[1], m[2]))
			}
		}
	}
	if strings.HasSuffix(name, ".dart") {
		for _, m := range flutterHexRe.FindAllStringSubmatch(text, -1) {
			add(roleOf(m[1]), "#"+m[2][2:], 11, m[0])
		}
		for _, m := range flutterNameRe.FindAllStringSubmatch(text, -1) {
			if hex, ok := material500[m[2]]; ok {
				add(roleOf(m[1]), hex, 10, m[0])
			}
		}
	}
	if name == "colors.xml" {
		for _, m := range androidRe.FindAllStringSubmatch(text, -1) {
			h := m[2]
			if len(h) == 9 { // #AARRGGBB
				h = "#" + h[3:]
			}
			add(roleOf(m[1]), h, 10, m[1]+" "+m[2])
		}
	}
	for _, m := range themeColorRe.FindAllStringSubmatch(text, -1) {
		add("primary", m[1], 6, "theme-color "+m[1])
	}
	if strings.HasSuffix(name, ".vue") || strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".tsx") || strings.HasSuffix(name, ".jsx") || strings.HasSuffix(name, ".svelte") {
		for _, m := range twClassRe.FindAllStringSubmatch(text, -1) {
			family[m[1]]++
		}
	}
	if !strings.HasSuffix(name, ".dart") && !strings.HasSuffix(name, ".json") {
		for _, h := range anyHexRe.FindAllString(text, -1) {
			if c, ok := ParseHex(h); ok && c.Chromatic() {
				hexes[c.Hex()]++
			}
		}
	}
}

// decide memilih satu warna per peran: bobot tertinggi menang, lalu yang paling sering muncul.
// Tanpa nama peran sama sekali, keluarga kelas Tailwind atau hex jenuh yang paling sering dipakai menjadi warna utama.
func decide(roles map[string][]vote, family, hexes map[string]int) Colors {
	out := Colors{Source: "detected"}
	pick := func(vs []vote) *vote {
		if len(vs) == 0 {
			return nil
		}
		type agg struct {
			v     vote
			score int
		}
		by := map[string]*agg{}
		for _, v := range vs {
			c, _ := ParseHex(v.hex)
			if !c.Chromatic() { // putih/hitam/abu sebagai "primary" (mis. shadcn bawaan) bukan warna merek
				continue
			}
			a := by[v.hex]
			if a == nil {
				a = &agg{v: v}
				by[v.hex] = a
			}
			if v.weight > a.v.weight {
				a.v = v
			}
			a.score += v.weight
		}
		var best *agg
		for _, a := range by {
			if best == nil || a.v.weight > best.v.weight || (a.v.weight == best.v.weight && a.score > best.score) ||
				(a.v.weight == best.v.weight && a.score == best.score && a.v.hex < best.v.hex) {
				best = a
			}
		}
		if best == nil {
			return nil
		}
		return &best.v
	}
	if v := pick(roles["primary"]); v != nil {
		out.Primary = v.hex
		out.Evidence = append(out.Evidence, "Utama "+v.hex+" — "+v.why)
	}
	if v := pick(roles["secondary"]); v != nil && v.hex != out.Primary {
		out.Secondary = v.hex
		out.Evidence = append(out.Evidence, "Pendamping "+v.hex+" — "+v.why)
	}
	if v := pick(roles["accent"]); v != nil && v.hex != out.Primary {
		out.Accent = v.hex
		out.Evidence = append(out.Evidence, "Aksen "+v.hex+" — "+v.why)
	}
	if out.Primary == "" {
		if f, n := topKey(family); n >= 3 {
			out.Primary = tailwind500[f]
			out.Evidence = append(out.Evidence, fmt.Sprintf("Utama %s — kelas Tailwind '%s' paling sering dipakai (%d kali)", out.Primary, f, n))
		} else if h, n := topKey(hexes); n >= 3 {
			out.Primary = h
			out.Evidence = append(out.Evidence, fmt.Sprintf("Utama %s — warna jenuh paling sering di stylesheet (%d kali)", h, n))
		}
	}
	if out.Accent == "" && out.Primary != "" {
		// Aksen cadangan: hex jenuh tersering yang huenya jelas berbeda dari warna utama.
		p, _ := ParseHex(out.Primary)
		ph, _, _ := p.HSL()
		keys := sortedKeys(hexes)
		for _, h := range keys {
			c, _ := ParseHex(h)
			hh, _, _ := c.HSL()
			d := hh - ph
			if d < 0 {
				d = -d
			}
			if d > 180 {
				d = 360 - d
			}
			if d >= 40 && hexes[h] >= 3 {
				out.Accent = h
				out.Evidence = append(out.Evidence, fmt.Sprintf("Aksen %s — warna jenuh lain yang sering dipakai (%d kali)", h, hexes[h]))
				break
			}
		}
	}
	if out.Primary == "" {
		return Colors{}
	}
	return out
}

func topKey(m map[string]int) (string, int) {
	keys := sortedKeys(m)
	if len(keys) == 0 {
		return "", 0
	}
	return keys[0], m[keys[0]]
}

// sortedKeys: paling sering lebih dulu, seri diurutkan menurut nama agar hasil deterministik.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
