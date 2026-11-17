package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/vibeasik/vibeasik/pkg/knowledge"
)

func TestParseRankWeights(t *testing.T) {
	w, err := parseRankWeights("0.6, 0.2,0.1,0.1")
	if err != nil {
		t.Fatal(err)
	}
	if w.Relevance != 0.6 || w.Importance != 0.1 || w.RecencyDays != knowledge.DefaultRankWeights.RecencyDays || w.DomainBonus != knowledge.DefaultRankWeights.DomainBonus {
		t.Fatalf("bobot = %+v", w)
	}
	for _, bad := range []string{"0.5,0.5", "a,b,c,d", "0.5,-1,0,0"} {
		if _, err := parseRankWeights(bad); err == nil {
			t.Errorf("%q harus ditolak", bad)
		}
	}
}

func TestParseEvalTime(t *testing.T) {
	d, err := parseEvalTime("2026-11-18")
	if err != nil || d.Hour() != 0 || d.Day() != 18 || d.Location() != time.Local {
		t.Fatalf("tanggal = %v, %v", d, err)
	}
	if _, err := parseEvalTime("2026-11-18T10:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseEvalTime("kemarin"); err == nil {
		t.Fatal("format salah harus ditolak")
	}
}

func TestPrintEvalReport(t *testing.T) {
	rep := &knowledge.EvalReport{K: 10, Limit: 15, Weights: knowledge.DefaultRankWeights, Nodes: 3, Todos: 2, Tiers: []knowledge.EvalTier{{
		Name:     knowledge.EvalTierStrong,
		Total:    knowledge.EvalMetrics{Queries: 2, Labels: 3, Recall: 0.5, NDCG: 0.25, MRR: 1},
		ByDomain: []knowledge.DomainMetrics{{Domain: "go", EvalMetrics: knowledge.EvalMetrics{Queries: 2, Labels: 3, Recall: 0.5}}},
	}}}
	var b bytes.Buffer
	printEvalReport(&b, rep, 5)
	out := b.String()
	for _, want := range []string{"recall@10", "kuat", "0.5000", "0.2500", "Per domain", " go "} {
		if !strings.Contains(out, want) {
			t.Errorf("keluaran tidak memuat %q:\n%s", want, out)
		}
	}
}

func TestApplyRankOptions(t *testing.T) {
	w := knowledge.DefaultRankWeights
	if err := applyRankOptions(&w, "lexical=bm25, domain_bonus=0.3,feedback=3,tail_ratio=0.8,max_hits=10"); err != nil {
		t.Fatal(err)
	}
	if w.Lexical != knowledge.LexicalBM25 || w.DomainBonus != 0.3 || w.Feedback != 3 || w.TailRatio != 0.8 || w.MaxHits != 10 ||
		w.Relevance != knowledge.DefaultRankWeights.Relevance {
		t.Fatalf("bobot = %+v", w)
	}
	for _, bad := range []string{"lexical=dense", "domain_bonus", "nope=1", "feedback=x"} {
		if err := applyRankOptions(&w, bad); err == nil {
			t.Errorf("%q harus ditolak", bad)
		}
	}
}
