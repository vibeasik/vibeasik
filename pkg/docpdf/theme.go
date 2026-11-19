package docpdf

import "github.com/vibeasik/vibeasik/pkg/brandcolor"

// Theme adalah warna dokumen yang diturunkan dari warna merek proyek. Warna merek hanya mengisi bidang identitas
// (sampul, header, footer, nomor bagian, kepala tabel); isi tetap tinta gelap di atas putih agar mudah dibaca.
type Theme struct {
	Primary     brandcolor.RGB // bidang sampul & header
	OnPrimary   brandcolor.RGB // teks di atas Primary
	Secondary   brandcolor.RGB // bidang footer
	OnSecondary brandcolor.RGB
	Accent      brandcolor.RGB // garis aksen tipis
	BrandText   brandcolor.RGB // warna merek yang cukup kontras untuk teks di atas putih
	Tint        brandcolor.RGB // latar sangat tipis (kepala tabel, kotak catatan)
	TintStrong  brandcolor.RGB
	Ink         brandcolor.RGB
	Body        brandcolor.RGB
	Muted       brandcolor.RGB
	Line        brandcolor.RGB
	Zebra       brandcolor.RGB
	CodeBg      brandcolor.RGB
	Swatches    []brandcolor.RGB // palet asli untuk ditampilkan di sampul
	Detected    bool
}

// Warna bawaan bila proyek belum punya warna merek: biru tua netral yang terbaca di cetak hitam-putih.
const defaultPrimary = "#0b4f7c"

func hex(s string) brandcolor.RGB {
	c, _ := brandcolor.ParseHex(s)
	return c
}

// NewTheme menyusun tema dari warna merek; warna kosong diturunkan dari warna utama.
func NewTheme(c brandcolor.Colors) Theme {
	c.Normalize()
	t := Theme{
		Ink: hex("#111827"), Body: hex("#273142"), Muted: hex("#6b7280"), Line: hex("#e3e6eb"),
		Zebra: hex("#f8f9fb"), CodeBg: hex("#f5f7fa"), Detected: !c.Empty(),
	}
	primary := hex(defaultPrimary)
	if !c.Empty() {
		primary = hex(c.Primary)
		t.Swatches = append(t.Swatches, primary)
	}
	t.Primary = primary
	t.OnPrimary = brandcolor.OnColor(primary)

	var accent brandcolor.RGB
	hasAccent := c.Accent != ""
	if hasAccent {
		accent = hex(c.Accent)
	}
	switch {
	case c.Secondary != "":
		t.Secondary = hex(c.Secondary)
		t.Swatches = append(t.Swatches, t.Secondary)
	case hasAccent && brandcolor.Contrast(accent, primary) >= 1.8 && accent.Luminance() < 0.3:
		// Aksen gelap yang kontras dengan warna utama (mis. kuning + navy) lebih serasi untuk footer daripada
		// warna utama yang digelapkan.
		t.Secondary = accent
	default:
		t.Secondary = primary.Mix(brandcolor.Black, 0.45)
	}
	t.OnSecondary = brandcolor.OnColor(t.Secondary)

	if hasAccent {
		t.Accent = accent
		t.Swatches = append(t.Swatches, t.Accent)
	} else {
		t.Accent = primary.Mix(brandcolor.White, 0.35)
	}
	t.BrandText = brandcolor.Darken(primary, brandcolor.White, 4.5)
	t.Tint = primary.Mix(brandcolor.White, 0.93)
	t.TintStrong = primary.Mix(brandcolor.White, 0.82)
	return t
}

// toneColor: warna teks untuk status sel/badge.
func (t Theme) toneColor(tone string) (brandcolor.RGB, bool) {
	switch tone {
	case "success", "get":
		if tone == "get" {
			return hex("#0369a1"), true
		}
		return hex("#047857"), true
	case "post":
		return hex("#047857"), true
	case "warning", "warn", "put", "patch":
		return hex("#b45309"), true
	case "error", "delete":
		return hex("#be123c"), true
	case "info":
		return t.BrandText, true
	case "muted":
		return t.Muted, true
	}
	return brandcolor.RGB{}, false
}
