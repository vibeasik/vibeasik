package vibeasiktraffic

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestMain menyalakan SDK sekali di awal (sebelum goroutine latar mulai) agar tes tidak berebut variabel global.
// Anak proses dengan PV_DISABLED=1 dibiarkan nonaktif untuk menguji jalur SDK mati.
func TestMain(m *testing.M) {
	if os.Getenv("PV_DISABLED") != "1" {
		endpoint, key, instance = "http://127.0.0.1:1/ingest", "k", "uji"
	}
	os.Exit(m.Run())
}

func pvSetup(t *testing.T) {
	t.Helper()
	mu.Lock()
	buf = nil
	mu.Unlock()
	pvRate = &pvLimiter{hits: map[string][]time.Time{}}
}

// viewCount menghitung event pageview di buffer (heartbeat latar diabaikan); mu dipegang di dalam.
func viewCount() int {
	mu.Lock()
	defer mu.Unlock()
	n := 0
	for _, e := range buf {
		if e.Kind == "pageview" {
			n++
		}
	}
	return n
}

func lastView(t *testing.T) Event {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	for _, e := range buf {
		if e.Kind == "pageview" {
			return e
		}
	}
	t.Fatal("tidak ada event pageview")
	return Event{}
}

func pvPost(h http.Handler, body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, PageviewPath, strings.NewReader(body))
	r.Host = "Contoh.ID:8080"
	r.RemoteAddr = "203.0.113.7:5555"
	r.Header.Set("User-Agent", "Mozilla/5.0 uji")
	r.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPageviewRecords(t *testing.T) {
	pvSetup(t)
	w := pvPost(PageviewHandler(), `{"p":"/harga?x=1#a","r":"https://Google.com/search?q=1"}`, map[string]string{"Origin": "https://contoh.id:8080"})
	if w.Code != 204 {
		t.Fatalf("code = %d", w.Code)
	}
	ev := lastView(t)
	if ev.Kind != "pageview" || ev.Route != "contoh.id" || ev.Path != "/harga" || ev.Referer != "google.com" || ev.Method != "GET" || ev.Status != 200 || ev.IP != "203.0.113.7" {
		t.Errorf("event = %+v", ev)
	}
}

func TestPageviewSameHostReferrerKept(t *testing.T) {
	pvSetup(t)
	pvPost(PageviewHandler(), `{"p":"/a","r":"https://contoh.id/x"}`, nil)
	if ev := lastView(t); ev.Referer != "contoh.id" {
		t.Errorf("perujuk host sama harus dikirim apa adanya: %+v", ev)
	}
}

func TestPageviewRejects(t *testing.T) {
	pvSetup(t)
	h := PageviewHandler()
	cases := []struct {
		name string
		body string
		hdr  map[string]string
		want int
	}{
		{"origin lain", `{"p":"/a"}`, map[string]string{"Origin": "https://jahat.example"}, 403},
		{"sec-fetch cross-site", `{"p":"/a"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"badan terlalu besar", `{"p":"/a","r":"` + strings.Repeat("x", 3000) + `"}`, nil, 400},
		{"path tanpa slash", `{"p":"a"}`, nil, 400},
		{"path protocol-relative", `{"p":"//evil.example"}`, nil, 400},
		{"path terlalu panjang", `{"p":"/` + strings.Repeat("a", 300) + `"}`, nil, 400},
		{"bukan JSON", `halo`, nil, 400},
	}
	for _, c := range cases {
		if w := pvPost(h, c.body, c.hdr); w.Code != c.want {
			t.Errorf("%s: code = %d, want %d", c.name, w.Code, c.want)
		}
	}
	if n := viewCount(); n != 0 {
		t.Errorf("kiriman ditolak tidak boleh tercatat: %d", n)
	}
	r := httptest.NewRequest(http.MethodGet, PageviewPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Errorf("GET = %d, want 405", w.Code)
	}
}

func TestPageviewDisabled(t *testing.T) {
	if os.Getenv("PV_DISABLED") == "1" {
		if w := pvPost(PageviewHandler(), `{"p":"/a"}`, nil); w.Code != 204 {
			t.Fatalf("code = %d", w.Code)
		}
		if viewCount() != 0 {
			t.Fatal("SDK nonaktif tidak boleh mencatat")
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPageviewDisabled$")
	cmd.Env = append(os.Environ(), "PV_DISABLED=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("anak proses gagal: %v\n%s", err, out)
	}
}

func TestPageviewRateLimit(t *testing.T) {
	pvSetup(t)
	h := PageviewHandler()
	for i := 0; i < pvPerMin+10; i++ {
		if w := pvPost(h, `{"p":"/a"}`, nil); w.Code != 204 {
			t.Fatalf("code = %d", w.Code)
		}
	}
	if n := viewCount(); n != pvPerMin {
		t.Errorf("tercatat %d, want %d", viewCount(), pvPerMin)
	}
}

func TestPageviewLimiterPrunes(t *testing.T) {
	l := &pvLimiter{hits: map[string][]time.Time{}}
	t0 := time.Now()
	l.allow("a", t0)
	l.allow("b", t0)
	l.allow("c", t0.Add(10*time.Minute))
	if len(l.hits) != 1 {
		t.Errorf("entri lama harus dibersihkan: %d", len(l.hits))
	}
}
