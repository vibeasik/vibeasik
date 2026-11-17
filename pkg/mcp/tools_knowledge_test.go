package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vibeasik/vibeasik/pkg/knowledge"
	"github.com/vibeasik/vibeasik/pkg/models"
)

func TestKnowledgeLearnSupersedesAndAppliesTo(t *testing.T) {
	s := newCharServer(t)
	learn := func(args map[string]interface{}) (string, error) {
		res, handled, err := s.callKnowledgeTool("vibeasik_knowledge_learn", args)
		if !handled {
			t.Fatal("tool learn harus ditangani")
		}
		if err != nil {
			return "", err
		}
		return res.Content[0].Text, nil
	}
	if _, err := learn(map[string]interface{}{"title": "Iterator range-over-func", "content": "Fungsi iterator dipanggil lewat for range sejak Go 1.23.", "domain": "go"}); err != nil {
		t.Fatal(err)
	}
	var old models.KnowledgeNode
	s.db.Order("id DESC").First(&old)

	until := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	out, err := learn(map[string]interface{}{
		"title": "Iterator iter.Seq", "content": "Pakai iter.Seq dan iter.Seq2 dari paket iter untuk iterator yang bisa di-range.",
		"domain": "go", "applies_to": "go>=1.23", "valid_until": until, "supersedes": fmt.Sprintf("K%d", old.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"applies_to": "go\u003e=1.23"`) || strings.Contains(out, "supersede_error") {
		t.Fatalf("hasil learn harus memuat applies_to tanpa galat penggantian:\n%s", out)
	}
	s.db.First(&old, old.ID)
	if old.SupersededByID == nil || old.Status != models.KnowledgeNodeActive {
		t.Fatalf("node lama tetap aktif dan ditandai digantikan: %+v", old)
	}
	res, _, err := s.callKnowledgeTool("vibeasik_knowledge_recall", map[string]interface{}{"query": "iterator range Go", "domains": "go"})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].Text
	if strings.Contains(text, fmt.Sprintf(`"id": %d,`, old.ID)) || !strings.Contains(text, `"applies_to": "go\u003e=1.23"`) {
		t.Fatalf("recall melewati node yang digantikan dan menampilkan applies_to:\n%s", text)
	}

	if _, err := learn(map[string]interface{}{"title": "a", "content": "b", "supersedes": "lama"}); err == nil {
		t.Fatal("supersedes yang bukan id harus ditolak")
	}
	if _, err := learn(map[string]interface{}{"title": "a", "content": "b", "valid_until": "2000-01-01"}); err == nil {
		t.Fatal("valid_until yang sudah lewat harus ditolak")
	}
}

// Lewat MCP, agent tidak boleh menyembunyikan patokan bersama: node kurasi tetap dikirim temu-kembali dan hanya
// ditandai perlu ditinjau dengan usulan penggantinya; catatan agent & hasil belajar celah boleh langsung diganti.
func TestKnowledgeLearnSupersedeGuardsCuratedNodes(t *testing.T) {
	s := newCharServer(t)
	learn := func(args map[string]interface{}) map[string]interface{} {
		t.Helper()
		res, _, err := s.callKnowledgeTool("vibeasik_knowledge_learn", args)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]interface{}
		if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	seed := func(source, title, content string, owner *uint) models.KnowledgeNode {
		t.Helper()
		n := models.KnowledgeNode{Kind: models.KnowledgeKindRule, Domain: "go", Title: title, Content: content,
			Source: source, OwnerEmployeeID: owner, Weight: 70, Helpful: 9, Status: models.KnowledgeNodeActive}
		if err := s.db.Create(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	curated := []string{models.KnowledgeSrcGuideline, models.KnowledgeSrcChecklist, models.KnowledgeSrcPromoted,
		models.KnowledgeSrcReview, models.KnowledgeSrcVerdict, models.KnowledgeSrcDesign, models.KnowledgeSrcTheory, "playbook"}
	for i, src := range curated {
		old := seed(src, fmt.Sprintf("Patokan %d konteks database", i), fmt.Sprintf("Selalu teruskan context ke kueri database, butir %d.", i), nil)
		out := learn(map[string]interface{}{
			"title": fmt.Sprintf("Pengganti %d untuk patokan", i), "content": fmt.Sprintf("Abaikan context pada kueri, cukup timeout global %d.", i),
			"domain": "go", "supersedes": float64(old.ID),
		})
		if out["supersede_pending_review"] != true || !strings.Contains(fmt.Sprint(out["supersede_note"]), "NOT replaced") {
			t.Fatalf("%s: jawaban tool harus menyatakan penggantian menunggu tinjauan: %v", src, out)
		}
		repl := uint(out["node"].(map[string]interface{})["id"].(float64))
		var got models.KnowledgeNode
		s.db.First(&got, old.ID)
		if got.SupersededByID != nil || got.Status != models.KnowledgeNodeActive || !got.NeedsReview ||
			!strings.Contains(got.StatusReason, fmt.Sprintf("K%d", repl)) {
			t.Fatalf("%s: node lama tidak boleh disembunyikan, harus ditandai perlu ditinjau dengan usulan K%d: %+v", src, repl, got)
		}
		var stored models.KnowledgeNode
		if s.db.First(&stored, repl).Error != nil || stored.Source != models.KnowledgeSrcAgent {
			t.Fatalf("%s: node baru tetap disimpan sebagai catatan agent: %+v", src, stored)
		}
	}
	// Tanda usulan bertahan melewati kurasi walau bukti node lamanya baik.
	knowledge.Curate(s.db)
	var flagged int64
	s.db.Model(&models.KnowledgeNode{}).Where("source = ? AND needs_review = ?", models.KnowledgeSrcGuideline, true).Count(&flagged)
	if flagged != 1 {
		t.Fatalf("kurasi tidak boleh melepas tanda usulan penggantian, tersisa %d", flagged)
	}
	// Jalur pemilik (CLI knowledge supersede) tetap bisa menerapkan usulan itu.
	var guide, proposal models.KnowledgeNode
	s.db.Where("source = ?", models.KnowledgeSrcGuideline).First(&guide)
	s.db.Where("title = ?", "Pengganti 0 untuk patokan").First(&proposal)
	if _, err := knowledge.Supersede(s.db, guide.ID, proposal.ID, "ditinjau pemilik"); err != nil {
		t.Fatalf("penggantian oleh pemilik harus tetap jalan: %v", err)
	}

	for i, src := range []string{models.KnowledgeSrcAgent, models.KnowledgeSrcGap} {
		old := seed(src, fmt.Sprintf("Catatan lama %d build Gleam", i), fmt.Sprintf("Gleam build memakai perintah lama versi %d.", i), nil)
		out := learn(map[string]interface{}{
			"title": fmt.Sprintf("Catatan baru %d build Gleam", i), "content": fmt.Sprintf("Sejak rilis baru %d, gleam build memakai target bawaan erlang.", i),
			"domain": "gleam", "supersedes": fmt.Sprintf("K%d", old.ID),
		})
		if _, pending := out["supersede_pending_review"]; pending || out["supersede_error"] != nil {
			t.Fatalf("%s: catatan agent boleh langsung digantikan: %v", src, out)
		}
		var got models.KnowledgeNode
		s.db.First(&got, old.ID)
		if got.SupersededByID == nil || got.NeedsReview {
			t.Fatalf("%s: node lama harus ditandai digantikan tanpa tinjauan: %+v", src, got)
		}
	}
}
