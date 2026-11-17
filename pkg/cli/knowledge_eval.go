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

func printEvalReport(out io.Writer, rep *knowledge.EvalReport, domains int) {
	w := rep.Weights
	fmt.Fprintf(out, "Evaluasi temu-kembali (k=%d, daftar %d, acuan %s)\n", rep.K, rep.Limit, rep.Now.Format(time.RFC3339))
	fmt.Fprintf(out, "Node aktif %d · todo diperingkat %d · bobot relevansi %.2f utilitas %.2f kepentingan %.2f kebaruan %.2f\n\n",
		rep.Nodes, rep.Todos, w.Relevance, w.Utility, w.Importance, w.Recency)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(tw, "tingkat\tkueri\tlabel\trecall@%d\tnDCG@%d\tMRR@%d\tbocor\ttak aktif\t\n", rep.K, rep.K, rep.K)
	for _, t := range rep.Tiers {
		m := t.Total
		fmt.Fprintf(tw, "%s\t%d\t%d\t%.4f\t%.4f\t%.4f\t%d\t%d\t\n", t.Name, m.Queries, m.Labels, m.Recall, m.NDCG, m.MRR, t.DroppedLeak, t.DroppedInactive)
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
	knowledgeCmd.AddCommand(knowledgeEvalCmd)
}
