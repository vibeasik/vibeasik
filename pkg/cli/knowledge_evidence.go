package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/database"
	"github.com/vibeasik/vibeasik/pkg/knowledge"
)

var (
	evidenceApply bool
	evidenceJSON  bool
	evidenceTop   int
)

var knowledgeEvidenceCmd = &cobra.Command{
	Use:   "evidence",
	Short: "Pilah ulang bukti ✓/✗ lama menjadi bukti kuat & lemah (bawaan: kering, tanpa menulis)",
	Long: `Pemakaian node yang dinilai sebelum asal buktinya dicatat dipilah ulang: ✓ di todo yang menyebut node itu
di baris "PENGETAHUAN: K<id>" catatan penyelesaiannya tetap bukti kuat; ✓ lain dan semua ✗ dipindah ke
penghitung bukti lemah, yang hanya menggeser peringkat dan tidak dipakai untuk promosi atau pensiun.

Ringkasan menampilkan cakupan riwayat (catatan pemakaian punya masa simpan), sebaran perubahan utilitas,
node hasil promosi yang tidak lagi memenuhi syarat, dan node yang kini terkena ambang pensiun.
Tanpa --apply database dibaca dalam transaksi read-only. Dengan --apply kolom baru dimigrasikan (menambah saja)
lalu penghitung node dan asal bukti pemakaian ditulis dalam satu transaksi; judul dan isi node tidak disentuh.
Aman dijalankan ulang: pemakaian yang sudah dipilah dilewati.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.Open(dbPath)
		if err != nil {
			return err
		}
		var rep *knowledge.EvidenceReport
		if evidenceApply {
			rep, err = knowledge.RecountEvidence(db, true, evidenceTop)
		} else {
			rep, err = recountReadOnly(db)
		}
		if err != nil {
			return err
		}
		if evidenceJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(rep)
		}
		printEvidenceReport(cmd.OutOrStdout(), rep)
		return nil
	},
}

func recountReadOnly(db *gorm.DB) (*knowledge.EvidenceReport, error) {
	tx := db.Begin(&sql.TxOptions{ReadOnly: true})
	if tx.Error != nil {
		tx = db.Begin()
		if tx.Error != nil {
			return nil, tx.Error
		}
	}
	defer tx.Rollback()
	return knowledge.RecountEvidence(tx, false, evidenceTop)
}

func printEvidenceReport(w io.Writer, rep *knowledge.EvidenceReport) {
	mode := "KERING (tidak ada yang ditulis; pakai --apply untuk menerapkan)"
	if rep.Applied {
		mode = "DITERAPKAN"
	}
	c := rep.Coverage
	fmt.Fprintf(w, "Pilah ulang bukti pengetahuan — %s\n", mode)
	fmt.Fprintf(w, "Bobot bukti lemah di utilitas: %.2f\n\n", rep.Weight)
	fmt.Fprintf(w, "Cakupan (masa simpan pemakaian %d hari", c.RetentionDays)
	if c.OldestUse != nil {
		fmt.Fprintf(w, ", pemakaian tertua %s", c.OldestUse.Format("2006-01-02"))
	}
	fmt.Fprintln(w, "):")
	fmt.Fprintf(w, "  pemakaian bernilai %d, belum punya asal bukti %d, di %d konteks (%d todo, %d todo mengutip K<id>)\n",
		c.Rated, c.Legacy, c.Contexts, c.TodoContexts, c.TodosCiting)
	fmt.Fprintf(w, "  node aktif berbukti kuat %d, %d di antaranya punya pemakaian di masa simpan\n", c.NodesWithEvidence, c.NodesCovered)
	fmt.Fprintf(w, "  ✓ kuat node aktif %d, terjelaskan pemakaian %d · ✗ kuat %d, terjelaskan %d\n",
		c.HelpfulTotal, c.HelpfulLegacy, c.HarmfulTotal, c.HarmfulLegacy)
	if c.MissingColumns {
		fmt.Fprintln(w, "  skema belum punya kolom bukti lemah/asal bukti: dihitung seolah kosong (engine versi baru atau --apply memigrasikannya)")
	}
	r := rep.Rows
	fmt.Fprintf(w, "\nPemakaian: tetap kuat (dikutip) %d · ✓ dipindah ke lemah %d · ✗ dipindah ke lemah %d\n", r.KeptCited, r.MovedHelpful, r.MovedHarmful)
	fmt.Fprintf(w, "  tidak dipindah karena penghitung kuat sudah 0: ✓ %d, ✗ %d · sumber berkedudukan tinggi %d · lewat peleburan %d · node hilang %d\n",
		r.ClampedHelpful, r.ClampedHarmful, r.Authoritative, r.ViaMerge, r.MissingNode)
	fmt.Fprintf(w, "\nNode aktif berubah penghitungnya: %d (utilitas bergeser ≥ 0.001: %d) · rata-rata Δ %+.3f · rata-rata |Δ| %.3f\n",
		rep.NodesTouched, rep.UtilityChanged, rep.MeanDelta, rep.MeanAbsDelta)
	parts := make([]string, 0, len(rep.Distribution))
	for _, b := range rep.Distribution {
		if b.Nodes > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", b.Range, b.Nodes))
		}
	}
	if len(parts) > 0 {
		fmt.Fprintln(w, "  sebaran Δ: "+strings.Join(parts, " · "))
	}
	fmt.Fprintf(w, "\nPromosi: node pribadi memenuhi syarat %d → %d; node hasil promosi aktif %d, tidak lagi memenuhi syarat %d (sudah tidak memenuhi sebelumnya %d)\n",
		rep.EligibleBefore, rep.EligibleAfter, rep.Promoted, rep.PromotedIneligible, rep.PromotedAlreadyOff)
	fmt.Fprintf(w, "Pensiun: terkena ambang pensiun %d → %d (baru %d) · perlu ditinjau %d → %d (baru %d)\n",
		rep.RetireBefore, rep.RetireAfter, rep.RetireNew, rep.FlagBefore, rep.FlagAfter, rep.FlagNew)
	if len(rep.Top) == 0 {
		return
	}
	fmt.Fprintln(w, "\nPerubahan terbesar:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NODE\tSUMBER\t✓\t✗\t✓ LEMAH\t✗ LEMAH\tUTILITAS\tJUDUL\t")
	for _, ch := range rep.Top {
		fmt.Fprintf(tw, "K%d\t%s\t%d→%d\t%d→%d\t%d→%d\t%d→%d\t%.3f→%.3f\t%s\t\n", ch.ID, ch.Source,
			ch.Helpful[0], ch.Helpful[1], ch.Harmful[0], ch.Harmful[1], ch.WeakHelpful[0], ch.WeakHelpful[1],
			ch.WeakHarmful[0], ch.WeakHarmful[1], ch.Utility[0], ch.Utility[1], ch.Title)
	}
	tw.Flush()
}

func init() {
	f := knowledgeEvidenceCmd.Flags()
	f.BoolVar(&evidenceApply, "apply", false, "Tulis hasil pilah ulang ke database (bawaan: kering)")
	f.BoolVar(&evidenceJSON, "json", false, "Keluaran JSON lengkap")
	f.IntVar(&evidenceTop, "top", 15, "Jumlah perubahan terbesar yang ditampilkan")
	knowledgeCmd.AddCommand(knowledgeEvidenceCmd)
}
