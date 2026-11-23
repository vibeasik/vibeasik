package traffic

import (
	"strings"
	"testing"
	"time"

	"github.com/vibeasik/vibeasik/pkg/models"
)

const (
	uaChrome = "Mozilla/5.0 (Windows NT 10.0) Chrome/120 Safari/537"
	uaPhone  = "Mozilla/5.0 (Linux; Android 14) Chrome/120 Mobile Safari/537"
	uaBot    = "Mozilla/5.0 (compatible; Googlebot/2.1)"
)

func pvEvent(ts int64, route, path, ref, ua, ip string) Event {
	return Event{Kind: KindPageview, TS: ts, Method: "GET", Status: 200, Route: route, Path: path, Referer: ref, UA: ua, IP: ip}
}

func TestIsSpecialKindPageview(t *testing.T) {
	if !IsSpecialKind(KindPageview) || KindPageview != "pageview" {
		t.Fatal("pageview harus jenis khusus")
	}
}

func TestIngestPageviewDims(t *testing.T) {
	db := testDB(t)
	src := models.TrafficSource{Name: "uji-pv", Mode: models.TrafficModeDirect, IngestKey: "k", Enabled: true}
	db.Create(&src)
	now := time.Now().UnixMilli()
	evs := []Event{
		pvEvent(now, "contoh.id", "/harga", "google.com", uaChrome, "1.1.1.1"),
		pvEvent(now, "contoh.id", "/harga", "contoh.id", uaChrome, "1.1.1.1"), // pengunjung sama, navigasi internal
		pvEvent(now, "contoh.id", "/blog", "", uaPhone, "2.2.2.2"),
		pvEvent(now, "", "/solo", "", uaChrome, "3.3.3.3"),
		pvEvent(now, "contoh.id", "/harga", "", uaBot, "9.9.9.9"),
	}
	n, err := Ingest(db, &src, evs, "inst")
	if err != nil || n != len(evs) {
		t.Fatalf("Ingest = %d, %v", n, err)
	}
	count := func(gran, dim, value string) int64 {
		var sum int64
		db.Model(&models.TrafficDimStat{}).Select("COALESCE(SUM(count),0)").
			Where("source_id = ? AND gran = ? AND dim = ? AND value = ?", src.ID, gran, dim, value).Scan(&sum)
		return sum
	}
	for _, g := range []string{models.TrafficGranMinute, models.TrafficGranHour} {
		if c := count(g, "page", "contoh.id/harga"); c != 2 {
			t.Errorf("page contoh.id/harga gran %s = %d, want 2 (bot dikecualikan)", g, c)
		}
	}
	if count("h", "page", "/solo") != 1 || count("h", "page", "contoh.id/blog") != 1 {
		t.Error("page tanpa host memakai path saja; dengan host memakai host+path")
	}
	if count("h", "pref", "google.com") != 1 || count("h", "pref", RefDirect) != 3 || count("h", "pref", RefInternal) != 1 {
		t.Errorf("pref salah: google=%d langsung=%d internal=%d", count("h", "pref", "google.com"), count("h", "pref", RefDirect), count("h", "pref", RefInternal))
	}
	if count("h", "pdev", "bot") != 1 || count("h", "pdev", "desktop") != 3 || count("h", "pdev", "mobile") != 1 {
		t.Error("pdev: bot ikut, kelas lain terhitung")
	}
	var vis int64
	db.Model(&models.TrafficDimStat{}).Where("dim = ?", "pvis").Count(&vis)
	if vis != 3 {
		t.Errorf("pvis = %d baris, want 3 pengunjung manusia (bot dikecualikan)", vis)
	}
	var ids []models.TrafficDimStat
	db.Where("dim = ?", "pvis").Find(&ids)
	for _, r := range ids {
		if len(r.Value) != 12 || strings.Contains(r.Value, ".") {
			t.Errorf("id pengunjung %q harus 12 hex tanpa IP", r.Value)
		}
	}
	var routes, ips, recent int64
	db.Model(&models.TrafficRouteStat{}).Count(&routes)
	db.Model(&models.TrafficDimStat{}).Where("dim IN ?", []string{"ip", "gpath", "status"}).Count(&ips)
	db.Model(&models.TrafficRecentEvent{}).Count(&recent)
	if routes+ips+recent != 0 {
		t.Errorf("pageview tidak boleh masuk rute/ip/gpath/recent: %d %d %d", routes, ips, recent)
	}
	var got models.TrafficSource
	db.First(&got, src.ID)
	if got.TotalEvents != 5 || got.LastEventAt == nil {
		t.Errorf("total_events=%d last_event_at=%v", got.TotalEvents, got.LastEventAt)
	}
}

func TestPageviewReport(t *testing.T) {
	db := testDB(t)
	src := models.TrafficSource{Name: "uji-pv-rep", Mode: models.TrafficModeDirect, IngestKey: "k", Enabled: true}
	db.Create(&src)
	now := time.Now()
	ms := now.UnixMilli()
	var evs []Event
	for i := 0; i < 3; i++ {
		evs = append(evs, pvEvent(ms, "contoh.id", "/harga", "google.com", uaChrome, "1.1.1.1"))
	}
	evs = append(evs,
		pvEvent(ms, "contoh.id", "/blog", "", uaPhone, "2.2.2.2"),
		pvEvent(ms, "", "/solo", "", uaChrome, "3.3.3.3"),
		pvEvent(ms, "contoh.id", "/harga", "", uaBot, "9.9.9.9"),
	)
	if _, err := Ingest(db, &src, evs, "inst"); err != nil {
		t.Fatal(err)
	}
	for _, win := range []string{"1h", "7d"} {
		rep, err := BuildReport(db, src, ParseWindow(win))
		if err != nil {
			t.Fatal(err)
		}
		if rep.Totals.PageViews != 5 || rep.Totals.Visitors != 3 || rep.Totals.Requests != 0 {
			t.Errorf("%s: page_views=%d visitors=%d requests=%d", win, rep.Totals.PageViews, rep.Totals.Visitors, rep.Totals.Requests)
		}
		top := rep.Pages[0]
		if len(rep.Pages) != 3 || top.Page != "contoh.id/harga" || top.Host != "contoh.id" || top.Path != "/harga" || top.Views != 3 || top.Share != 0.6 {
			t.Errorf("%s: pages = %+v", win, rep.Pages)
		}
		for _, p := range rep.Pages {
			if p.Page == "/solo" && (p.Host != "" || p.Path != "/solo") {
				t.Errorf("%s: halaman tanpa host = %+v", win, p)
			}
		}
		if rep.PageReferers[0].Value != RefDirect || rep.PageReferers[0].Count != 3 {
			t.Errorf("%s: referer = %+v", win, rep.PageReferers)
		}
		if len(rep.PageDevices) != 3 {
			t.Errorf("%s: devices = %+v", win, rep.PageDevices)
		}
		var sum int64
		for _, p := range rep.PageSeries {
			sum += p.Views
		}
		if len(rep.PageSeries) < 2 || sum != 5 {
			t.Errorf("%s: series len=%d sum=%d", win, len(rep.PageSeries), sum)
		}
	}
	empty := models.TrafficSource{Name: "kosong", Mode: models.TrafficModeDirect, IngestKey: "k2", Enabled: true}
	db.Create(&empty)
	rep, _ := BuildReport(db, empty, ParseWindow("1h"))
	if rep.Pages == nil || rep.PageSeries == nil || rep.PageReferers == nil || rep.PageDevices == nil {
		t.Error("daftar kunjungan harus [] bukan null")
	}
}

func TestSplitPage(t *testing.T) {
	cases := map[string][2]string{"a.id/x/y": {"a.id", "/x/y"}, "a.id": {"a.id", "/"}, "/p": {"", "/p"}}
	for in, want := range cases {
		if h, p := splitPage(in); h != want[0] || p != want[1] {
			t.Errorf("splitPage(%q) = %q,%q", in, h, p)
		}
	}
}
