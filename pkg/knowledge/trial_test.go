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
		mk("qa lama tak pernah muncul", models.KnowledgeSrcReview, 31*day, nil),
		mk("qa sering muncul tanpa bukti", models.KnowledgeSrcReview, 31*day, func(n *models.KnowledgeNode) { n.Uses = 5 }),
		mk("qa 90 hari jarang muncul", models.KnowledgeSrcReview, 91*day, func(n *models.KnowledgeNode) { n.Uses = 2 }),
		mk("agent lama tanpa bukti", models.KnowledgeSrcAgent, 40*day, nil),
		mk("riset celah lama", models.KnowledgeSrcGap, 31*day, nil),
		mk("vonis audit menganggur", models.KnowledgeSrcVerdict, 61*day, nil),
	}
	kept := []uint{
		mk("qa muda", models.KnowledgeSrcReview, 20*day, nil),
		mk("qa jarang muncul menunggu vonis", models.KnowledgeSrcReview, 31*day, func(n *models.KnowledgeNode) { n.Uses = 4 }),
		mk("qa lama terbukti", models.KnowledgeSrcReview, 31*day, func(n *models.KnowledgeNode) { n.Helpful = 1 }),
		mk("agent lama bukti lemah", models.KnowledgeSrcAgent, 31*day, func(n *models.KnowledgeNode) { n.WeakHelpful = 1 }),
		mk("manual lama", models.KnowledgeSrcManual, 90*day, nil),
		mk("promosi lama", models.KnowledgeSrcPromoted, 90*day, nil),
		mk("vonis audit dipakai", models.KnowledgeSrcVerdict, 61*day, func(n *models.KnowledgeNode) { n.Uses = 1 }),
		mk("vonis audit 40 hari", models.KnowledgeSrcVerdict, 40*day, nil),
		mk("qa pribadi", models.KnowledgeSrcReview, 31*day, func(n *models.KnowledgeNode) { n.OwnerEmployeeID = &emp }),
		mk("guideline berkas", models.KnowledgeSrcGuideline, 90*day, func(n *models.KnowledgeNode) { n.Key = "guideline:x" }),
	}
	if got := expireShared(db, now); got != len(gone) {
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
