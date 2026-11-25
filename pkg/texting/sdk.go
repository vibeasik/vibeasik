package texting

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed sdk/vibeasik-texts.js
var browserScript string

// ScriptPath lokasi saran skrip di folder komponen (disajikan sebagai berkas statis /vibeasik-texts.js).
const ScriptPath = "public/vibeasik-texts.js"

// Script isi skrip browser (tanpa templating; kunci & URL dipasang lewat atribut data-*).
func Script() string { return browserScript }

// Usage = potongan pemasangan per jenis aplikasi web. Skrip hanya boleh dipasang di dev/staging.
type Usage struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Code  string `json:"code"`
}

// Usages mengembalikan cara memasang skrip untuk beberapa kerangka umum.
func Usages(url, key string) []Usage {
	return []Usage{
		{"html", "HTML biasa / Vite / React / Vue",
			fmt.Sprintf("<!-- di <head> index.html; hapus/atur hanya untuk dev & staging -->\n<script src=\"/vibeasik-texts.js\" data-url=\"%s\" data-key=\"%s\" defer></script>", url, key)},
		{"nuxt", "Nuxt",
			fmt.Sprintf("// nuxt.config.ts\nexport default defineNuxtConfig({\n  app: {\n    head: {\n      script: process.env.NODE_ENV === 'production' ? [] : [\n        { src: '/vibeasik-texts.js', 'data-url': '%s', 'data-key': '%s', defer: true }\n      ]\n    }\n  }\n})", url, key)},
		{"next", "Next.js",
			fmt.Sprintf("// app/layout.tsx — di dalam <body>\n{process.env.NODE_ENV !== 'production' && (\n  <script src=\"/vibeasik-texts.js\" data-url=\"%s\" data-key=\"%s\" defer />\n)}", url, key)},
	}
}

// InstallScript menulis skrip ke folder komponen; berkas yang sudah ada tidak ditimpa kecuali overwrite.
func InstallScript(dir string, overwrite bool) (written bool, err error) {
	target := filepath.Join(dir, filepath.FromSlash(ScriptPath))
	if _, statErr := os.Stat(target); statErr == nil && !overwrite {
		return false, nil
	}
	if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(target, []byte(browserScript), 0o644)
}
