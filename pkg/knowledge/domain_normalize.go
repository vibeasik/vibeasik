package knowledge

import (
	"errors"
	"sort"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Penyatuan label domain baris lama. Hanya kolom domain yang diubah (node, celah, klaster); judul, isi, status,
// bukti, dan updated_at tidak disentuh, dan tidak ada baris yang dihapus. Idempoten: setelah diterapkan,
// jalankan ulang tidak menghasilkan perubahan.

// DomainChange: satu label lama di satu tabel beserta label tetapnya.
type DomainChange struct {
	Table  string `json:"table"`
	From   string `json:"from"`
	To     string `json:"to"`
	Rows   int64  `json:"rows"`
	Active int64  `json:"active"` // baris berstatus aktif (node) atau terbuka/dipelajari (celah)
}

// DomainCount: jumlah baris sebuah label.
type DomainCount struct {
	Table  string `json:"table"`
	Domain string `json:"domain"`
	Rows   int64  `json:"rows"`
	Active int64  `json:"active"`
}

// DomainReport: hasil penyatuan (kering atau diterapkan).
type DomainReport struct {
	Applied bool           `json:"applied"`
	Changes []DomainChange `json:"changes"`
	// Unknown: label yang bukan domain tetap dan tidak punya alias — dibiarkan (bisa jadi ekosistem baru).
	Unknown []DomainCount `json:"unknown"`
	// Node aktif per domain sebelum & sesudah penyatuan (label kosong tidak dihitung sebagai domain).
	Before []DomainCount `json:"before"`
	After  []DomainCount `json:"after"`
}

type domainTable struct {
	name   string
	model  any
	active string // ekspresi SQL penanda baris aktif
}

var domainTables = []domainTable{
	{"knowledge_nodes", &models.KnowledgeNode{}, "CASE WHEN status = '" + models.KnowledgeNodeActive + "' THEN 1 ELSE 0 END"},
	{"knowledge_gaps", &models.KnowledgeGap{}, "CASE WHEN status IN ('" + models.KnowledgeGapOpen + "','" + models.KnowledgeGapLearning + "') THEN 1 ELSE 0 END"},
	{"knowledge_clusters", &models.KnowledgeCluster{}, "1"},
}

// NormalizeDomains menyusun tabel label lama → label tetap untuk node, celah, dan klaster. Dengan apply=false
// tidak ada yang ditulis; dengan apply=true perubahan dijalankan dalam satu transaksi.
func NormalizeDomains(db *gorm.DB, apply bool) (DomainReport, error) {
	var rep DomainReport
	if db == nil {
		return rep, errors.New("database belum siap")
	}
	before, after := map[string]int64{}, map[string]int64{}
	for _, t := range domainTables {
		var rows []struct {
			Domain string
			N      int64
			Active int64
		}
		if err := db.Model(t.model).Select("domain, COUNT(*) AS n, SUM(" + t.active + ") AS active").
			Group("domain").Scan(&rows).Error; err != nil {
			return rep, err
		}
		for _, r := range rows {
			to := CanonicalDomain(r.Domain)
			if t.name == "knowledge_nodes" && r.Active > 0 {
				if r.Domain != "" {
					before[r.Domain] += r.Active
				}
				if to != "" {
					after[to] += r.Active
				}
			}
			if to != r.Domain {
				rep.Changes = append(rep.Changes, DomainChange{Table: t.name, From: r.Domain, To: to, Rows: r.N, Active: r.Active})
			}
			if to != "" && !KnownDomain(to) {
				rep.Unknown = append(rep.Unknown, DomainCount{Table: t.name, Domain: to, Rows: r.N, Active: r.Active})
			}
		}
	}
	rep.Before, rep.After = sortedCounts(before), sortedCounts(after)
	if !apply || len(rep.Changes) == 0 {
		return rep, nil
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, c := range rep.Changes {
			for _, t := range domainTables {
				if t.name != c.Table {
					continue
				}
				// UpdateColumn: hanya kolom domain, tanpa hook dan tanpa menyentuh updated_at.
				if err := tx.Model(t.model).Where("domain = ?", c.From).UpdateColumn("domain", c.To).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return rep, err
	}
	rep.Applied = true
	invalidate()
	return rep, nil
}

func sortedCounts(m map[string]int64) []DomainCount {
	out := make([]DomainCount, 0, len(m))
	for d, n := range m {
		out = append(out, DomainCount{Table: "knowledge_nodes", Domain: d, Rows: n, Active: n})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Active > out[j].Active || out[i].Active == out[j].Active && out[i].Domain < out[j].Domain
	})
	return out
}
