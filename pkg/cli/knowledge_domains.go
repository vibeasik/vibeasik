package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeasik/vibeasik/pkg/database"
	"github.com/vibeasik/vibeasik/pkg/knowledge"
)

var (
	domainsApply bool
	domainsJSON  bool
)

var knowledgeDomainsCmd = &cobra.Command{
	Use:   "domains",
	Short: "Satukan label domain pengetahuan ke daftar tetap (bawaan: kering, tanpa menulis)",
	Long: `Menampilkan label domain lama → domain tetap beserta jumlah baris di node, celah, dan klaster.
Tanpa --apply tidak ada yang ditulis. Dengan --apply hanya kolom domain yang diubah; judul, isi, status,
dan bukti node tetap, dan tidak ada baris yang dihapus. Aman dijalankan ulang.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.Open(dbPath) // tanpa migrasi skema: perintah ini hanya membaca, atau mengubah kolom domain
		if err != nil {
			return err
		}
		rep, err := knowledge.NormalizeDomains(db, domainsApply)
		if err != nil {
			return err
		}
		if domainsJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(rep)
		}
		printDomainReport(rep)
		return nil
	},
}

func printDomainReport(rep knowledge.DomainReport) {
	mode := "KERING (tidak ada yang ditulis; pakai --apply untuk menerapkan)"
	if rep.Applied {
		mode = "DITERAPKAN"
	}
	fmt.Printf("Penyatuan domain pengetahuan — %s\n\n", mode)
	if len(rep.Changes) == 0 {
		fmt.Println("Semua label sudah memakai domain tetap.")
	} else {
		fmt.Printf("%-20s %-24s %-16s %7s %7s\n", "TABEL", "DOMAIN LAMA", "DOMAIN BARU", "BARIS", "AKTIF")
		for _, c := range rep.Changes {
			fmt.Printf("%-20s %-24s %-16s %7d %7d\n", c.Table, quoteEmpty(c.From), c.To, c.Rows, c.Active)
		}
	}
	if len(rep.Unknown) > 0 {
		fmt.Println("\nLabel tak dikenal (dibiarkan, bisa jadi ekosistem baru):")
		for _, u := range rep.Unknown {
			fmt.Printf("  %-20s %-24s %d baris\n", u.Table, u.Domain, u.Rows)
		}
	}
	fmt.Printf("\nDomain node aktif: %d sebelum → %d sesudah\n", len(rep.Before), len(rep.After))
	parts := make([]string, 0, len(rep.After))
	small := []string{}
	for _, d := range rep.After {
		parts = append(parts, fmt.Sprintf("%s %d", d.Domain, d.Active))
		if d.Active <= 3 {
			small = append(small, d.Domain)
		}
	}
	fmt.Println("  " + strings.Join(parts, " · "))
	if len(small) > 0 {
		fmt.Printf("Domain dengan ≤ 3 node aktif: %s\n", strings.Join(small, ", "))
	}
}

func quoteEmpty(s string) string {
	if s == "" {
		return `""`
	}
	return s
}

func init() {
	knowledgeDomainsCmd.Flags().BoolVar(&domainsApply, "apply", false, "terapkan perubahan (bawaan hanya menampilkan)")
	knowledgeDomainsCmd.Flags().BoolVar(&domainsJSON, "json", false, "keluaran JSON")
	knowledgeCmd.AddCommand(knowledgeDomainsCmd)
}
