package knowledge

import (
	"testing"
	"time"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Satu baris per node per konteks; pengerjaan ulang yang dinilai lagi menggantikan bukti lamanya.
func TestRecordUseOncePerContextAcrossRework(t *testing.T) {
	db := evidenceDB(t)
	ids := plainNodes(t, db, models.KnowledgeSrcAgent, models.KnowledgeSrcAgent)
	a, b := ids[0], ids[1]
	todoWithNote(t, db, 5, "PENGETAHUAN: K"+itoa(a))
	ctx := "todo:5"
	rows := func() int64 {
		var n int64
		db.Model(&models.KnowledgeUse{}).Where("context = ?", ctx).Count(&n)
		return n
	}
	check := func(step string, id uint, helpful, weakHelpful, weakHarmful, uses int) {
		t.Helper()
		n := node(t, db, id)
		if n.Helpful != helpful || n.WeakHelpful != weakHelpful || n.WeakHarmful != weakHarmful || n.Uses != uses || n.Harmful != 0 {
			t.Fatalf("%s: node %d helpful=%d weak+=%d weak-=%d uses=%d harmful=%d, mau %d %d %d %d 0", step, id,
				n.Helpful, n.WeakHelpful, n.WeakHarmful, n.Uses, n.Harmful, helpful, weakHelpful, weakHarmful, uses)
		}
	}

	recordUse(db, ctx, 0, []uint{a, b, a})
	recordUse(db, ctx, 0, []uint{a, b}) // prompt dibangun ulang sebelum dinilai
	if rows() != 2 {
		t.Fatalf("baris pemakaian = %d, mau 2", rows())
	}
	if Reward(db, ctx, 90) != 2 {
		t.Fatal("penilaian pertama harus menilai 2 pemakaian")
	}
	check("nilai pertama", a, 1, 0, 0, 1)
	check("nilai pertama", b, 0, 1, 0, 1)

	// Pengerjaan ulang: prompt dibangun lagi. Tidak ada baris baru, hitungan pakai tetap, bukti lama tetap
	// terhitung sampai penilaian baru datang.
	recordUse(db, ctx, 0, []uint{a, b})
	if rows() != 2 {
		t.Fatalf("pengerjaan ulang menambah baris: %d", rows())
	}
	check("dibuka lagi", a, 1, 0, 0, 1)
	if o, e := evidenceOf(db, ctx, a); o != "" || e != reopenCited {
		t.Fatalf("pemakaian dibuka lagi = %q/%q", o, e)
	}
	// Penilaian ulang yang buruk menggantikan ✓ lama, bukan menambahinya.
	if Reward(db, ctx, 30) != 2 {
		t.Fatal("penilaian ulang harus menilai pemakaian yang dibuka lagi")
	}
	check("nilai ulang buruk", a, 0, 0, 1, 1)
	check("nilai ulang buruk", b, 0, 0, 1, 1)

	// Dinilai bagus lagi: kutipan tetap dihitung sekali per konteks.
	recordUse(db, ctx, 0, []uint{a, b})
	Reward(db, ctx, 90)
	check("nilai ulang bagus", a, 1, 0, 0, 1)
	check("nilai ulang bagus", b, 0, 1, 0, 1)
	if Reward(db, ctx, 20) != 0 {
		t.Fatal("tanpa pengerjaan ulang, penilaian kedua tidak boleh menilai lagi")
	}

	// Pengerjaan ulang yang tidak pernah dinilai: nilai lama dipulihkan, penghitung tidak berubah.
	recordUse(db, ctx, 0, []uint{a, b})
	db.Model(&models.Todo{}).Where("id = ?", 5).Updates(map[string]interface{}{"score": 90, "completed_at": time.Now().Add(-72 * time.Hour)})
	if n := SettleStale(db, time.Now()); n != 2 {
		t.Fatalf("SettleStale menutup %d, mau 2", n)
	}
	if o, e := evidenceOf(db, ctx, a); o != models.KnowledgeOutcomeHelpful || e != models.KnowledgeEvidenceCited {
		t.Fatalf("nilai lama a tidak pulih: %q/%q", o, e)
	}
	if o, e := evidenceOf(db, ctx, b); o != models.KnowledgeOutcomeHelpful || e != models.KnowledgeEvidenceOutcome {
		t.Fatalf("nilai lama b tidak pulih: %q/%q", o, e)
	}
	check("dipulihkan", a, 1, 0, 0, 1)
	check("dipulihkan", b, 0, 1, 0, 1)
}

// Catatan lama (sebelum asal bukti dicatat) dicabut dari penghitung kuat; pemakaian bervonis tidak dibuka lagi.
func TestRecordUseReopensLegacyAndKeepsVerdicts(t *testing.T) {
	db := evidenceDB(t)
	ids := plainNodes(t, db, models.KnowledgeSrcAgent, models.KnowledgeSrcAgent)
	legacy, judged := ids[0], ids[1]
	ctx := "todo:6"
	db.Model(&models.KnowledgeNode{}).Where("id IN ?", ids).UpdateColumns(map[string]interface{}{"helpful": 1, "uses": 1})
	db.Create(&[]models.KnowledgeUse{
		{NodeID: legacy, Context: ctx, Outcome: models.KnowledgeOutcomeHelpful},
		{NodeID: judged, Context: ctx, Outcome: models.KnowledgeOutcomeHelpful, Evidence: models.KnowledgeEvidenceVerdict},
	})
	db.Create(&models.KnowledgeVerdict{Context: ctx, NodeID: judged, Helpful: true})

	recordUse(db, ctx, 0, ids)
	if o, e := evidenceOf(db, ctx, legacy); o != "" || e != reopenLegacyHelped {
		t.Fatalf("catatan lama dibuka lagi = %q/%q", o, e)
	}
	if o, e := evidenceOf(db, ctx, judged); o != models.KnowledgeOutcomeHelpful || e != models.KnowledgeEvidenceVerdict {
		t.Fatalf("pemakaian bervonis ikut dibuka: %q/%q", o, e)
	}
	// Vonis baru tidak menimpa pemakaian yang dibuka lagi; Reward berikutnya mencabut ✓ lama lalu mengikuti vonis.
	Judge(db, ctx, map[uint]bool{legacy: false})
	if o, e := evidenceOf(db, ctx, legacy); o != "" || e != reopenLegacyHelped {
		t.Fatalf("Judge menimpa pemakaian yang dibuka lagi: %q/%q", o, e)
	}
	Reward(db, ctx, 60) // netral
	if n := node(t, db, legacy); n.Helpful != 0 || n.Harmful != 1 || n.Uses != 1 {
		t.Fatalf("✓ catatan lama harus dicabut, vonis ✗ tetap: helpful=%d harmful=%d uses=%d", n.Helpful, n.Harmful, n.Uses)
	}
	if o, e := evidenceOf(db, ctx, legacy); o != models.KnowledgeOutcomeHarmful || e != models.KnowledgeEvidenceVerdict {
		t.Fatalf("pemakaian harus mengikuti vonis: %q/%q", o, e)
	}
	if n := node(t, db, judged); n.Helpful != 1 {
		t.Fatalf("vonis tidak boleh tersentuh Reward: helpful=%d", n.Helpful)
	}
	var count int64
	db.Model(&models.KnowledgeUse{}).Where("context = ?", ctx).Count(&count)
	if count != 2 {
		t.Fatalf("baris pemakaian = %d, mau 2", count)
	}
}
