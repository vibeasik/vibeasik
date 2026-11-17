package knowledge

import (
	"fmt"
	"testing"
	"time"

	"github.com/vibeasik/vibeasik/pkg/models"
)

func TestRecountEvidenceDryRunThenApply(t *testing.T) {
	db := evidenceDB(t)
	ids := plainNodes(t, db, models.KnowledgeSrcAgent, models.KnowledgeSrcAgent, models.KnowledgeSrcDesign, models.KnowledgeSrcAgent)
	cited, flat, gallery, merged := ids[0], ids[1], ids[2], ids[3]
	db.Model(&models.KnowledgeNode{}).Where("id IN ?", []uint{cited, flat}).Updates(map[string]interface{}{"helpful": 2, "harmful": 1})
	// merged dilebur ke flat: pemakaiannya dihitung di flat.
	db.Model(&models.KnowledgeNode{}).Where("id = ?", merged).Updates(map[string]interface{}{"status": models.KnowledgeNodeMerged, "merged_into_id": flat})
	todoWithNote(t, db, 1, fmt.Sprintf("PENGETAHUAN: K%d", cited))
	now := time.Now()
	legacy := []models.KnowledgeUse{
		{NodeID: cited, Context: "todo:1", Outcome: models.KnowledgeOutcomeHelpful, CreatedAt: now},
		{NodeID: flat, Context: "todo:1", Outcome: models.KnowledgeOutcomeHelpful, CreatedAt: now},
		{NodeID: gallery, Context: "todo:1", Outcome: models.KnowledgeOutcomeHelpful, CreatedAt: now},
		{NodeID: cited, Context: "todo:2", Outcome: models.KnowledgeOutcomeHarmful, CreatedAt: now},
		{NodeID: merged, Context: "todo:2", Outcome: models.KnowledgeOutcomeHarmful, CreatedAt: now},
		{NodeID: flat, Context: "todo:3", Outcome: models.KnowledgeOutcomeHelpful, CreatedAt: now},
		{NodeID: flat, Context: "todo:4", Outcome: models.KnowledgeOutcomeHelpful, CreatedAt: now}, // melebihi ✓ kuat flat
		{NodeID: flat, Context: "todo:5", Outcome: models.KnowledgeOutcomeNeutral, CreatedAt: now},
	}
	db.Create(&legacy)

	rep, err := RecountEvidence(db, false, 5)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Coverage.Legacy != 7 || rep.Rows.KeptCited != 1 || rep.Rows.MovedHelpful != 2 || rep.Rows.ClampedHelpful != 1 ||
		rep.Rows.MovedHarmful != 2 || rep.Rows.Authoritative != 1 || rep.Rows.ViaMerge != 1 {
		t.Fatalf("putusan pemakaian: %+v %+v", rep.Coverage, rep.Rows)
	}
	if rep.NodesTouched != 2 || rep.UtilityChanged != 2 {
		t.Fatalf("dua node aktif berubah: %+v", rep)
	}
	if n := node(t, db, flat); n.Helpful != 2 || n.WeakHelpful != 0 {
		t.Fatalf("mode kering tidak menulis: %+v", n)
	}

	rep, err = RecountEvidence(db, true, 5)
	if err != nil || !rep.Applied {
		t.Fatalf("terapkan: %v", err)
	}
	if n := node(t, db, cited); n.Helpful != 2 || n.Harmful != 0 || n.WeakHarmful != 1 {
		t.Fatalf("node dikutip: ✓ tetap kuat, ✗ jadi lemah: %+v", n)
	}
	if n := node(t, db, flat); n.Helpful != 0 || n.WeakHelpful != 2 || n.Harmful != 0 || n.WeakHarmful != 1 {
		t.Fatalf("node ikut-ikutan: bukti pindah ke lemah tanpa negatif: %+v", n)
	}
	if n := node(t, db, gallery); n.Helpful != 0 || n.WeakHelpful != 0 {
		t.Fatalf("sumber berkedudukan tinggi tidak disentuh: %+v", n)
	}
	if _, ev := evidenceOf(db, "todo:1", cited); ev != models.KnowledgeEvidenceCited {
		t.Fatalf("asal bukti tercatat: %q", ev)
	}
	again, err := RecountEvidence(db, true, 5)
	if err != nil || again.Coverage.Legacy != 0 || again.NodesTouched != 0 {
		t.Fatalf("hitung ulang kedua tidak mengubah apa pun: %v %+v", err, again)
	}
	if n := node(t, db, flat); n.WeakHelpful != 2 {
		t.Fatalf("idempoten: %+v", n)
	}
}
