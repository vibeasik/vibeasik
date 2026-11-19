package docpdf

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/vibeasik/vibeasik/pkg/brandcolor"
)

// Ukuran halaman A4 dalam mm dan bidang tulis.
const (
	pageW     = 210.0
	pageH     = 297.0
	marginX   = 20.0
	headerH   = 13.0
	footerH   = 11.0
	contentY  = headerH + 12
	bottomGap = footerH + 11 // jarak isi dari tepi bawah
	textW     = pageW - 2*marginX

	bodySize = 9.6
	bodyLH   = 5.1 // ±1.5 × ukuran huruf isi
)

// Render menghasilkan PDF. Dirender dua kali: putaran pertama mencatat halaman awal tiap bagian, putaran kedua
// memakai angka itu untuk daftar isi dan "halaman x dari y" (tata letaknya sama persis di kedua putaran).
func Render(doc Document, colors brandcolor.Colors, now time.Time) ([]byte, error) {
	doc.Sanitize()
	th := NewTheme(colors)
	first := newRenderer(doc, th, nil, 0, now)
	if err := first.run(); err != nil {
		return nil, err
	}
	final := newRenderer(doc, th, first.sectionPage, first.pdf.PageNo(), now)
	if err := final.run(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := final.pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type renderer struct {
	pdf         *fpdf.Fpdf
	th          Theme
	doc         Document
	now         time.Time
	knownPages  []int // halaman awal tiap bagian dari putaran pertama (nil di putaran pertama)
	totalPages  int
	sectionPage []int
	links       []int
	running     string // judul bagian yang tampil di header
}

func newRenderer(doc Document, th Theme, pages []int, total int, now time.Time) *renderer {
	p := fpdf.New("P", "mm", "A4", "")
	p.SetCompression(true)
	p.SetCreationDate(now)
	p.SetModificationDate(now)
	p.SetTitle(doc.Title, true)
	p.SetAuthor(firstNonEmpty(doc.Authors, "vibeasik Studio"), true)
	p.SetCreator("vibeasik", true)
	p.SetSubject(firstNonEmpty(doc.Kicker, "Dokumen teknis"), true)
	p.AddUTF8FontFromBytes("sans", "", goregular.TTF)
	p.AddUTF8FontFromBytes("sans", "B", gobold.TTF)
	p.AddUTF8FontFromBytes("sans", "I", goitalic.TTF)
	p.AddUTF8FontFromBytes("sans", "BI", gobolditalic.TTF)
	p.AddUTF8FontFromBytes("mono", "", gomono.TTF)
	p.AddUTF8FontFromBytes("mono", "B", gomonobold.TTF)
	p.SetMargins(marginX, contentY, marginX)
	p.SetAutoPageBreak(true, bottomGap)
	p.SetCellMargin(0)
	r := &renderer{pdf: p, th: th, doc: doc, now: now, knownPages: pages, totalPages: total,
		sectionPage: make([]int, len(doc.Sections)), links: make([]int, len(doc.Sections))}
	p.SetHeaderFuncMode(r.header, true)
	p.SetFooterFunc(r.footer)
	return r
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func (r *renderer) run() error {
	for i := range r.doc.Sections {
		r.links[i] = r.pdf.AddLink()
	}
	r.cover()
	r.toc()
	for i := range r.doc.Sections {
		r.section(i)
	}
	return r.pdf.Error()
}

// ---------- warna & huruf ----------

func (r *renderer) fill(c brandcolor.RGB) { r.pdf.SetFillColor(int(c.R), int(c.G), int(c.B)) }
func (r *renderer) ink(c brandcolor.RGB)  { r.pdf.SetTextColor(int(c.R), int(c.G), int(c.B)) }
func (r *renderer) draw(c brandcolor.RGB) { r.pdf.SetDrawColor(int(c.R), int(c.G), int(c.B)) }
func (r *renderer) font(family, style string, size float64) {
	r.pdf.SetFont(family, style, size)
}

func (r *renderer) limitY() float64 { return pageH - bottomGap }

// ensure memindah ke halaman baru bila sisa ruang kurang dari h mm.
func (r *renderer) ensure(h float64) {
	if r.pdf.GetY()+h > r.limitY() {
		r.pdf.AddPage()
	}
}

// ---------- header & footer ----------

func (r *renderer) header() {
	if r.pdf.PageNo() == 1 {
		return
	}
	p := r.pdf
	r.fill(r.th.Primary)
	p.Rect(0, 0, pageW, headerH, "F")
	r.fill(r.th.Accent)
	p.Rect(0, headerH, pageW, 0.9, "F")
	r.ink(r.th.OnPrimary)
	r.font("sans", "B", 8.4)
	title := r.fit(r.doc.Title, textW*0.48)
	p.SetXY(marginX, 0)
	p.CellFormat(textW*0.5, headerH, title, "", 0, "LM", false, 0, "")
	r.font("sans", "", 8.4)
	p.SetXY(marginX+textW*0.5, 0)
	p.CellFormat(textW*0.5, headerH, r.fit(r.running, textW*0.48), "", 0, "RM", false, 0, "")
	p.SetXY(marginX, contentY)
}

func (r *renderer) footer() {
	n := r.pdf.PageNo()
	if n == 1 {
		return
	}
	p := r.pdf
	y := pageH - footerH
	r.fill(r.th.Secondary)
	p.Rect(0, y, pageW, footerH, "F")
	r.fill(r.th.Accent)
	p.Rect(0, y, pageW, 0.9, "F")
	r.ink(r.th.OnSecondary)
	r.font("sans", "", 7.8)
	left := firstNonEmpty(r.doc.Kicker, "Dokumen teknis")
	if r.doc.Version != "" {
		left += " · Versi " + r.doc.Version
	}
	if r.doc.Date != "" {
		left += " · " + r.doc.Date
	}
	p.SetXY(marginX, y+0.9)
	p.CellFormat(textW*0.7, footerH-0.9, r.fit(left, textW*0.68), "", 0, "LM", false, 0, "")
	page := fmt.Sprintf("Halaman %d", n)
	if r.totalPages > 0 {
		page = fmt.Sprintf("Halaman %d dari %d", n, r.totalPages)
	}
	r.font("sans", "B", 7.8)
	p.SetXY(marginX+textW*0.7, y+0.9)
	p.CellFormat(textW*0.3, footerH-0.9, page, "", 0, "RM", false, 0, "")
}

// fit memotong teks sebaris dengan elipsis agar muat w mm pada huruf aktif.
func (r *renderer) fit(s string, w float64) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if r.pdf.GetStringWidth(s) <= w {
		return s
	}
	rs := []rune(s)
	for len(rs) > 1 && r.pdf.GetStringWidth(string(rs)+"…") > w {
		rs = rs[:len(rs)-1]
	}
	return strings.TrimSpace(string(rs)) + "…"
}

// ---------- sampul ----------

func (r *renderer) cover() {
	p := r.pdf
	p.AddPage()
	// Sampul digambar bebas sampai tepi bawah: pindah halaman otomatis dimatikan sementara.
	p.SetAutoPageBreak(false, 0)
	defer p.SetAutoPageBreak(true, bottomGap)
	th := r.th
	bandH := 196.0
	r.fill(th.Primary)
	p.Rect(0, 0, pageW, bandH, "F")
	// Bentuk dekoratif: dua lingkaran besar sedikit lebih terang/gelap dari warna utama, terpotong tepi halaman.
	r.fill(th.Primary.Mix(th.OnPrimary, 0.07))
	p.Circle(pageW-18, 34, 70, "F")
	r.fill(th.Primary.Mix(th.OnPrimary, 0.12))
	p.Circle(pageW+8, 150, 46, "F")
	r.fill(th.Accent)
	p.Rect(0, bandH, pageW, 2.4, "F")

	soft := th.OnPrimary.Mix(th.Primary, 0.22)
	r.ink(soft)
	r.font("sans", "B", 9)
	p.SetXY(marginX+2, 46)
	p.CellFormat(textW, 6, strings.ToUpper(firstNonEmpty(r.doc.Kicker, "Dokumen teknis")), "", 1, "L", false, 0, "")
	r.fill(th.OnPrimary)
	p.Rect(marginX+2, 55, 14, 1.1, "F")

	r.ink(th.OnPrimary)
	r.font("sans", "B", 32)
	p.SetXY(marginX+2, 64)
	title := p.SplitText(r.doc.Title, textW-24)
	if len(title) > 4 {
		title = append(title[:3], r.fit(strings.Join(title[3:], " "), textW-24))
	}
	for _, ln := range title {
		p.SetX(marginX + 2)
		p.CellFormat(textW-24, 13, ln, "", 1, "L", false, 0, "")
	}
	if r.doc.Subtitle != "" {
		r.ink(soft)
		r.font("sans", "", 12.5)
		p.SetXY(marginX+2, p.GetY()+5)
		lines := p.SplitText(plain(r.doc.Subtitle), textW-34)
		if len(lines) > 6 {
			lines = append(lines[:5], r.fit(strings.Join(lines[5:], " "), textW-34))
		}
		for _, ln := range lines {
			p.SetX(marginX + 2)
			p.CellFormat(textW-34, 6.6, ln, "", 1, "L", false, 0, "")
		}
	}
	// Versi & penyusun di dasar bidang warna.
	var by []string
	if r.doc.Version != "" {
		by = append(by, "Versi "+r.doc.Version)
	}
	if r.doc.Authors != "" {
		by = append(by, r.doc.Authors)
	}
	if len(by) > 0 {
		r.ink(th.OnPrimary)
		r.font("sans", "B", 10)
		p.SetXY(marginX+2, bandH-20)
		p.CellFormat(textW, 6, r.fit(strings.Join(by, "  ·  "), textW), "", 1, "L", false, 0, "")
	}

	// Keterangan proyek: dua kolom label kecil + nilai.
	y := bandH + 14
	colW := (textW - 8) / 2
	for i, m := range r.doc.Meta {
		if m.V == "" {
			continue
		}
		x := marginX + float64(i%2)*(colW+8)
		if i%2 == 0 && i > 0 {
			y += 13
		}
		if y > pageH-36 {
			break
		}
		r.ink(th.Muted)
		r.font("sans", "B", 7)
		p.SetXY(x, y)
		p.CellFormat(colW, 4, strings.ToUpper(m.K), "", 0, "L", false, 0, "")
		r.ink(th.Ink)
		r.font(fontFor(m.V), "", 9.6)
		p.SetXY(x, y+4.4)
		p.CellFormat(colW, 5.5, r.fit(m.V, colW), "", 0, "L", false, 0, "")
	}

	// Kaki sampul: palet merek & keterangan penyusun.
	fy := pageH - 20.0
	r.draw(th.Line)
	p.SetLineWidth(0.25)
	p.Line(marginX, fy-4, pageW-marginX, fy-4)
	r.ink(th.Muted)
	r.font("sans", "", 7.8)
	p.SetXY(marginX, fy)
	made := "Disusun oleh vibeasik Studio"
	if r.doc.Date != "" {
		made += " · " + r.doc.Date
	}
	p.CellFormat(textW*0.6, 5, made, "", 0, "L", false, 0, "")
	if len(th.Swatches) > 0 {
		label := "Warna merek proyek"
		r.font("sans", "", 7.8)
		lw := p.GetStringWidth(label) + 3
		x := pageW - marginX - float64(len(th.Swatches))*6.5
		p.SetXY(x-lw, fy)
		p.CellFormat(lw, 5, label, "", 0, "L", false, 0, "")
		for i, s := range th.Swatches {
			r.fill(s)
			p.RoundedRect(x+float64(i)*6.5, fy+0.2, 5, 4.6, 1.2, "1234", "F")
		}
	}
}

// fontFor memakai huruf mono untuk nilai yang tampak teknis (path, URL, branch).
func fontFor(v string) string {
	if strings.ContainsAny(v, "/@:") && !strings.Contains(v, " ") {
		return "mono"
	}
	return "sans"
}

// ---------- daftar isi ----------

func (r *renderer) toc() {
	p := r.pdf
	r.running = "Daftar isi"
	p.AddPage()
	r.ink(r.th.Ink)
	r.font("sans", "B", 21)
	p.SetX(marginX)
	p.CellFormat(textW, 10, "Daftar isi", "", 1, "L", false, 0, "")
	r.fill(r.th.Primary)
	p.Rect(marginX, p.GetY()+1, 16, 1.1, "F")
	p.SetY(p.GetY() + 9)

	for i, s := range r.doc.Sections {
		sub := s.Level > 0
		h := 7.4
		if sub {
			h = 5.6
		}
		if p.GetY()+h > r.limitY() {
			p.AddPage()
		}
		y := p.GetY()
		x := marginX
		if sub {
			x += 11
		}
		numW := 11.0
		if sub {
			numW = 12
		}
		page := ""
		if r.knownPages != nil && i < len(r.knownPages) && r.knownPages[i] > 0 {
			page = fmt.Sprintf("%d", r.knownPages[i])
		}
		size := 10.4
		if sub {
			size = 9
		}
		r.font("sans", "B", size)
		r.ink(r.th.BrandText)
		if sub {
			r.ink(r.th.Muted)
			r.font("sans", "", size)
		}
		p.SetXY(x, y)
		p.CellFormat(numW, h, s.No, "", 0, "L", false, 0, "")
		r.ink(r.th.Ink)
		if sub {
			r.ink(r.th.Body)
		}
		r.font("sans", "", size)
		title := r.fit(s.Title, textW-numW-(x-marginX)-18)
		tw := p.GetStringWidth(title)
		p.CellFormat(tw, h, title, "", 0, "L", false, 0, "")
		// Titik pemandu sampai nomor halaman.
		r.font("sans", "B", size)
		pw := p.GetStringWidth(page)
		lx1 := x + numW + tw + 2
		lx2 := pageW - marginX - pw - 2
		if lx2 > lx1 {
			r.draw(r.th.Line.Mix(brandcolor.Black, 0.12))
			p.SetLineWidth(0.3)
			p.SetDashPattern([]float64{0.3, 1.2}, 0)
			p.Line(lx1, y+h*0.68, lx2, y+h*0.68)
			p.SetDashPattern([]float64{}, 0)
		}
		r.ink(r.th.Ink)
		p.SetXY(pageW-marginX-pw, y)
		p.CellFormat(pw, h, page, "", 0, "R", false, 0, "")
		p.Link(x, y, pageW-marginX-x, h, r.links[i])
		p.SetXY(marginX, y+h)
		if !sub && i+1 < len(r.doc.Sections) && r.doc.Sections[i+1].Level == 0 {
			p.SetY(p.GetY() + 0.6)
		}
	}
}

// ---------- bagian ----------

func (r *renderer) section(i int) {
	p := r.pdf
	s := r.doc.Sections[i]
	if s.Level == 0 {
		// Bagian utama dimulai di halaman baru bila sisa ruang tinggal sedikit, agar judul tidak menggantung.
		if i == 0 || p.GetY()+62 > r.limitY() {
			r.running = s.No + "  " + s.Title
			p.AddPage()
		} else {
			p.SetY(p.GetY() + 9)
			r.running = s.No + "  " + s.Title
		}
		y := p.GetY()
		r.sectionPage[i] = p.PageNo()
		p.SetLink(r.links[i], y, p.PageNo())
		p.Bookmark(strings.TrimSpace(s.No+" "+s.Title), 0, y)
		r.font("sans", "B", 17)
		r.ink(r.th.BrandText)
		numW := p.GetStringWidth(s.No) + 4
		p.SetX(marginX)
		p.CellFormat(numW, 9, s.No, "", 0, "L", false, 0, "")
		r.ink(r.th.Ink)
		lines := p.SplitText(s.Title, textW-numW)
		for li, ln := range lines {
			p.SetX(marginX + numW)
			p.CellFormat(textW-numW, 8.6, ln, "", 1, "L", false, 0, "")
			if li == 3 {
				break
			}
		}
		ly := p.GetY() + 2
		r.draw(r.th.Line)
		p.SetLineWidth(0.3)
		p.Line(marginX, ly, pageW-marginX, ly)
		r.fill(r.th.Primary)
		p.Rect(marginX, ly-0.5, 22, 1.0, "F")
		p.SetY(ly + 5.5)
	} else {
		r.ensure(22)
		p.SetY(p.GetY() + 4)
		y := p.GetY()
		r.sectionPage[i] = p.PageNo()
		p.SetLink(r.links[i], y, p.PageNo())
		p.Bookmark(strings.TrimSpace(s.No+" "+s.Title), 1, y)
		r.font("sans", "B", 12)
		r.ink(r.th.BrandText)
		numW := p.GetStringWidth(s.No) + 3
		p.SetX(marginX)
		p.CellFormat(numW, 7, s.No, "", 0, "L", false, 0, "")
		r.ink(r.th.Ink)
		p.CellFormat(textW-numW, 7, r.fit(s.Title, textW-numW), "", 1, "L", false, 0, "")
		p.SetY(p.GetY() + 2.5)
	}
	for _, b := range s.Blocks {
		r.block(b)
	}
}
