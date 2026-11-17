package knowledge

import (
	"slices"
	"testing"
	"time"

	"github.com/vibeasik/vibeasik/pkg/models"
)

func TestCanonicalDomainAliases(t *testing.T) {
	cases := map[string]string{
		"clean-code": "code_quality", "clean_code": "code_quality", "code_quality": "code_quality", "Refactoring": "code_quality",
		"frontend architecture": "architecture", "release-engineering": "devops", "release": "devops", "deployment": "devops",
		"UX": "ui_ux", "ui_ux": "ui_ux", " docs ": "documentation", "documentation": "documentation",
		"frontend": "web", "vue": "web", "mounted": "flutter", "gradle": "android", "other": "observability",
		"marketing": "business", "event-driven-backend": "backend", "go": "go", "security": "security",
		"gleam": "gleam", "": "",
	}
	for in, want := range cases {
		if got := CanonicalDomain(in); got != want {
			t.Errorf("CanonicalDomain(%q) = %q, mau %q", in, got, want)
		}
	}
}

func TestAliasTargetsAreCanonical(t *testing.T) {
	canon := CanonicalDomains()
	for from, to := range domainAliases {
		if domainKey(from) != from {
			t.Errorf("kunci alias %q harus berbentuk baku %q", from, domainKey(from))
		}
		if !KnownDomain(to) || !slices.Contains(canon, to) {
			t.Errorf("alias %q → %q: tujuan bukan domain tetap", from, to)
		}
		if KnownDomain(from) {
			t.Errorf("alias %q menimpa domain tetap", from)
		}
		if CanonicalDomain(to) != to || CanonicalDomain(CanonicalDomain(from)) != to {
			t.Errorf("alias %q tidak idempoten", from)
		}
	}
}

func TestEcosystemOfAliases(t *testing.T) {
	for d, eco := range map[string]bool{
		"frontend": true, "gradle": true, "web": true, "clean-code": false, "docs": false, "documentation": false,
		"code_quality": false, "marketing": false, "gleam": true,
	} {
		if IsEcosystem(d) != eco {
			t.Errorf("IsEcosystem(%q) = %v, mau %v", d, !eco, eco)
		}
	}
	ds := DetectDomains("Perbaiki android/app/build.gradle dan android/app/proguard-rules.pro")
	if !slices.Contains(ds, "android") || slices.Contains(ds, "gradle") || slices.Contains(ds, "pro") {
		t.Fatalf("ekstensi yang punya alias harus dikenali sebagai domain tetapnya, dapat %v", ds)
	}
}

func TestWritePathsUseCanonicalDomain(t *testing.T) {
	db := testDB(t)
	n := mustLearn(t, db, Note{Kind: "rule", Domain: "Clean-Code", Title: "Pecah fungsi panjang", Content: "Fungsi di atas 60 baris dipecah per langkah."})
	if n.Domain != "code_quality" {
		t.Fatalf("node baru harus memakai domain tetap, dapat %q", n.Domain)
	}
	g, err := ReportGap(db, "docs", "Gerbang CI dokumentasi", "")
	if err != nil || g.Domain != "documentation" {
		t.Fatalf("celah baru harus memakai domain tetap, dapat %+v %v", g, err)
	}
}

func TestNormalizeDomainsDryRunApplyIdempotent(t *testing.T) {
	db := testDB(t)
	old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	seed := []models.KnowledgeNode{
		{Kind: "rule", Domain: "clean-code", Title: "A", Content: "isi a", Status: models.KnowledgeNodeActive, Helpful: 3, Weight: 70},
		{Kind: "rule", Domain: "frontend architecture", Title: "B", Content: "isi b", Status: models.KnowledgeNodeActive},
		{Kind: "lesson", Domain: "frontend", Title: "C", Content: "isi c", Status: models.KnowledgeNodeRetired, StatusReason: "usang"},
		{Kind: "lesson", Domain: "go", Title: "D", Content: "isi d", Status: models.KnowledgeNodeActive},
		{Kind: "lesson", Domain: "", Title: "E", Content: "isi e", Status: models.KnowledgeNodeActive},
	}
	for i := range seed {
		seed[i].CreatedAt, seed[i].UpdatedAt = old, old
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatal(err)
	}
	db.Create(&[]models.KnowledgeGap{
		{Domain: "release", Topic: "Rilis Play", Status: models.KnowledgeGapOpen},
		{Domain: "helpers", Topic: "Teknologi baru: helpers", Auto: true, Status: models.KnowledgeGapLearning},
	})
	db.Create(&models.KnowledgeCluster{Label: "vue", Domain: "vue"})

	snapshot := func() []models.KnowledgeNode {
		var out []models.KnowledgeNode
		db.Order("id").Find(&out)
		return out
	}
	orig := snapshot()

	dry, err := NormalizeDomains(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Applied || len(dry.Changes) != 5 {
		t.Fatalf("mode kering harus mendaftar 5 perubahan tanpa menerapkan, dapat %+v", dry.Changes)
	}
	for i, n := range snapshot() {
		if n.Domain != orig[i].Domain {
			t.Fatalf("mode kering menulis domain node #%d", n.ID)
		}
	}
	if len(dry.Unknown) != 1 || dry.Unknown[0].Domain != "helpers" {
		t.Fatalf("label tak dikenal harus dilaporkan, dapat %+v", dry.Unknown)
	}
	if len(dry.Before) != 3 || len(dry.After) != 3 { // clean-code, frontend architecture, go → code_quality, architecture, go
		t.Fatalf("hitungan domain aktif keliru: sebelum %+v sesudah %+v", dry.Before, dry.After)
	}

	rep, err := NormalizeDomains(db, true)
	if err != nil || !rep.Applied {
		t.Fatalf("penerapan gagal: %v", err)
	}
	want := []string{"code_quality", "architecture", "web", "go", ""}
	for i, n := range snapshot() {
		o := orig[i]
		if n.Domain != want[i] {
			t.Errorf("node %s: domain %q, mau %q", n.Title, n.Domain, want[i])
		}
		if n.Title != o.Title || n.Content != o.Content || n.Status != o.Status || n.StatusReason != o.StatusReason ||
			n.Kind != o.Kind || n.Helpful != o.Helpful || n.Weight != o.Weight || !n.UpdatedAt.Equal(o.UpdatedAt) {
			t.Errorf("node %s: hanya domain yang boleh berubah\nlama %+v\nbaru %+v", n.Title, o, n)
		}
	}
	var gaps []models.KnowledgeGap
	db.Order("id").Find(&gaps)
	if gaps[0].Domain != "devops" || gaps[1].Domain != "helpers" || gaps[0].Topic != "Rilis Play" {
		t.Fatalf("celah: %+v", gaps)
	}
	var cl models.KnowledgeCluster
	db.First(&cl)
	if cl.Domain != "web" || cl.Label != "vue" {
		t.Fatalf("klaster: %+v", cl)
	}
	var total int64
	db.Model(&models.KnowledgeNode{}).Count(&total)
	if total != int64(len(seed)) {
		t.Fatalf("tidak boleh ada node yang hilang: %d dari %d", total, len(seed))
	}

	again, err := NormalizeDomains(db, true)
	if err != nil || len(again.Changes) != 0 || again.Applied {
		t.Fatalf("penerapan ulang harus tanpa perubahan, dapat %+v %v", again.Changes, err)
	}
}
