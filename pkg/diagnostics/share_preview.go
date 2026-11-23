package diagnostics

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "golang.org/x/image/webp"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Pemeriksa pratinjau bagikan per halaman (patokan SEO-003, SEO-004, KONTEN-014): mengunduh HTML mentah
// seperti perayap media sosial (tanpa JavaScript) dan memeriksa kartu pratinjau tiap halaman.

// Kode masalah (ShareIssue.Code).
const (
	ShareIssueNoTitle       = "no-og-title"
	ShareIssueNoDescription = "no-og-description"
	ShareIssueDescLength    = "og-description-length"
	ShareIssueNoImage       = "no-og-image"
	ShareIssueImageURL      = "og-image-url"
	ShareIssueImageFetch    = "og-image-unreachable"
	ShareIssueImageSize     = "og-image-too-large"
	ShareIssueImageDims     = "og-image-dimensions"
	ShareIssueNoImageMeta   = "no-og-image-meta"
	ShareIssueNoURL         = "no-og-url"
	ShareIssueURLMismatch   = "og-url-not-canonical"
	ShareIssueNoCanonical   = "no-canonical"
	ShareIssueNoType        = "no-og-type"
	ShareIssueNoSiteName    = "no-og-site-name"
	ShareIssueNoTwitter     = "no-twitter-card"
	ShareIssueDuplicate     = "duplicate-meta"
	ShareIssueFetch         = "page-unreachable"
)

// ID aturan temuan audit pratinjau bagikan (dipakai audit statis dan runtime).
const (
	RuleShareMissing   = "SEO-SHARE-MISSING"
	RuleShareGeneric   = "SEO-SHARE-GENERIC"
	RuleShareImage     = "SEO-SHARE-IMAGE"
	RuleShareNoTwitter = "SEO-SHARE-NO-TWITTER"
	RuleShareDuplicate = "SEO-SHARE-DUPLICATE"
)

const (
	shareDescMin      = 110
	shareDescMax      = 160
	shareImageW       = 1200
	shareImageH       = 630
	shareImageMaxByte = 300 * 1024
	shareHTMLMaxByte  = 2 << 20
	shareImageCap     = 4 << 20
	shareMaxPages     = 200
	shareMaxRedirects = 5
)

// UA perayap yang dipakai untuk mengunduh tiap halaman.
var shareUserAgents = []string{
	"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)",
	"WhatsApp/2.23.20.0 A",
}

// ShareIssue satu masalah pada kartu pratinjau sebuah halaman.
type ShareIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SharePageResult hasil pemeriksaan satu halaman. OK true bila tanpa masalah.
type SharePageResult struct {
	Path        string       `json:"path"`
	URL         string       `json:"url"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Image       string       `json:"image,omitempty"`
	Issues      []ShareIssue `json:"issues,omitempty"`
	OK          bool         `json:"ok"`
}

// ShareOptions mengatur CheckSharePreviews; nilai nol memakai bawaan.
type ShareOptions struct {
	Client      *http.Client // bawaan: timeout 10 dtk, maksimal 5 pengalihan, hanya ke host yang sama dengan baseURL
	MaxPages    int          // bawaan 200
	SkipImage   bool         // jangan unduh og:image (hanya periksa tag)
	AllowHTTP   bool         // izinkan baseURL http (mis. server lokal); gambar tetap wajib https di hasil
	UserAgents  []string     // bawaan: facebookexternalhit/1.1 dan WhatsApp/2.x
	DynamicHint []string     // contoh path rute dinamis tambahan (mis. /produk/kopi-arabika)
}

var (
	shareTagRe   = regexp.MustCompile(`(?is)<(meta|link)\b((?:[^>"']|"[^"]*"|'[^']*')*)>`)
	shareCmtRe   = regexp.MustCompile(`(?s)<!--.*?-->`)
	shareAttrRe  = regexp.MustCompile(`(?is)([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*(?:=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+)))?`)
	shareLocRe   = regexp.MustCompile(`(?is)<loc>\s*(.*?)\s*</loc>`)
	shareHeadEnd = regexp.MustCompile(`(?i)</head\s*>`)
)

// shareMeta menampung tag meta yang relevan dari satu dokumen HTML.
type shareMeta struct {
	og        map[string]string
	twitter   map[string]string
	canonical string
}

func parseShareAttrs(raw string) map[string]string {
	attrs := map[string]string{}
	for _, m := range shareAttrRe.FindAllStringSubmatch(raw, -1) {
		name := strings.ToLower(m[1])
		val := m[2] + m[3] + m[4]
		if _, dup := attrs[name]; !dup {
			attrs[name] = strings.TrimSpace(html.UnescapeString(val))
		}
	}
	return attrs
}

// parseShareMeta membaca <meta>/<link> di <head> tanpa bergantung pada urutan atribut atau tanda kutip.
func parseShareMeta(doc []byte) shareMeta {
	if loc := shareHeadEnd.FindIndex(doc); loc != nil {
		doc = doc[:loc[0]]
	}
	doc = shareCmtRe.ReplaceAll(doc, nil)
	m := shareMeta{og: map[string]string{}, twitter: map[string]string{}}
	for _, tag := range shareTagRe.FindAllSubmatch(doc, -1) {
		attrs := parseShareAttrs(string(tag[2]))
		if strings.EqualFold(string(tag[1]), "link") {
			if strings.EqualFold(attrs["rel"], "canonical") && m.canonical == "" {
				m.canonical = attrs["href"]
			}
			continue
		}
		key := strings.ToLower(attrs["property"])
		if key == "" {
			key = strings.ToLower(attrs["name"])
		}
		content, ok := attrs["content"]
		if !ok || key == "" {
			continue
		}
		switch {
		case strings.HasPrefix(key, "og:"):
			if _, dup := m.og[key]; !dup {
				m.og[key] = content
			}
		case strings.HasPrefix(key, "twitter:"):
			if _, dup := m.twitter[key]; !dup {
				m.twitter[key] = content
			}
		}
	}
	return m
}

func normalizeShareURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + u.EscapedPath() +
		map[bool]string{true: "?" + u.RawQuery, false: ""}[u.RawQuery != ""]
}

// CheckShareHTML memeriksa tag pratinjau bagikan pada HTML mentah satu halaman (fungsi murni, tanpa jaringan).
// pageURL adalah URL halaman yang diperiksa; gambar tidak diunduh (lihat CheckSharePreviews).
func CheckShareHTML(pageURL string, doc []byte) SharePageResult {
	res := SharePageResult{URL: pageURL, Path: sharePath(pageURL)}
	m := parseShareMeta(doc)
	add := func(code, msg string) { res.Issues = append(res.Issues, ShareIssue{code, msg}) }

	res.Title, res.Description, res.Image = m.og["og:title"], m.og["og:description"], m.og["og:image"]
	if res.Title == "" {
		add(ShareIssueNoTitle, "og:title hilang")
	}
	checkShareDescription(&res, add)
	checkShareImageTags(&res, m, pageURL, add)
	if m.og["og:url"] == "" {
		add(ShareIssueNoURL, "og:url hilang")
	} else if m.canonical == "" {
		add(ShareIssueNoCanonical, "link rel=canonical hilang, og:url tidak bisa dibandingkan")
	} else if normalizeShareURL(m.og["og:url"]) != normalizeShareURL(m.canonical) {
		add(ShareIssueURLMismatch, fmt.Sprintf("og:url (%s) berbeda dari canonical (%s)", m.og["og:url"], m.canonical))
	}
	if m.og["og:type"] == "" {
		add(ShareIssueNoType, "og:type hilang")
	}
	if m.og["og:site_name"] == "" {
		add(ShareIssueNoSiteName, "og:site_name hilang")
	}
	if !strings.EqualFold(m.twitter["twitter:card"], "summary_large_image") {
		add(ShareIssueNoTwitter, "twitter:card summary_large_image hilang")
	}
	res.OK = len(res.Issues) == 0
	return res
}

func checkShareDescription(res *SharePageResult, add func(code, msg string)) {
	d := strings.TrimSpace(res.Description)
	if d == "" {
		add(ShareIssueNoDescription, "og:description hilang")
		return
	}
	if n := utf8.RuneCountInString(d); n < shareDescMin || n > shareDescMax {
		add(ShareIssueDescLength, fmt.Sprintf("og:description %d karakter, seharusnya %d-%d", n, shareDescMin, shareDescMax))
	}
}

func checkShareImageTags(res *SharePageResult, m shareMeta, pageURL string, add func(code, msg string)) {
	img := m.og["og:image"]
	if img == "" {
		add(ShareIssueNoImage, "og:image hilang")
		return
	}
	if u, err := url.Parse(img); err != nil || u.Scheme != "https" || u.Host == "" {
		add(ShareIssueImageURL, fmt.Sprintf("og:image harus URL absolut https, ditemukan %q", img))
	}
	if m.og["og:image:width"] == "" || m.og["og:image:height"] == "" || m.og["og:image:alt"] == "" {
		add(ShareIssueNoImageMeta, "og:image:width, og:image:height, dan og:image:alt wajib ada")
	}
}

func sharePath(pageURL string) string {
	u, err := url.Parse(pageURL)
	if err != nil || u.Path == "" {
		return "/"
	}
	return u.Path
}

// MarkShareDuplicates menandai halaman yang memakai judul, deskripsi, atau gambar og yang sama dengan halaman
// lain; tiap halaman yang kembar ditandai (bukan hanya yang kedua).
func MarkShareDuplicates(results []SharePageResult) {
	fields := []struct {
		name string
		get  func(SharePageResult) string
	}{
		{"og:title", func(r SharePageResult) string { return r.Title }},
		{"og:description", func(r SharePageResult) string { return r.Description }},
		{"og:image", func(r SharePageResult) string { return r.Image }},
	}
	for _, f := range fields {
		groups := map[string][]int{}
		for i, r := range results {
			if v := strings.TrimSpace(f.get(r)); v != "" {
				groups[v] = append(groups[v], i)
			}
		}
		for _, idx := range groups {
			if len(idx) < 2 {
				continue
			}
			for _, i := range idx {
				others := make([]string, 0, len(idx)-1)
				for _, j := range idx {
					if j != i {
						others = append(others, results[j].Path)
					}
				}
				sort.Strings(others)
				results[i].Issues = append(results[i].Issues, ShareIssue{ShareIssueDuplicate,
					fmt.Sprintf("%s sama dengan halaman lain (%s)", f.name, strings.Join(firstN(others, 3), ", "))})
				results[i].OK = false
			}
		}
	}
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// CheckSharePreviews memeriksa kartu pratinjau tiap halaman situs di baseURL. paths kosong = daftar diambil dari
// /sitemap.xml (indeks sitemap diikuti satu tingkat) ditambah "/" dan opts.DynamicHint. Setiap halaman diunduh
// dengan tiap UA perayap tanpa JavaScript; hasil UA yang berbeda-beda dilaporkan sebagai masalah halaman itu.
// Semua permintaan dibatasi ke host baseURL (termasuk pengalihan); og:image di host lain hanya diperiksa
// bentuk URL-nya bila SkipImage, atau diunduh hanya bila https dan host publik bukan loopback.
func CheckSharePreviews(ctx context.Context, baseURL string, paths []string, opts ShareOptions) ([]SharePageResult, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Host == "" || (base.Scheme != "https" && !(opts.AllowHTTP && base.Scheme == "http")) {
		return nil, fmt.Errorf("baseURL tidak valid atau bukan https: %q", baseURL)
	}
	client := opts.Client
	if client == nil {
		client = shareClient(base.Host)
	}
	uas := opts.UserAgents
	if len(uas) == 0 {
		uas = shareUserAgents
	}
	max := opts.MaxPages
	if max <= 0 || max > shareMaxPages {
		max = shareMaxPages
	}
	if len(paths) == 0 {
		paths = discoverSharePaths(ctx, client, base, opts.DynamicHint)
	}
	paths = uniqueSharePaths(paths, max)

	results := make([]SharePageResult, 0, len(paths))
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		pageURL := base.ResolveReference(&url.URL{Path: p}).String()
		res := checkSharePage(ctx, client, pageURL, uas, opts)
		res.Path = p
		results = append(results, res)
	}
	MarkShareDuplicates(results)
	return results, nil
}

func shareClient(host string) *http.Client {
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= shareMaxRedirects {
			return fmt.Errorf("terlalu banyak pengalihan")
		}
		if !strings.EqualFold(req.URL.Host, host) {
			return fmt.Errorf("pengalihan ke host lain (%s) ditolak", req.URL.Host)
		}
		return nil
	}}
}

func checkSharePage(ctx context.Context, client *http.Client, pageURL string, uas []string, opts ShareOptions) SharePageResult {
	var res SharePageResult
	for i, ua := range uas {
		body, status, err := shareGet(ctx, client, pageURL, ua, shareHTMLMaxByte)
		if err != nil || status != http.StatusOK {
			msg := fmt.Sprintf("halaman tidak bisa diunduh dengan UA %q (status %d)", shortUA(ua), status)
			if err != nil {
				msg = fmt.Sprintf("halaman tidak bisa diunduh dengan UA %q: %v", shortUA(ua), err)
			}
			if i == 0 {
				res = SharePageResult{URL: pageURL, Path: sharePath(pageURL)}
			}
			res.Issues = append(res.Issues, ShareIssue{ShareIssueFetch, msg})
			continue
		}
		cur := CheckShareHTML(pageURL, body)
		if i == 0 || len(res.Issues) == 0 && res.Title == "" {
			res = cur
			continue
		}
		res = mergeShareUA(res, cur, ua)
	}
	if !opts.SkipImage && res.Image != "" {
		checkShareImage(ctx, client, &res, uas[0])
	}
	res.OK = len(res.Issues) == 0
	return res
}

// mergeShareUA menggabungkan hasil UA berikutnya: masalah baru ditambahkan dan perbedaan meta antar-UA dicatat.
func mergeShareUA(first, next SharePageResult, ua string) SharePageResult {
	seen := map[string]bool{}
	for _, is := range first.Issues {
		seen[is.Code+is.Message] = true
	}
	for _, is := range next.Issues {
		if !seen[is.Code+is.Message] {
			first.Issues = append(first.Issues, ShareIssue{is.Code, is.Message + " (UA " + shortUA(ua) + ")"})
		}
	}
	if next.Title != first.Title || next.Description != first.Description || next.Image != first.Image {
		first.Issues = append(first.Issues, ShareIssue{ShareIssueNoTitle,
			"meta berbeda antar user-agent perayap (UA " + shortUA(ua) + "); pastikan HTML sama untuk semua perayap"})
	}
	return first
}

func shortUA(ua string) string {
	if i := strings.IndexAny(ua, "/ "); i > 0 {
		return ua[:i]
	}
	return ua
}

// shareGet mengunduh url dengan batas ukuran body.
func shareGet(ctx context.Context, client *http.Client, rawURL, ua string, limit int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,image/*;q=0.9,*/*;q=0.5")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if int64(len(body)) > limit {
		return body[:limit], resp.StatusCode, nil
	}
	return body, resp.StatusCode, nil
}

// checkShareImage mengunduh og:image (bila https dan host aman) lalu memeriksa ukuran berkas dan dimensinya.
func checkShareImage(ctx context.Context, client *http.Client, res *SharePageResult, ua string) {
	u, err := url.Parse(res.Image)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return // sudah ditandai ShareIssueImageURL
	}
	imgClient := client
	if !shareSameHost(res.URL, u.Host) {
		if isPrivateShareHost(u.Hostname()) {
			res.Issues = append(res.Issues, ShareIssue{ShareIssueImageFetch, "og:image mengarah ke alamat privat/lokal"})
			return
		}
		imgClient = shareClient(u.Host)
	}
	data, status, err := shareGet(ctx, imgClient, res.Image, ua, shareImageCap)
	if err != nil || status != http.StatusOK {
		res.Issues = append(res.Issues, ShareIssue{ShareIssueImageFetch, fmt.Sprintf("og:image tidak bisa diunduh (status %d)", status)})
		return
	}
	for _, is := range checkShareImageBytes(data) {
		res.Issues = append(res.Issues, is)
	}
}

// checkShareImageBytes memeriksa ukuran dan dimensi satu gambar bagikan.
func checkShareImageBytes(data []byte) []ShareIssue {
	var out []ShareIssue
	if len(data) > shareImageMaxByte {
		size := fmt.Sprintf("%d KB", len(data)/1024)
		if len(data) > shareImageCap {
			size = "lebih dari " + strconv.Itoa(shareImageCap/1024) + " KB"
		}
		out = append(out, ShareIssue{ShareIssueImageSize, fmt.Sprintf("og:image %s, melebihi 300 KB (WhatsApp mengabaikan gambar besar)", size)})
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return append(out, ShareIssue{ShareIssueImageFetch, "og:image bukan gambar yang bisa dibaca (JPEG/PNG/GIF/WebP)"})
	}
	if cfg.Width != shareImageW || cfg.Height != shareImageH {
		out = append(out, ShareIssue{ShareIssueImageDims, fmt.Sprintf("og:image %dx%d, seharusnya %dx%d", cfg.Width, cfg.Height, shareImageW, shareImageH)})
	}
	return out
}

func shareSameHost(pageURL, host string) bool {
	u, err := url.Parse(pageURL)
	return err == nil && strings.EqualFold(u.Host, host)
}

func isPrivateShareHost(h string) bool {
	h = strings.ToLower(h)
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}

func uniqueSharePaths(paths []string, max int) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		if seen[p] || len(out) >= max {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// discoverSharePaths mengambil daftar rute dari sitemap.xml (satu tingkat indeks) ditambah "/" dan petunjuk dinamis.
func discoverSharePaths(ctx context.Context, client *http.Client, base *url.URL, hints []string) []string {
	paths := append([]string{"/"}, hints...)
	sitemap := base.ResolveReference(&url.URL{Path: "/sitemap.xml"})
	for _, loc := range sitemapLocs(ctx, client, sitemap.String(), shareUserAgents[0]) {
		u, err := url.Parse(loc)
		if err != nil || !strings.EqualFold(u.Host, base.Host) {
			continue
		}
		if strings.HasSuffix(strings.ToLower(u.Path), ".xml") {
			for _, sub := range sitemapLocs(ctx, client, loc, shareUserAgents[0]) {
				if su, err := url.Parse(sub); err == nil && strings.EqualFold(su.Host, base.Host) {
					paths = append(paths, su.Path)
				}
			}
			continue
		}
		paths = append(paths, u.Path)
	}
	return paths
}

func sitemapLocs(ctx context.Context, client *http.Client, rawURL, ua string) []string {
	body, status, err := shareGet(ctx, client, rawURL, ua, shareHTMLMaxByte)
	if err != nil || status != http.StatusOK {
		return nil
	}
	var locs []string
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "loc" {
			var s string
			if dec.DecodeElement(&s, &se) == nil {
				locs = append(locs, strings.TrimSpace(s))
			}
		}
	}
	return locs
}

// ShareFindings mengubah hasil per halaman menjadi temuan audit: satu temuan per halaman bermasalah dengan
// path-nya. Urutan prioritas aturan: SEO-SHARE-MISSING, SEO-SHARE-DUPLICATE, SEO-SHARE-IMAGE, SEO-SHARE-NO-TWITTER;
// masalah lain pada halaman yang sama dicantumkan di deskripsi.
func ShareFindings(results []SharePageResult) []models.Finding {
	var out []models.Finding
	for _, r := range results {
		if r.OK || len(r.Issues) == 0 {
			continue
		}
		rule := shareRuleForIssues(r.Issues)
		msgs := make([]string, len(r.Issues))
		for i, is := range r.Issues {
			msgs[i] = is.Message
		}
		out = append(out, models.Finding{
			Category:     "frontend",
			Severity:     "WARNING",
			RuleID:       rule,
			Title:        shareRuleTitle(rule) + ": " + r.Path,
			Description:  "Pratinjau bagikan halaman " + r.Path + " bermasalah di HTML mentah yang dilihat perayap media sosial: " + strings.Join(msgs, "; ") + ".",
			Suggestion:   shareRuleAdvice(rule),
			TargetFile:   r.Path,
			Line:         1,
			SuggestedFix: shareRuleAdvice(rule),
			TheoryBasis:  "Open Graph protocol; patokan SEO-003, SEO-004, dan KONTEN-014.",
		})
	}
	return out
}

var shareIssueRule = map[string]string{
	ShareIssueNoTitle: RuleShareMissing, ShareIssueNoDescription: RuleShareMissing, ShareIssueNoImage: RuleShareMissing,
	ShareIssueFetch: RuleShareMissing, ShareIssueNoURL: RuleShareMissing, ShareIssueNoType: RuleShareMissing,
	ShareIssueNoSiteName: RuleShareMissing, ShareIssueNoImageMeta: RuleShareMissing, ShareIssueDescLength: RuleShareMissing,
	ShareIssueURLMismatch: RuleShareMissing, ShareIssueNoCanonical: RuleShareMissing,
	ShareIssueDuplicate: RuleShareDuplicate,
	ShareIssueImageURL:  RuleShareImage, ShareIssueImageFetch: RuleShareImage, ShareIssueImageSize: RuleShareImage,
	ShareIssueImageDims: RuleShareImage,
	ShareIssueNoTwitter: RuleShareNoTwitter,
}

var shareRulePriority = []string{RuleShareMissing, RuleShareDuplicate, RuleShareImage, RuleShareNoTwitter}

func shareRuleForIssues(issues []ShareIssue) string {
	have := map[string]bool{}
	for _, is := range issues {
		have[shareIssueRule[is.Code]] = true
	}
	for _, r := range shareRulePriority {
		if have[r] {
			return r
		}
	}
	return RuleShareMissing
}

func shareRuleTitle(rule string) string {
	switch rule {
	case RuleShareGeneric:
		return "Pratinjau Bagikan Memakai Meta Umum Situs, Bukan Data Halaman"
	case RuleShareImage:
		return "Gambar Bagikan Tidak Memenuhi Syarat"
	case RuleShareNoTwitter:
		return "twitter:card summary_large_image Tidak Ada"
	case RuleShareDuplicate:
		return "Pratinjau Bagikan Kembar dengan Halaman Lain"
	}
	return "Pratinjau Bagikan Tidak Lengkap"
}

func shareRuleAdvice(rule string) string {
	switch rule {
	case RuleShareGeneric:
		return "Isi og:title, og:description, dan og:image dari data halaman (nama, ringkasan, foto produk dipotong 1200x630) lewat useSeoMeta/generateMetadata setelah data diambil di server; halaman tanpa foto memakai gambar bagikan otomatis berisi judul dan merek (mis. /og/<slug>.png). Lihat SEO-004."
	case RuleShareImage:
		return "Pakai og:image URL absolut https, 1200x630 piksel, di bawah 300 KB (JPEG/PNG), dengan og:image:width/height/alt. Lihat SEO-003."
	case RuleShareNoTwitter:
		return "Tambahkan twitter:card summary_large_image (useSeoMeta({ twitterCard: 'summary_large_image' }) atau metadata.twitter.card). Lihat SEO-003."
	case RuleShareDuplicate:
		return "Beri tiap halaman judul, deskripsi, dan gambar bagikan sendiri dari datanya; jangan memakai meta situs yang sama untuk banyak halaman. Lihat SEO-004."
	}
	return "Lengkapi og:title, og:description (110-160 karakter), og:image (1200x630, <300 KB, absolut https) beserta og:image:width/height/alt, og:url = canonical, og:type, og:site_name, dan twitter:card di HTML server halaman ini. Lihat SEO-003 dan SEO-004."
}

// CheckShareIcons mengunduh /favicon.ico dan membandingkannya dengan sidik jari ikon bawaan starter; hasilnya
// temuan SEO-FAVICON-STARTER (ikon starter) atau SEO-ICON-INCOMPLETE (tidak bisa diunduh).
func CheckShareIcons(ctx context.Context, baseURL string, opts ShareOptions) ([]models.Finding, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Host == "" || (base.Scheme != "https" && !(opts.AllowHTTP && base.Scheme == "http")) {
		return nil, fmt.Errorf("baseURL tidak valid atau bukan https: %q", baseURL)
	}
	client := opts.Client
	if client == nil {
		client = shareClient(base.Host)
	}
	icoURL := base.ResolveReference(&url.URL{Path: "/favicon.ico"}).String()
	data, status, err := shareGet(ctx, client, icoURL, shareUserAgents[0], 1<<20)
	mk := func(rule, sev, title, desc string) models.Finding {
		advice := "Ganti dengan favicon.ico bermerek; uji dengan curl -sI " + icoURL + " (harus 200, bukan ikon starter). Lihat SEO-012."
		return models.Finding{Category: "frontend", Severity: sev, RuleID: rule, Title: title, Description: desc,
			Suggestion: advice, SuggestedFix: advice, TargetFile: "/favicon.ico", Line: 1,
			TheoryBasis: "Patokan SEO-012 dan SEO-013 (ikon situs bermerek)."}
	}
	if err != nil || status != http.StatusOK || len(data) == 0 {
		return []models.Finding{mk(RuleIconIncomplete, "INFO", "/favicon.ico Tidak Bisa Diunduh",
			fmt.Sprintf("/favicon.ico dibalas status %d; perayap dan aplikasi yang memintanya langsung tidak mendapat ikon.", status))}, nil
	}
	if fw, ok := StarterFaviconName(data); ok {
		return []models.Finding{mk(RuleFaviconStarter, "WARNING", "/favicon.ico Adalah Ikon Bawaan Starter "+fw,
			fmt.Sprintf("/favicon.ico (%d byte) identik dengan ikon bawaan starter %s.", len(data), fw))}, nil
	}
	return nil, nil
}
