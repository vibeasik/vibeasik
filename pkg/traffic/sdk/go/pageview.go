package vibeasiktraffic

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// PageviewPath = alamat tujuan skrip browser (vibeasik-pageview.js). Pasang:
//
//	mux.Handle(vibeasiktraffic.PageviewPath, vibeasiktraffic.PageviewHandler())
const PageviewPath = "/_vibeasik/pv"

const (
	pvMaxBody  = 2048 // byte
	pvMaxPath  = 255
	pvPerMin   = 60 // kiriman per IP per menit
	pvMaxKeys  = 10000
	pvWindow   = time.Minute
	pvSweepGap = 5 * time.Minute
)

type pvLimiter struct {
	mu    sync.Mutex
	hits  map[string][]time.Time
	sweep time.Time
}

var pvRate = &pvLimiter{hits: map[string][]time.Time{}}

// allow mencatat satu kiriman dan menolak bila IP sudah melewati batas dalam satu menit terakhir.
func (l *pvLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.sweep) > pvSweepGap || len(l.hits) > pvMaxKeys {
		l.prune(now)
		l.sweep = now
	}
	recent := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if now.Sub(t) < pvWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= pvPerMin {
		l.hits[ip] = recent
		return false
	}
	l.hits[ip] = append(recent, now)
	return true
}

func (l *pvLimiter) prune(now time.Time) {
	for ip, ts := range l.hits {
		if len(ts) == 0 || now.Sub(ts[len(ts)-1]) >= pvWindow {
			delete(l.hits, ip)
		}
	}
}

// pvHost = host dari r.Host: huruf kecil, tanpa port.
func pvHost(hostport string) string {
	h := hostport
	if sh, _, err := net.SplitHostPort(hostport); err == nil {
		h = sh
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

// pvSameSite menolak kiriman lintas situs (Sec-Fetch-Site / Origin yang hostnya berbeda).
func pvSameSite(r *http.Request, host string) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	o := r.Header.Get("Origin")
	if o == "" || o == "null" {
		return o == ""
	}
	u, err := url.Parse(o)
	return err == nil && pvHost(u.Host) == host
}

// pvPath memeriksa path halaman: harus diawali "/", query/hash dibuang, maksimum 255 karakter.
func pvPath(p string) (string, bool) {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || len(p) > pvMaxPath || strings.ContainsAny(p, "\r\n\x00") {
		return "", false
	}
	return p, true
}

// pvReferrer = host perujuk saja; kosong bila tidak sah atau sama dengan host halaman.
func pvReferrer(ref, host string) string {
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	if h := pvHost(u.Host); h != host {
		return h
	}
	return ""
}

type pvBody struct {
	P string `json:"p"`
	R string `json:"r"`
}

// readPageview membaca badan (≤ 2 KB) dan memvalidasi path.
func readPageview(r *http.Request) (in pvBody, path string, ok bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, pvMaxBody+1))
	if err != nil || len(body) > pvMaxBody || json.Unmarshal(body, &in) != nil {
		return in, "", false
	}
	path, ok = pvPath(in.P)
	return in, path, ok
}

// PageviewHandler menerima kiriman skrip browser (POST, badan JSON {"p","r"} ≤ 2 KB, Content-Type apa pun)
// dan mencatatnya sebagai event pageview. Host halaman diambil dari r.Host, bukan dari badan.
// SDK nonaktif: dibalas 204 tanpa mencatat.
func PageviewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		host := pvHost(r.Host)
		if !pvSameSite(r, host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !Enabled() {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		in, path, ok := readPageview(r)
		if !ok {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		ip := ClientIP(r.Header, r.RemoteAddr)
		if pvRate.allow(ip, time.Now()) {
			Record(Event{
				TS: time.Now().UnixMilli(), Kind: "pageview", Method: http.MethodGet, Path: path, Status: http.StatusOK, Route: host,
				IP: ip, Country: Country(r.Header), UA: r.UserAgent(), Referer: pvReferrer(in.R, host),
			})
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
