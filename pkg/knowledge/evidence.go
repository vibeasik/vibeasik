package knowledge

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Hitung ulang kekuatan bukti: sebelum pemakaian mencatat asal buktinya, Reward memberi ✓/✗ yang sama ke semua
// node yang disisipkan ke sebuah pekerjaan, dan semuanya masuk penghitung helpful/harmful (bukti kuat).
// RecountEvidence membaca ulang pemakaian lama yang bernilai (evidence kosong) dan memilahnya:
//
//   - ✓ di todo yang node-nya dikutip di baris "PENGETAHUAN: K<id>" catatan penyelesaiannya → tetap kuat (cited);
//   - ✓ lain dan semua ✗ → dipindah dari penghitung kuat ke penghitung lemah (outcome).
//
// Pemakaian di node yang sudah dilebur dihitung di node tujuannya, karena peleburan menjumlahkan penghitungnya.
// Penghitung kuat tidak pernah dikurangi sampai negatif. Vonis per node lama (Judge) tidak tercatat terpisah dari
// pembagian rata, jadi ikut dianggap lemah bila tidak dikutip; ✓/✗ yang tidak berasal dari pemakaian (umpan balik,
// vonis audit, penguatan saat Learn, Judge atas node yang tidak disisipkan) tidak tersentuh. Catatan pemakaian
// dibuang setelah useRetention, jadi bukti yang lebih tua dari itu tidak bisa dipilah lagi.
//
// Tanpa apply tidak ada yang ditulis. Dengan apply, kolom baru dimigrasikan (menambah saja) lalu penghitung node
// dan asal bukti pemakaian ditulis dalam satu transaksi; pemakaian yang sudah dipilah tidak dihitung dua kali.

// EvidenceCoverage: seberapa banyak riwayat yang bisa dipilah.
type EvidenceCoverage struct {
	RetentionDays int        `json:"retention_days"`
	OldestUse     *time.Time `json:"oldest_use,omitempty"`
	Rated         int        `json:"rated_uses"`  // pemakaian bernilai helpful/harmful
	Legacy        int        `json:"legacy_uses"` // di antaranya yang belum punya asal bukti (dipilah sekarang)
	Contexts      int        `json:"contexts"`
	TodoContexts  int        `json:"todo_contexts"`
	TodosCiting   int        `json:"todos_citing"` // todo dengan baris PENGETAHUAN berisi K<id>
	// Node aktif yang punya bukti kuat, dan berapa di antaranya punya minimal satu pemakaian lama di masa simpan.
	NodesWithEvidence int `json:"nodes_with_evidence"`
	NodesCovered      int `json:"nodes_covered"`
	// Bagian penghitung kuat (node aktif, tanpa sumber berkedudukan tinggi) yang bisa dijelaskan oleh pemakaian
	// lama di masa simpan.
	HelpfulTotal   int  `json:"helpful_total"`
	HelpfulLegacy  int  `json:"helpful_legacy"`
	HarmfulTotal   int  `json:"harmful_total"`
	HarmfulLegacy  int  `json:"harmful_legacy"`
	MissingColumns bool `json:"missing_columns"` // skema belum dimigrasikan (dihitung seolah bukti lemah 0)
}

// EvidenceRows: putusan per pemakaian lama.
type EvidenceRows struct {
	KeptCited      int `json:"kept_cited"`
	MovedHelpful   int `json:"moved_helpful"`
	MovedHarmful   int `json:"moved_harmful"`
	ClampedHelpful int `json:"clamped_helpful"` // penghitung kuat sudah 0: tidak dipindah
	ClampedHarmful int `json:"clamped_harmful"`
	Authoritative  int `json:"authoritative"` // node sumber berkedudukan tinggi: penghitungnya memang tidak pernah berubah
	ViaMerge       int `json:"via_merge"`     // dihitung di node tujuan peleburan
	MissingNode    int `json:"missing_node"`
}

// EvidenceBucket: jumlah node aktif per rentang perubahan utilitas.
type EvidenceBucket struct {
	Range string `json:"range"`
	Nodes int    `json:"nodes"`
}

// EvidenceChange: perubahan satu node.
type EvidenceChange struct {
	ID          uint       `json:"id"`
	Title       string     `json:"title"`
	Source      string     `json:"source"`
	Helpful     [2]int     `json:"helpful"` // sebelum, sesudah
	Harmful     [2]int     `json:"harmful"`
	WeakHelpful [2]int     `json:"weak_helpful"`
	WeakHarmful [2]int     `json:"weak_harmful"`
	Utility     [2]float64 `json:"utility"`
}

// EvidenceReport: hasil satu hitung ulang.
type EvidenceReport struct {
	Applied  bool             `json:"applied"`
	Now      time.Time        `json:"now"`
	Weight   float64          `json:"weak_weight"`
	Coverage EvidenceCoverage `json:"coverage"`
	Rows     EvidenceRows     `json:"rows"`
	// Node aktif yang penghitungnya berubah, dan yang utilitasnya bergeser ≥ 0.001; sebaran dihitung atas node
	// yang penghitungnya berubah saja.
	NodesTouched   int              `json:"nodes_touched"`
	UtilityChanged int              `json:"utility_changed"`
	MeanDelta      float64          `json:"mean_delta"`
	MeanAbsDelta   float64          `json:"mean_abs_delta"`
	Distribution   []EvidenceBucket `json:"distribution"`
	// Promosi: node promoted:<id> aktif yang node asalnya tidak lagi memenuhi syarat menurut bukti kuat.
	Promoted              int    `json:"promoted"`
	PromotedIneligible    int    `json:"promoted_ineligible"`  // memenuhi sebelum, tidak sesudah
	PromotedAlreadyOff    int    `json:"promoted_already_off"` // sudah tidak memenuhi sebelum hitung ulang
	PromotedIneligibleIDs []uint `json:"promoted_ineligible_ids,omitempty"`
	EligibleBefore        int    `json:"eligible_before"` // node pribadi aktif yang memenuhi syarat promosi
	EligibleAfter         int    `json:"eligible_after"`
	// Pensiun (prune): node aktif yang terkena ambang sebelum & sesudah, dan yang baru terkena.
	RetireBefore int              `json:"retire_before"`
	RetireAfter  int              `json:"retire_after"`
	RetireNew    int              `json:"retire_new"`
	RetireNewIDs []uint           `json:"retire_new_ids,omitempty"`
	FlagBefore   int              `json:"flag_before"`
	FlagAfter    int              `json:"flag_after"`
	FlagNew      int              `json:"flag_new"`
	Top          []EvidenceChange `json:"top"`
}

var evidenceBuckets = []struct {
	label string
	hi    float64 // batas atas (eksklusif)
}{
	{"< -0.20", -0.20}, {"-0.20 … -0.10", -0.10}, {"-0.10 … -0.05", -0.05}, {"-0.05 … -0.01", -0.01},
	{"-0.01 … 0", -0.0005}, {"≈ 0", 0.0005}, {"0 … 0.01", 0.01}, {"0.01 … 0.05", 0.05}, {"0.05 … 0.10", 0.10},
	{"0.10 … 0.20", 0.20}, {"> 0.20", math.Inf(1)},
}

type evidenceUse struct {
	ID        uint
	NodeID    uint
	Context   string
	Outcome   string
	Evidence  string
	CreatedAt time.Time
}

// RecountEvidence menghitung ulang bukti kuat/lemah dari pemakaian lama. top = jumlah perubahan terbesar yang
// dilaporkan (0 = 15).
func RecountEvidence(db *gorm.DB, apply bool, top int) (*EvidenceReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database belum siap")
	}
	if top <= 0 {
		top = 15
	}
	if apply {
		if err := db.AutoMigrate(&models.KnowledgeNode{}, &models.KnowledgeUse{}, &models.KnowledgeVerdict{}); err != nil {
			return nil, fmt.Errorf("migrasi kolom bukti: %w", err)
		}
	}
	m := db.Migrator()
	hasWeak := m.HasColumn(&models.KnowledgeNode{}, "weak_helpful") && m.HasColumn(&models.KnowledgeNode{}, "weak_harmful")
	hasEvidence := m.HasColumn(&models.KnowledgeUse{}, "evidence")
	hasVerdicts := m.HasTable(&models.KnowledgeVerdict{})
	rep := &EvidenceReport{Applied: apply, Now: time.Now(), Weight: models.KnowledgeWeakWeight}
	rep.Coverage.RetentionDays = int(useRetention / (24 * time.Hour))
	rep.Coverage.MissingColumns = !hasWeak || !hasEvidence

	// Node (semua status: pemakaian bisa menunjuk node yang sudah dilebur/dipensiunkan).
	cols := []string{"id", "source_key", "source", "status", "owner_employee_id", "merged_into_id", "title", "helpful", "harmful"}
	if hasWeak {
		cols = append(cols, "weak_helpful", "weak_harmful")
	}
	var list []models.KnowledgeNode
	if err := db.Select(cols).Order("id").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("membaca node: %w", err)
	}
	nodes := make(map[uint]*models.KnowledgeNode, len(list))
	for i := range list {
		nodes[list[i].ID] = &list[i]
	}

	// Pemakaian bernilai.
	ucols := []string{"id", "node_id", "context", "outcome", "created_at"}
	if hasEvidence {
		ucols = append(ucols, "evidence")
	}
	var uses []evidenceUse
	if err := db.Model(&models.KnowledgeUse{}).Select(ucols).
		Where("outcome IN ?", []string{models.KnowledgeOutcomeHelpful, models.KnowledgeOutcomeHarmful}).
		Order("id").Scan(&uses).Error; err != nil {
		return nil, fmt.Errorf("membaca pemakaian: %w", err)
	}
	var oldest struct{ T sql.NullTime }
	if err := db.Model(&models.KnowledgeUse{}).Select("MIN(created_at) AS t").Scan(&oldest).Error; err == nil && oldest.T.Valid {
		t := oldest.T.Time
		rep.Coverage.OldestUse = &t
	}
	rep.Coverage.Rated = len(uses)

	// Kutipan per todo (hanya todo yang pemakaiannya perlu dipilah).
	contexts := map[string]bool{}
	todoIDs := map[uint]bool{}
	legacy := make([]evidenceUse, 0, len(uses))
	for _, u := range uses {
		if u.Evidence != "" {
			continue
		}
		legacy = append(legacy, u)
		contexts[u.Context] = true
		if id := todoOf(u.Context); id != 0 {
			todoIDs[id] = true
		}
	}
	rep.Coverage.Legacy = len(legacy)
	rep.Coverage.Contexts = len(contexts)
	rep.Coverage.TodoContexts = len(todoIDs)
	cites, err := loadCitations(db, todoIDs)
	if err != nil {
		return nil, err
	}
	rep.Coverage.TodosCiting = len(cites)
	verdicts := map[string]bool{}
	if hasVerdicts {
		var vs []models.KnowledgeVerdict
		if err := db.Select("context", "node_id").Find(&vs).Error; err == nil {
			for _, v := range vs {
				verdicts[v.Context+"#"+itoa(v.NodeID)] = true
			}
		}
	}

	// Putusan per pemakaian.
	type move struct{ helpful, harmful int }
	moves := map[uint]*move{}
	legacyByNode := map[uint][2]int{} // node aktif → (helpful, harmful) pemakaian lama
	var citedRows, outcomeRows []uint
	strongSeen := map[string]bool{}
	for _, u := range legacy {
		target, via := resolveMerged(nodes, u.NodeID)
		n := nodes[target]
		if n == nil {
			rep.Rows.MissingNode++
			continue
		}
		if via {
			rep.Rows.ViaMerge++
		}
		if models.KnowledgeAuthoritative(n.Source) {
			rep.Rows.Authoritative++
			outcomeRows = append(outcomeRows, u.ID)
			continue
		}
		c := legacyByNode[target]
		if u.Outcome == models.KnowledgeOutcomeHelpful {
			c[0]++
		} else {
			c[1]++
		}
		legacyByNode[target] = c
		key := u.Context + "#" + itoa(u.NodeID)
		if u.Outcome == models.KnowledgeOutcomeHelpful && !strongSeen[key] &&
			(cites[todoOf(u.Context)][u.NodeID] || verdicts[key]) {
			strongSeen[key] = true
			rep.Rows.KeptCited++
			citedRows = append(citedRows, u.ID)
			continue
		}
		outcomeRows = append(outcomeRows, u.ID)
		mv := moves[target]
		if mv == nil {
			mv = &move{}
			moves[target] = mv
		}
		if u.Outcome == models.KnowledgeOutcomeHelpful {
			if mv.helpful < n.Helpful {
				mv.helpful++
				rep.Rows.MovedHelpful++
			} else {
				rep.Rows.ClampedHelpful++
			}
		} else {
			if mv.harmful < n.Harmful {
				mv.harmful++
				rep.Rows.MovedHarmful++
			} else {
				rep.Rows.ClampedHarmful++
			}
		}
	}

	// Ringkasan node aktif.
	var sum, sumAbs float64
	counts := make([]int, len(evidenceBuckets))
	var changes []EvidenceChange
	for i := range list {
		n := list[i]
		if n.Status != models.KnowledgeNodeActive {
			continue
		}
		after := n
		if mv := moves[n.ID]; mv != nil {
			after.Helpful -= mv.helpful
			after.Harmful -= mv.harmful
			after.WeakHelpful += mv.helpful
			after.WeakHarmful += mv.harmful
		}
		if n.Helpful+n.Harmful > 0 {
			rep.Coverage.NodesWithEvidence++
			if c, ok := legacyByNode[n.ID]; ok && c[0]+c[1] > 0 {
				rep.Coverage.NodesCovered++
			}
		}
		if !models.KnowledgeAuthoritative(n.Source) {
			rep.Coverage.HelpfulTotal += n.Helpful
			rep.Coverage.HarmfulTotal += n.Harmful
		}
		if c, ok := legacyByNode[n.ID]; ok {
			rep.Coverage.HelpfulLegacy += c[0]
			rep.Coverage.HarmfulLegacy += c[1]
		}
		if promotable(n) {
			rep.EligibleBefore++
		}
		if promotable(after) {
			rep.EligibleAfter++
		}
		before, now := pruneAction(n), pruneAction(after)
		switch before {
		case "retire":
			rep.RetireBefore++
		case "flag":
			rep.FlagBefore++
		}
		switch now {
		case "retire":
			rep.RetireAfter++
			if before != "retire" {
				rep.RetireNew++
				if len(rep.RetireNewIDs) < 50 {
					rep.RetireNewIDs = append(rep.RetireNewIDs, n.ID)
				}
			}
		case "flag":
			rep.FlagAfter++
			if before != "flag" {
				rep.FlagNew++
			}
		}
		if moves[n.ID] == nil {
			continue
		}
		rep.NodesTouched++
		ub, ua := n.Utility(), after.Utility()
		d := ua - ub
		if math.Abs(d) >= 0.001 {
			rep.UtilityChanged++
		}
		sum += d
		sumAbs += math.Abs(d)
		counts[bucketOf(d)]++
		changes = append(changes, EvidenceChange{
			ID: n.ID, Title: clip(n.Title, 90), Source: n.Source,
			Helpful: [2]int{n.Helpful, after.Helpful}, Harmful: [2]int{n.Harmful, after.Harmful},
			WeakHelpful: [2]int{n.WeakHelpful, after.WeakHelpful}, WeakHarmful: [2]int{n.WeakHarmful, after.WeakHarmful},
			Utility: [2]float64{round3(ub), round3(ua)},
		})
	}
	if rep.NodesTouched > 0 {
		rep.MeanDelta = round3(sum / float64(rep.NodesTouched))
		rep.MeanAbsDelta = round3(sumAbs / float64(rep.NodesTouched))
	}
	for i, b := range evidenceBuckets {
		rep.Distribution = append(rep.Distribution, EvidenceBucket{Range: b.label, Nodes: counts[i]})
	}
	sort.SliceStable(changes, func(i, j int) bool {
		return math.Abs(changes[i].Utility[1]-changes[i].Utility[0]) > math.Abs(changes[j].Utility[1]-changes[j].Utility[0])
	})
	if len(changes) > top {
		changes = changes[:top]
	}
	rep.Top = changes

	// Node hasil promosi yang node asalnya tidak lagi memenuhi syarat.
	for i := range list {
		p := list[i]
		if p.Status != models.KnowledgeNodeActive || !strings.HasPrefix(p.Key, "promoted:") {
			continue
		}
		rep.Promoted++
		id, err := strconv.ParseUint(strings.TrimPrefix(p.Key, "promoted:"), 10, 64)
		if err != nil {
			continue
		}
		origin := nodes[uint(id)]
		if origin == nil {
			continue
		}
		after := *origin
		if mv := moves[origin.ID]; mv != nil {
			after.Helpful -= mv.helpful
			after.Harmful -= mv.harmful
		}
		switch {
		case !promotable(*origin):
			rep.PromotedAlreadyOff++
		case !promotable(after):
			rep.PromotedIneligible++
			if len(rep.PromotedIneligibleIDs) < 50 {
				rep.PromotedIneligibleIDs = append(rep.PromotedIneligibleIDs, p.ID)
			}
		}
	}

	if !apply {
		return rep, nil
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		grouped := map[move][]uint{}
		for id, mv := range moves {
			if mv.helpful > 0 || mv.harmful > 0 {
				grouped[*mv] = append(grouped[*mv], id)
			}
		}
		for mv, ids := range grouped {
			for _, part := range chunks(ids, settleBatch) {
				if err := tx.Model(&models.KnowledgeNode{}).Where("id IN ?", part).UpdateColumns(map[string]interface{}{
					"helpful":      gorm.Expr("helpful - ?", mv.helpful),
					"harmful":      gorm.Expr("harmful - ?", mv.harmful),
					"weak_helpful": gorm.Expr("weak_helpful + ?", mv.helpful),
					"weak_harmful": gorm.Expr("weak_harmful + ?", mv.harmful),
				}).Error; err != nil {
					return fmt.Errorf("menulis penghitung node: %w", err)
				}
			}
		}
		for ev, ids := range map[string][]uint{models.KnowledgeEvidenceCited: citedRows, models.KnowledgeEvidenceOutcome: outcomeRows} {
			for _, part := range chunks(ids, settleBatch) {
				if err := tx.Model(&models.KnowledgeUse{}).Where("id IN ? AND (evidence = ? OR evidence IS NULL)", part, "").
					UpdateColumn("evidence", ev).Error; err != nil {
					return fmt.Errorf("menulis asal bukti pemakaian: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	invalidate()
	return rep, nil
}

func bucketOf(d float64) int {
	for i, b := range evidenceBuckets {
		if d < b.hi {
			return i
		}
	}
	return len(evidenceBuckets) - 1
}

func chunks(ids []uint, n int) [][]uint {
	var out [][]uint
	for i := 0; i < len(ids); i += n {
		out = append(out, ids[i:min(i+n, len(ids))])
	}
	return out
}

// todoOf: ID todo dari konteks todo:<id>; 0 untuk konteks lain.
func todoOf(ctx string) uint {
	if !strings.HasPrefix(ctx, "todo:") {
		return 0
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(ctx, "todo:"), 10, 64)
	if err != nil {
		return 0
	}
	return uint(id)
}

// resolveMerged mengikuti merged_into_id sampai node yang tidak dilebur (maks 8 lompatan).
func resolveMerged(nodes map[uint]*models.KnowledgeNode, id uint) (uint, bool) {
	via := false
	for i := 0; i < 8; i++ {
		n := nodes[id]
		if n == nil || n.Status != models.KnowledgeNodeMerged || n.MergedIntoID == nil || *n.MergedIntoID == 0 {
			return id, via
		}
		id, via = *n.MergedIntoID, true
	}
	return id, via
}

// loadCitations: node yang dikutip di catatan penyelesaian tiap todo (todo tanpa kutipan tidak muncul).
func loadCitations(db *gorm.DB, todoIDs map[uint]bool) (map[uint]map[uint]bool, error) {
	ids := make([]uint, 0, len(todoIDs))
	for id := range todoIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := map[uint]map[uint]bool{}
	for _, part := range chunks(ids, settleBatch) {
		var todos []models.Todo
		if err := db.Select("id", "rationale").Where("id IN ?", part).Find(&todos).Error; err != nil {
			return nil, fmt.Errorf("membaca catatan todo: %w", err)
		}
		for _, t := range todos {
			if c := CitedNodes(t.Rationale); len(c) > 0 {
				out[t.ID] = toSet(c)
			}
		}
	}
	return out, nil
}
