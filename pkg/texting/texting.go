// Package texting menerima teks yang tampil di website (dikirim skrip browser) dan menyimpannya per halaman & section.
package texting

import (
	"crypto/sha1" //nolint:gosec // hanya kunci dedup, bukan keamanan
	"encoding/hex"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vibeasik/vibeasik/pkg/models"
)

const (
	MaxTextsPerIngest = 1000  // teks per kiriman
	MaxTextRunes      = 2000  // teks lebih panjang dipotong
	MinTextRunes      = 2     // teks lebih pendek diabaikan (ikon, tanda baca)
	MaxEntriesPerSrc  = 20000 // batas entri per sumber; di atasnya hanya entri yang sudah ada diperbarui
)

// Item = satu teks yang dikirim browser.
type Item struct {
	Page     string `json:"page"`
	Section  string `json:"section"`
	Selector string `json:"selector"`
	Tag      string `json:"tag"`
	Text     string `json:"text"`
}

// Result = ringkasan satu kiriman.
type Result struct {
	Accepted int `json:"accepted"` // teks valid yang diproses
	Created  int `json:"created"`  // entri baru
	Updated  int `json:"updated"`  // entri lama yang terlihat lagi
	Skipped  int `json:"skipped"`  // ditolak (terlalu pendek, kosong, atau batas entri tercapai)
}

// NormalizeText merapikan spasi (tab/baris baru → satu spasi), memangkas, dan memotong di MaxTextRunes.
func NormalizeText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > MaxTextRunes {
		s = string([]rune(s)[:MaxTextRunes])
	}
	return s
}

// NormalizePage mengambil path saja (tanpa skema, host, query, hash); kosong menjadi "/".
func NormalizePage(p string) string {
	p = strings.TrimSpace(p)
	if u, err := url.Parse(p); err == nil && u.Path != "" {
		p = u.Path
	} else if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
		if p == "" {
			p = "/"
		}
	}
	return clip(p, 255)
}

// Hash = kunci dedup teks pada satu sumber: halaman + teks yang sudah dinormalkan. Section sengaja tidak ikut:
// namanya bisa berubah ketika judul section diedit, dan itu tidak boleh membuat semua teks di dalamnya tampil
// sebagai teks baru. Teks yang sama pada satu halaman (mis. beberapa tombol "Selengkapnya") menjadi satu entri.
func Hash(page, text string) string {
	sum := sha1.Sum([]byte(page + "\x00" + text)) //nolint:gosec
	return hex.EncodeToString(sum[:])
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	for len(string(r)) > n {
		r = r[:len(r)-1]
	}
	return string(r)
}

// Ingest menyimpan kiriman teks untuk satu sumber: teks baru dibuat, yang sudah ada diperbarui waktu terlihatnya.
func Ingest(db *gorm.DB, src *models.TextSource, items []Item, now time.Time) (Result, error) {
	var res Result
	type prepared struct {
		models.TextEntry
	}
	byHash := make(map[string]*prepared, len(items))
	order := make([]string, 0, len(items))
	for _, it := range items {
		text := NormalizeText(it.Text)
		if utf8.RuneCountInString(text) < MinTextRunes {
			res.Skipped++
			continue
		}
		page := NormalizePage(it.Page)
		section := clip(NormalizeText(it.Section), 255)
		h := Hash(page, text)
		if _, dup := byHash[h]; dup { // teks sama berulang pada halaman yang sama = satu entri; Seen menghitung muat halaman
			continue
		}
		byHash[h] = &prepared{models.TextEntry{
			SourceID: src.ID, Hash: h, Page: page, Section: section,
			Selector: clip(it.Selector, 512), Tag: strings.ToLower(clip(it.Tag, 32)), Text: text,
			Seen: 1, FirstSeenAt: now, LastSeenAt: now,
		}}
		order = append(order, h)
		res.Accepted++
	}
	if len(order) == 0 {
		return res, nil
	}

	var existing []models.TextEntry
	if err := db.Select("id, hash, selector, section").Where("source_id = ? AND hash IN ?", src.ID, order).Find(&existing).Error; err != nil {
		return res, err
	}
	have := make(map[string]models.TextEntry, len(existing))
	ids := make([]uint, 0, len(existing))
	for _, e := range existing {
		have[e.Hash] = e
		ids = append(ids, e.ID)
	}

	var total int64
	if len(existing) < len(order) {
		db.Model(&models.TextEntry{}).Where("source_id = ?", src.ID).Count(&total)
	}
	var fresh []models.TextEntry
	for _, h := range order {
		p := byHash[h]
		if e, ok := have[h]; ok {
			// Lokasi bisa pindah/berganti nama tanpa teksnya berubah: simpan yang terbaru.
			if (p.Selector != "" && p.Selector != e.Selector) || (p.Section != "" && p.Section != e.Section) {
				db.Model(&models.TextEntry{}).Where("id = ?", e.ID).Updates(map[string]any{"selector": p.Selector, "section": p.Section})
			}
			continue
		}
		if total+int64(len(fresh)) >= MaxEntriesPerSrc {
			res.Skipped++
			res.Accepted--
			continue
		}
		fresh = append(fresh, p.TextEntry)
	}
	if len(ids) > 0 {
		if err := db.Model(&models.TextEntry{}).Where("id IN ?", ids).
			Updates(map[string]any{"last_seen_at": now, "seen": gorm.Expr("seen + 1")}).Error; err != nil {
			return res, err
		}
		res.Updated = len(ids)
	}
	if len(fresh) > 0 {
		// OnConflict: dua kiriman bersamaan untuk teks yang sama tidak boleh menggagalkan satu sama lain.
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&fresh, 100).Error; err != nil {
			return res, err
		}
		res.Created = len(fresh)
	}
	return res, nil
}
