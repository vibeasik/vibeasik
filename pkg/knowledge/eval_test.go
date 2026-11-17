package knowledge

import (
	"math"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestRankMetrics(t *testing.T) {
	rel := map[uint]bool{2: true, 5: true, 9: true}
	// Relevan di posisi 2 dan 4 dari k=4; node 9 tidak terambil.
	r, n, rr := RankMetrics([]uint{1, 2, 3, 5, 9}, rel, 4)
	wantNDCG := (1/math.Log2(3) + 1/math.Log2(5)) / (1 + 1/math.Log2(3) + 1/math.Log2(4))
	if !near(r, 2.0/3) || !near(n, wantNDCG) || !near(rr, 0.5) {
		t.Fatalf("RankMetrics = %v %v %v, mau %v %v %v", r, n, rr, 2.0/3, wantNDCG, 0.5)
	}
	if r, n, rr := RankMetrics([]uint{2, 5, 9}, rel, 10); !near(r, 1) || !near(n, 1) || !near(rr, 1) {
		t.Fatalf("peringkat sempurna harus 1/1/1, dapat %v %v %v", r, n, rr)
	}
	if r, n, rr := RankMetrics([]uint{1, 3}, rel, 10); r != 0 || n != 0 || rr != 0 {
		t.Fatalf("tanpa yang relevan harus 0, dapat %v %v %v", r, n, rr)
	}
	// IDCG dibatasi k: 3 label, k=1, posisi 1 relevan → nDCG 1, recall 1/3.
	if r, n, _ := RankMetrics([]uint{9, 1}, rel, 1); !near(r, 1.0/3) || !near(n, 1) {
		t.Fatalf("k=1: recall %v nDCG %v", r, n)
	}
}

func TestCitedNodes(t *testing.T) {
	text := "Analisis awal\n\nCatatan penyelesaian (Agent, 1 Nov 2026 10:00): beres.\nPENGETAHUAN: K12, K7, K12 (dipakai)\n" +
		"Catatan penyelesaian (Agent, 2 Nov 2026 10:00): PENGETAHUAN: K1654. PELAJARAN: lihat K999 bukan rujukan.\n" +
		"PENGETAHUAN: K5, K6 PELAJARAN: K8 juga bukan"
	if got, want := CitedNodes(text), []uint{12, 7, 1654, 5, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CitedNodes = %v, mau %v", got, want)
	}
	if got := CitedNodes("PENGETAHUAN: tidak ada"); len(got) != 0 {
		t.Fatalf("tanpa K<id> harus kosong, dapat %v", got)
	}
}

func TestDefaultRankWeightsKeepRecallFormula(t *testing.T) {
	want := RankWeights{Relevance: 0.55, Utility: 0.20, Importance: 0.15, Recency: 0.10, RecencyDays: 45, DomainBonus: 0.12, MinRelevance: 0.08}
	if DefaultRankWeights != want {
		t.Fatalf("bobot bawaan berubah: %+v", DefaultRankWeights)
	}
	db := testDB(t)
	n := mustLearn(t, db, Note{Title: "Batasi ukuran unggahan berkas", Content: "Tolak unggahan multipart di atas batas sebelum dibaca ke memori.", Domain: "security", Weight: 80})
	now := time.Now()
	db.Model(&models.KnowledgeNode{}).Where("id = ?", n.ID).UpdateColumns(map[string]interface{}{"helpful": 3, "harmful": 1, "last_used_at": now})
	res := Recall(db, Query{Text: "ukuran unggahan multipart"})
	if len(res.Hits) != 1 || res.Hits[0].ID != n.ID {
		t.Fatalf("hit = %+v", res.Hits)
	}
	// Satu kandidat → relevansi ternormalisasi 1; utilitas (3+1)/(4+2); bobot 0.8; kebaruan ≈ 1.
	if got, want := res.Hits[0].Score, round3(0.55*1+0.20*4.0/6+0.15*0.8+0.10*1); got != want {
		t.Fatalf("skor Recall = %v, mau %v (rumus lama)", got, want)
	}
	// Peringkat inti dengan bobot bawaan = Recall; varian bobot mengubah skor tanpa menyentuh Recall.
	ix := current(db)
	q := Query{Text: "ukuran unggahan multipart", Limit: 10}
	if base := rank(db, ix, q, DefaultRankWeights, time.Now(), nil); !reflect.DeepEqual(hitScores(base.Hits), hitScores(res.Hits)) {
		t.Fatalf("rank bawaan %v ≠ Recall %v", hitScores(base.Hits), hitScores(res.Hits))
	}
	alt := DefaultRankWeights
	alt.Importance = 0
	if v := rank(db, ix, q, alt, time.Now(), nil); v.Hits[0].Score >= res.Hits[0].Score {
		t.Fatalf("varian tanpa kepentingan harus menurunkan skor: %v", v.Hits[0].Score)
	}
}

func hitScores(h []Hit) map[uint]float64 {
	m := map[uint]float64{}
	for _, x := range h {
		m[x.ID] = x.Score
	}
	return m
}

func evalDB(t *testing.T) *gorm.DB {
	db := testDB(t)
	if err := db.AutoMigrate(&models.Todo{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func setCreated(db *gorm.DB, id uint, at time.Time) {
	db.Model(&models.KnowledgeNode{}).Where("id = ?", id).UpdateColumn("created_at", at)
}

func TestEvaluateDropsLeakedNodesAndWritesNothing(t *testing.T) {
	db := evalDB(t)
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local)
	old := mustLearn(t, db, Note{Title: "Validasi token sesi di middleware", Content: "Periksa tanda tangan token sesi sebelum handler berjalan.", Domain: "security"})
	// Node yang lebih cocok dengan kueri tetapi baru dibuat setelah todo: tidak boleh ikut bersaing maupun dinilai.
	late := mustLearn(t, db, Note{Title: "Rotasi token sesi middleware login", Content: "Token sesi middleware login dirotasi setelah login berhasil.", Domain: "security"})
	other := mustLearn(t, db, Note{Title: "Gunakan indeks komposit untuk kueri laporan", Content: "Kueri laporan bulanan memakai indeks komposit tanggal dan status.", Domain: "database"})
	setCreated(db, old.ID, base)
	setCreated(db, other.ID, base)
	setCreated(db, late.ID, base.Add(48*time.Hour))
	invalidate()

	score := 90
	todo := models.Todo{Title: "Rotasi token sesi middleware login", Description: "token sesi", Status: models.TodoStatusCompleted, Score: &score,
		Rationale: "Catatan penyelesaian (Agent, 3 Okt 2026 10:00): selesai.\nPENGETAHUAN: K" + itoa(old.ID) + ", K" + itoa(late.ID) + ", K99999"}
	todo.CreatedAt = base.Add(24 * time.Hour)
	if err := db.Create(&todo).Error; err != nil {
		t.Fatal(err)
	}
	ctx, usedAt := "todo:"+itoa(todo.ID), base.Add(72*time.Hour)
	db.Create(&[]models.KnowledgeUse{
		{NodeID: old.ID, Context: ctx, Outcome: models.KnowledgeOutcomeHelpful, CreatedAt: usedAt},
		{NodeID: late.ID, Context: ctx, Outcome: models.KnowledgeOutcomeHelpful, CreatedAt: usedAt},
		{NodeID: other.ID, Context: ctx, Outcome: models.KnowledgeOutcomeNeutral, CreatedAt: usedAt},
	})
	// Todo gagal QA tidak memberi label kuat.
	low := 40
	failed := models.Todo{Title: "Indeks komposit laporan", Status: models.TodoStatusCompleted, Score: &low, Rationale: "PENGETAHUAN: K" + itoa(other.ID)}
	failed.CreatedAt = base.Add(24 * time.Hour)
	db.Create(&failed)

	var usesBefore int64
	db.Model(&models.KnowledgeUse{}).Count(&usesBefore)
	var nodesBefore []models.KnowledgeNode
	db.Order("id").Find(&nodesBefore)

	opt := EvalOptions{K: 10, Now: time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local)}
	rep, err := Evaluate(db, opt)
	if err != nil {
		t.Fatal(err)
	}
	strong, weak := rep.Tiers[0], rep.Tiers[1]
	if strong.Name != EvalTierStrong || strong.Total.Queries != 1 || strong.Total.Labels != 1 || strong.DroppedLeak != 1 || strong.DroppedInactive != 1 {
		t.Fatalf("tingkat kuat = %+v", strong)
	}
	if weak.Total.Queries != 1 || weak.Total.Labels != 1 || weak.DroppedLeak != 1 {
		t.Fatalf("tingkat lemah = %+v", weak)
	}
	// Tanpa aturan kebocoran node "late" (judulnya = judul todo) berada di atas; dengan aturan itu node lama teratas.
	if strong.Total.MRR != 1 || strong.Total.Recall != 1 {
		t.Fatalf("node bocor masih ikut bersaing: %+v", strong.Total)
	}
	if leaky := rank(db, current(db), DefaultEvalQuery(todo), DefaultRankWeights, opt.Now, nil); len(leaky.Hits) == 0 || leaky.Hits[0].ID != late.ID {
		t.Fatalf("prasyarat test: tanpa aturan kebocoran node baru harus teratas, dapat %+v", leaky.Hits)
	}

	// Tidak ada tulis: pemakaian, penghitung, dan waktu pakai tetap.
	var usesAfter int64
	db.Model(&models.KnowledgeUse{}).Count(&usesAfter)
	var nodesAfter []models.KnowledgeNode
	db.Order("id").Find(&nodesAfter)
	if usesAfter != usesBefore || !reflect.DeepEqual(nodesBefore, nodesAfter) {
		t.Fatalf("Evaluate menulis ke database: uses %d→%d", usesBefore, usesAfter)
	}

	// Deterministik: putaran kedua sama persis.
	again, _ := Evaluate(db, opt)
	if !reflect.DeepEqual(rep, again) {
		t.Fatalf("dua putaran berbeda:\n%+v\n%+v", rep, again)
	}

	// Todo yang dibuat setelah acuan waktu tidak ikut.
	if r, _ := Evaluate(db, EvalOptions{Now: base}); r.Todos != 0 {
		t.Fatalf("todo sesudah acuan waktu ikut dinilai: %d", r.Todos)
	}
}
