package docpdf

import (
	"math"
	"regexp"
	"strings"

	"github.com/vibeasik/vibeasik/pkg/brandcolor"
)

func (r *renderer) block(b Block) {
	switch b.Type {
	case "lead":
		r.rich(b.Text, 11.4, 6.1, r.th.Ink, "")
		r.gap(3.2)
	case "muted":
		r.rich(b.Text, 8.6, 4.4, r.th.Muted, "I")
		r.gap(2.6)
	case "h":
		r.heading(b.Text)
	case "list", "olist":
		r.list(b.Items, b.Type == "olist")
	case "table":
		r.table(b)
	case "kv":
		r.kv(b.Pairs)
	case "code":
		r.code(b)
	case "stats":
		r.stats(b.Pairs)
	case "note":
		r.note(b)
	default: // p
		r.rich(b.Text, bodySize, bodyLH, r.th.Body, "")
		r.gap(2.6)
	}
}

func (r *renderer) gap(h float64) {
	if r.pdf.GetY()+h < r.limitY() {
		r.pdf.SetY(r.pdf.GetY() + h)
	}
}

// heading kecil di dalam bagian; dijaga agar tidak tertinggal sendirian di dasar halaman.
func (r *renderer) heading(t string) {
	if strings.TrimSpace(t) == "" {
		return
	}
	r.ensure(34) // judul + kepala tabel + satu baris pertama harus satu halaman
	r.gap(2)
	r.font("sans", "B", 10.4)
	r.ink(r.th.Ink)
	r.pdf.SetX(marginX)
	r.pdf.MultiCell(textW, 5.4, t, "", "L", false)
	r.gap(1.4)
}

// ---------- teks kaya sebaris (**tebal**, *miring*, `kode`) ----------

var inlineRe = regexp.MustCompile("\\*\\*([^*]+)\\*\\*|`([^`]+)`|(?:^|[^*\\w])\\*([^*\\n]+)\\*")

type run struct {
	text  string
	style string // "", B, I, code
}

func parseInline(s string) []run {
	var out []run
	last := 0
	for _, m := range inlineRe.FindAllStringSubmatchIndex(s, -1) {
		start := m[0]
		var rn run
		switch {
		case m[2] >= 0:
			rn = run{s[m[2]:m[3]], "B"}
		case m[4] >= 0:
			rn = run{s[m[4]:m[5]], "code"}
		case m[6] >= 0:
			start = m[6] - 1 // tanda * pembuka; karakter sebelum tanda tetap ikut teks biasa
			rn = run{s[m[6]:m[7]], "I"}
		}
		if start > last {
			out = append(out, run{s[last:start], ""})
		}
		out = append(out, rn)
		last = m[1]
	}
	if last < len(s) {
		out = append(out, run{s[last:], ""})
	}
	return out
}

// plain membuang tanda markdown sebaris (untuk mengukur tinggi teks).
func plain(s string) string {
	var b strings.Builder
	for _, rn := range parseInline(s) {
		b.WriteString(rn.text)
	}
	return b.String()
}

// rich menulis teks mengalir dengan gaya sebaris, mulai dari margin kiri aktif.
func (r *renderer) rich(s string, size, lh float64, color brandcolor.RGB, base string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	p := r.pdf
	l, _, _, _ := p.GetMargins()
	if p.GetX() < l {
		p.SetX(l)
	}
	for _, rn := range parseInline(s) {
		switch rn.style {
		case "B":
			r.font("sans", mergeStyle(base, "B"), size)
			r.ink(r.th.Ink)
		case "I":
			r.font("sans", mergeStyle(base, "I"), size)
			r.ink(color)
		case "code":
			r.font("mono", "", size*0.9)
			r.ink(r.th.BrandText)
		default:
			r.font("sans", base, size)
			r.ink(color)
		}
		p.Write(lh, rn.text)
	}
	p.Ln(lh)
}

func mergeStyle(a, b string) string {
	if strings.Contains(a, b) {
		return a
	}
	if a == "I" && b == "B" {
		return "BI"
	}
	return a + b
}

// ---------- daftar ----------

func (r *renderer) list(items []string, ordered bool) {
	p := r.pdf
	l, t, rm, _ := p.GetMargins()
	for i, it := range items {
		if strings.TrimSpace(it) == "" {
			continue
		}
		r.ensure(bodyLH + 1)
		y := p.GetY()
		mark := "•"
		if ordered {
			mark = itoa(i+1) + "."
		}
		r.font("sans", "B", bodySize)
		r.ink(r.th.BrandText)
		p.SetXY(l+1, y)
		p.CellFormat(6, bodyLH, mark, "", 0, "L", false, 0, "")
		p.SetLeftMargin(l + 7)
		p.SetXY(l+7, y)
		r.rich(it, bodySize, bodyLH, r.th.Body, "")
		p.SetLeftMargin(l)
		p.SetY(p.GetY() + 0.9)
	}
	p.SetMargins(l, t, rm)
	r.gap(1.8)
}

func itoa(n int) string {
	const digits = "0123456789"
	if n < 10 {
		return digits[n : n+1]
	}
	return itoa(n/10) + digits[n%10:n%10+1]
}

// ---------- tabel ----------

const (
	cellSize = 8.3
	cellLH   = 4.15
	subSize  = 7.2
	subLH    = 3.55
	padX     = 2.2
	padY     = 1.7
)

func (r *renderer) colWidths(b Block, cols int) []float64 {
	w := make([]float64, cols)
	if len(b.Widths) == cols {
		sum := 0.0
		for _, x := range b.Widths {
			sum += math.Max(0, x)
		}
		if sum > 0 {
			for i, x := range b.Widths {
				w[i] = math.Max(0, x) / sum * textW
			}
			return w
		}
	}
	// Lebar otomatis menurut panjang isi (kepala & rata-rata sel), dengan batas bawah agar kolom pendek tetap terbaca.
	need := make([]float64, cols)
	for c := 0; c < cols; c++ {
		if c < len(b.Head) {
			need[c] = float64(len([]rune(b.Head[c]))) + 2
		}
		total, n := 0.0, 0
		for _, row := range b.Rows {
			if c < len(row) {
				ln := len([]rune(row[c].T))
				if s := len([]rune(row[c].Sub)); s > ln {
					ln = s
				}
				total += math.Min(float64(ln), 70)
				n++
			}
		}
		if n > 0 {
			need[c] = math.Max(need[c], total/float64(n)*1.15+2)
		}
		need[c] = math.Max(need[c], 6)
	}
	sum := 0.0
	for _, x := range need {
		sum += x
	}
	for i := range w {
		w[i] = need[i] / sum * textW
	}
	// Kolom minimal 15 mm; kekurangan diambil dari kolom terlebar.
	for i := range w {
		if w[i] < 15 {
			d := 15 - w[i]
			w[i] = 15
			widest := 0
			for j := range w {
				if w[j] > w[widest] {
					widest = j
				}
			}
			w[widest] -= d
		}
	}
	return w
}

type cellLayout struct {
	lines, sub []string
}

func (r *renderer) layoutCell(c Cell, w float64, head bool) cellLayout {
	var l cellLayout
	inner := w - 2*padX
	switch {
	case head:
		r.font("sans", "B", 7.8)
	case c.Mono:
		r.font("mono", map[bool]string{true: "B", false: ""}[c.Bold], cellSize*0.94)
	default:
		r.font("sans", map[bool]string{true: "B", false: ""}[c.Bold], cellSize)
	}
	if t := strings.TrimSpace(c.T); t != "" {
		l.lines = r.splitHard(t, inner)
	}
	if s := strings.TrimSpace(c.Sub); s != "" {
		r.font("sans", "", subSize)
		l.sub = r.splitHard(s, inner)
	}
	return l
}

// splitHard memecah teks sesuai lebar; kata yang lebih panjang dari kolom (path, URL) dipotong per karakter.
func (r *renderer) splitHard(s string, w float64) []string {
	var out []string
	for _, ln := range r.pdf.SplitText(s, w) {
		for r.pdf.GetStringWidth(ln) > w+0.01 && len([]rune(ln)) > 1 {
			rs := []rune(ln)
			k := len(rs)
			for k > 1 && r.pdf.GetStringWidth(string(rs[:k])) > w {
				k--
			}
			out = append(out, string(rs[:k]))
			ln = string(rs[k:])
		}
		out = append(out, ln)
	}
	return out
}

func (r *renderer) table(b Block) {
	cols := len(b.Head)
	for _, row := range b.Rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 || len(b.Rows) == 0 {
		return
	}
	p := r.pdf
	widths := r.colWidths(b, cols)
	maxH := r.limitY() - contentY - 12

	drawHead := func() {
		if len(b.Head) == 0 {
			return
		}
		lay := make([]cellLayout, cols)
		h := 0.0
		for c := 0; c < cols; c++ {
			t := ""
			if c < len(b.Head) {
				t = b.Head[c]
			}
			lay[c] = r.layoutCell(Cell{T: t}, widths[c], true)
			h = math.Max(h, float64(len(lay[c].lines))*3.9)
		}
		h += 2 * padY
		y := p.GetY()
		r.fill(r.th.Tint)
		p.Rect(marginX, y, textW, h, "F")
		r.fill(r.th.Primary)
		p.Rect(marginX, y+h-0.45, textW, 0.45, "F")
		x := marginX
		for c := 0; c < cols; c++ {
			r.font("sans", "B", 7.8)
			r.ink(r.th.Ink)
			align := "L"
			if c < len(b.Rows[0]) && b.Rows[0][c].Align == "right" {
				align = "R"
			}
			for li, ln := range lay[c].lines {
				p.SetXY(x+padX, y+padY+float64(li)*3.9)
				p.CellFormat(widths[c]-2*padX, 3.9, ln, "", 0, align, false, 0, "")
			}
			x += widths[c]
		}
		p.SetY(y + h)
	}

	r.ensure(22)
	drawHead()
	for ri, row := range b.Rows {
		lay := make([]cellLayout, cols)
		h := 0.0
		for c := 0; c < cols; c++ {
			cell := Cell{}
			if c < len(row) {
				cell = row[c]
			}
			lay[c] = r.layoutCell(cell, widths[c], false)
			ch := float64(len(lay[c].lines))*cellLH + float64(len(lay[c].sub))*subLH
			if len(lay[c].lines) > 0 && len(lay[c].sub) > 0 {
				ch += 0.6
			}
			h = math.Max(h, ch)
		}
		h = math.Max(h, cellLH) + 2*padY
		if h > maxH { // sel raksasa dipotong agar baris muat satu halaman
			h = maxH
		}
		if p.GetY()+h > r.limitY() {
			p.AddPage()
			drawHead()
		}
		y := p.GetY()
		if ri%2 == 1 {
			r.fill(r.th.Zebra)
			p.Rect(marginX, y, textW, h, "F")
		}
		x := marginX
		for c := 0; c < cols; c++ {
			cell := Cell{}
			if c < len(row) {
				cell = row[c]
			}
			align := "L"
			if cell.Align == "right" {
				align = "R"
			}
			cy := y + padY
			style := map[bool]string{true: "B", false: ""}[cell.Bold]
			for _, ln := range lay[c].lines {
				if cy+cellLH > y+h-padY+0.01 {
					break
				}
				if cell.Mono {
					r.font("mono", style, cellSize*0.94)
				} else {
					r.font("sans", style, cellSize)
				}
				col := r.th.Body
				if cell.Bold {
					col = r.th.Ink
				}
				if tc, ok := r.th.toneColor(cell.Tone); ok {
					col = tc
					if !cell.Mono {
						r.font("sans", "B", cellSize)
					}
				}
				r.ink(col)
				p.SetXY(x+padX, cy)
				p.CellFormat(widths[c]-2*padX, cellLH, ln, "", 0, align, false, 0, "")
				cy += cellLH
			}
			if len(lay[c].sub) > 0 {
				if len(lay[c].lines) > 0 {
					cy += 0.6
				}
				r.font("sans", "", subSize)
				r.ink(r.th.Muted)
				for _, ln := range lay[c].sub {
					if cy+subLH > y+h-padY+0.01 {
						break
					}
					p.SetXY(x+padX, cy)
					p.CellFormat(widths[c]-2*padX, subLH, ln, "", 0, align, false, 0, "")
					cy += subLH
				}
			}
			x += widths[c]
		}
		r.draw(r.th.Line)
		p.SetLineWidth(0.2)
		p.Line(marginX, y+h, marginX+textW, y+h)
		p.SetY(y + h)
	}
	r.gap(4)
}

// kv: tabel dua kolom tanpa kepala (label redup, nilai tegas).
func (r *renderer) kv(pairs []Pair) {
	if len(pairs) == 0 {
		return
	}
	rows := make([][]Cell, 0, len(pairs))
	for _, pr := range pairs {
		rows = append(rows, []Cell{{T: pr.K, Tone: "muted"}, {T: firstNonEmpty(pr.V, "—"), Mono: fontFor(pr.V) == "mono", Tone: pr.Tone}})
	}
	r.table(Block{Rows: rows, Widths: []float64{0.28, 0.72}})
}

// ---------- kode ----------

func (r *renderer) code(b Block) {
	p := r.pdf
	text := strings.TrimRight(b.Text, "\n ")
	if text == "" {
		return
	}
	const size, lh = 7.5, 3.75
	r.font("mono", "", size)
	inner := textW - 8
	var lines []string
	for _, ln := range strings.Split(text, "\n") {
		if rs := []rune(ln); len(rs) > maxCodeLine {
			ln = string(rs[:maxCodeLine]) + "…"
		}
		if ln == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, r.splitHard(ln, inner)...)
	}
	if len(lines) > 400 {
		lines = append(lines[:399], "… (dipotong)")
	}
	titleBar := b.Title != "" || b.Lang != ""
	need := 8.0 + math.Min(float64(len(lines)), 6)*lh
	if titleBar {
		need += 6.5
	}
	r.ensure(need)
	bar := func(cont bool) {
		y := p.GetY()
		r.fill(r.th.Tint)
		p.Rect(marginX, y, textW, 6.5, "F")
		r.fill(r.th.Primary)
		p.Rect(marginX, y, 1.0, 6.5, "F")
		r.font("mono", "B", 7.4)
		r.ink(r.th.Ink)
		t := b.Title
		if cont {
			t += " (lanjutan)"
		}
		p.SetXY(marginX+4, y)
		p.CellFormat(textW-30, 6.5, r.fit(t, textW-32), "", 0, "LM", false, 0, "")
		if b.Lang != "" {
			r.font("sans", "B", 6.6)
			r.ink(r.th.Muted)
			p.SetXY(pageW-marginX-24, y)
			p.CellFormat(21, 6.5, strings.ToUpper(b.Lang), "", 0, "RM", false, 0, "")
		}
		p.SetY(y + 6.5)
	}
	if titleBar {
		bar(false)
	}
	// Latar digambar per baris agar blok bisa berlanjut ke halaman berikutnya tanpa kotak yang terpotong.
	pad := func(h float64) {
		r.fill(r.th.CodeBg)
		p.Rect(marginX, p.GetY(), textW, h, "F")
		r.fill(r.th.Primary)
		p.Rect(marginX, p.GetY(), 1.0, h, "F")
		p.SetY(p.GetY() + h)
	}
	pad(2.4)
	for _, ln := range lines {
		if p.GetY()+lh+2.4 > r.limitY() {
			pad(1.2)
			p.AddPage()
			if titleBar {
				bar(true)
			}
			pad(1.6)
		}
		y := p.GetY()
		r.fill(r.th.CodeBg)
		p.Rect(marginX, y, textW, lh, "F")
		r.fill(r.th.Primary)
		p.Rect(marginX, y, 1.0, lh, "F")
		r.font("mono", "", size)
		col := r.th.Ink
		if t := strings.TrimSpace(ln); strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
			col = r.th.Muted
		}
		r.ink(col)
		p.SetXY(marginX+4, y)
		p.CellFormat(inner, lh, ln, "", 0, "L", false, 0, "")
		p.SetY(y + lh)
	}
	pad(2.4)
	r.gap(4)
}

// ---------- angka ringkas ----------

func (r *renderer) stats(pairs []Pair) {
	if len(pairs) == 0 {
		return
	}
	p := r.pdf
	const per, gapX, h = 4, 3.0, 17.5
	for start := 0; start < len(pairs); start += per {
		end := start + per
		if end > len(pairs) {
			end = len(pairs)
		}
		n := end - start
		w := (textW - gapX*float64(per-1)) / per
		r.ensure(h + 2)
		y := p.GetY()
		for i := 0; i < n; i++ {
			pr := pairs[start+i]
			x := marginX + float64(i)*(w+gapX)
			r.fill(r.th.Tint)
			p.RoundedRect(x, y, w, h, 2.2, "1234", "F")
			r.font("sans", "B", 15)
			col := r.th.Ink
			if tc, ok := r.th.toneColor(pr.Tone); ok {
				col = tc
			}
			r.ink(col)
			p.SetXY(x+4, y+3)
			p.CellFormat(w-8, 7.5, r.fit(pr.V, w-8), "", 0, "L", false, 0, "")
			r.font("sans", "", 7.4)
			r.ink(r.th.Muted)
			p.SetXY(x+4, y+11)
			p.CellFormat(w-8, 4, r.fit(pr.K, w-8), "", 0, "L", false, 0, "")
		}
		p.SetY(y + h + gapX)
	}
	r.gap(1.5)
}

// ---------- kotak catatan ----------

func (r *renderer) note(b Block) {
	p := r.pdf
	text := strings.TrimSpace(b.Text)
	if text == "" && b.Title == "" {
		return
	}
	const size, lh = 9, 4.7
	inner := textW - 12
	r.font("sans", "", size)
	lines := p.SplitText(plain(text), inner)
	h := float64(len(lines))*lh + 7
	if b.Title != "" {
		h += 5.5
	}
	if h > 90 {
		// Catatan panjang ditulis sebagai paragraf biasa agar bisa berlanjut antarhalaman.
		r.heading(b.Title)
		r.rich(text, bodySize, bodyLH, r.th.Body, "")
		r.gap(2.6)
		return
	}
	r.ensure(h + 2)
	y := p.GetY()
	bar := r.th.Primary
	if tc, ok := r.th.toneColor(b.Tone); ok && b.Tone != "info" {
		bar = tc
	}
	r.fill(r.th.Tint)
	p.Rect(marginX, y, textW, h, "F")
	r.fill(bar)
	p.Rect(marginX, y, 1.2, h, "F")
	p.SetXY(marginX+6, y+3.5)
	if b.Title != "" {
		r.font("sans", "B", 8)
		r.ink(r.th.BrandText)
		p.CellFormat(inner, 4.5, strings.ToUpper(b.Title), "", 1, "L", false, 0, "")
		p.SetXY(marginX+6, p.GetY()+1)
	}
	l, t, rm, _ := p.GetMargins()
	p.SetLeftMargin(marginX + 6)
	p.SetRightMargin(marginX + 6)
	r.rich(text, size, lh, r.th.Body, "")
	p.SetMargins(l, t, rm)
	p.SetY(y + h)
	r.gap(4)
}
