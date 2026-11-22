package knowledge

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Evaluasi temu-kembali: set uji dibangun dari riwayat todo sendiri, lalu peringkat Recall diukur dengan
// recall@k, nDCG@k, dan MRR@k. Kueri = teks todo yang sama dengan yang dipakai saat prompt pegawai dibangun;
// jawaban benar berasal dari dua sumber yang dilaporkan terpisah:
//
//   - kuat:  node yang disebut agent di baris "PENGETAHUAN: K<id>, …" catatan penyelesaian todo yang lulus QA;
//   - lemah: node yang pemakaiannya di konteks todo:<id> bernilai helpful (Reward/Judge).
//
// Evaluasi memakai fungsi peringkat yang sama dengan Recall (rank) tanpa efek samping: tidak mencatat pemakaian,
// celah, atau apa pun. Node yang dibuat setelah todo dibuat tidak dihitung sebagai kandidat maupun label. Bawaannya
// bukti & waktu pakai yang berasal dari todo yang sedang diuji dikeluarkan dari penghitung node (eval_holdout.go);
// EvalOptions.KeepOwnEvidence mematikannya agar angka cara ukur lama bisa diulang.
//
// Label hanya ada untuk node yang pernah ditampilkan peringkat lama, jadi peringkat baru yang mengangkat node yang
// belum pernah dinilai tampak lebih buruk walau mungkin benar. Untuk itu dilaporkan juga nDCG terkondensasi
// (dihitung hanya atas node yang punya penilaian di todo itu: pernah dipakai di konteks todo:<id> apa pun hasilnya,
// atau disebut di baris PENGETAHUAN) dan porsi k teratas yang belum pernah dinilai. Todo dibagi deterministik
// per id ke bagian latih (70 %) dan uji (30 %) agar penyetelan tidak diukur pada data yang sama.

// EvalOptions mengatur satu putaran evaluasi.
type EvalOptions struct {
	K       int         // metrik dihitung pada k teratas (bawaan 10)
	Limit   int         // panjang daftar yang diperingkat seperti Query.Limit produksi (bawaan 15, minimal K)
	Weights RankWeights // bobot skor (kosong = DefaultRankWeights)
	// Now: acuan waktu kebaruan sekaligus batas data (todo & pemakaian sesudahnya diabaikan). Wajib diisi
	// agar dua putaran memberi angka yang sama; kosong = awal hari ini.
	Now time.Time
	// QueryFor membentuk kueri dari todo (kosong = DefaultEvalQuery). Pemanggil yang punya aturan domain
	// tambahan (mis. domain desain untuk peran tertentu) bisa menggantinya agar sama persis dengan produksi.
	QueryFor func(models.Todo) Query
	// MinScore: nilai QA minimal agar todo dianggap lulus untuk label kuat (bawaan 75 = ambang helpful Reward).
	MinScore int
	// Split: EvalSplitTrain / EvalSplitTest = hanya todo di bagian itu (EvalSplitOf); kosong = semua todo.
	Split string
	// PerQuery: sertakan metrik per todo (EvalTier.Queries) untuk perbandingan berpasangan antar varian.
	PerQuery bool
	// KeepOwnEvidence: pakai penghitung & waktu pakai node apa adanya, termasuk sumbangan todo yang sedang
	// diuji (cara ukur lama). Kosong = sumbangan itu dikeluarkan.
	KeepOwnEvidence bool
}

// Bagian set uji.
const (
	EvalSplitTrain = "latih"
	EvalSplitTest  = "uji"
)

// EvalSplitOf menempatkan todo ke bagian latih (±70 %) atau uji (±30 %) secara deterministik dari id-nya
// (hash FNV-1a, jadi tidak bergantung urutan id maupun waktu).
func EvalSplitOf(todoID uint) string {
	h := uint32(2166136261)
	for _, c := range []byte(strconv.FormatUint(uint64(todoID), 10)) {
		h ^= uint32(c)
		h *= 16777619
	}
	if h%10 < 7 {
		return EvalSplitTrain
	}
	return EvalSplitTest
}

// EvalMetrics: rata-rata metrik atas kueri yang punya minimal satu label.
type EvalMetrics struct {
	Queries int     `json:"queries"`
	Labels  int     `json:"labels"`
	Recall  float64 `json:"recall_at_k"`
	NDCG    float64 `json:"ndcg_at_k"`
	MRR     float64 `json:"mrr_at_k"`
	// CondNDCG: nDCG@k atas daftar yang hanya memuat node yang punya penilaian di todo itu.
	CondNDCG float64 `json:"cond_ndcg_at_k"`
	// Unjudged: porsi k teratas yang belum pernah dinilai di todo itu (0–1).
	Unjudged float64 `json:"unjudged_at_k"`
	// RecallList: recall atas seluruh daftar yang terkirim (bukan hanya k teratas).
	RecallList float64 `json:"recall_list"`
	// Delivered / Linked: rata-rata jumlah node terkirim per kueri, dan yang di antaranya tetangga bertaut.
	Delivered float64 `json:"delivered"`
	Linked    float64 `json:"linked"`
}

// EvalQuery: metrik satu todo pada satu tingkat label (hanya bila EvalOptions.PerQuery).
type EvalQuery struct {
	TodoID   uint    `json:"todo_id"`
	Recall   float64 `json:"recall_at_k"`
	NDCG     float64 `json:"ndcg_at_k"`
	CondNDCG float64 `json:"cond_ndcg_at_k"`
}

// DomainMetrics: metrik per domain utama kueri (domain pertama hasil deteksi; "-" bila tidak terdeteksi).
type DomainMetrics struct {
	Domain string `json:"domain"`
	EvalMetrics
}

// EvalTier: hasil satu tingkat label.
type EvalTier struct {
	Name     string          `json:"name"`
	Total    EvalMetrics     `json:"total"`
	ByDomain []DomainMetrics `json:"by_domain"`
	// Label yang dibuang: node dibuat setelah todo (bocor), atau node tidak aktif/tidak dikenal saat ini.
	DroppedLeak     int `json:"dropped_leak"`
	DroppedInactive int `json:"dropped_inactive"`
	// Queries: metrik per todo, urut id (hanya bila EvalOptions.PerQuery).
	Queries []EvalQuery `json:"queries,omitempty"`
}

// EvalReport: hasil lengkap satu putaran evaluasi.
type EvalReport struct {
	K       int         `json:"k"`
	Limit   int         `json:"limit"`
	Now     time.Time   `json:"now"`
	Weights RankWeights `json:"weights"`
	Split   string      `json:"split,omitempty"`
	// Holdout: bukti & waktu pakai dari todo yang sedang diuji dikeluarkan (kebalikan KeepOwnEvidence).
	Holdout bool       `json:"holdout"`
	Nodes   int        `json:"nodes"` // node aktif di indeks
	Todos   int        `json:"todos"` // todo yang diperingkat (punya label di salah satu tingkat)
	Tiers   []EvalTier `json:"tiers"`
}

// Nama tingkat label.
const (
	EvalTierStrong = "kuat"
	EvalTierWeak   = "lemah"
)

// DefaultEvalQuery meniru kueri prompt pegawai untuk sebuah todo: judul, keterangan, aplikasi target, dan
// kategori; pengetahuan pribadi pegawai yang ditugaskan ikut; daftar 15 butir.
func DefaultEvalQuery(t models.Todo) Query {
	q := Query{Text: t.Title + " " + t.Description + " " + t.TargetApps + " " + t.Category, Limit: 15}
	if t.EmployeeID != nil {
		q.EmployeeID = *t.EmployeeID
	}
	return q
}

var (
	knowledgeLine = regexp.MustCompile(`PENGETAHUAN:([^\n]*)`)
	knowledgeRef  = regexp.MustCompile(`\bK(\d+)\b`)
	// Baris PENGETAHUAN sering langsung disambung bagian lain ("K12. PELAJARAN: …"); potong di sana.
	lineEnd = regexp.MustCompile(`\.\s|\b[A-Z]{5,}:|\|`)
)

// Baris umpan balik di catatan penyelesaian: butir yang keliru dan pengalaman rekan yang tidak cocok bagi penulisnya.
var (
	wrongLine = regexp.MustCompile(`KELIRU:([^\n]*)`)
	unfitLine = regexp.MustCompile(`TIDAK COCOK:([^\n]*)`)
)

// CitedNodes mengambil ID node dari semua baris "PENGETAHUAN: K<id>, …" pada teks (urut, tanpa duplikat).
func CitedNodes(text string) []uint { return labeledNodes(knowledgeLine, text) }

// WrongNodes: ID node di baris "KELIRU: K<id>, …".
func WrongNodes(text string) []uint { return labeledNodes(wrongLine, text) }

// UnfitNodes: ID node di baris "TIDAK COCOK: K<id>, …".
func UnfitNodes(text string) []uint { return labeledNodes(unfitLine, text) }

func labeledNodes(line *regexp.Regexp, text string) []uint {
	var out []uint
	seen := map[uint]bool{}
	for _, m := range line.FindAllStringSubmatch(text, -1) {
		seg := m[1]
		if loc := lineEnd.FindStringIndex(seg); loc != nil {
			seg = seg[:loc[0]]
		}
		for _, r := range knowledgeRef.FindAllStringSubmatch(seg, -1) {
			id, err := strconv.ParseUint(r[1], 10, 32)
			if err != nil || id == 0 || seen[uint(id)] {
				continue
			}
			seen[uint(id)] = true
			out = append(out, uint(id))
		}
	}
	return out
}

// RankMetrics menghitung recall@k, nDCG@k (relevansi biner), dan reciprocal rank@k satu kueri.
func RankMetrics(ranked []uint, relevant map[uint]bool, k int) (recall, ndcg, rr float64) {
	if len(relevant) == 0 || k <= 0 {
		return 0, 0, 0
	}
	found, dcg := 0, 0.0
	for i, id := range ranked {
		if i >= k {
			break
		}
		if relevant[id] {
			found++
			dcg += 1 / math.Log2(float64(i)+2)
			if rr == 0 {
				rr = 1 / float64(i+1)
			}
		}
	}
	idcg := 0.0
	for i := 0; i < min(len(relevant), k); i++ {
		idcg += 1 / math.Log2(float64(i)+2)
	}
	return float64(found) / float64(len(relevant)), dcg / idcg, rr
}

// CondensedNDCG: nDCG@k setelah daftar disaring ke node yang punya penilaian (judged), dan porsi k teratas daftar
// asli yang belum dinilai.
func CondensedNDCG(ranked []uint, judged, relevant map[uint]bool, k int) (ndcg, unjudged float64) {
	cond := make([]uint, 0, len(ranked))
	miss, n := 0, 0
	for i, id := range ranked {
		if judged[id] {
			cond = append(cond, id)
		} else if i < k {
			miss++
		}
		if i < k {
			n++
		}
	}
	_, ndcg, _ = RankMetrics(cond, relevant, k)
	if n > 0 {
		unjudged = float64(miss) / float64(n)
	}
	return ndcg, unjudged
}

// Evaluate membangun set uji dari todo & pemakaian pengetahuan di db, lalu mengukur peringkat. Hanya membaca.
func Evaluate(db *gorm.DB, opt EvalOptions) (*EvalReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database belum siap")
	}
	if opt.K <= 0 {
		opt.K = 10
	}
	if opt.Weights == (RankWeights{}) {
		opt.Weights = DefaultRankWeights
	}
	if opt.Now.IsZero() {
		y, m, d := time.Now().Date()
		opt.Now = time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	}
	if opt.QueryFor == nil {
		opt.QueryFor = DefaultEvalQuery
	}
	if opt.MinScore <= 0 {
		opt.MinScore = 75
	}

	// Waktu dibuatnya setiap node aktif (aturan kebocoran) — dari DB, bukan dari indeks.
	var nodeRows []struct {
		ID        uint
		CreatedAt time.Time
	}
	if err := db.Model(&models.KnowledgeNode{}).Select("id", "created_at").
		Where("status = ?", models.KnowledgeNodeActive).Order("id").Scan(&nodeRows).Error; err != nil {
		return nil, fmt.Errorf("membaca node: %w", err)
	}
	created := make(map[uint]time.Time, len(nodeRows))
	for _, r := range nodeRows {
		created[r.ID] = r.CreatedAt
	}

	// Label kuat: todo selesai yang lulus QA dan menyebut K<id>.
	var todos []models.Todo
	if err := db.Select("id", "title", "description", "target_apps", "category", "employee_id", "status", "score", "rationale", "created_at").
		Where("created_at < ? AND status IN ?", opt.Now, []string{models.TodoStatusCompleted, models.TodoStatusMerged}).
		Order("id").Find(&todos).Error; err != nil {
		return nil, fmt.Errorf("membaca todo: %w", err)
	}
	strong := map[uint][]uint{}
	for _, t := range todos {
		if t.Score == nil || *t.Score < opt.MinScore {
			continue
		}
		if ids := CitedNodes(t.Rationale); len(ids) > 0 {
			strong[t.ID] = ids
		}
	}

	// Label lemah: pemakaian bernilai helpful di konteks todo:<id>.
	var uses []models.KnowledgeUse
	if err := db.Select("node_id", "context").
		Where("context LIKE ? AND outcome = ? AND created_at < ?", "todo:%", models.KnowledgeOutcomeHelpful, opt.Now).
		Order("id").Find(&uses).Error; err != nil {
		return nil, fmt.Errorf("membaca pemakaian: %w", err)
	}
	weak := map[uint][]uint{}
	// Node yang pernah dinilai per todo: semua pemakaian di konteks todo:<id> (apa pun hasilnya) + label kuat.
	var allUses []models.KnowledgeUse
	if err := db.Select("node_id", "context").
		Where("context LIKE ? AND created_at < ?", "todo:%", opt.Now).
		Order("id").Find(&allUses).Error; err != nil {
		return nil, fmt.Errorf("membaca pemakaian: %w", err)
	}
	judged := map[uint]map[uint]bool{}
	mark := func(todo, node uint) {
		if judged[todo] == nil {
			judged[todo] = map[uint]bool{}
		}
		judged[todo][node] = true
	}
	for _, u := range allUses {
		if id, err := strconv.ParseUint(strings.TrimPrefix(u.Context, "todo:"), 10, 32); err == nil && id > 0 {
			mark(uint(id), u.NodeID)
		}
	}
	for todo, ids := range strong {
		for _, id := range ids {
			mark(todo, id)
		}
	}
	for _, u := range uses {
		id, err := strconv.ParseUint(strings.TrimPrefix(u.Context, "todo:"), 10, 32)
		if err != nil || id == 0 {
			continue
		}
		weak[uint(id)] = append(weak[uint(id)], u.NodeID)
	}

	var hold *holdout
	if !opt.KeepOwnEvidence {
		var err error
		if hold, err = loadHoldout(db); err != nil {
			return nil, err
		}
	}

	ix := current(db)
	rep := &EvalReport{K: opt.K, Now: opt.Now, Weights: opt.Weights, Split: opt.Split, Holdout: hold != nil, Nodes: ix.n}
	type acc struct {
		tier            EvalTier
		total           EvalMetrics
		byDomain        map[string]*EvalMetrics
		droppedLeak     int
		droppedInactive int
	}
	tiers := []*acc{
		{tier: EvalTier{Name: EvalTierStrong}, byDomain: map[string]*EvalMetrics{}},
		{tier: EvalTier{Name: EvalTierWeak}, byDomain: map[string]*EvalMetrics{}},
	}
	labelsOf := []map[uint][]uint{strong, weak}

	for _, t := range todos {
		if len(strong[t.ID]) == 0 && len(weak[t.ID]) == 0 {
			continue
		}
		if opt.Split != "" && EvalSplitOf(t.ID) != opt.Split {
			continue
		}
		cutoff := t.CreatedAt
		// Label per tingkat setelah aturan kebocoran & status.
		rels := make([]map[uint]bool, len(tiers))
		hasLabel := false
		for i, a := range tiers {
			rels[i] = map[uint]bool{}
			for _, id := range labelsOf[i][t.ID] {
				if rels[i][id] {
					continue
				}
				c, ok := created[id]
				switch {
				case !ok:
					a.droppedInactive++
				case c.After(cutoff):
					a.droppedLeak++
				default:
					rels[i][id] = true
				}
			}
			hasLabel = hasLabel || len(rels[i]) > 0
		}
		if !hasLabel {
			continue
		}

		q := opt.QueryFor(t)
		limit := max(q.Limit, opt.K, opt.Limit)
		if limit > 50 {
			limit = 50
		}
		q.Limit = limit
		rep.Limit = max(rep.Limit, limit)
		res := rankWith(db, ix, q, opt.Weights, opt.Now, func(e *entry) bool {
			c, ok := created[e.id]
			return !ok || c.After(cutoff)
		}, hold.adjustFor(t.ID))
		ranked := make([]uint, len(res.Hits))
		linked := 0
		for i, h := range res.Hits {
			ranked[i] = h.ID
			if h.Via != "" {
				linked++
			}
		}
		domain := "-"
		if len(res.Domains) > 0 {
			domain = res.Domains[0]
		}
		rep.Todos++
		for i, a := range tiers {
			if len(rels[i]) == 0 {
				continue
			}
			r, n, rr := RankMetrics(ranked, rels[i], opt.K)
			cn, un := CondensedNDCG(ranked, judged[t.ID], rels[i], opt.K)
			rl, _, _ := RankMetrics(ranked, rels[i], len(ranked))
			if opt.PerQuery {
				a.tier.Queries = append(a.tier.Queries, EvalQuery{TodoID: t.ID, Recall: r, NDCG: n, CondNDCG: cn})
			}
			d := a.byDomain[domain]
			if d == nil {
				d = &EvalMetrics{}
				a.byDomain[domain] = d
			}
			for _, m := range []*EvalMetrics{&a.total, d} {
				m.Queries++
				m.Labels += len(rels[i])
				m.Recall += r
				m.NDCG += n
				m.MRR += rr
				m.CondNDCG += cn
				m.Unjudged += un
				m.RecallList += rl
				m.Delivered += float64(len(ranked))
				m.Linked += float64(linked)
			}
		}
	}

	for _, a := range tiers {
		t := a.tier
		t.Total = averaged(a.total)
		t.DroppedLeak, t.DroppedInactive = a.droppedLeak, a.droppedInactive
		for d, m := range a.byDomain {
			t.ByDomain = append(t.ByDomain, DomainMetrics{Domain: d, EvalMetrics: averaged(*m)})
		}
		sort.Slice(t.ByDomain, func(i, j int) bool {
			x, y := t.ByDomain[i], t.ByDomain[j]
			return x.Queries > y.Queries || x.Queries == y.Queries && x.Domain < y.Domain
		})
		rep.Tiers = append(rep.Tiers, t)
	}
	if rep.Limit == 0 {
		rep.Limit = max(opt.K, opt.Limit)
	}
	return rep, nil
}

// averaged mengubah jumlah metrik menjadi rata-rata per kueri (dibulatkan 4 angka desimal).
func averaged(m EvalMetrics) EvalMetrics {
	if m.Queries == 0 {
		return m
	}
	n := float64(m.Queries)
	r4 := func(f float64) float64 { return math.Round(f/n*10000) / 10000 }
	m.Recall, m.NDCG, m.MRR = r4(m.Recall), r4(m.NDCG), r4(m.MRR)
	m.CondNDCG, m.Unjudged, m.RecallList = r4(m.CondNDCG), r4(m.Unjudged), r4(m.RecallList)
	m.Delivered, m.Linked = r4(m.Delivered), r4(m.Linked)
	return m
}
