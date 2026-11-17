package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeasik/vibeasik/pkg/database"
	"github.com/vibeasik/vibeasik/pkg/knowledge"
)

var (
	evalK       int
	evalLimit   int
	evalJSON    bool
	evalNow     string
	evalWeights string
	evalDomains int
	evalSplit   string
	evalRank    string
)

var knowledgeEvalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Ukur mutu temu-kembali pengetahuan (recall@k, nDCG@k, MRR) dari riwayat todo; hanya membaca database",
	Long: `Membangun set uji dari riwayat todo: kueri = teks todo seperti saat prompt pegawai dibangun, jawaban benar =
node yang disebut di baris "PENGETAHUAN: K<id>" catatan penyelesaian todo yang lulus QA (tingkat kuat) dan
node yang pemakaiannya bernilai helpful (tingkat lemah). Node yang dibuat setelah todo dibuat tidak dihitung.

Angka hanya bermakna sebagai pembanding antar varian peringkat: label hanya ada untuk node yang pernah
ditampilkan peringkat lama. Database dibuka tanpa migrasi dan dibaca dalam transaksi read-only.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opt := knowledge.EvalOptions{K: evalK, Limit: evalLimit}
		switch evalSplit {
		case "", "semua":
		case knowledge.EvalSplitTrain, knowledge.EvalSplitTest:
			opt.Split = evalSplit
		default:
			return fmt.Errorf("--split harus %s, %s, atau semua", knowledge.EvalSplitTrain, knowledge.EvalSplitTest)
		}
		if evalNow != "" {
			t, err := parseEvalTime(evalNow)
			if err != nil {
				return err
			}
			opt.Now = t
		}
		if evalWeights != "" {
			w, err := parseRankWeights(evalWeights)
			if err != nil {
				return err
			}
			opt.Weights = w
		}
		if evalRank != "" {
			if opt.Weights == (knowledge.RankWeights{}) {
				opt.Weights = knowledge.DefaultRankWeights
			}
			if err := applyRankOptions(&opt.Weights, evalRank); err != nil {
				return err
			}
		}
		db, err := database.Open(dbPath)
		if err != nil {
			return err
		}
		// Transaksi read-only: tidak ada tulis yang lolos, dan semua kueri melihat potret data yang sama.
		tx := db.Begin(&sql.TxOptions{ReadOnly: true})
		if tx.Error != nil {
			tx = db.Begin()
			if tx.Error != nil {
				return tx.Error
			}
		}
		defer tx.Rollback()
		rep, err := knowledge.Evaluate(tx, opt)
		if err != nil {
			return err
		}
		if evalJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(rep)
		}
		printEvalReport(cmd.OutOrStdout(), rep, evalDomains)
		return nil
	},
}

// parseEvalTime menerima tanggal (2006-01-02, waktu lokal 00:00) atau RFC3339.
func parseEvalTime(s string) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("--now harus berformat 2006-01-02 atau RFC3339: %q", s)
	}
	return t, nil
}

// parseRankWeights: "relevansi,utilitas,kepentingan,kebaruan" (mis. 0.55,0.20,0.15,0.10); sisanya bawaan.
func parseRankWeights(s string) (knowledge.RankWeights, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return knowledge.RankWeights{}, fmt.Errorf("--weights butuh 4 angka: relevansi,utilitas,kepentingan,kebaruan")
	}
	var f [4]float64
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || v < 0 {
			return knowledge.RankWeights{}, fmt.Errorf("--weights: %q bukan angka ≥ 0", p)
		}
		f[i] = v
	}
	w := knowledge.DefaultRankWeights
	w.Relevance, w.Utility, w.Importance, w.Recency = f[0], f[1], f[2], f[3]
	return w, nil
}

// applyRankOptions menerapkan "kunci=nilai,…" (nama JSON RankWeights, mis. lexical=bm25,domain_bonus=0.2)
// pada bobot. Kunci yang tidak dikenal ditolak.
func applyRankOptions(w *knowledge.RankWeights, s string) error {
	for _, kv := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok {
			return fmt.Errorf("--rank: %q bukan kunci=nilai", kv)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "lexical" {
			switch v {
			case knowledge.LexicalTFIDF, knowledge.LexicalBM25, knowledge.LexicalRRF:
				w.Lexical = v
				continue
			}
			return fmt.Errorf("--rank: lexical %q tidak dikenal (tfidf, bm25, rrf)", v)
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("--rank: %s=%q bukan angka", k, v)
		}
		ints := map[string]*int{"feedback": &w.Feedback, "feedback_terms": &w.FeedbackTerms, "max_linked": &w.MaxLinked, "max_hits": &w.MaxHits}
		floats := map[string]*float64{
			"relevance": &w.Relevance, "utility": &w.Utility, "importance": &w.Importance, "recency": &w.Recency,
			"recency_days": &w.RecencyDays, "domain_bonus": &w.DomainBonus, "min_relevance": &w.MinRelevance,
			"feedback_weight": &w.FeedbackWeight, "tail_ratio": &w.TailRatio, "min_score": &w.MinScore,
		}
		switch {
		case ints[k] != nil:
			*ints[k] = int(f)
		case floats[k] != nil:
			*floats[k] = f
		default:
			return fmt.Errorf("--rank: kunci %q tidak dikenal", k)
		}
	}
	return nil
}

func printEvalReport(out io.Writer, rep *knowledge.EvalReport, domains int) {
	w := rep.Weights
	split := rep.Split
	if split == "" {
		split = "semua"
	}
	fmt.Fprintf(out, "Evaluasi temu-kembali (k=%d, daftar %d, acuan %s, bagian %s)\n", rep.K, rep.Limit, rep.Now.Format(time.RFC3339), split)
	fmt.Fprintf(out, "Node aktif %d · todo diperingkat %d · bobot relevansi %.2f utilitas %.2f kepentingan %.2f kebaruan %.2f\n",
		rep.Nodes, rep.Todos, w.Relevance, w.Utility, w.Importance, w.Recency)
	if b, err := json.Marshal(w); err == nil {
		fmt.Fprintf(out, "Peringkat %s\n\n", b)
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(tw, "tingkat\tkueri\tlabel\trecall@%d\tnDCG@%d\tMRR@%d\tnDCG@%d terkond.\tbelum dinilai\trecall daftar\tnode/kueri\ttautan\tbocor\ttak aktif\t\n", rep.K, rep.K, rep.K, rep.K)
	for _, t := range rep.Tiers {
		m := t.Total
		fmt.Fprintf(tw, "%s\t%d\t%d\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\t%.2f\t%.2f\t%d\t%d\t\n", t.Name, m.Queries, m.Labels, m.Recall, m.NDCG, m.MRR,
			m.CondNDCG, m.Unjudged, m.RecallList, m.Delivered, m.Linked, t.DroppedLeak, t.DroppedInactive)
	}
	tw.Flush()
	if domains <= 0 {
		return
	}
	for _, t := range rep.Tiers {
		fmt.Fprintf(out, "\nPer domain (tingkat %s, %d teratas menurut jumlah kueri):\n", t.Name, domains)
		tw = tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
		fmt.Fprintf(tw, "domain\tkueri\tlabel\trecall@%d\tnDCG@%d\tMRR@%d\t\n", rep.K, rep.K, rep.K)
		for i, d := range t.ByDomain {
			if i >= domains {
				break
			}
			fmt.Fprintf(tw, "%s\t%d\t%d\t%.4f\t%.4f\t%.4f\t\n", d.Domain, d.Queries, d.Labels, d.Recall, d.NDCG, d.MRR)
		}
		tw.Flush()
	}
}

func init() {
	f := knowledgeEvalCmd.Flags()
	f.IntVar(&evalK, "k", 10, "Metrik dihitung pada k butir teratas")
	f.IntVar(&evalLimit, "limit", 0, "Panjang daftar yang diperingkat (bawaan sama dengan prompt pegawai: 15)")
	f.BoolVar(&evalJSON, "json", false, "Keluaran JSON lengkap (termasuk per domain)")
	f.StringVar(&evalNow, "now", "", "Acuan waktu & batas data, 2006-01-02 atau RFC3339 (bawaan awal hari ini)")
	f.StringVar(&evalWeights, "weights", "", "Varian bobot relevansi,utilitas,kepentingan,kebaruan (bawaan bobot produksi)")
	f.IntVar(&evalDomains, "domains", 10, "Jumlah domain yang ditampilkan per tingkat (0 = tanpa rincian)")
	f.StringVar(&evalSplit, "split", "", "Bagian todo: latih (±70 %), uji (±30 %), atau semua (bawaan)")
	f.StringVar(&evalRank, "rank", "", "Varian peringkat kunci=nilai,… mis. lexical=bm25,domain_bonus=0.3,tail_ratio=0.8")
	knowledgeCmd.AddCommand(knowledgeEvalCmd)
}
