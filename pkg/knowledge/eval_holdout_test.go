package knowledge

import (
	"testing"
	"time"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Node yang buktinya hanya berasal dari todo T: netral saat mengevaluasi T, utuh saat mengevaluasi todo lain.
func TestHoldoutRemovesOwnTodoEvidence(t *testing.T) {
	db := evalDB(t)
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local)
	used := base.Add(72 * time.Hour)
	ids := plainNodes(t, db, models.KnowledgeSrcAgent, models.KnowledgeSrcAgent, models.KnowledgeSrcAgent)
	cited, weak, legacy := ids[0], ids[1], ids[2]
	text := "Tutup rows hasil kueri dengan defer"
	for _, id := range ids {
		db.Model(&models.KnowledgeNode{}).Where("id = ?", id).UpdateColumns(map[string]interface{}{"title": text, "content": text, "created_at": base})
	}
	// Penghitung saat ini: semuanya dari todo:7 (cited ✓ kuat, weak ✓ lemah, legacy ✗ catatan lama tanpa asal bukti).
	db.Model(&models.KnowledgeNode{}).Where("id = ?", cited).UpdateColumns(map[string]interface{}{"helpful": 1, "uses": 1, "last_used_at": used})
	db.Model(&models.KnowledgeNode{}).Where("id = ?", weak).UpdateColumns(map[string]interface{}{"weak_helpful": 1, "uses": 1, "last_used_at": used})
	db.Model(&models.KnowledgeNode{}).Where("id = ?", legacy).UpdateColumns(map[string]interface{}{"harmful": 1, "uses": 1, "last_used_at": used})
	db.Create(&[]models.KnowledgeUse{
		{NodeID: cited, Context: "todo:7", Outcome: models.KnowledgeOutcomeHelpful, Evidence: models.KnowledgeEvidenceCited, CreatedAt: used},
		{NodeID: weak, Context: "todo:7", Outcome: models.KnowledgeOutcomeHelpful, Evidence: models.KnowledgeEvidenceOutcome, CreatedAt: used},
		{NodeID: legacy, Context: "todo:7", Outcome: models.KnowledgeOutcomeHarmful, CreatedAt: used},
	})
	invalidate()

	hold, err := loadHoldout(db)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{Text: text, Limit: 10}
	utility := func(todo uint) map[uint]float64 {
		res := rankWith(db, current(db), q, DefaultRankWeights, used, nil, hold.adjustFor(todo))
		out := map[uint]float64{}
		for _, h := range res.Hits {
			out[h.ID] = h.UtilityV
		}
		return out
	}
	own, other := utility(7), utility(8)
	for _, id := range ids {
		if own[id] != 0.5 {
			t.Errorf("node %d saat mengevaluasi todo sumbernya: utilitas %v, mau 0.5 (netral)", id, own[id])
		}
	}
	if other[cited] != round3(2.0/3) || other[weak] != round3(1.25/2.25) || other[legacy] != round3(1.0/3) {
		t.Errorf("todo lain harus melihat penghitung utuh: %v", other)
	}

	// Kebaruan: pemakaian terakhir node berasal dari todo:7 → seakan belum pernah dipakai saat mengevaluasi todo:7;
	// bila ada pemakaian konteks lain, waktunya yang dipakai.
	n := node(t, db, cited)
	hold.adjustFor(7)(&n)
	if n.LastUsedAt != nil || n.Helpful != 0 {
		t.Fatalf("todo:7 masih tersisa di node: helpful=%d last_used=%v", n.Helpful, n.LastUsedAt)
	}
	earlier := base.Add(30 * time.Hour)
	db.Create(&models.KnowledgeUse{NodeID: cited, Context: "plan:3", Outcome: models.KnowledgeOutcomeNeutral, CreatedAt: earlier})
	hold, _ = loadHoldout(db)
	n = node(t, db, cited)
	hold.adjustFor(7)(&n)
	if n.LastUsedAt == nil || !n.LastUsedAt.Equal(earlier) {
		t.Fatalf("waktu pakai tanpa todo:7 = %v, mau %v", n.LastUsedAt, earlier)
	}
	n = node(t, db, cited)
	hold.adjustFor(8)(&n)
	if n.LastUsedAt == nil || !n.LastUsedAt.Equal(used) || n.Helpful != 1 {
		t.Fatalf("todo lain harus melihat waktu pakai & penghitung utuh: %+v", n)
	}

	// Vonis per node di todo:7 juga dikeluarkan; node sumber berkedudukan tinggi tidak disentuh.
	db.Create(&models.KnowledgeVerdict{Context: "todo:7", NodeID: weak, Helpful: false})
	db.Model(&models.KnowledgeNode{}).Where("id = ?", weak).UpdateColumn("harmful", 1)
	hold, _ = loadHoldout(db)
	n = node(t, db, weak)
	hold.adjustFor(7)(&n)
	if n.Harmful != 0 || n.WeakHelpful != 0 || n.Utility() != 0.5 {
		t.Fatalf("vonis todo:7 masih tersisa: %+v", n)
	}
}

// Evaluate bawaan mengeluarkan bukti todo yang diuji; KeepOwnEvidence mengulang cara ukur lama.
func TestEvaluateHoldoutIsDefault(t *testing.T) {
	db := evalDB(t)
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local)
	text := "Tutup rows hasil kueri dengan defer"
	ids := plainNodes(t, db, models.KnowledgeSrcAgent, models.KnowledgeSrcAgent)
	label, rival := ids[0], ids[1]
	for _, id := range ids {
		db.Model(&models.KnowledgeNode{}).Where("id = ?", id).UpdateColumns(map[string]interface{}{"title": text, "content": text, "created_at": base, "updated_at": base})
	}
	// Pesaing sedikit lebih penting; label hanya unggul karena ✓ kuat dari todo yang diuji sendiri.
	db.Model(&models.KnowledgeNode{}).Where("id = ?", rival).UpdateColumn("weight", 70)
	db.Model(&models.KnowledgeNode{}).Where("id = ?", label).UpdateColumn("helpful", 1)
	score := 90
	todo := models.Todo{Title: text, Status: models.TodoStatusCompleted, Score: &score, Rationale: "PENGETAHUAN: K" + itoa(label)}
	todo.CreatedAt = base.Add(time.Hour)
	db.Create(&todo)
	db.Create(&models.KnowledgeUse{NodeID: label, Context: "todo:" + itoa(todo.ID), Outcome: models.KnowledgeOutcomeHelpful,
		Evidence: models.KnowledgeEvidenceCited, CreatedAt: base.Add(2 * time.Hour)})
	invalidate()

	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local)
	held, err := Evaluate(db, EvalOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	old, err := Evaluate(db, EvalOptions{Now: now, KeepOwnEvidence: true})
	if err != nil {
		t.Fatal(err)
	}
	if !held.Holdout || old.Holdout {
		t.Fatalf("Holdout = %v / %v", held.Holdout, old.Holdout)
	}
	if got := old.Tiers[0].Total.MRR; got != 1 {
		t.Fatalf("cara ukur lama: label teratas karena bukti todo sendiri, MRR %v", got)
	}
	if got := held.Tiers[0].Total.MRR; got != 0.5 {
		t.Fatalf("tanpa bukti todo sendiri pesaing yang lebih penting teratas, MRR %v", got)
	}
	// Recall produksi tetap memakai penghitung apa adanya.
	if res := Recall(db, Query{Text: text}); len(res.Hits) < 2 || res.Hits[0].ID != label {
		t.Fatalf("Recall berubah: %+v", res.Hits)
	}
}
