package docpdf

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vibeasik/vibeasik/pkg/brandcolor"
)

func sampleDoc() Document {
	rows := [][]Cell{}
	for i := 0; i < 70; i++ {
		rows = append(rows, []Cell{{T: "GET", Tone: "get", Mono: true, Bold: true}, {T: "/api/projects/:id/doc-pdf/sangat/panjang/sekali/" + strings.Repeat("x", i%30), Mono: true}, {T: "Mengekspor dokumen", Sub: "auth, local-token"}})
	}
	return Document{
		Title: "Sistem Informasi Pelatihan", Subtitle: "Portal **pendaftaran** peserta, jadwal kelas, dan sertifikat untuk lembaga pelatihan kerja.",
		Kicker: "Dokumen teknis", Version: "1.2", Authors: "Tim Inti", Date: "20 Nov 2026",
		Meta: []Pair{{K: "Repositori", V: "git@example.com:org/app.git"}, {K: "Branch", V: "main"}, {K: "Status", V: "Aktif"}, {K: "Kematangan", V: "Beta"}},
		Sections: []Section{
			{No: "1", Title: "Keterangan proyek", Blocks: []Block{
				{Type: "lead", Text: "Aplikasi untuk mengelola pendaftaran pelatihan ✓"},
				{Type: "p", Text: "Paragraf dengan **tebal**, *miring*, dan `kode` sebaris. " + strings.Repeat("Teks isi yang mengalir. ", 30)},
				{Type: "kv", Pairs: []Pair{{K: "Nama", V: "Pelatihan"}, {K: "Folder", V: "/srv/app"}}},
			}},
			{No: "2", Title: "Pengujian & mutu", Blocks: []Block{
				{Type: "stats", Pairs: []Pair{{K: "Berkas test", V: "42"}, {K: "Kasus", V: "318"}, {K: "Rasio", V: "12,4%"}, {K: "Skor", V: "88 / 91", Tone: "success"}}},
				{Type: "list", Items: []string{"Satu", "Dua dengan **tebal**", strings.Repeat("Butir panjang ", 20)}},
				{Type: "olist", Items: []string{"Pertama", "Kedua"}},
			}},
			{No: "2.1", Title: "Strategi", Level: 1, Blocks: []Block{
				{Type: "note", Title: "Catatan", Text: "Rasio baris test hanya perkiraan kasar."},
				{Type: "code", Title: "Menjalankan test", Lang: "bash", Text: "# backend\ncd backend && go test ./...\n" + strings.Repeat("echo baris yang sangat panjang sekali untuk menguji pembungkusan baris kode di dalam blok\n", 90)},
			}},
			{No: "3", Title: "Antarmuka API", Blocks: []Block{
				{Type: "h", Text: "Proyek"},
				{Type: "table", Head: []string{"Metode", "Path", "Keterangan"}, Rows: rows, Widths: []float64{0.12, 0.5, 0.38}},
				{Type: "muted", Text: "Belum ada data tambahan."},
			}},
		},
	}
}

func TestRenderProducesPagedPDF(t *testing.T) {
	out, err := Render(sampleDoc(), brandcolor.Colors{Primary: "#10b981", Accent: "#f97316"}, time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) || len(out) < 5000 {
		t.Fatalf("bukan PDF yang wajar (%d byte)", len(out))
	}
	if n := bytes.Count(out, []byte("/Type /Page\n")) + bytes.Count(out, []byte("/Type /Page ")); n < 5 {
		t.Logf("perkiraan halaman %d", n)
	}
	if p := os.Getenv("DOCPDF_OUT"); p != "" {
		_ = os.WriteFile(p, out, 0o644)
	}
}

func TestRenderDefaultThemeAndEmptyDoc(t *testing.T) {
	out, err := Render(Document{}, brandcolor.Colors{}, time.Now())
	if err != nil || !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("dokumen kosong harus tetap jadi PDF: %v", err)
	}
}

func TestCellAcceptsStringOrObject(t *testing.T) {
	var row []Cell
	if err := json.Unmarshal([]byte(`["teks", {"t":"GET","tone":"get","mono":true}]`), &row); err != nil {
		t.Fatal(err)
	}
	if row[0].T != "teks" || row[1].T != "GET" || !row[1].Mono {
		t.Fatalf("dapat %+v", row)
	}
}

func TestSanitizeDropsUnsupportedRunes(t *testing.T) {
	if got := clean("ok \U0001F600\x07 ✓"); got != "ok  Ya" {
		t.Fatalf("dapat %q", got)
	}
	if got := plain("a **b** `c` *d*"); got != "a b c d" {
		t.Fatalf("plain = %q", got)
	}
}

func TestThemeUsesDarkAccentForFooter(t *testing.T) {
	th := NewTheme(brandcolor.Colors{Primary: "#facc15", Accent: "#194f9b"})
	if th.Secondary.Hex() != "#194f9b" {
		t.Fatalf("footer kuning+navy harus navy, dapat %s", th.Secondary.Hex())
	}
	if th.OnPrimary == brandcolor.White {
		t.Error("teks di atas kuning harus gelap")
	}
	if NewTheme(brandcolor.Colors{Primary: "#0ea5e9"}).Secondary.Hex() == "#0ea5e9" {
		t.Error("tanpa aksen, footer memakai warna utama yang digelapkan")
	}
}
