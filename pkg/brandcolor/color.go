// Package brandcolor membaca warna merek aplikasi dari kodenya (CSS, konfigurasi Tailwind/Nuxt UI, tema Flutter,
// resource Android, manifest web) dan menyediakan alat hitung warna untuk tema dokumen.
package brandcolor

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// RGB warna 8 bit per kanal.
type RGB struct{ R, G, B uint8 }

// Hex menulis warna sebagai #rrggbb.
func (c RGB) Hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

var hexRe = regexp.MustCompile(`^#?([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// ParseHex membaca #rgb, #rgba, #rrggbb, atau #rrggbbaa (kanal alfa diabaikan).
func ParseHex(s string) (RGB, bool) {
	m := hexRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return RGB{}, false
	}
	h := m[1]
	if len(h) <= 4 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	v, err := strconv.ParseUint(h[:6], 16, 32)
	if err != nil {
		return RGB{}, false
	}
	return RGB{uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
}

// NormalizeHex mengembalikan #rrggbb huruf kecil, atau "" bila bukan warna hex yang sah.
func NormalizeHex(s string) string {
	if c, ok := ParseHex(s); ok {
		return c.Hex()
	}
	return ""
}

var (
	rgbFnRe   = regexp.MustCompile(`(?i)^rgba?\(\s*([\d.]+%?)[\s,]+([\d.]+%?)[\s,]+([\d.]+%?)`)
	hslFnRe   = regexp.MustCompile(`(?i)^hsla?\(\s*([\d.]+)(?:deg)?[\s,]+([\d.]+)%[\s,]+([\d.]+)%`)
	oklchFnRe = regexp.MustCompile(`(?i)^oklch\(\s*([\d.]+%?)\s+([\d.]+%?)\s+([\d.]+)`)
	// Komponen HSL tanpa fungsi, gaya shadcn: "--primary: 222.2 47.4% 11.2%".
	bareHSLRe = regexp.MustCompile(`^([\d.]+)\s+([\d.]+)%\s+([\d.]+)%$`)
)

// ParseCSS membaca nilai warna CSS: hex, rgb(), hsl(), oklch(), atau komponen HSL telanjang.
func ParseCSS(v string) (RGB, bool) {
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important"))
	if c, ok := ParseHex(v); ok && strings.HasPrefix(v, "#") {
		return c, true
	}
	if m := rgbFnRe.FindStringSubmatch(v); m != nil {
		ch := func(s string) uint8 {
			if strings.HasSuffix(s, "%") {
				f, _ := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
				return clamp8(f * 2.55)
			}
			f, _ := strconv.ParseFloat(s, 64)
			return clamp8(f)
		}
		return RGB{ch(m[1]), ch(m[2]), ch(m[3])}, true
	}
	if m := hslFnRe.FindStringSubmatch(v); m != nil {
		return hslFrom(m[1], m[2], m[3]), true
	}
	if m := bareHSLRe.FindStringSubmatch(v); m != nil {
		return hslFrom(m[1], m[2], m[3]), true
	}
	if m := oklchFnRe.FindStringSubmatch(v); m != nil {
		l := pct(m[1], 1)
		c := pct(m[2], 0.4)
		h, _ := strconv.ParseFloat(m[3], 64)
		return fromOKLCH(l, c, h), true
	}
	return RGB{}, false
}

func pct(s string, full float64) float64 {
	if strings.HasSuffix(s, "%") {
		f, _ := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		return f / 100 * full
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func hslFrom(hs, ss, ls string) RGB {
	h, _ := strconv.ParseFloat(hs, 64)
	s, _ := strconv.ParseFloat(ss, 64)
	l, _ := strconv.ParseFloat(ls, 64)
	return FromHSL(h, s/100, l/100)
}

func clamp8(f float64) uint8 { return uint8(math.Max(0, math.Min(255, math.Round(f)))) }

// FromHSL: h 0–360, s & l 0–1.
func FromHSL(h, s, l float64) RGB {
	h = math.Mod(math.Mod(h, 360)+360, 360) / 360
	if s == 0 {
		return RGB{clamp8(l * 255), clamp8(l * 255), clamp8(l * 255)}
	}
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	hue := func(t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	return RGB{clamp8(hue(h+1.0/3) * 255), clamp8(hue(h) * 255), clamp8(hue(h-1.0/3) * 255)}
}

// HSL mengembalikan h 0–360, s & l 0–1.
func (c RGB) HSL() (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h * 60, s, l
}

// fromOKLCH mengubah OKLCH (CSS Color 4) ke sRGB, dengan kanal di luar gamut dipotong.
func fromOKLCH(l, c, hDeg float64) RGB {
	hr := hDeg * math.Pi / 180
	a, b := c*math.Cos(hr), c*math.Sin(hr)
	l_ := l + 0.3963377774*a + 0.2158037573*b
	m_ := l - 0.1055613458*a - 0.0638541728*b
	s_ := l - 0.0894841775*a - 1.2914855480*b
	L, M, S := l_*l_*l_, m_*m_*m_, s_*s_*s_
	r := 4.0767416621*L - 3.3077115913*M + 0.2309699292*S
	g := -1.2684380046*L + 2.6097574011*M - 0.3413193965*S
	bl := -0.0041960863*L - 0.7034186147*M + 1.7076147010*S
	enc := func(x float64) uint8 {
		x = math.Max(0, math.Min(1, x))
		if x <= 0.0031308 {
			return clamp8(12.92 * x * 255)
		}
		return clamp8((1.055*math.Pow(x, 1/2.4) - 0.055) * 255)
	}
	return RGB{enc(r), enc(g), enc(bl)}
}

// Luminance relatif WCAG (0 hitam – 1 putih).
func (c RGB) Luminance() float64 {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// Contrast rasio kontras WCAG antara dua warna (1–21).
func Contrast(a, b RGB) float64 {
	la, lb := a.Luminance(), b.Luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Mix mencampur c dengan o; t = porsi o (0 = c utuh, 1 = o utuh).
func (c RGB) Mix(o RGB, t float64) RGB {
	f := func(a, b uint8) uint8 { return clamp8(float64(a)*(1-t) + float64(b)*t) }
	return RGB{f(c.R, o.R), f(c.G, o.G), f(c.B, o.B)}
}

var (
	White = RGB{255, 255, 255}
	Black = RGB{0, 0, 0}
)

// Chromatic: warna yang cukup jenuh dan tidak terlalu gelap/terang untuk menjadi warna merek.
func (c RGB) Chromatic() bool {
	_, s, l := c.HSL()
	return s >= 0.28 && l >= 0.18 && l <= 0.82
}

// Darken menggelapkan warna sampai kontrasnya terhadap bg minimal min (batas 12 langkah).
func Darken(c, bg RGB, min float64) RGB {
	for i := 0; i < 12 && Contrast(c, bg) < min; i++ {
		c = c.Mix(Black, 0.12)
	}
	return c
}

// OnColor memilih teks putih atau tinta gelap yang paling terbaca di atas warna latar bg.
func OnColor(bg RGB) RGB {
	ink := RGB{17, 24, 39}
	if Contrast(White, bg) >= Contrast(ink, bg) {
		return White
	}
	return ink
}
