package diagnostics

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibeasik/vibeasik/pkg/models"
)

const completeNuxtPage = `<script setup>
const { data: p } = await useFetch('/api/produk/' + useRoute().params.slug)
useSeoMeta({
  ogTitle: p.value.nama,
  ogDescription: p.value.ringkasan,
  ogImage: p.value.fotoBagikan,
  twitterCard: 'summary_large_image',
})
</script>
<template><h1>{{ p.nama }}</h1></template>
`

func shareComp(dir string) *models.Component {
	return &models.Component{Role: models.RoleFrontendClient, AbsolutePath: dir, RelativePath: "web", Framework: "nuxt"}
}

// writeBrandIcons menulis set ikon bermerek lengkap (isi tidak sama dengan ikon starter mana pun).
func writeBrandIcons(t *testing.T, dir string) {
	t.Helper()
	writeFixture(t, dir, "public/favicon.ico", "ICO-bermerek")
	for _, f := range []string{"favicon.svg", "favicon-32x32.png", "apple-touch-icon.png", "icon-192.png", "icon-512.png"} {
		writeFixture(t, dir, "public/"+f, "x")
	}
}

func findingFor(fs []models.Finding, route string) (models.Finding, bool) {
	for _, f := range fs {
		if strings.HasSuffix(f.Title, ": "+route) {
			return f, true
		}
	}
	return models.Finding{}, false
}

func TestSharePagesPerPageFindings(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "package.json", `{"dependencies":{"nuxt":"^4.0.0"}}`)
	writeFixture(t, dir, "app/app.vue", "<script setup>useSeoMeta({ twitterCard: 'summary_large_image' })</script>\n")
	writeFixture(t, dir, "app/pages/produk/[slug].vue", "<template><h1>Produk</h1></template>\n")
	writeFixture(t, dir, "app/pages/layanan/[slug].vue", "<script setup>\nuseSeoMeta({\n  ogTitle: 'Toko Kami',\n  ogDescription: 'Toko terbaik',\n  ogImage: 'https://example.com/og.jpg',\n})\n</script>\n")
	writeFixture(t, dir, "app/pages/kategori/[slug].vue", completeNuxtPage)
	writeFixture(t, dir, "app/pages/admin/index.vue", "<template>panel</template>\n")
	writeFixture(t, dir, "app/pages/login.vue", "<template>masuk</template>\n")

	findings, n := inspectSharePages(shareComp(dir))
	if n != 3 {
		t.Fatalf("pages scanned = %d, want 3 (admin/login skipped)", n)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %d, want 2: %v", len(findings), findings)
	}
	f, ok := findingFor(findings, "/produk/[slug]")
	if !ok || f.RuleID != RuleShareMissing || f.Line != 1 || !strings.HasSuffix(f.TargetFile, "app/pages/produk/[slug].vue") {
		t.Errorf("product page without meta: %+v", f)
	}
	if f, ok := findingFor(findings, "/layanan/[slug]"); !ok || f.RuleID != RuleShareGeneric || f.Line != 2 {
		t.Errorf("service page with constant meta: %+v", f)
	}
	if _, ok := findingFor(findings, "/kategori/[slug]"); ok {
		t.Error("complete page must have no finding")
	}
}

func TestSharePagesImageAndTwitter(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "package.json", `{"dependencies":{"nuxt":"^4.0.0"}}`)
	meta := func(img string) string {
		return "<script setup>\nuseSeoMeta({ ogTitle: 'Judul', ogDescription: 'Ringkasan', ogImage: " + img + ", twitterCard: 'summary_large_image' })\n</script>\n"
	}
	writeFixture(t, dir, "app/pages/tentang.vue", meta("'/og.jpg'"))
	writeFixture(t, dir, "app/pages/kontak.vue", meta("'http://example.com/og.jpg'"))
	writeFixture(t, dir, "app/pages/galeri.vue", meta("'/besar.jpg'"))
	writeFixture(t, dir, "app/pages/acara.vue", "<script setup>\nuseSeoMeta({ ogTitle: 'A', ogDescription: 'B', ogImage: 'https://example.com/a.jpg' })\n</script>\n")
	writeFixture(t, dir, "public/besar.jpg", string(jpegBytes(t, 800, 400)))

	got, _ := inspectSharePages(shareComp(dir))
	want := map[string]string{"/tentang": RuleShareImage, "/kontak": RuleShareImage, "/galeri": RuleShareImage, "/acara": RuleShareNoTwitter}
	if len(got) != len(want) {
		t.Fatalf("got %d findings: %v", len(got), got)
	}
	for route, rule := range want {
		if f, ok := findingFor(got, route); !ok || f.RuleID != rule {
			t.Errorf("%s: %+v want %s", route, f, rule)
		}
	}
}

func TestSharePagesNextAndStatic(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "package.json", `{"dependencies":{"next":"15"}}`)
	writeFixture(t, dir, "app/(web)/produk/[id]/page.tsx", "export default function P() { return null }\n")
	writeFixture(t, dir, "app/(web)/blog/page.tsx", "export const metadata = { openGraph: { title: 'a', description: 'b', images: ['https://x.id/a.jpg'] }, twitter: { card: 'summary_large_image' } }\n")
	writeFixture(t, dir, "app/dashboard/page.tsx", "export default function D() { return null }\n")
	got, n := inspectSharePages(shareComp(dir))
	if n != 2 || len(got) != 1 {
		t.Fatalf("n=%d findings=%v", n, got)
	}
	if f, ok := findingFor(got, "/produk/[id]"); !ok || f.RuleID != RuleShareMissing {
		t.Errorf("next product page: %+v", f)
	}
}

func TestSiteIconsStarterAndComplete(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "package.json", `{}`)
	writeFixture(t, dir, "public/favicon.ico", string(starterICO()))
	got := ruleSet(inspectSiteIcons(shareComp(dir)))
	if _, ok := got[RuleFaviconStarter]; !ok {
		t.Fatalf("starter favicon not flagged: %v", got)
	}
	if _, ok := got[RuleIconIncomplete]; !ok {
		t.Errorf("incomplete icon set not flagged: %v", got)
	}

	full := t.TempDir()
	writeFixture(t, full, "package.json", `{}`)
	writeBrandIcons(t, full)
	if got := inspectSiteIcons(shareComp(full)); len(got) != 0 {
		t.Fatalf("branded complete set must pass: %v", ruleSet(got))
	}
}

func TestSiteIconsLinkMismatch(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "package.json", `{}`)
	writeBrandIcons(t, dir)
	writeFixture(t, dir, "nuxt.config.ts", "export default { app: { head: { link: [{ rel: 'icon', href: '/hilang.svg' }] } } }\n")
	f, ok := ruleSet(inspectSiteIcons(shareComp(dir)))[RuleIconIncomplete]
	if !ok || !strings.Contains(f.Description, "/hilang.svg") {
		t.Fatalf("mismatched link not flagged: %+v", f)
	}
}

func TestStarterFaviconFingerprint(t *testing.T) {
	if _, ok := StarterFaviconName(starterICO()); !ok {
		t.Error("starter fingerprint must match")
	}
	if _, ok := StarterFaviconName(bytes.Repeat([]byte{1}, 4286)); ok {
		t.Error("same size but different hash must not match")
	}
	if _, ok := StarterFaviconName([]byte("ICO-bermerek")); ok {
		t.Error("branded icon must not match")
	}
}

// starterICO membaca salinan ikon bawaan starter dari testdata.
func starterICO() []byte {
	data, err := os.ReadFile(filepath.Join("testdata", "starter_favicon.ico"))
	if err != nil {
		panic(err)
	}
	return data
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 3), uint8(x ^ y), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pngBytes(t *testing.T, w, h int, noisy bool) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(1)
	for i := range img.Pix {
		if noisy {
			seed = seed*1664525 + 1013904223
			img.Pix[i] = byte(seed >> 24)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
