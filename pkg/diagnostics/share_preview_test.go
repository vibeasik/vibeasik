package diagnostics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const goodDesc = "Kopi arabika sangrai medium dari kebun dataran tinggi, dikemas segar setiap pekan dan dikirim ke seluruh Indonesia dengan harga bersahabat."

func sharePageHTML(title, desc, img, extra string) string {
	return fmt.Sprintf(`<!doctype html><html><head><title>%s</title>
<link rel="canonical" href="https://shop.example/p">
<meta content="%s" property="og:title">
<meta property='og:description' content='%s'>
%s
<meta property="og:url" content="https://shop.example/p/">
<meta property="og:type" content="website"><meta property="og:site_name" content="Toko">
<meta name="twitter:card" content="summary_large_image">%s</head><body></body></html>`, title, title, desc, img, extra)
}

func imgTags(src string) string {
	return fmt.Sprintf(`<meta property="og:image" content="%s"><meta property="og:image:width" content="1200"><meta property="og:image:height" content="630"><meta property="og:image:alt" content="Foto">`, src)
}

func issueCodes(r SharePageResult) map[string]bool {
	m := map[string]bool{}
	for _, i := range r.Issues {
		m[i.Code] = true
	}
	return m
}

func TestCheckShareHTML(t *testing.T) {
	url := "https://shop.example/p"
	ok := CheckShareHTML(url, []byte(sharePageHTML("Kopi A &amp; B", goodDesc, imgTags("https://shop.example/a.jpg"), "")))
	if !ok.OK || ok.Title != "Kopi A & B" {
		t.Fatalf("complete page must pass (entities decoded, any attribute order): %+v", ok)
	}
	cases := []struct {
		name, html, code string
	}{
		{"tanpa og:image", sharePageHTML("T", goodDesc, "", ""), ShareIssueNoImage},
		{"og:image relatif", sharePageHTML("T", goodDesc, imgTags("/a.jpg"), ""), ShareIssueImageURL},
		{"og:image http", sharePageHTML("T", goodDesc, imgTags("http://shop.example/a.jpg"), ""), ShareIssueImageURL},
		{"deskripsi pendek", sharePageHTML("T", "pendek", imgTags("https://shop.example/a.jpg"), ""), ShareIssueDescLength},
		{"og:url beda canonical", strings.Replace(sharePageHTML("T", goodDesc, imgTags("https://shop.example/a.jpg"), ""), "https://shop.example/p/", "https://lain.example/p", 1), ShareIssueURLMismatch},
	}
	for _, c := range cases {
		r := CheckShareHTML(url, []byte(c.html))
		if r.OK || !issueCodes(r)[c.code] {
			t.Errorf("%s: want %s, got %+v", c.name, c.code, r.Issues)
		}
	}
}

func TestCheckShareHTMLTrickyMarkup(t *testing.T) {
	url := "https://shop.example/p"
	base := sharePageHTML("T", goodDesc, imgTags("https://shop.example/a.jpg"), "")
	// tanda > di dalam nilai atribut tidak boleh memutus tag
	tricky := strings.Replace(base, `<meta property="og:site_name" content="Toko">`, `<meta property="og:site_name" content="Toko > Kopi">`, 1)
	if r := CheckShareHTML(url, []byte(tricky)); !r.OK {
		t.Errorf("> in attribute value: %+v", r.Issues)
	}
	// tag di dalam komentar tidak dihitung
	commented := strings.Replace(base, `<meta property="og:type" content="website">`, `<!-- <meta property="og:type" content="website"> -->`, 1)
	if r := CheckShareHTML(url, []byte(commented)); !issueCodes(r)[ShareIssueNoType] {
		t.Errorf("commented tag must not count: %+v", r.Issues)
	}
	// tanpa canonical
	noCanon := strings.Replace(base, `<link rel="canonical" href="https://shop.example/p">`, ``, 1)
	if r := CheckShareHTML(url, []byte(noCanon)); r.OK || !issueCodes(r)[ShareIssueNoCanonical] {
		t.Errorf("missing canonical: %+v", r.Issues)
	}
}

func TestMarkShareDuplicates(t *testing.T) {
	mk := func(p string) SharePageResult {
		r := CheckShareHTML("https://shop.example"+p, []byte(sharePageHTML("Toko Kami", goodDesc, imgTags("https://shop.example/og.jpg"), "")))
		return r
	}
	rs := []SharePageResult{mk("/produk/a"), mk("/produk/b")}
	MarkShareDuplicates(rs)
	for _, r := range rs {
		if r.OK || !issueCodes(r)[ShareIssueDuplicate] {
			t.Errorf("%s must be flagged as duplicate: %+v", r.Path, r.Issues)
		}
	}
	fs := ShareFindings(rs)
	if len(fs) != 2 || fs[0].RuleID != RuleShareDuplicate || fs[0].TargetFile != "/produk/a" {
		t.Errorf("one finding per page expected: %+v", fs)
	}
}

type shareSite struct {
	mu  sync.Mutex
	uas map[string]map[string]bool
}

func newShareSite(t *testing.T, big []byte, good []byte) (*httptest.Server, *shareSite) {
	t.Helper()
	site := &shareSite{uas: map[string]map[string]bool{}}
	mux := http.NewServeMux()
	var srv *httptest.Server
	page := func(title, img string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			site.mu.Lock()
			if site.uas[r.URL.Path] == nil {
				site.uas[r.URL.Path] = map[string]bool{}
			}
			site.uas[r.URL.Path][strings.SplitN(r.UserAgent(), "/", 2)[0]] = true
			site.mu.Unlock()
			desc := goodDesc + " " + title
			html := sharePageHTML(title, desc, img, "")
			html = strings.ReplaceAll(html, "https://shop.example/p/", srv.URL+r.URL.Path)
			html = strings.ReplaceAll(html, "https://shop.example/p", srv.URL+r.URL.Path)
			w.Write([]byte(html))
		}
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { page("Beranda", imgTags("IMG/home.png"))(w, r) })
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<urlset><url><loc>%s/produk/besar</loc></url><url><loc>%s/produk/ok</loc></url><url><loc>https://lain.example/x</loc></url></urlset>`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/produk/besar", page("Besar", imgTags("IMG/big.jpg")))
	mux.HandleFunc("/produk/ok", page("Lengkap", imgTags("IMG/good.png")))
	mux.HandleFunc("/big.jpg", func(w http.ResponseWriter, r *http.Request) { w.Write(big) })
	mux.HandleFunc("/home.png", func(w http.ResponseWriter, r *http.Request) { w.Write(good) })
	mux.HandleFunc("/good.png", func(w http.ResponseWriter, r *http.Request) { w.Write(good) })
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		body := strings.ReplaceAll(rec.Body.String(), "IMG", srv.URL)
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, site
}

func TestCheckSharePreviewsEndToEnd(t *testing.T) {
	big := pngBytes(t, 1200, 630, true) // derau acak: jauh di atas 300 KB
	if len(big) < 600*1024 {
		t.Fatalf("fixture too small: %d", len(big))
	}
	good := pngBytes(t, 1200, 630, false)
	srv, site := newShareSite(t, big, good)

	res, err := CheckSharePreviews(context.Background(), srv.URL, nil, ShareOptions{Client: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]SharePageResult{}
	for _, r := range res {
		by[r.Path] = r
	}
	if len(res) != 3 {
		t.Fatalf("pages = %d, want 3 (host asing di sitemap dilewati)", len(res))
	}
	if r := by["/produk/ok"]; !r.OK || by["/"].OK != true {
		t.Errorf("halaman lengkap harus lulus: %+v %+v", r.Issues, by["/"].Issues)
	}
	if r := by["/produk/besar"]; r.OK || !issueCodes(r)[ShareIssueImageSize] {
		t.Errorf("gambar besar harus ditandai: %+v", r.Issues)
	}
	for _, p := range []string{"/", "/produk/besar", "/produk/ok"} {
		if !site.uas[p]["facebookexternalhit"] || !site.uas[p]["WhatsApp"] {
			t.Errorf("%s harus diunduh dengan kedua UA: %v", p, site.uas[p])
		}
	}
}

func TestCheckShareImageBytes(t *testing.T) {
	if is := checkShareImageBytes(pngBytes(t, 1200, 630, false)); len(is) != 0 {
		t.Errorf("good image: %v", is)
	}
	codes := func(b []byte) string {
		var s []string
		for _, i := range checkShareImageBytes(b) {
			s = append(s, i.Code)
		}
		return strings.Join(s, ",")
	}
	if c := codes(pngBytes(t, 1200, 630, true)); !strings.Contains(c, ShareIssueImageSize) {
		t.Errorf("big image: %s", c)
	}
	if c := codes(pngBytes(t, 600, 300, false)); c != ShareIssueImageDims {
		t.Errorf("wrong dims: %s", c)
	}
	if c := codes([]byte("bukan gambar")); !strings.Contains(c, ShareIssueImageFetch) {
		t.Errorf("not image: %s", c)
	}
}

func TestCheckShareRejectsBadBase(t *testing.T) {
	if _, err := CheckSharePreviews(context.Background(), "http://example.com", nil, ShareOptions{}); err == nil {
		t.Error("http base must be rejected without AllowHTTP")
	}
}

func TestCheckShareIconsStarter(t *testing.T) {
	ico := starterICO()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.Write(ico)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	got, err := CheckShareIcons(context.Background(), srv.URL, ShareOptions{AllowHTTP: true})
	if err != nil || len(got) != 1 || got[0].RuleID != RuleFaviconStarter {
		t.Fatalf("got %v err %v", got, err)
	}
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	got, _ = CheckShareIcons(context.Background(), other.URL, ShareOptions{AllowHTTP: true})
	if len(got) != 1 || got[0].RuleID != RuleIconIncomplete {
		t.Fatalf("404 favicon: %v", got)
	}
}
