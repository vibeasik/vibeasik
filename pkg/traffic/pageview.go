package traffic

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Dimensi kunjungan halaman (tabel traffic_dim_stats yang sama dengan dimensi lain).
const (
	DimPage    = "page" // host+path (atau path), menit & jam, tanpa bot
	DimPageRef = "pref" // host perujuk atau RefDirect, jam
	DimPageDev = "pdev" // kelas UA, jam (bot ikut)
	DimPageVis = "pvis" // id pengunjung harian tanpa IP mentah, jam, tanpa bot
	RefDirect  = "(langsung)"
)

const maxPages = 50

// pageHost = host halaman dari field route; kosong bila route bukan host (situs satu host).
func pageHost(route string) string {
	h := strings.ToLower(clip(strings.TrimSpace(route), 255))
	if strings.HasPrefix(h, "/") || strings.ContainsAny(h, " \t?#") {
		return ""
	}
	return h
}

// pageValue = nilai dimensi "page": host+path bila host ada, selain itu path.
func pageValue(host, path string) string {
	if host == "" {
		return path
	}
	return host + path
}

// referrerOf = host perujuk luar; kosong atau sama dengan host halaman menjadi RefDirect.
func referrerOf(ref, host string) string {
	if ref == "" || ref == host {
		return RefDirect
	}
	return ref
}

// visitorID = pengenal pengunjung harian: 12 hex pertama sha256(ip|ua|tanggal UTC|salt sumber). IP mentah tidak disimpan.
func visitorID(n normEvent, src *models.TrafficSource) string {
	salt := fmt.Sprintf("%d:%d", src.ID, src.CreatedAt.Unix())
	sum := sha256.Sum256([]byte(n.ip + "|" + n.ev.UA + "|" + n.at.UTC().Format("2006-01-02") + "|" + salt))
	return hex.EncodeToString(sum[:])[:12]
}

// addPageview menambah dimensi kunjungan satu event ke akumulator.
func (a *ingestAcc) addPageview(n normEvent, src *models.TrafficSource) {
	host := pageHost(n.ev.Route)
	a.addDim(models.TrafficGranHour, n.hour, DimPageRef, referrerOf(n.ref, host), false)
	a.addDim(models.TrafficGranHour, n.hour, DimPageDev, n.uaClass, false)
	if n.uaClass == "bot" {
		return
	}
	page := pageValue(host, n.path)
	a.addDim(models.TrafficGranMinute, n.minute, DimPage, page, false)
	a.addDim(models.TrafficGranHour, n.hour, DimPage, page, false)
	a.addDim(models.TrafficGranHour, n.hour, DimPageVis, visitorID(n, src), false)
}

// persistPageviews menyimpan dimensi kunjungan; pageview menambah total_events & last_event_at.
func persistPageviews(tx *gorm.DB, src *models.TrafficSource, views []normEvent) error {
	if len(views) == 0 {
		return nil
	}
	acc := &ingestAcc{dims: map[dimKey][2]int64{}}
	lastAt := views[0].at
	for _, n := range views {
		acc.addPageview(n, src)
		if n.at.After(lastAt) {
			lastAt = n.at
		}
	}
	if err := mergeDims(tx, src.ID, acc.dims); err != nil {
		return err
	}
	updates := map[string]any{"total_events": gorm.Expr("total_events + ?", len(views))}
	if src.LastEventAt == nil || lastAt.After(*src.LastEventAt) {
		updates["last_event_at"] = lastAt
	}
	return tx.Model(&models.TrafficSource{}).Where("id = ?", src.ID).Updates(updates).Error
}

// PageRow = satu halaman pada laporan kunjungan.
type PageRow struct {
	Page  string  `json:"page"`
	Host  string  `json:"host"`
	Path  string  `json:"path"`
	Views int64   `json:"views"`
	Share float64 `json:"share"`
}

// PagePoint = jumlah kunjungan pada satu titik grafik.
type PagePoint struct {
	T     int64 `json:"t"`
	Views int64 `json:"views"`
}

// splitPage memecah nilai dimensi page menjadi host dan path.
func splitPage(v string) (host, path string) {
	if strings.HasPrefix(v, "/") {
		return "", v
	}
	i := strings.IndexByte(v, '/')
	if i < 0 {
		return v, "/"
	}
	return v[:i], v[i:]
}

type pageSums struct {
	rows []struct {
		Value string
		Count int64
	}
	points []struct{ Bucket, Count int64 }
}

func loadPageSums(db *gorm.DB, srcID uint, gran string, fromUnix int64) (*pageSums, error) {
	s := &pageSums{}
	base := func() *gorm.DB {
		return db.Model(&models.TrafficDimStat{}).Where("source_id = ? AND dim = ? AND gran = ? AND bucket >= ?", srcID, DimPage, gran, fromUnix)
	}
	if err := base().Select("value, SUM(count) AS count").Group("value").Order("SUM(count) DESC, value ASC").Limit(maxPages).Scan(&s.rows).Error; err != nil {
		return nil, err
	}
	if err := base().Select("bucket, SUM(count) AS count").Group("bucket").Scan(&s.points).Error; err != nil {
		return nil, err
	}
	return s, nil
}

// pageKV mengurutkan dimensi kunjungan; Share = porsi terhadap seluruh nilai dimensi itu (sebelum dipotong).
func pageKV(dims map[string][]KV, name string, n int) []KV {
	var total int64
	for _, kv := range dims[name] {
		total += kv.Count
	}
	for i := range dims[name] {
		dims[name][i].Share = ratio(float64(dims[name][i].Count), float64(total))
		dims[name][i].Err = 0
	}
	return topKV(dims, name, n)
}

func fillPageSeries(rep *Report, w Window, points []struct{ Bucket, Count int64 }, now time.Time) {
	step := int64(w.Step.Seconds())
	byStep := map[int64]int64{}
	for _, p := range points {
		byStep[p.Bucket/step*step] += p.Count
	}
	for k := rep.From.Unix() / step * step; k <= now.Unix(); k += step {
		rep.PageSeries = append(rep.PageSeries, PagePoint{T: k, Views: byStep[k]})
	}
}

// fillPages mengisi bagian kunjungan halaman laporan. Total & halaman memakai granularitas jendela (menit/jam);
// perujuk, perangkat, dan pengunjung unik memakai rangkuman per jam (dims sudah dimuat).
func fillPages(db *gorm.DB, rep *Report, w Window, fromUnix int64, dims map[string][]KV, now time.Time) error {
	rep.Pages, rep.PageSeries = []PageRow{}, []PagePoint{}
	rep.PageReferers = pageKV(dims, DimPageRef, 15)
	rep.PageDevices = pageKV(dims, DimPageDev, 10)
	rep.Totals.Visitors = int64(len(dims[DimPageVis]))
	s, err := loadPageSums(db, rep.Source.ID, w.Gran, fromUnix)
	if err != nil {
		return err
	}
	for _, p := range s.points {
		rep.Totals.PageViews += p.Count
	}
	for _, r := range s.rows {
		host, path := splitPage(r.Value)
		rep.Pages = append(rep.Pages, PageRow{Page: r.Value, Host: host, Path: path, Views: r.Count, Share: ratio(float64(r.Count), float64(rep.Totals.PageViews))})
	}
	sort.SliceStable(rep.Pages, func(i, j int) bool { return rep.Pages[i].Views > rep.Pages[j].Views })
	if rep.Totals.PageViews > 0 {
		fillPageSeries(rep, w, s.points, now)
	}
	return nil
}
