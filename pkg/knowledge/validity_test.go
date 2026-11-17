package knowledge

import (
	"strings"
	"testing"
	"time"

	"github.com/vibeasik/vibeasik/pkg/models"
)

func hitIDs(h []Hit) map[uint]bool {
	out := map[uint]bool{}
	for _, x := range h {
		out[x.ID] = true
	}
	return out
}

const goCtxQuery = "Goroutine worker di internal/queue/worker.go bocor saat context dibatalkan"

func TestSupersedeHidesOldNodeButKeepsIt(t *testing.T) {
	db := testDB(t)
	ids := seedCorpus(t, db)
	old := ids["go-ctx"]
	repl := mustLearn(t, db, Note{Kind: models.KnowledgeKindRule, Domain: "go", AppliesTo: "go>=1.26",
		Title: "Goroutine berhenti lewat context", Content: "Worker goroutine memilih ctx.Done() di setiap loop; context dibatalkan = goroutine keluar."})
	if !hitIDs(Recall(db, Query{Text: goCtxQuery, Limit: 10}).Hits)[old] {
		t.Fatal("sebelum digantikan node lama harus terkirim")
	}
	var before models.KnowledgeNode
	db.First(&before, old)

	head, err := Supersede(db, old, repl.ID, "versi baru")
	if err != nil || head != repl.ID {
		t.Fatalf("Supersede: head=%d err=%v", head, err)
	}
	res := Recall(db, Query{Text: goCtxQuery, Limit: 10})
	got := hitIDs(res.Hits)
	if got[old] || !got[repl.ID] {
		t.Fatalf("node lama tidak boleh terkirim, penggantinya harus: %v", got)
	}
	var after models.KnowledgeNode
	db.First(&after, old)
	if after.Status != models.KnowledgeNodeActive || after.Title != before.Title || after.Content != before.Content ||
		after.SupersededByID == nil || *after.SupersededByID != repl.ID || !strings.Contains(after.StatusReason, "versi baru") {
		t.Fatalf("node lama tetap aktif & utuh, hanya penunjuknya yang ditulis: %+v", after)
	}
	if !strings.Contains(Lines(res.Hits), "berlaku go>=1.26") {
		t.Fatalf("keterangan berlaku-untuk harus tampil di butir prompt:\n%s", Lines(res.Hits))
	}

	// Pengganti dipensiunkan → node lama terlihat lagi dan kurator menandainya perlu ditinjau.
	if err := Retire(db, repl.ID, "uji"); err != nil {
		t.Fatal(err)
	}
	if !hitIDs(Recall(db, Query{Text: goCtxQuery, Limit: 10}).Hits)[old] {
		t.Fatal("pengganti yang tidak aktif tidak boleh terus menyembunyikan node lama")
	}
	if r := Curate(db); r.Outdated != 1 {
		t.Fatalf("node yang penggantinya tidak aktif harus ditandai perlu ditinjau, dapat %+v", r)
	}
	if err := Restore(db, old); err != nil {
		t.Fatal(err)
	}
	db.First(&after, old)
	if after.SupersededByID != nil {
		t.Fatal("Restore harus melepas penunjuk pengganti")
	}
}

func TestSupersedeFollowsChainAndRejectsCycles(t *testing.T) {
	db := testDB(t)
	mk := func(title string) uint {
		n := models.KnowledgeNode{Kind: "lesson", Domain: "go", Title: title, Content: title + " isi", Weight: 60, Status: "active"}
		db.Create(&n)
		return n.ID
	}
	a, b, c, d := mk("a"), mk("b"), mk("c"), mk("d")
	if _, err := Supersede(db, a, b, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Supersede(db, b, c, ""); err != nil {
		t.Fatal(err)
	}
	var n models.KnowledgeNode
	db.First(&n, a)
	if n.SupersededByID == nil || *n.SupersededByID != c {
		t.Fatalf("node yang digantikan node lama ikut diarahkan ke ujung rantai, dapat %v", n.SupersededByID)
	}
	if head, err := Supersede(db, d, b, ""); err != nil || head != c {
		t.Fatalf("pengganti yang sudah digantikan → ujung rantai: head=%d err=%v", head, err)
	}
	if _, err := Supersede(db, c, a, ""); err == nil {
		t.Fatal("rantai melingkar harus ditolak")
	}
	if _, err := Supersede(db, a, a, ""); err == nil {
		t.Fatal("node tidak bisa menggantikan dirinya sendiri")
	}
	emp := uint(4)
	p := models.KnowledgeNode{Kind: "lesson", Domain: "go", Title: "p", Content: "p", Weight: 60, Status: "active", OwnerEmployeeID: &emp}
	db.Create(&p)
	if _, err := Supersede(db, c, p.ID, ""); err == nil {
		t.Fatal("node pribadi tidak boleh menggantikan pustaka bersama")
	}
	g := models.KnowledgeNode{Kind: "concept", Domain: "web", Title: "g", Content: "g", Weight: 60, Status: "active", Source: models.KnowledgeSrcDesign}
	db.Create(&g)
	if _, err := Supersede(db, g.ID, c, ""); err == nil {
		t.Fatal("node sumber berkedudukan tinggi hanya boleh digantikan node dari sumber yang sama")
	}
}

func TestLearnSupersedesInsteadOfReinforcing(t *testing.T) {
	db := testDB(t)
	old := mustLearn(t, db, Note{Title: "Bungkus error dengan konteks", Content: "Gunakan fmt.Errorf dengan %w agar penyebab error tidak hilang saat dilempar ke atas.", Domain: "go"})
	n, merged, err := Learn(db, Note{Title: "Bungkus error dengan konteks", Content: "Gunakan fmt.Errorf dengan %w agar penyebab error tidak hilang.", Domain: "go",
		Supersedes: old.ID, AppliesTo: "  go>=1.20  "})
	if err != nil || merged || n.ID == old.ID || n.AppliesTo != "go>=1.20" {
		t.Fatalf("butir pengganti harus jadi node baru: merged=%v id=%d err=%v applies=%q", merged, n.ID, err, n.AppliesTo)
	}
	var o models.KnowledgeNode
	db.First(&o, old.ID)
	if o.SupersededByID == nil || *o.SupersededByID != n.ID || o.Helpful != 0 {
		t.Fatalf("node lama ditandai digantikan tanpa diperkuat: %+v", o)
	}
	// Butir yang sama lagi memperkuat penggantinya, bukan node lama yang sudah digantikan.
	again, merged, err := Learn(db, Note{Title: "Bungkus error dengan konteks", Content: "Gunakan fmt.Errorf dengan %w agar penyebab error tidak hilang.", Domain: "go"})
	if err != nil || !merged || again.ID != n.ID {
		t.Fatalf("penguatan harus jatuh ke pengganti: merged=%v id=%d/%d err=%v", merged, again.ID, n.ID, err)
	}
	if _, _, err := Learn(db, Note{Title: "x", Content: "y", Supersedes: 99999}); err == nil {
		t.Fatal("node yang hendak digantikan wajib ada")
	}
}

func TestExpiredNodeSkippedAndFlaggedNotRetired(t *testing.T) {
	db := testDB(t)
	ids := seedCorpus(t, db)
	id := ids["go-ctx"]
	until := time.Now().Add(-time.Hour)
	if err := SetValidity(db, id, "go<1.22", &until); err != nil {
		t.Fatal(err)
	}
	if hitIDs(Recall(db, Query{Text: goCtxQuery, Limit: 10}).Hits)[id] {
		t.Fatal("node kedaluwarsa tidak boleh terkirim")
	}
	// Peringkat pada waktu sebelum batas berlaku (evaluasi riwayat) masih memuatnya.
	q := Query{Text: goCtxQuery, Limit: 10}
	if !hitIDs(rank(db, current(db), q, DefaultRankWeights, until.Add(-time.Hour), nil).Hits)[id] {
		t.Fatal("sebelum batas berlaku node masih kandidat")
	}
	r := Curate(db)
	var n models.KnowledgeNode
	db.First(&n, id)
	if r.Outdated != 1 || !n.NeedsReview || n.Status != models.KnowledgeNodeActive {
		t.Fatalf("kurator menandai perlu ditinjau, tidak memensiunkan: %+v status=%s review=%v", r, n.Status, n.NeedsReview)
	}
	if r := Curate(db); r.Outdated != 0 {
		t.Fatal("tanda yang sama tidak dihitung ulang")
	}
	db.First(&n, id)
	if !n.NeedsReview {
		t.Fatal("pemulihan tanda tidak boleh melepas node yang masih kedaluwarsa")
	}
	if err := SetValidity(db, id, "go<1.22", nil); err != nil {
		t.Fatal(err)
	}
	Curate(db)
	db.First(&n, id)
	if n.NeedsReview || !hitIDs(Recall(db, Query{Text: goCtxQuery, Limit: 10}).Hits)[id] {
		t.Fatal("batas yang dihapus mengembalikan node ke temu-kembali dan melepas tandanya")
	}
}

func TestCurateDoesNotMergeSupersededNode(t *testing.T) {
	db := testDB(t)
	seedCorpus(t, db)
	a := models.KnowledgeNode{Kind: "lesson", Domain: "go", Title: "Tutup rows query", Content: "Selalu defer rows.Close setelah query database.", Weight: 60, Status: "active"}
	b := models.KnowledgeNode{Kind: "lesson", Domain: "go", Title: "Tutup rows query", Content: "Selalu defer rows.Close setelah query database.", Weight: 50, Status: "active"}
	db.Create(&a)
	db.Create(&b)
	if _, err := Supersede(db, b.ID, a.ID, "uji"); err != nil {
		t.Fatal(err)
	}
	Curate(db)
	var x, y models.KnowledgeNode
	db.First(&x, a.ID)
	db.First(&y, b.ID)
	if x.Status != models.KnowledgeNodeActive || y.Status != models.KnowledgeNodeActive || x.Content != a.Content {
		t.Fatalf("node yang digantikan tidak dilebur ke penggantinya: %s/%s", x.Status, y.Status)
	}
}
