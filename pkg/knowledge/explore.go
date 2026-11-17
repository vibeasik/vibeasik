package knowledge

import (
	"hash/fnv"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Slot coba (eksplorasi): sebagian besar pustaka belum pernah dikirim ke pekerjaan apa pun, sehingga utilitasnya
// tetap tebakan 0.5 dan peringkat terus memilih node yang sama. Recall untuk pekerjaan nyata (Query.Context terisi)
// menambahkan PALING BANYAK satu node di ujung daftar — tidak menggantikan hasil biasa — dari kandidat peringkat
// berikutnya yang relevansinya masih layak dan buktinya masih sedikit. Pilihannya Thompson sampling: tiap kandidat
// diberi sampel Beta(1+✓+w·✓lemah, 1+✗+w·✗lemah) dan sampel terbesar menang, jadi node yang belum teruji mendapat
// giliran acak dan node yang mulai terbukti membantu lebih sering terpilih. Benih acaknya dari Context dan id node,
// sehingga pekerjaan yang sama mendapat node coba yang sama. rank tidak tersentuh: evaluasi tetap deterministik.

// ViaTrial menandai node coba di Hit.Via (tetangga bertaut memakai "tautan").
const ViaTrial = "coba"

// ExploreEnv mematikan slot coba bila bernilai 0, false, off, atau no (bawaan menyala).
const ExploreEnv = "VIBEASIK_KNOWLEDGE_EXPLORE"

const (
	exploreDepth       = 15  // kandidat peringkat berikutnya yang dipertimbangkan di luar daftar biasa
	exploreMaxEvidence = 3.0 // bukti efektif (kuat + KnowledgeWeakWeight·lemah) di bawah ini = buktinya masih sedikit
	// exploreMinRelRatio: relevansi mentah kandidat minimal sekian kali relevansi butir langsung terendah di daftar
	// biasa, jadi node coba tidak pernah kurang relevan daripada butir yang sudah dikirim. Pada riwayat todo
	// kandidat peringkat berikutnya umumnya lebih relevan daripada butir terendah (tersingkir karena utilitas,
	// bobot, kebaruan, atau batas klaster), sehingga ambang ini hampir tidak mengurangi jumlah todo yang terisi.
	exploreMinRelRatio = 1.0
)

func exploreEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(ExploreEnv))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

// evidence: jumlah bukti efektif sebuah node (bukti lemah berbobot KnowledgeWeakWeight).
func evidence(n models.KnowledgeNode) float64 {
	return float64(n.Helpful+n.Harmful) + models.KnowledgeWeakWeight*float64(n.WeakHelpful+n.WeakHarmful)
}

// trialCandidates: kandidat slot coba dari pool (daftar peringkat yang lebih panjang) yang belum ada di hits.
// Kandidat wajib cocok kata dengan kueri: relevansi yang hanya berasal dari bonus domain (domains = domain
// kueri) tidak cukup.
func trialCandidates(hits, pool []Hit, domains []string, ratio float64) []Hit {
	want := setOf(domains)
	have := map[uint]bool{}
	floor := math.Inf(1)
	for _, h := range hits {
		have[h.ID] = true
		if h.Via == "" {
			floor = math.Min(floor, h.Relevance)
		}
	}
	if math.IsInf(floor, 1) {
		return nil
	}
	min := ratio * floor
	var out []Hit
	for _, c := range pool {
		if have[c.ID] || c.Via != "" || models.KnowledgeAuthoritative(c.Source) || evidence(c.KnowledgeNode) >= exploreMaxEvidence || c.Relevance < min {
			continue
		}
		if want[c.Domain] && c.Relevance-DefaultRankWeights.DomainBonus < 0.001 {
			continue
		}
		out = append(out, c)
	}
	return out
}

// pickTrial: Thompson sampling atas kandidat dengan benih (ctx, id node). Seri dipecah oleh id terkecil.
func pickTrial(ctx string, cands []Hit) *Hit {
	h := fnv.New64a()
	h.Write([]byte(ctx))
	seed := h.Sum64()
	best, bestS := -1, -1.0
	for i, c := range cands {
		r := rand.New(rand.NewPCG(seed, uint64(c.ID)))
		a := 1 + float64(c.Helpful) + models.KnowledgeWeakWeight*float64(c.WeakHelpful)
		b := 1 + float64(c.Harmful) + models.KnowledgeWeakWeight*float64(c.WeakHarmful)
		if s := betaSample(r, a, b); s > bestS || s == bestS && c.ID < cands[best].ID {
			best, bestS = i, s
		}
	}
	if best < 0 {
		return nil
	}
	t := cands[best]
	t.Via = ViaTrial
	return &t
}

// explore menambahkan node coba ke res (paling banyak satu) untuk pekerjaan nyata q.Context.
func explore(db *gorm.DB, ix *index, q Query, res *Result, now time.Time) {
	if q.Context == "" || len(res.Hits) == 0 || !exploreEnabled() {
		return
	}
	wide := q
	wide.Limit = q.Limit + exploreDepth
	pool := rank(db, ix, wide, DefaultRankWeights, now, nil)
	if t := pickTrial(q.Context, trialCandidates(res.Hits, pool.Hits, res.Domains, exploreMinRelRatio)); t != nil {
		res.Hits = append(res.Hits, *t)
	}
}

// betaSample: sampel Beta(a, b) untuk a, b ≥ 1 lewat dua sampel Gamma (Marsaglia–Tsang).
func betaSample(r *rand.Rand, a, b float64) float64 {
	x, y := gammaSample(r, a), gammaSample(r, b)
	return x / (x + y)
}

func gammaSample(r *rand.Rand, k float64) float64 {
	d := k - 1.0/3
	c := 1 / math.Sqrt(9*d)
	for {
		x := r.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := r.Float64()
		if u < 1-0.0331*x*x*x*x || math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}
