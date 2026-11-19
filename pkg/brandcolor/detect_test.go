package brandcolor

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseCSSFormats(t *testing.T) {
	cases := map[string]string{
		"#0EA5E9":                  "#0ea5e9",
		"#abc":                     "#aabbcc",
		"rgb(14, 165, 233)":        "#0ea5e9",
		"hsl(199 89% 48%)":         "#0ea5e9",
		"oklch(68.5% 0.169 237.3)": "#00a6f4", // sky-500 Tailwind v4
		"199 89% 48%":              "#0ea5e9",
	}
	for in, want := range cases {
		c, ok := ParseCSS(in)
		if !ok {
			t.Fatalf("%q tidak terbaca", in)
		}
		// Toleransi pembulatan konversi warna: tiap kanal boleh meleset sedikit.
		w, _ := ParseHex(want)
		if d := absDiff(c.R, w.R) + absDiff(c.G, w.G) + absDiff(c.B, w.B); d > 9 {
			t.Errorf("%q → %s, ingin ≈ %s", in, c.Hex(), want)
		}
	}
	if _, ok := ParseCSS("var(--x)"); ok {
		t.Error("var() bukan warna")
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestDetectNuxtUIAndCSSVariables(t *testing.T) {
	root := t.TempDir()
	write(t, root, "app/app.config.ts", `export default defineAppConfig({ ui: { colors: { primary: 'emerald', neutral: 'slate' } } })`)
	write(t, root, "app/assets/css/main.css", `:root { --color-accent: #f97316; --primary: 0 0% 9%; }`)
	write(t, root, "node_modules/x/style.css", `:root { --primary: #ff0000 }`)
	got := Detect([]string{root})
	if got.Primary != "#10b981" || got.Accent != "#f97316" {
		t.Fatalf("dapat %+v, ingin primary emerald & accent oranye", got)
	}
	if got.Source != "detected" || len(got.Evidence) < 2 {
		t.Errorf("bukti kurang: %+v", got)
	}
}

func TestDetectFlutterAndAndroid(t *testing.T) {
	root := t.TempDir()
	write(t, root, "lib/theme.dart", `final theme = ThemeData(colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF1B5E20), secondary: Colors.amber));`)
	write(t, root, "android/app/src/main/res/values/colors.xml", `<resources><color name="colorAccent">#FFE91E63</color></resources>`)
	got := Detect([]string{root})
	if got.Primary != "#1b5e20" || got.Secondary != "#ffc107" || got.Accent != "#e91e63" {
		t.Fatalf("dapat %+v", got)
	}
}

func TestDetectFallsBackToTailwindClassesAndEmpty(t *testing.T) {
	root := t.TempDir()
	write(t, root, "pages/index.vue", `<div class="bg-sky-600 text-sky-50"><a class="text-sky-700 border-sky-200">x</a><b class="text-rose-500"/></div>`)
	if got := Detect([]string{root}); got.Primary != "#0ea5e9" {
		t.Fatalf("dapat %+v, ingin sky", got)
	}
	if got := Detect([]string{t.TempDir(), "", "/tidak/ada"}); !got.Empty() {
		t.Fatalf("folder kosong harus tanpa warna, dapat %+v", got)
	}
}

func TestOnColorAndDarken(t *testing.T) {
	yellow, _ := ParseHex("#faa61a")
	if OnColor(yellow) == White {
		t.Error("teks di atas kuning harus gelap")
	}
	navy, _ := ParseHex("#0b1f3a")
	if OnColor(navy) != White {
		t.Error("teks di atas navy harus putih")
	}
	if Contrast(Darken(yellow, White, 4.5), White) < 4.5 {
		t.Error("Darken harus mencapai kontras 4.5")
	}
}
