package knowledge

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

const regressQuery = "Tulis test regresi sebelum memperbaiki bug modul pembayaran dan invoice"

func trialOf(h []Hit) *Hit {
	for i := range h {
		if h[i].Via == ViaTrial {
			return &h[i]
		}
	}
	return nil
}

func exploreCorpus(t *testing.T) *gorm.DB {
	t.Helper()
	db := testDB(t)
	seedCorpus(t, db)
	// Node yang cocok kata dengan kueri tetapi berbobot rendah: tersingkir dari daftar biasa, layak jadi node coba.
	for i, tp := range []string{"kartu", "transfer", "dompet", "cicilan", "refund", "promo", "tagihan", "pajak"} {
		db.Create(&models.KnowledgeNode{Kind: models.KnowledgeKindLesson, Domain: "testing", Weight: 5, Status: models.KnowledgeNodeActive,
			Title: "Regresi pembayaran " + tp, Content: fmt.Sprintf("Test regresi wajib ditulis sebelum memperbaiki bug modul pembayaran %s dan invoice (varian %d).", tp, i)})
	}
	invalidate()
	return db
}

func TestTrialSlotAppendsWithoutReplacingRegularHits(t *testing.T) {
	db := exploreCorpus(t)
	plain := Recall(db, Query{Text: regressQuery, Limit: 4})
	if trialOf(plain.Hits) != nil {
		t.Fatal("recall tanpa konteks tidak mendapat node coba")
	}
	res := Recall(db, Query{Text: regressQuery, Limit: 4, Context: "todo:41", DryRun: true})
	if len(res.Hits) != len(plain.Hits)+1 {
		t.Fatalf("slot coba menambah tepat satu node: %d vs %d", len(res.Hits), len(plain.Hits))
	}
	for i, h := range plain.Hits {
		if res.Hits[i].ID != h.ID || res.Hits[i].Via == ViaTrial {
			t.Fatalf("hasil biasa tidak boleh berubah di posisi %d", i)
		}
	}
	tr := res.Hits[len(res.Hits)-1]
	if tr.Via != ViaTrial || !strings.Contains(Lines(res.Hits), fmt.Sprintf("K%d · pelajaran · testing · coba]", tr.ID)) {
		t.Fatalf("node coba di ujung daftar dan ditandai: %+v\n%s", tr, Lines(res.Hits))
	}
	again := Recall(db, Query{Text: regressQuery, Limit: 4, Context: "todo:41", DryRun: true})
	if trialOf(again.Hits).ID != tr.ID {
		t.Fatal("konteks yang sama harus mendapat node coba yang sama")
	}
	seen := map[uint]bool{}
	for i := range 20 {
		if x := trialOf(Recall(db, Query{Text: regressQuery, Limit: 4, Context: fmt.Sprintf("todo:%d", 100+i), DryRun: true}).Hits); x != nil {
			seen[x.ID] = true
		}
	}
	if len(seen) < 2 {
		t.Fatalf("konteks berbeda harus bisa mendapat node coba berbeda, dapat %v", seen)
	}

	// Pemakaian node coba ikut dicatat sehingga hasil pekerjaannya memberi bukti.
	rec := Recall(db, Query{Text: regressQuery, Limit: 4, Context: "todo:42"})
	var used int64
	db.Model(&models.KnowledgeUse{}).Where("context = ? AND node_id = ?", "todo:42", trialOf(rec.Hits).ID).Count(&used)
	if used != 1 {
		t.Fatal("pemakaian node coba harus dicatat")
	}

	t.Setenv(ExploreEnv, "off")
	if trialOf(Recall(db, Query{Text: regressQuery, Limit: 4, Context: "todo:41", DryRun: true}).Hits) != nil {
		t.Fatal("slot coba bisa dimatikan lewat variabel lingkungan")
	}
}

func TestTrialSlotRespectsOwnershipAndEcosystem(t *testing.T) {
	db := exploreCorpus(t)
	other := uint(9)
	for i := range 6 {
		mustLearn(t, db, Note{Kind: models.KnowledgeKindLesson, Domain: "testing", OwnerEmployeeID: &other,
			Title: fmt.Sprintf("Catatan pribadi regresi %d", i), Content: fmt.Sprintf("Test regresi modul pembayaran invoice versi %c.", 'a'+i)})
	}
	trials := 0
	for i := range 40 {
		res := Recall(db, Query{Text: regressQuery + " di lib/main.dart", Limit: 3, EmployeeID: 3, Context: fmt.Sprintf("todo:%d", i), DryRun: true})
		tr := trialOf(res.Hits)
		if tr == nil {
			continue
		}
		trials++
		if tr.OwnerEmployeeID != nil || tr.Domain == "go" || tr.Domain == "web" {
			t.Fatalf("node coba wajib lolos penyaringan biasa: %+v", tr.KnowledgeNode)
		}
	}
	if trials == 0 {
		t.Fatal("uji ini harus menghasilkan node coba")
	}
}

func TestTrialCandidatesFilters(t *testing.T) {
	hit := func(id uint, rel float64, mod func(*Hit)) Hit {
		h := Hit{KnowledgeNode: models.KnowledgeNode{ID: id, Source: models.KnowledgeSrcAgent}, Relevance: rel}
		if mod != nil {
			mod(&h)
		}
		return h
	}
	regular := []Hit{hit(1, 0.5, nil), hit(2, 0.3, nil), hit(3, 0, func(h *Hit) { h.Via = "tautan" })}
	pool := []Hit{
		regular[0], regular[1],
		hit(10, 0.35, nil), // layak
		hit(11, 0.25, nil), // kurang relevan daripada butir terendah daftar biasa (0.3)
		hit(12, 0.4, func(h *Hit) { h.Source = models.KnowledgeSrcDesign }),
		hit(13, 0.4, func(h *Hit) { h.Helpful, h.Harmful = 2, 1 }),
		hit(14, 0.4, func(h *Hit) { h.Helpful, h.WeakHelpful = 2, 3 }), // 2 + 0.75 < 3: masih sedikit
		hit(15, 0.4, func(h *Hit) { h.Via = "tautan" }),
		hit(16, 0.12, func(h *Hit) { h.Domain = "testing" }), // hanya bonus domain, tanpa kecocokan kata
	}
	var got []uint
	for _, c := range trialCandidates(regular, pool, []string{"testing"}, 0.3) {
		got = append(got, c.ID)
	}
	if fmt.Sprint(got) != "[10 11 14]" {
		t.Fatalf("kandidat slot coba (rasio 0.3) = %v, mau [10 11 14]", got)
	}
	got = got[:0]
	for _, c := range trialCandidates(regular, pool, []string{"testing"}, exploreMinRelRatio) {
		got = append(got, c.ID)
	}
	if fmt.Sprint(got) != "[10 14]" {
		t.Fatalf("kandidat slot coba = %v, mau [10 14]", got)
	}
	if trialCandidates([]Hit{regular[2]}, pool, nil, exploreMinRelRatio) != nil {
		t.Fatal("tanpa butir langsung di daftar biasa tidak ada ambang relevansi, jadi tidak ada kandidat")
	}
}

func TestBetaSampleMean(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	sum := 0.0
	const n = 20000
	for range n {
		sum += betaSample(r, 3, 1)
	}
	if m := sum / n; m < 0.74 || m > 0.76 {
		t.Fatalf("rata-rata Beta(3,1) harus ±0.75, dapat %.4f", m)
	}
}
