package knowledge

import (
	"testing"
	"time"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Butir bersama tulisan mesin dibatasi bobotnya dan ditolak bila berupa potongan laporan; butir manusia tidak.
func TestSharedTrialOnLearn(t *testing.T) {
	db := testDB(t)
	qa, _, err := Learn(db, Note{Domain: "go", Title: "Batasi ukuran body request", Content: "Bungkus body dengan MaxBytesReader sebelum decode JSON.",
		Source: models.KnowledgeSrcReview, Weight: 70})
	if err != nil || qa.Weight != sharedTrialWeight {
		t.Fatalf("qa_review: err=%v weight=%v, ingin %d", err, qa, sharedTrialWeight)
	}
	man, _, err := Learn(db, Note{Domain: "vue", Title: "Komponen kecil satu tanggung jawab", Content: "Pecah komponen di atas 200 baris.",
		Source: models.KnowledgeSrcManual, Weight: 80})
	if err != nil || man.Weight != 80 {
		t.Fatalf("manual: err=%v weight=%d, ingin 80", err, man.Weight)
	}
	for _, c := range []string{"PEGAWAI: Ana — backend. Sudah dites.", "Perbaikan todo #412 di handler", "Ubah /home/ana/app/main.go (L42)"} {
		if _, _, err := Learn(db, Note{Domain: "go", Title: "Pelajaran", Content: c, Source: models.KnowledgeSrcAgent}); err == nil {
			t.Errorf("potongan laporan diterima: %q", c)
		}
	}
	emp := hireForTest(t, db, "Ana")
	if _, _, err := Learn(db, Note{Domain: "go", Title: "Catatan pribadi", Content: "todo #5 mengajari saya soal timeout.",
		Source: models.KnowledgeSrcReview, OwnerEmployeeID: &emp}); err != nil {
		t.Errorf("pengetahuan pribadi tidak ikut masa coba: %v", err)
	}
}

// Kurator memensiunkan butir bersama mesin yang 30 hari tanpa ✓ dan butir asli lain yang 60 hari tak pernah dipakai.
func TestExpireShared(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	emp := hireForTest(t, db, "Budi")
	mk := func(title, src string, age time.Duration, edit func(*models.KnowledgeNode)) uint {
		n := models.KnowledgeNode{Kind: "lesson", Domain: "go", Title: title, Content: title + " isi", Source: src, Weight: 60,
			Status: models.KnowledgeNodeActive, CreatedAt: now.Add(-age)}
		if edit != nil {
			edit(&n)
		}
		if err := db.Create(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n.ID
	}
	day := 24 * time.Hour
	gone := []uint{
		mk("qa terbukti keliru", models.KnowledgeSrcReview, 5*day, func(n *models.KnowledgeNode) { n.Harmful = 2 }),
		mk("agent menyesatkan", models.KnowledgeSrcAgent, 40*day, func(n *models.KnowledgeNode) { n.Harmful = 3; n.Uses = 6 }),
		mk("riset celah keliru", models.KnowledgeSrcGap, 1*day, func(n *models.KnowledgeNode) { n.Harmful = 2 }),
	}
	kept := []uint{
		mk("qa lama tak pernah muncul", models.KnowledgeSrcReview, 200*day, nil),
		mk("qa sering muncul tanpa bukti", models.KnowledgeSrcReview, 120*day, func(n *models.KnowledgeNode) { n.Uses = 9 }),
		mk("qa sekali keliru", models.KnowledgeSrcReview, 31*day, func(n *models.KnowledgeNode) { n.Harmful = 1 }),
		mk("qa keliru lemah saja", models.KnowledgeSrcReview, 31*day, func(n *models.KnowledgeNode) { n.WeakHarmful = 5 }),
		mk("qa pernah terbukti", models.KnowledgeSrcReview, 31*day, func(n *models.KnowledgeNode) { n.Helpful = 1; n.Harmful = 2 }),
		mk("vonis audit menganggur", models.KnowledgeSrcVerdict, 200*day, nil),
		mk("manual keliru dua kali", models.KnowledgeSrcManual, 90*day, func(n *models.KnowledgeNode) { n.Harmful = 2 }),
		mk("qa pribadi keliru", models.KnowledgeSrcReview, 31*day, func(n *models.KnowledgeNode) { n.OwnerEmployeeID = &emp; n.Harmful = 2 }),
	}
	if got := expireShared(db); got != len(gone) {
		t.Errorf("dipensiunkan %d, ingin %d", got, len(gone))
	}
	status := func(id uint) string {
		var n models.KnowledgeNode
		db.First(&n, id)
		return n.Status + " " + n.Title
	}
	for _, id := range gone {
		if s := status(id); s[:7] != models.KnowledgeNodeRetired {
			t.Errorf("harus pensiun: %s", s)
		}
	}
	for _, id := range kept {
		if s := status(id); s[:6] != models.KnowledgeNodeActive {
			t.Errorf("harus tetap: %s", s)
		}
	}
}
