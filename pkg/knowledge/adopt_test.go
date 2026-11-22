package knowledge

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// spreadEvidence membuat vonis ✓ kuat untuk node dari todo milik pasangan (pegawai, proyek).
func spreadEvidence(t *testing.T, db *gorm.DB, nodeID uint, pairs [][2]uint) {
	t.Helper()
	for _, p := range pairs {
		emp, proj := p[0], p[1]
		todo := models.Todo{Title: fmt.Sprintf("todo %d-%d", emp, proj), EmployeeID: &emp, ProjectID: &proj}
		if err := db.Create(&todo).Error; err != nil {
			t.Fatal(err)
		}
		db.Create(&models.KnowledgeVerdict{Context: fmt.Sprintf("todo:%d", todo.ID), NodeID: nodeID, Helpful: true})
	}
}

func hireForTest(t *testing.T, db *gorm.DB, name string) uint {
	t.Helper()
	e := models.Employee{Name: name, Role: "backend_engineer", Status: models.EmployeeActive}
	if err := db.Create(&e).Error; err != nil {
		t.Fatal(err)
	}
	return e.ID
}

func TestFeedbackLinesParsed(t *testing.T) {
	note := "PENGETAHUAN: K12, K40. PELAJARAN: x\nKELIRU: K7\nTIDAK COCOK: K40, K41"
	if got := CitedNodes(note); fmt.Sprint(got) != "[12 40]" {
		t.Errorf("PENGETAHUAN = %v", got)
	}
	if got := WrongNodes(note); fmt.Sprint(got) != "[7]" {
		t.Errorf("KELIRU = %v", got)
	}
	if got := UnfitNodes(note); fmt.Sprint(got) != "[40 41]" {
		t.Errorf("TIDAK COCOK = %v", got)
	}
}

// Pengalaman yang mulai terbukti ditawarkan ke rekan sebidang, ikut tampil di temu-kembalinya, lalu dipertahankan
// atau dibuang menurut hasil masa coba.
func TestShareTrialKeepAndDrop(t *testing.T) {
	db := testDB(t)
	owner, peer, other, idle := hireForTest(t, db, "Ana"), hireForTest(t, db, "Budi"), hireForTest(t, db, "Cici"), hireForTest(t, db, "Dodi")
	old := time.Now().Add(-10 * 24 * time.Hour)
	exp := models.KnowledgeNode{Kind: "procedure", Domain: "go", Title: "Batasi body request di handler Go",
		Content: "Bungkus r.Body dengan http.MaxBytesReader sebelum decode JSON.", Source: models.KnowledgeSrcEmployee,
		Key: "employee:1", OwnerEmployeeID: &owner, OwnerName: "Ana", Weight: 60, Helpful: 2, Status: models.KnowledgeNodeActive, CreatedAt: old}
	shared := models.KnowledgeNode{Kind: "rule", Domain: "go", Title: "Tutup rows", Content: "Selalu defer rows.Close().", Weight: 70, Status: models.KnowledgeNodeActive}
	db.Create(&exp)
	db.Create(&shared)
	// peer & other bekerja di domain go 30 hari terakhir; idle tidak
	for _, e := range []uint{peer, other} {
		emp := e
		db.Create(&models.KnowledgeUse{NodeID: shared.ID, Context: fmt.Sprintf("todo:%d", 100+e), EmployeeID: &emp})
	}
	if n := offerShares(db, time.Now()); n != 2 {
		t.Fatalf("tawaran ke rekan sebidang = %d, mau 2", n)
	}
	var idleN int64
	db.Model(&models.KnowledgeAdoption{}).Where("employee_id = ?", idle).Count(&idleN)
	if idleN != 0 {
		t.Fatal("pegawai yang tidak bekerja di domain itu tidak ditawari")
	}
	if n := offerShares(db, time.Now()); n != 0 {
		t.Fatal("tawaran tidak boleh berulang")
	}
	invalidate()
	res := Recall(db, Query{Text: "batasi body request MaxBytesReader decode JSON go", EmployeeID: peer, Context: "todo:900"})
	found := false
	for _, h := range res.Hits {
		found = found || h.ID == exp.ID
	}
	if !found {
		t.Fatalf("butir adopsi harus tampil untuk penerimanya: %+v", res.Hits)
	}
	var a models.KnowledgeAdoption
	db.Where("employee_id = ? AND node_id = ?", peer, exp.ID).First(&a)
	if a.Uses != 1 {
		t.Fatalf("pemakaian adopsi tercatat: %+v", a)
	}
	if res := Recall(db, Query{Text: "batasi body request MaxBytesReader decode JSON go", EmployeeID: idle}); len(res.Hits) > 0 {
		for _, h := range res.Hits {
			if h.ID == exp.ID {
				t.Fatal("pengalaman pribadi tidak boleh tampil untuk pegawai yang tidak mengadopsinya")
			}
		}
	}
	// peer memakainya dan lulus → dipertahankan; other menandai tidak cocok → dibuang
	SettleAdoptions(db, peer, true, []uint{exp.ID}, nil, nil)
	SettleAdoptions(db, other, true, nil, nil, []uint{exp.ID})
	decideTrials(db, time.Now())
	db.Where("employee_id = ? AND node_id = ?", peer, exp.ID).First(&a)
	var b models.KnowledgeAdoption
	db.Where("employee_id = ? AND node_id = ?", other, exp.ID).First(&b)
	if a.Status != models.KnowledgeAdoptionKept || b.Status != models.KnowledgeAdoptionDropped {
		t.Fatalf("peer=%s (mau kept) other=%s (mau dropped)", a.Status, b.Status)
	}
	net := EmployeeNetwork(db, peer)
	if len(net.Nodes) != 1 || net.Nodes[0].Relation != "adopted" || net.Domains["go"] == 0 {
		t.Fatalf("jaring penerima: %+v", net)
	}
}

func TestTrialDroppedWhenNeverCited(t *testing.T) {
	db := testDB(t)
	peer := hireForTest(t, db, "Eka")
	owner := uint(77)
	n := models.KnowledgeNode{Kind: "lesson", Domain: "web", Title: "x", Content: "y", OwnerEmployeeID: &owner, Status: models.KnowledgeNodeActive}
	db.Create(&n)
	db.Create(&models.KnowledgeAdoption{NodeID: n.ID, EmployeeID: peer, Status: models.KnowledgeAdoptionTrial, Uses: trialMaxUses})
	if decideTrials(db, time.Now()) != 1 {
		t.Fatal("masa coba yang disodorkan 5 kali tanpa dikutip harus ditutup")
	}
	var a models.KnowledgeAdoption
	db.First(&a)
	if a.Status != models.KnowledgeAdoptionDropped {
		t.Fatalf("status = %s", a.Status)
	}
}

// Promosi butuh bukti dari banyak pegawai & proyek, lolos peninjau, dan butir asalnya digantikan patokan.
func TestPromotionNeedsSpreadAndReviewer(t *testing.T) {
	db := testDB(t)
	owner := uint(3)
	n := models.KnowledgeNode{Kind: "procedure", Domain: "go", Title: "Batasi body request", Content: "Pakai MaxBytesReader.",
		Source: models.KnowledgeSrcEmployee, Key: "employee:9", OwnerEmployeeID: &owner, OwnerName: "Ana", Weight: 60, Helpful: 9, Status: models.KnowledgeNodeActive}
	db.Create(&n)
	spreadEvidence(t, db, n.ID, [][2]uint{{3, 1}, {3, 1}}) // satu pegawai, satu proyek
	if promote(db) != 0 {
		t.Fatal("bukti dari satu pegawai di satu proyek tidak cukup")
	}
	spreadEvidence(t, db, n.ID, [][2]uint{{4, 2}, {5, 2}})
	calls := 0
	SetPromotionReviewer(func(*gorm.DB, models.KnowledgeNode, string) (PromotionReview, error) {
		calls++
		return PromotionReview{Approve: false, Reason: "masih terlalu sempit"}, nil
	})
	t.Cleanup(func() { SetPromotionReviewer(nil) })
	if promote(db) != 0 || calls != 1 {
		t.Fatalf("peninjau menolak → tidak naik (panggilan %d)", calls)
	}
	if promote(db) != 0 || calls != 1 {
		t.Fatal("yang ditolak tidak ditinjau ulang sebelum buktinya bertambah")
	}
	db.Model(&models.KnowledgeNode{}).Where("id = ?", n.ID).UpdateColumn("helpful", 12)
	SetPromotionReviewer(func(*gorm.DB, models.KnowledgeNode, string) (PromotionReview, error) {
		return PromotionReview{Approve: true, Title: "Batasi ukuran body request", Content: "Bungkus body dengan http.MaxBytesReader sebelum decode.", AppliesTo: "go>=1.20"}, nil
	})
	if promote(db) != 1 {
		t.Fatal("bukti bertambah dan disetujui → naik menjadi patokan")
	}
	var p models.KnowledgeNode
	db.Where("source_key = ?", "promoted:"+itoa(n.ID)).First(&p)
	db.First(&n, n.ID)
	if p.Title != "Batasi ukuran body request" || p.AppliesTo != "go>=1.20" || n.SupersededByID == nil || *n.SupersededByID != p.ID {
		t.Fatalf("patokan kanonis & asal digantikan: patokan=%+v asal=%+v", p, n)
	}
	// peninjau gagal → dicoba lagi lain kali, tidak dipromosikan apa adanya
	m := models.KnowledgeNode{Kind: "lesson", Domain: "go", Title: "Lain", Content: "Isi lain.", Source: models.KnowledgeSrcEmployee,
		Key: "employee:10", OwnerEmployeeID: &owner, Weight: 60, Helpful: 9, Status: models.KnowledgeNodeActive}
	db.Create(&m)
	spreadEvidence(t, db, m.ID, [][2]uint{{3, 1}, {4, 2}, {5, 2}})
	SetPromotionReviewer(func(*gorm.DB, models.KnowledgeNode, string) (PromotionReview, error) {
		return PromotionReview{}, errors.New("pool mati")
	})
	if promote(db) != 0 {
		t.Fatal("peninjau tidak tersedia → tidak dipromosikan")
	}
}

func TestSecurityPromotionNeedsMoreEvidence(t *testing.T) {
	owner := uint(1)
	n := models.KnowledgeNode{Domain: "security", OwnerEmployeeID: &owner, Helpful: 9}
	s := promotionSpread{Employees: 3, Projects: 2}
	if promotable(n, s) {
		t.Fatal("security butuh ✓ ≥ 10 dan ≥ 3 proyek")
	}
	n.Helpful, s.Projects = 10, 3
	if !promotable(n, s) {
		t.Fatal("security dengan bukti cukup boleh naik")
	}
	s.DropRate = 0.5
	if promotable(n, s) {
		t.Fatal("terlalu banyak penerima membuangnya")
	}
}

func TestDemoteFlagsWeakPromotedStandard(t *testing.T) {
	db := testDB(t)
	p := models.KnowledgeNode{Kind: "rule", Domain: "go", Title: "Patokan", Content: "Isi.", Source: models.KnowledgeSrcPromoted,
		Key: "promoted:1", Weight: 70, Helpful: 1, Harmful: 3, Status: models.KnowledgeNodeActive}
	db.Create(&p)
	if demote(db, time.Now()) != 1 {
		t.Fatal("patokan yang tidak lagi terbukti ditandai perlu ditinjau")
	}
	db.First(&p, p.ID)
	if !p.NeedsReview || p.Status != models.KnowledgeNodeActive {
		t.Fatalf("ditandai, tidak dipensiunkan: %+v", p)
	}
}

// Pengetahuan pegawai yang isinya tidak menyebut ekosistem memakai bidang dari stack pemiliknya.
func TestEmployeeNoteDomainFallsBackToStack(t *testing.T) {
	db := testDB(t)
	e := models.Employee{Name: "Fani", Role: "mobile_engineer", Status: models.EmployeeActive, Stack: []string{"flutter", "dart", "riverpod"}}
	db.Create(&e)
	db.Create(&models.EmployeeKnowledge{EmployeeID: e.ID, Topic: "Pisahkan state layar", Content: "Simpan state form di satu tempat agar mudah diuji.",
		Kind: "technical", Source: models.KnowledgeAgenda, Weight: 60, Status: models.KnowledgeActive})
	notes, ok := employeeNotes(db, e.ID)
	if !ok || len(notes) != 1 || notes[0].Domain != "flutter" {
		t.Fatalf("domain cadangan dari stack: %+v", notes)
	}
}

func TestProjectSpecificExperienceNotShared(t *testing.T) {
	cases := map[string]bool{
		"Perbaiki kompleksitas di admin_participant.go (L428)":   true,
		"Lihat todo #4331 untuk konteks":                         true,
		"Bungkus body dengan http.MaxBytesReader sebelum decode": false,
		"Pakai index komposit untuk filter + sort":               false,
	}
	for text, want := range cases {
		if got := ProjectSpecific(models.KnowledgeNode{Title: "x", Content: text}); got != want {
			t.Errorf("%q: dapat %v, mau %v", text, got, want)
		}
	}
	db := testDB(t)
	peer := hireForTest(t, db, "Gita")
	owner := uint(70)
	n := models.KnowledgeNode{Kind: "lesson", Domain: "go", Title: "Refactor", Content: "Pecah fungsi di handler.go (L12).", OwnerEmployeeID: &owner, Status: models.KnowledgeNodeActive}
	db.Create(&n)
	db.Create(&models.KnowledgeAdoption{NodeID: n.ID, EmployeeID: peer, Status: models.KnowledgeAdoptionTrial})
	decideTrials(db, time.Now())
	var a models.KnowledgeAdoption
	db.First(&a)
	if a.Status != models.KnowledgeAdoptionDropped {
		t.Fatalf("masa coba butir khusus aplikasi harus ditutup: %+v", a)
	}
}
