package mcp

import (
	"fmt"
	"strings"
	"testing"
	"time"

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
