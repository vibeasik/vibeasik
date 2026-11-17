package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/database"
	"github.com/vibeasik/vibeasik/pkg/knowledge"
	"github.com/vibeasik/vibeasik/pkg/models"
)

var (
	supersedeReason string
	supersedeUndo   bool
	validityApplies string
	validityUntil   string
)

var knowledgeSupersedeCmd = &cobra.Command{
	Use:   "supersede <lama> [baru]",
	Short: "Tandai node digantikan node lain: node lama tetap tersimpan, tidak lagi dikirim temu-kembali",
	Long: `Node lama tetap aktif dan isinya tidak diubah; temu-kembali (dan evaluasi) melewatinya selama penggantinya
aktif. Bila pengganti sendiri sudah digantikan, penunjuk diarahkan ke ujung rantai; rantai melingkar ditolak.
ID boleh ditulis 12 atau K12. --undo membatalkan penggantian node lama.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldID, err := parseNodeID(args[0])
		if err != nil {
			return err
		}
		db, err := openKnowledgeForWrite()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if supersedeUndo {
			if len(args) != 1 {
				return fmt.Errorf("--undo hanya menerima node lama")
			}
			if err := knowledge.Restore(db, oldID); err != nil {
				return err
			}
			fmt.Fprintf(out, "K%d tidak lagi digantikan dan kembali dikirim temu-kembali.\n", oldID)
			return nil
		}
		if len(args) != 2 {
			return fmt.Errorf("sebutkan node lama dan node pengganti")
		}
		newID, err := parseNodeID(args[1])
		if err != nil {
			return err
		}
		head, err := knowledge.Supersede(db, oldID, newID, supersedeReason)
		if err != nil {
			return err
		}
		if head != newID {
			fmt.Fprintf(out, "K%d sudah digantikan; diarahkan ke ujung rantai K%d.\n", newID, head)
		}
		fmt.Fprintf(out, "K%d digantikan K%d (tetap tersimpan; batalkan dengan --undo).\n", oldID, head)
		return nil
	},
}

var knowledgeValidityCmd = &cobra.Command{
	Use:   "validity <id>",
	Short: "Lihat atau ubah masa berlaku node (berlaku-untuk, berlaku-sampai)",
	Long: `Tanpa opsi: menampilkan masa berlaku node. --applies-to menulis keterangan berlaku-untuk (mis. "go>=1.26",
kosong = hapus). --until menulis batas berlaku (YYYY-MM-DD, atau "none" = tanpa batas); sesudahnya node tidak
dikirim temu-kembali dan kurator menandainya perlu ditinjau. Isi node tidak diubah.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseNodeID(args[0])
		if err != nil {
			return err
		}
		db, err := openKnowledgeForWrite()
		if err != nil {
			return err
		}
		var n models.KnowledgeNode
		if db.Where("id = ?", id).Limit(1).Find(&n).Error != nil || n.ID == 0 {
			return fmt.Errorf("node %d tidak ditemukan", id)
		}
		f := cmd.Flags()
		if f.Changed("applies-to") || f.Changed("until") {
			applies, until := n.AppliesTo, n.ValidUntil
			if f.Changed("applies-to") {
				applies = validityApplies
			}
			if f.Changed("until") {
				if until, err = parseUntil(validityUntil); err != nil {
					return err
				}
			}
			if err := knowledge.SetValidity(db, id, applies, until); err != nil {
				return err
			}
			db.Where("id = ?", id).Limit(1).Find(&n)
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "K%d %s [%s]\n", n.ID, n.Title, n.Status)
		fmt.Fprintf(out, "  berlaku-untuk: %s\n", orDash(n.AppliesTo))
		until := "-"
		if n.ValidUntil != nil {
			until = n.ValidUntil.Format("2006-01-02")
			if n.Expired(time.Now()) {
				until += " (sudah lewat)"
			}
		}
		fmt.Fprintf(out, "  berlaku-sampai: %s\n", until)
		by := "-"
		if n.SupersededByID != nil {
			by = "K" + strconv.FormatUint(uint64(*n.SupersededByID), 10)
		}
		fmt.Fprintf(out, "  digantikan-oleh: %s\n", by)
		return nil
	},
}

// openKnowledgeForWrite membuka database tanpa migrasi penuh, lalu menambahkan kolom node yang belum ada
// (migrasi menambah saja) agar perintah tulis jalan di database yang belum dibuka engine versi ini.
func openKnowledgeForWrite() (*gorm.DB, error) {
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&models.KnowledgeNode{}); err != nil {
		return nil, err
	}
	return db, nil
}

func parseNodeID(s string) (uint, error) {
	v, err := strconv.ParseUint(strings.TrimLeft(strings.TrimSpace(s), "Kk"), 10, 32)
	if err != nil || v == 0 {
		return 0, fmt.Errorf("id node tidak sah: %q (contoh: 12 atau K12)", s)
	}
	return uint(v), nil
}

func parseUntil(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "none") {
		return nil, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return nil, fmt.Errorf("--until harus YYYY-MM-DD atau none: %w", err)
	}
	return &t, nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func init() {
	f := knowledgeSupersedeCmd.Flags()
	f.StringVar(&supersedeReason, "reason", "", "Alasan penggantian (disimpan di status_reason node lama)")
	f.BoolVar(&supersedeUndo, "undo", false, "Batalkan penggantian node lama")
	v := knowledgeValidityCmd.Flags()
	v.StringVar(&validityApplies, "applies-to", "", `Keterangan berlaku-untuk, mis. "go>=1.26" (kosong = hapus)`)
	v.StringVar(&validityUntil, "until", "", `Batas berlaku YYYY-MM-DD, atau "none"`)
	knowledgeCmd.AddCommand(knowledgeSupersedeCmd, knowledgeValidityCmd)
}
