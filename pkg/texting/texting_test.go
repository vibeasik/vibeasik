package texting

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/vibeasik/vibeasik/pkg/models"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.TextSource{}, &models.TextEntry{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestNormalize(t *testing.T) {
	if got := NormalizeText("  Halo \n\t dunia  "); got != "Halo dunia" {
		t.Fatalf("NormalizeText = %q", got)
	}
	if got := NormalizeText(strings.Repeat("a", MaxTextRunes+50)); len([]rune(got)) != MaxTextRunes {
		t.Fatalf("teks panjang tidak dipotong: %d", len([]rune(got)))
	}
	cases := map[string]string{
		"":                          "/",
		"/harga/?a=1#x":             "/harga",
		"https://x.test/tentang/#a": "/tentang",
		"kontak":                    "/kontak",
		"/":                         "/",
	}
	for in, want := range cases {
		if got := NormalizePage(in); got != want {
			t.Errorf("NormalizePage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIngestDedupAndUpdate(t *testing.T) {
	db := testDB(t)
	src := models.TextSource{Name: "web", IngestKey: "k", Enabled: true}
	db.Create(&src)
	now := time.Now()
	items := []Item{
		{Page: "/", Section: "Hero", Tag: "h1", Text: "Solusi terbaik untuk bisnis Anda"},
		{Page: "/", Section: "Hero", Tag: "h1", Text: "Solusi  terbaik\nuntuk bisnis Anda"}, // sama setelah dinormalkan
		{Page: "/", Section: "Hero", Tag: "p", Text: "x"},                                   // terlalu pendek
		{Page: "/harga?ref=a", Section: "Section: Paket", Tag: "p", Text: "Mulai dari Rp99.000"},
	}
	res, err := Ingest(db, &src, items, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted != 2 || res.Created != 2 || res.Skipped != 1 {
		t.Fatalf("hasil pertama = %+v", res)
	}
	later := now.Add(time.Minute)
	res, err = Ingest(db, &src, items[:1], later)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 || res.Updated != 1 {
		t.Fatalf("hasil kedua = %+v", res)
	}
	var e models.TextEntry
	db.Where("page = ?", "/").First(&e)
	if e.Seen != 2 || !e.LastSeenAt.After(e.FirstSeenAt) {
		t.Fatalf("entri tidak diperbarui: seen=%d first=%v last=%v", e.Seen, e.FirstSeenAt, e.LastSeenAt)
	}
	var n int64
	db.Model(&models.TextEntry{}).Where("source_id = ?", src.ID).Count(&n)
	if n != 2 {
		t.Fatalf("jumlah entri = %d, want 2", n)
	}
}

func TestIngestEditedTextBecomesNewEntry(t *testing.T) {
	db := testDB(t)
	src := models.TextSource{Name: "web", IngestKey: "k", Enabled: true}
	db.Create(&src)
	now := time.Now()
	Ingest(db, &src, []Item{{Page: "/", Section: "Hero", Text: "Teks lama"}}, now)
	Ingest(db, &src, []Item{{Page: "/", Section: "Hero", Text: "Teks baru"}}, now.Add(time.Hour))
	var n int64
	db.Model(&models.TextEntry{}).Where("source_id = ?", src.ID).Count(&n)
	if n != 2 {
		t.Fatalf("entri = %d, want 2 (lama + baru)", n)
	}
}

func TestIngestSectionRenameKeepsEntry(t *testing.T) {
	db := testDB(t)
	src := models.TextSource{Name: "web", IngestKey: "k", Enabled: true}
	db.Create(&src)
	now := time.Now()
	Ingest(db, &src, []Item{{Page: "/", Section: "Section: Judul lama", Selector: "main > p", Text: "Isi paragraf yang sama"}}, now)
	Ingest(db, &src, []Item{{Page: "/", Section: "Section: Judul baru", Selector: "main > p", Text: "Isi paragraf yang sama"}}, now.Add(time.Hour))
	var rows []models.TextEntry
	db.Find(&rows)
	if len(rows) != 1 || rows[0].Section != "Section: Judul baru" {
		t.Fatalf("section ganti nama harus memperbarui entri yang sama: %+v", rows)
	}
}
