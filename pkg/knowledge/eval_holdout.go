package knowledge

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Evaluasi tanpa bukti dari todo yang sedang diuji. Penghitung node (✓/✗ kuat & lemah) dan waktu terakhir dipakai
// sebagian terbentuk dari todo yang sedang dinilai sendiri: node yang dikutip di todo T (= label kuat T) mendapat ✓
// kuat dari T, sehingga utilitasnya naik tepat untuk kueri T. Saat memeringkat untuk T, sumbangan T dikeluarkan:
//
//   - bukti dari pemakaian berkonteks todo:T, menurut asal buktinya (contributionCode): cited → ✓ kuat,
//     outcome → ✓/✗ lemah, catatan lama tanpa asal bukti → ✓/✗ kuat (cara penghitung lama terbentuk), dan
//     pemakaian yang dibuka lagi → bukti lama yang dititipkannya;
//   - vonis per node berkonteks todo:T (knowledge_verdicts) → ✓/✗ kuat. Pemakaian bervonis tidak dihitung lagi;
//   - waktu terakhir dipakai: bila pemakaian terakhir node berasal dari T, diganti waktu pemakaian terakhir dari
//     konteks lain (atau dianggap belum pernah dipakai).
//
// Node sumber berkedudukan tinggi diabaikan (penghitungnya tidak pernah berubah dan skornya memakai utilitas penuh).
// Pemakaian di node yang sudah dilebur dihitung di node tujuannya. Penghitung tidak pernah dikurangi sampai negatif.
//
// Yang tidak bisa direkonstruksi: bukti tanpa konteks (umpan balik manual/MCP, vonis audit, penguatan saat Learn),
// vonis lama sebelum knowledge_verdicts ada atas node yang tidak disisipkan ke T, catatan pemakaian yang sudah
// dibuang setelah masa simpan, dan waktu terakhir dipakai yang lebih tua dari catatan tersisa (last_used_at hanya
// menyimpan satu waktu; pemakaian konteks lain yang sudah dibuang tidak bisa dipulihkan, jadi node itu dianggap
// belum pernah dipakai). Bukti dari todo lain yang datang SESUDAH T dibuat juga tetap ikut — ini hanya
// mengeluarkan T sendiri, bukan memutar waktu ke saat T dibuat.

// usedRecencySlack: last_used_at ditulis sesaat sesudah baris pemakaiannya (recordUse), jadi selisih kecil
// dianggap pemakaian yang sama.
const usedRecencySlack = time.Minute

type nodeDelta struct{ helpful, harmful, weakHelpful, weakHarmful int }

type ctxTime struct {
	ctx string
	at  time.Time
}

// holdout: sumbangan tiap todo ke penghitung & kebaruan node.
type holdout struct {
	delta map[uint]map[uint]*nodeDelta // todo → node (tujuan peleburan) → sumbangan
	// latest: dua konteks berbeda dengan pemakaian terakhir per node (urut terbaru dulu).
	latest map[uint][2]ctxTime
}

func loadHoldout(db *gorm.DB) (*holdout, error) {
	h := &holdout{delta: map[uint]map[uint]*nodeDelta{}, latest: map[uint][2]ctxTime{}}
	var list []models.KnowledgeNode
	if err := db.Select("id", "source", "status", "merged_into_id").Order("id").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("membaca node: %w", err)
	}
	nodes := make(map[uint]*models.KnowledgeNode, len(list))
	for i := range list {
		nodes[list[i].ID] = &list[i]
	}
	target := func(id uint) (uint, bool) {
		t, _ := resolveMerged(nodes, id)
		n := nodes[t]
		return t, n != nil && !models.KnowledgeAuthoritative(n.Source)
	}
	add := func(todo, node uint, f func(*nodeDelta)) {
		t, ok := target(node)
		if !ok {
			return
		}
		m := h.delta[todo]
		if m == nil {
			m = map[uint]*nodeDelta{}
			h.delta[todo] = m
		}
		d := m[t]
		if d == nil {
			d = &nodeDelta{}
			m[t] = d
		}
		f(d)
	}

	evidence := "'' AS evidence"
	if db.Migrator().HasColumn(&models.KnowledgeUse{}, "evidence") {
		evidence = "COALESCE(evidence, '') AS evidence"
	}
	var uses []struct {
		NodeID    uint
		Context   string
		Outcome   string
		Evidence  string
		CreatedAt time.Time
	}
	if err := db.Model(&models.KnowledgeUse{}).Select("node_id", "context", "COALESCE(outcome, '') AS outcome", evidence, "created_at").
		Order("id").Scan(&uses).Error; err != nil {
		return nil, fmt.Errorf("membaca pemakaian: %w", err)
	}
	for _, u := range uses {
		t, _ := resolveMerged(nodes, u.NodeID)
		top := h.latest[t]
		switch {
		case top[0].ctx == u.Context:
			if u.CreatedAt.After(top[0].at) {
				top[0].at = u.CreatedAt
			}
		case u.CreatedAt.After(top[0].at):
			top[1], top[0] = top[0], ctxTime{u.Context, u.CreatedAt}
		case top[1].ctx == u.Context || u.CreatedAt.After(top[1].at):
			if u.CreatedAt.After(top[1].at) {
				top[1] = ctxTime{u.Context, u.CreatedAt}
			}
		}
		h.latest[t] = top

		todo := todoOf(u.Context)
		if todo == 0 {
			continue
		}
		c, ok := reopened[contributionCode(u.Outcome, u.Evidence)]
		if !ok {
			continue
		}
		add(todo, u.NodeID, func(d *nodeDelta) {
			switch c.column {
			case "helpful":
				d.helpful++
			case "harmful":
				d.harmful++
			case "weak_helpful":
				d.weakHelpful++
			case "weak_harmful":
				d.weakHarmful++
			}
		})
	}

	if db.Migrator().HasTable(&models.KnowledgeVerdict{}) {
		var vs []models.KnowledgeVerdict
		if err := db.Select("context", "node_id", "helpful").Where("context LIKE ?", "todo:%").Order("id").Find(&vs).Error; err != nil {
			return nil, fmt.Errorf("membaca vonis: %w", err)
		}
		for _, v := range vs {
			if todo := todoOf(v.Context); todo != 0 {
				ok := v.Helpful
				add(todo, v.NodeID, func(d *nodeDelta) {
					if ok {
						d.helpful++
					} else {
						d.harmful++
					}
				})
			}
		}
	}
	return h, nil
}

// adjustFor: pengubah node untuk peringkat todo (nil bila todo itu tidak menyumbang apa pun).
func (h *holdout) adjustFor(todoID uint) func(*models.KnowledgeNode) {
	if h == nil {
		return nil
	}
	ctx := "todo:" + itoa(todoID)
	delta := h.delta[todoID]
	return func(n *models.KnowledgeNode) {
		if d := delta[n.ID]; d != nil {
			n.Helpful = max(0, n.Helpful-d.helpful)
			n.Harmful = max(0, n.Harmful-d.harmful)
			n.WeakHelpful = max(0, n.WeakHelpful-d.weakHelpful)
			n.WeakHarmful = max(0, n.WeakHarmful-d.weakHarmful)
		}
		top, ok := h.latest[n.ID]
		if !ok || top[0].ctx != ctx || n.LastUsedAt == nil || n.LastUsedAt.After(top[0].at.Add(usedRecencySlack)) {
			return
		}
		if top[1].ctx == "" {
			n.LastUsedAt = nil
			return
		}
		at := top[1].at
		n.LastUsedAt = &at
	}
}
