package knowledge

import (
	"fmt"
	"math"
	"testing"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

func evidenceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testDB(t)
	if err := db.AutoMigrate(&models.Todo{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func plainNodes(t *testing.T, db *gorm.DB, sources ...string) []uint {
	t.Helper()
	ids := make([]uint, 0, len(sources))
	for i, src := range sources {
		n := models.KnowledgeNode{Kind: "lesson", Domain: "go", Title: fmt.Sprintf("Butir %d", i), Content: "isi", Source: src, Weight: 60, Status: models.KnowledgeNodeActive}
		if err := db.Create(&n).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}
	return ids
}

func todoWithNote(t *testing.T, db *gorm.DB, id uint, note string) {
	t.Helper()
	if err := db.Create(&models.Todo{ID: id, Title: "todo", Status: models.TodoStatusCompleted, Rationale: note}).Error; err != nil {
		t.Fatal(err)
	}
}

func node(t *testing.T, db *gorm.DB, id uint) models.KnowledgeNode {
	t.Helper()
	var n models.KnowledgeNode
	if err := db.First(&n, id).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func evidenceOf(db *gorm.DB, ctx string, id uint) (outcome, evidence string) {
	var u models.KnowledgeUse
	db.Where("context = ? AND node_id = ?", ctx, id).Order("id DESC").First(&u)
	return u.Outcome, u.Evidence
}

func TestUtilityUnchangedWithoutWeakEvidence(t *testing.T) {
	for _, c := range [][2]int{{0, 0}, {3, 1}, {0, 4}, {19, 0}} {
		n := models.KnowledgeNode{Helpful: c[0], Harmful: c[1]}
		want := float64(c[0]+1) / float64(c[0]+c[1]+2)
		if n.Utility() != want || n.StrongUtility() != want {
			t.Fatalf("tanpa bukti lemah utilitas tetap Beta lama: %v → %v/%v, mau %v", c, n.Utility(), n.StrongUtility(), want)
		}
	}
	n := models.KnowledgeNode{Helpful: 1, WeakHelpful: 4}
	want := (2 + 4*models.KnowledgeWeakWeight) / (2 + 4*models.KnowledgeWeakWeight + 1)
	if math.Abs(n.Utility()-want) > 1e-9 || n.StrongUtility() != 2.0/3 {
		t.Fatalf("bukti lemah berbobot kecil: %v (mau %v), kuat %v", n.Utility(), want, n.StrongUtility())
	}
}

func TestRewardGivesStrongOnlyToCitedNodes(t *testing.T) {
	db := evidenceDB(t)
	ids := plainNodes(t, db, models.KnowledgeSrcAgent, models.KnowledgeSrcAgent, models.KnowledgeSrcDesign)
	recordUse(db, "todo:7", 0, ids)
	todoWithNote(t, db, 7, fmt.Sprintf("selesai\nPENGETAHUAN: K%d, K%d, K999\nPELAJARAN: -", ids[0], ids[2]))
	if n := Reward(db, "todo:7", 88); n != 3 {
		t.Fatalf("semua pemakaian dinilai, dapat %d", n)
	}
	a, b, gallery := node(t, db, ids[0]), node(t, db, ids[1]), node(t, db, ids[2])
	if a.Helpful != 1 || a.WeakHelpful != 0 {
		t.Fatalf("node dikutip = ✓ kuat: %+v", a)
	}
	if b.Helpful != 0 || b.WeakHelpful != 1 {
		t.Fatalf("node hanya disisipkan = ✓ lemah: %+v", b)
	}
	if gallery.Helpful != 0 || gallery.WeakHelpful != 0 {
		t.Fatalf("sumber berkedudukan tinggi tidak digeser: %+v", gallery)
	}
	if _, ev := evidenceOf(db, "todo:7", ids[0]); ev != models.KnowledgeEvidenceCited {
		t.Fatalf("asal bukti node dikutip = cited, dapat %q", ev)
	}
	if _, ev := evidenceOf(db, "todo:7", ids[1]); ev != models.KnowledgeEvidenceOutcome {
		t.Fatalf("asal bukti node lain = outcome, dapat %q", ev)
	}
	var k999 int64
	db.Model(&models.KnowledgeNode{}).Where("id = ?", 999).Count(&k999)
	if k999 != 0 {
		t.Fatal("kutipan atas node yang tidak disisipkan tidak membuat apa pun")
	}
	// Todo yang dinilai ulang (prompt dibangun lagi) tidak memberi ✓ kedua dari kutipan yang sama: penilaian ulang
	// menggantikan yang lama.
	recordUse(db, "todo:7", 0, ids[:1])
	Reward(db, "todo:7", 90)
	if a = node(t, db, ids[0]); a.Helpful != 1 || a.WeakHelpful != 0 || a.Uses != 1 {
		t.Fatalf("kutipan sama di konteks sama hanya sekali kuat: %+v", a)
	}
}

func TestRewardFailureGivesOnlyWeakHarm(t *testing.T) {
	db := evidenceDB(t)
	ids := plainNodes(t, db, models.KnowledgeSrcAgent, models.KnowledgeSrcAgent)
	recordUse(db, "todo:8", 0, ids)
	todoWithNote(t, db, 8, fmt.Sprintf("PENGETAHUAN: K%d", ids[0]))
	Reward(db, "todo:8", 30)
	for _, id := range ids {
		n := node(t, db, id)
		if n.Harmful != 0 || n.WeakHarmful != 1 {
			t.Fatalf("todo gagal hanya memberi ✗ lemah (juga untuk node dikutip): %+v", n)
		}
		if out, ev := evidenceOf(db, "todo:8", id); out != models.KnowledgeOutcomeHarmful || ev != models.KnowledgeEvidenceOutcome {
			t.Fatalf("pemakaian tercatat harmful/outcome, dapat %s/%s", out, ev)
		}
	}
	recordUse(db, "audit:1:a.go", 0, ids)
	Reward(db, "audit:1:a.go", 50)
	if n := node(t, db, ids[0]); n.WeakHelpful != 0 || n.WeakHarmful != 1 {
		t.Fatalf("nilai netral tidak memberi bukti: %+v", n)
	}
}

func TestJudgeCountsOncePerContext(t *testing.T) {
	db := evidenceDB(t)
	ids := plainNodes(t, db, models.KnowledgeSrcAgent, models.KnowledgeSrcDesign)
	recordUse(db, "todo:9", 0, ids[:1])
	for i := 0; i < 3; i++ { // tiga putaran QA gate menilai patokan yang sama
		Judge(db, "todo:9", map[uint]bool{ids[0]: true, ids[1]: true})
	}
	if n := node(t, db, ids[0]); n.Helpful != 1 {
		t.Fatalf("vonis yang sama di konteks yang sama dihitung sekali: %+v", n)
	}
	if n := node(t, db, ids[1]); n.Helpful != 0 {
		t.Fatalf("sumber berkedudukan tinggi tidak dinilai: %+v", n)
	}
	if got := Judge(db, "todo:9", map[uint]bool{ids[0]: false}); got != 1 {
		t.Fatalf("vonis berubah = satu vonis baru, dapat %d", got)
	}
	if n := node(t, db, ids[0]); n.Helpful != 0 || n.Harmful != 1 {
		t.Fatalf("vonis yang berubah memindahkan bukti: %+v", n)
	}
	if out, ev := evidenceOf(db, "todo:9", ids[0]); out != models.KnowledgeOutcomeHarmful || ev != models.KnowledgeEvidenceVerdict {
		t.Fatalf("pemakaian mengikuti vonis: %s/%s", out, ev)
	}
	// Prompt dibangun ulang lalu todo dinilai: node yang sudah divonis tidak mendapat bukti hasil todo.
	recordUse(db, "todo:9", 0, ids[:1])
	todoWithNote(t, db, 9, fmt.Sprintf("PENGETAHUAN: K%d", ids[0]))
	Reward(db, "todo:9", 95)
	if n := node(t, db, ids[0]); n.Helpful != 0 || n.Harmful != 1 || n.WeakHelpful != 0 {
		t.Fatalf("node bervonis tidak dinilai lagi oleh Reward: %+v", n)
	}
	// Tanpa konteks setiap vonis adalah bukti baru (perilaku lama).
	Judge(db, "", map[uint]bool{ids[0]: true})
	Judge(db, "", map[uint]bool{ids[0]: true})
	if n := node(t, db, ids[0]); n.Helpful != 2 {
		t.Fatalf("vonis tanpa konteks selalu dihitung: %+v", n)
	}
}

func TestPromoteAndPruneIgnoreWeakEvidence(t *testing.T) {
	db := testDB(t)
	emp := uint(4)
	weakOnly := models.KnowledgeNode{Kind: "procedure", Domain: "web", Title: "Ikut todo bagus", Content: "isi a", Source: models.KnowledgeSrcEmployee,
		Key: "employee:1", OwnerEmployeeID: &emp, Weight: 60, Helpful: 1, WeakHelpful: 60, Status: models.KnowledgeNodeActive}
	strong := models.KnowledgeNode{Kind: "procedure", Domain: "web", Title: "Terbukti", Content: "isi b", Source: models.KnowledgeSrcEmployee,
		Key: "employee:2", OwnerEmployeeID: &emp, Weight: 60, Helpful: 9, WeakHarmful: 60, Status: models.KnowledgeNodeActive}
	weakBad := models.KnowledgeNode{Kind: "lesson", Domain: "go", Title: "Ikut todo buruk", Content: "isi c", Weight: 50, Harmful: 1, WeakHarmful: 80, Status: models.KnowledgeNodeActive}
	for _, n := range []*models.KnowledgeNode{&weakOnly, &strong, &weakBad} {
		db.Create(n)
	}
	spreadEvidence(t, db, strong.ID, [][2]uint{{4, 1}, {8, 1}, {9, 2}})
	if n := promote(db); n != 1 {
		t.Fatalf("hanya bukti kuat yang mempromosikan, dapat %d", n)
	}
	var promoted models.KnowledgeNode
	db.Where("source_key = ?", "promoted:"+itoa(strong.ID)).First(&promoted)
	if promoted.ID == 0 {
		t.Fatal("node berbukti kuat dipromosikan walau bukti lemahnya buruk")
	}
	if r, _ := prune(db); r != 0 {
		t.Fatalf("bukti lemah tidak memensiunkan, dapat %d", r)
	}
}
