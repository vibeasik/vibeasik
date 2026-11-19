// Package docpdf merender Dokumen Proyek menjadi PDF di engine (tanpa browser): sampul berwarna merek proyek,
// daftar isi bertautan dengan nomor halaman, header & footer berwarna, lalu isi bagian demi bagian.
// Isinya dikirim Studio sebagai blok sederhana (paragraf, daftar, tabel, kode, angka) sehingga renderer tidak
// perlu tahu asal datanya.
package docpdf

import (
	"encoding/json"
	"strings"
	"unicode"
)

// Document adalah dokumen utuh yang akan dirender.
type Document struct {
	Title    string    `json:"title"`
	Subtitle string    `json:"subtitle,omitempty"`
	Kicker   string    `json:"kicker,omitempty"` // label kecil di atas judul sampul, mis. "Dokumen teknis"
	Version  string    `json:"version,omitempty"`
	Authors  string    `json:"authors,omitempty"`
	Date     string    `json:"date,omitempty"`
	Meta     []Pair    `json:"meta,omitempty"` // keterangan di sampul (repositori, branch, status, …)
	Sections []Section `json:"sections"`
}

// Section adalah satu bagian bernomor; Level 1 = sub-bagian (mis. 12.1) yang ikut masuk daftar isi.
type Section struct {
	No     string  `json:"no"`
	Title  string  `json:"title"`
	Level  int     `json:"level,omitempty"`
	Blocks []Block `json:"blocks,omitempty"`
}

// Block adalah satu unsur isi. Type: p, lead, muted, h, list, olist, table, kv, code, stats, note.
type Block struct {
	Type   string    `json:"type"`
	Text   string    `json:"text,omitempty"`
	Items  []string  `json:"items,omitempty"`
	Head   []string  `json:"head,omitempty"`
	Rows   [][]Cell  `json:"rows,omitempty"`
	Widths []float64 `json:"widths,omitempty"` // porsi lebar kolom tabel (dinormalkan)
	Pairs  []Pair    `json:"pairs,omitempty"`  // kv & stats
	Title  string    `json:"title,omitempty"`  // judul blok kode / catatan
	Lang   string    `json:"lang,omitempty"`
	Tone   string    `json:"tone,omitempty"` // note: info | warn | success
}

// Pair: label & nilai (kv, stats, meta sampul).
type Pair struct {
	K    string `json:"k"`
	V    string `json:"v"`
	Tone string `json:"tone,omitempty"`
}

// Cell adalah sel tabel. Di JSON boleh string biasa atau objek {t, sub, tone, mono, bold, align}.
type Cell struct {
	T     string `json:"t"`
	Sub   string `json:"sub,omitempty"`  // baris kedua kecil berwarna redup
	Tone  string `json:"tone,omitempty"` // success | warning | error | info | muted | get | post | put | delete
	Mono  bool   `json:"mono,omitempty"`
	Bold  bool   `json:"bold,omitempty"`
	Align string `json:"align,omitempty"` // right
}

func (c *Cell) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*c = Cell{T: s}
		return nil
	}
	type plain Cell
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*c = Cell(p)
	return nil
}

// Batas isi agar satu permintaan tidak menghasilkan PDF raksasa.
const (
	maxSections = 80
	maxBlocks   = 300
	maxRows     = 1500
	maxCols     = 8
	maxItems    = 400
	maxText     = 20000
	maxCodeLine = 400
)

// clean membuang karakter kendali dan rune di luar BMP (font tidak punya glyph-nya; fpdf mengindeks lebar per rune BMP).
func clean(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= maxText {
			b.WriteString("…")
			break
		}
		n++
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\t':
			b.WriteString("    ")
		case r == '\r', r > 0xFFFF, unicode.IsControl(r):
		case r == '✓' || r == '✔':
			b.WriteString("Ya")
		case r == '✗' || r == '✘':
			b.WriteString("Tidak")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Sanitize membersihkan teks dan memangkas jumlah unsur ke batas aman.
func (d *Document) Sanitize() {
	d.Title, d.Subtitle, d.Kicker = clean(d.Title), clean(d.Subtitle), clean(d.Kicker)
	d.Version, d.Authors, d.Date = clean(d.Version), clean(d.Authors), clean(d.Date)
	if strings.TrimSpace(d.Title) == "" {
		d.Title = "Dokumen Proyek"
	}
	d.Meta = clip(d.Meta, 16)
	for i := range d.Meta {
		d.Meta[i].K, d.Meta[i].V = clean(d.Meta[i].K), clean(d.Meta[i].V)
	}
	d.Sections = clip(d.Sections, maxSections)
	for si := range d.Sections {
		s := &d.Sections[si]
		s.No, s.Title = clean(s.No), clean(s.Title)
		if s.Level < 0 || s.Level > 1 {
			s.Level = 1
		}
		s.Blocks = clip(s.Blocks, maxBlocks)
		for bi := range s.Blocks {
			b := &s.Blocks[bi]
			b.Text, b.Title, b.Lang = clean(b.Text), clean(b.Title), clean(b.Lang)
			b.Items = clip(b.Items, maxItems)
			for i := range b.Items {
				b.Items[i] = clean(b.Items[i])
			}
			b.Head = clip(b.Head, maxCols)
			for i := range b.Head {
				b.Head[i] = clean(b.Head[i])
			}
			b.Rows = clip(b.Rows, maxRows)
			for ri := range b.Rows {
				b.Rows[ri] = clip(b.Rows[ri], maxCols)
				for ci := range b.Rows[ri] {
					c := &b.Rows[ri][ci]
					c.T, c.Sub = clean(c.T), clean(c.Sub)
				}
			}
			b.Pairs = clip(b.Pairs, 60)
			for i := range b.Pairs {
				b.Pairs[i].K, b.Pairs[i].V = clean(b.Pairs[i].K), clean(b.Pairs[i].V)
			}
		}
	}
}

func clip[T any](in []T, n int) []T {
	if len(in) > n {
		return in[:n]
	}
	return in
}
