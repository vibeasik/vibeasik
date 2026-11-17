package knowledge

import (
	"math"
	"sort"
	"sync"
)

// Sisi relevansi peringkat. Bawaan = kosinus TF-IDF (vector.go). Varian lain untuk evaluasi dan penyetelan:
// BM25 (frekuensi kata jenuh + normalisasi panjang dokumen), gabungan BM25 + TF-IDF lewat Reciprocal Rank
// Fusion (RRF), dan perluasan kueri dari umpan balik semu (istilah teratas dari hasil awal ditambahkan ke kueri
// agar kueri yang maknanya sama tetapi katanya beda tetap bertemu). Semuanya murni Go, tanpa berkas model.

// Metode relevansi leksikal (RankWeights.Lexical).
const (
	LexicalTFIDF = "tfidf" // kosinus TF-IDF (bawaan; nilai kosong berarti ini)
	LexicalBM25  = "bm25"  // BM25 Okapi, dinormalisasi terhadap skor tertinggi kueri
	LexicalRRF   = "rrf"   // Reciprocal Rank Fusion atas TF-IDF, BM25, dan DenseRanker (bila dipasang)
)

const (
	// Parameter BM25 disetel pada bagian latih set evaluasi: normalisasi panjang penuh (b = 1) lebih baik daripada
	// nilai buku 0.75 karena node pendek (patokan, checklist) lebih sering dipakai daripada node panjang.
	bm25K1   = 2.0
	bm25B    = 1.0
	rrfK     = 60  // konstanta RRF baku (Cormack dkk. 2009)
	rrfDepth = 200 // hanya peringkat 200 teratas tiap daftar yang ikut digabung
)

// DenseRanker adalah titik sambung relevansi semantik (mis. embedding statis): bila dipasang lewat
// SetDenseRanker, peringkatnya ikut digabung pada metode LexicalRRF. Paket ini tidak menyediakan implementasinya.
type DenseRanker interface {
	// Rank mengembalikan paling banyak k id node, urut dari yang paling mirip dengan text; hanya id yang lolos
	// keep yang boleh dikembalikan.
	Rank(text string, keep func(id uint) bool, k int) []uint
}

var (
	denseMu sync.RWMutex
	dense   DenseRanker
)

// SetDenseRanker memasang (atau mencabut, dengan nil) peringkat semantik tambahan untuk LexicalRRF.
func SetDenseRanker(r DenseRanker) {
	denseMu.Lock()
	dense = r
	denseMu.Unlock()
}

func currentDense() DenseRanker {
	denseMu.RLock()
	defer denseMu.RUnlock()
	return dense
}

// lexical menghitung relevansi mentah tiap node yang cocok dengan kueri (sebelum bonus domain). skip menyaring
// node yang tidak boleh menjadi kandidat. Metode bawaan tanpa umpan balik sama persis dengan peringkat lama.
func (ix *index) lexical(text, domains string, w RankWeights, skip func(*entry) bool) map[uint]float64 {
	tf := termFreq(text, "", domains, nil)
	switch w.Lexical {
	case LexicalBM25:
		qw := ix.bm25Query(tf)
		s := ix.bm25(qw, skip)
		if w.Feedback > 0 {
			ix.addFeedback(qw, top(s, w.Feedback), w)
			s = ix.bm25(qw, skip)
		}
		return normMax(s)
	case LexicalRRF:
		qv := ix.weigh(tf, false)
		qw := ix.bm25Query(tf)
		if w.Feedback > 0 {
			seed := top(ix.bm25(qw, skip), w.Feedback)
			ix.addFeedback(qw, seed, w)
			qv = ix.expandVector(qv, seed, w)
		}
		lists := [][]uint{top(ix.cosine(qv, skip), rrfDepth), top(ix.bm25(qw, skip), rrfDepth)}
		if d := currentDense(); d != nil {
			lists = append(lists, d.Rank(text, func(id uint) bool {
				e := ix.byID[id]
				return e != nil && (skip == nil || !skip(e))
			}, rrfDepth))
		}
		return fuse(lists)
	default:
		qv := ix.weigh(tf, false)
		if w.Feedback > 0 {
			qv = ix.expandVector(qv, top(ix.cosine(qv, skip), w.Feedback), w)
		}
		return ix.cosine(qv, skip)
	}
}

// cosine: kosinus TF-IDF terhadap semua node yang berbagi kata (ambang 0.0001 seperti peringkat lama).
func (ix *index) cosine(qv vec, skip func(*entry) bool) map[uint]float64 {
	out := map[uint]float64{}
	for _, nb := range ix.nearest(qv, 0, 0.0001, skip) {
		out[nb.e.id] = nb.sim
	}
	return out
}

// bm25Query: bobot kata kueri = 1 + ln(frekuensi); kata yang tidak ada di korpus dibuang.
func (ix *index) bm25Query(tf map[string]float64) map[string]float64 {
	qw := map[string]float64{}
	for t, f := range tf {
		if ix.df[t] > 0 {
			qw[t] = 1 + math.Log(f)
		}
	}
	return qw
}

// bm25 menghitung skor BM25 mentah. Kata dijumlahkan berurutan agar hasilnya deterministik.
func (ix *index) bm25(qw map[string]float64, skip func(*entry) bool) map[uint]float64 {
	acc := map[int]float64{}
	n := float64(ix.n)
	for _, t := range sortedTerms(qw) {
		df := float64(ix.df[t])
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for _, i := range ix.postings[t] {
			e := ix.entries[i]
			f := e.tf[t]
			norm := 1 - bm25B + bm25B*e.dl/math.Max(ix.avgdl, 1)
			acc[i] += qw[t] * idf * f * (bm25K1 + 1) / (f + bm25K1*norm)
		}
	}
	out := make(map[uint]float64, len(acc))
	for i, s := range acc {
		if e := ix.entries[i]; s > 0 && (skip == nil || !skip(e)) {
			out[e.id] = s
		}
	}
	return out
}

// feedbackTerms: istilah pembeda terkuat di node hasil awal (centroid vektor TF-IDF), tanpa kata yang sudah ada
// di kueri dan tanpa kata yang muncul di lebih dari separuh korpus. Bobot dinormalisasi ke 1 untuk istilah teratas.
func (ix *index) feedbackTerms(have func(string) bool, seed []uint, terms int) map[string]float64 {
	cent := map[string]float64{}
	for _, id := range seed {
		if e := ix.byID[id]; e != nil {
			for t, x := range e.v {
				cent[t] += x
			}
		}
	}
	cands := make([]string, 0, len(cent))
	for _, t := range sortedTerms(cent) {
		if !have(t) && ix.df[t]*2 <= ix.n {
			cands = append(cands, t)
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cent[cands[i]] > cent[cands[j]] })
	if len(cands) > terms {
		cands = cands[:terms]
	}
	out := map[string]float64{}
	if len(cands) == 0 {
		return out
	}
	maxW := cent[cands[0]]
	for _, t := range cands {
		out[t] = cent[t] / maxW
	}
	return out
}

func feedbackSize(w RankWeights) (int, float64) {
	terms, beta := w.FeedbackTerms, w.FeedbackWeight
	if terms <= 0 {
		terms = 10
	}
	if beta <= 0 {
		beta = 0.3
	}
	return terms, beta
}

// addFeedback menambahkan istilah umpan balik ke kueri BM25 dengan bobot beta × bobot relatifnya.
func (ix *index) addFeedback(qw map[string]float64, seed []uint, w RankWeights) {
	terms, beta := feedbackSize(w)
	for t, x := range ix.feedbackTerms(func(t string) bool { _, ok := qw[t]; return ok }, seed, terms) {
		qw[t] += beta * x
	}
}

// expandVector: kueri TF-IDF + beta × centroid istilah umpan balik (ternormalisasi), lalu dinormalisasi ulang.
func (ix *index) expandVector(qv vec, seed []uint, w RankWeights) vec {
	terms, beta := feedbackSize(w)
	fb := ix.feedbackTerms(func(t string) bool { _, ok := qv[t]; return ok }, seed, terms)
	if len(fb) == 0 {
		return qv
	}
	fn := 0.0
	for _, x := range fb {
		fn += x * x
	}
	fn = math.Sqrt(fn)
	out := vec{}
	for t, x := range qv {
		out[t] = x
	}
	for t, x := range fb {
		out[t] += beta * x / fn
	}
	norm := 0.0
	for _, t := range sortedTerms(out) {
		norm += out[t] * out[t]
	}
	norm = math.Sqrt(norm)
	for t := range out {
		out[t] /= norm
	}
	return out
}

// top: n id dengan skor tertinggi (seri dipecah id terkecil).
func top(s map[uint]float64, n int) []uint {
	ids := make([]uint, 0, len(s))
	for id := range s {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return s[ids[i]] > s[ids[j]] || s[ids[i]] == s[ids[j]] && ids[i] < ids[j] })
	if n > 0 && len(ids) > n {
		ids = ids[:n]
	}
	return ids
}

// normMax membagi skor dengan skor tertinggi (0–1).
func normMax(s map[uint]float64) map[uint]float64 {
	m := 0.0
	for _, x := range s {
		m = math.Max(m, x)
	}
	if m == 0 {
		return s
	}
	for id := range s {
		s[id] /= m
	}
	return s
}

// fuse: Reciprocal Rank Fusion, Σ 1/(k + peringkat), dibagi nilai maksimum yang mungkin (peringkat 1 di semua
// daftar = 1) agar sebanding dengan ambang relevansi.
func fuse(lists [][]uint) map[uint]float64 {
	out := map[uint]float64{}
	nonEmpty := 0
	for _, l := range lists {
		if len(l) > 0 {
			nonEmpty++
		}
		for r, id := range l {
			out[id] += 1 / float64(rrfK+r+1)
		}
	}
	if nonEmpty == 0 {
		return out
	}
	best := float64(nonEmpty) / float64(rrfK+1)
	for id := range out {
		out[id] /= best
	}
	return out
}
