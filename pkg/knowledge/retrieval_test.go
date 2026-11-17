package knowledge

import (
	"reflect"
	"testing"
	"time"
)

func retrievalDB(t *testing.T) (*index, []uint) {
	t.Helper()
	db := testDB(t)
	notes := []Note{
		{Title: "Batasi ukuran unggahan multipart", Content: "Tolak unggahan multipart di atas batas sebelum dibaca ke memori.", Domain: "security"},
		{Title: "Rotasi token sesi setelah login", Content: "Token sesi dirotasi setelah login berhasil agar fiksasi sesi gagal.", Domain: "security"},
		{Title: "Indeks komposit untuk kueri laporan", Content: "Kueri laporan bulanan memakai indeks komposit tanggal dan status.", Domain: "database"},
		{Title: "Tutup rows setelah kueri", Content: "Setiap rows hasil kueri ditutup dengan defer rows.Close agar koneksi tidak bocor.", Domain: "go"},
	}
	var ids []uint
	for _, n := range notes {
		ids = append(ids, mustLearn(t, db, n).ID)
	}
	return current(db), ids
}

func TestBM25PrefersMatchingNode(t *testing.T) {
	ix, ids := retrievalDB(t)
	s := ix.lexical("unggahan multipart terlalu besar", "", RankWeights{Lexical: LexicalBM25}, nil)
	if got := top(s, 1); len(got) != 1 || got[0] != ids[0] {
		t.Fatalf("bm25 teratas = %v, mau %d (skor %v)", got, ids[0], s)
	}
	if s[ids[0]] != 1 {
		t.Fatalf("bm25 harus dinormalisasi ke 1 untuk node teratas, dapat %v", s[ids[0]])
	}
	// Penyaring kandidat berlaku.
	if s := ix.lexical("unggahan multipart", "", RankWeights{Lexical: LexicalBM25}, func(e *entry) bool { return e.id == ids[0] }); s[ids[0]] != 0 {
		t.Fatalf("node yang disaring masih dinilai: %v", s)
	}
	// Bawaan (kosong) sama persis dengan kosinus TF-IDF lama.
	old := map[uint]float64{}
	for _, nb := range ix.nearest(ix.queryVector("unggahan multipart", "", "", nil), 0, 0.0001, nil) {
		old[nb.e.id] = nb.sim
	}
	if got := ix.lexical("unggahan multipart", "", RankWeights{}, nil); !reflect.DeepEqual(got, old) {
		t.Fatalf("relevansi bawaan berubah: %v ≠ %v", got, old)
	}
}

func TestFeedbackAddsTermsFromTopNodes(t *testing.T) {
	ix, ids := retrievalDB(t)
	// "rows" hanya cocok dengan satu node; perluasan menambahkan kata judul node itu ("tutup") ke kueri.
	qv := ix.queryVector("rows", "", "", nil)
	ex := ix.expandVector(qv, []uint{ids[3]}, RankWeights{Feedback: 1})
	if ex["tutup"] == 0 || ex["rows"] <= ex["tutup"] {
		t.Fatalf("perluasan kueri = %v", ex)
	}
	qw := ix.bm25Query(termFreq("rows", "", "", nil))
	ix.addFeedback(qw, []uint{ids[3]}, RankWeights{Feedback: 1, FeedbackTerms: 2})
	if len(qw) != 3 || qw["rows"] <= 0 {
		t.Fatalf("kueri bm25 diperluas = %v", qw)
	}
}

func TestFuseReciprocalRank(t *testing.T) {
	got := fuse([][]uint{{1, 2, 3}, {2, 1}})
	if !near(got[1], got[2]) || got[1] <= got[3] {
		t.Fatalf("fuse = %v", got)
	}
	if want := (1.0/61 + 1.0/62) / (2.0 / 61); !near(got[1], want) {
		t.Fatalf("fuse[1] = %v, mau %v", got[1], want)
	}
}

type fakeDense []uint

func (f fakeDense) Rank(_ string, keep func(uint) bool, k int) []uint {
	var out []uint
	for _, id := range f {
		if keep(id) && len(out) < k {
			out = append(out, id)
		}
	}
	return out
}

func TestDenseRankerJoinsFusion(t *testing.T) {
	ix, ids := retrievalDB(t)
	w := RankWeights{Lexical: LexicalRRF}
	before := ix.lexical("unggahan multipart", "", w, nil)
	if before[ids[2]] != 0 {
		t.Fatalf("prasyarat: node laporan tidak cocok secara leksikal, dapat %v", before)
	}
	SetDenseRanker(fakeDense{ids[2]})
	defer SetDenseRanker(nil)
	if after := ix.lexical("unggahan multipart", "", w, nil); after[ids[2]] == 0 {
		t.Fatalf("peringkat semantik tidak ikut digabung: %v", after)
	}
	// Penyaring kandidat juga berlaku untuk peringkat semantik.
	if s := ix.lexical("unggahan multipart", "", w, func(e *entry) bool { return e.id == ids[2] }); s[ids[2]] != 0 {
		t.Fatalf("node yang disaring masuk lewat peringkat semantik: %v", s)
	}
}

func TestTrimTailAndCapLinked(t *testing.T) {
	h := []Hit{{Score: 0.9}, {Score: 0.8, Via: "tautan"}, {Score: 0.7}, {Score: 0.5, Via: "tautan"}, {Score: 0.4}}
	for i := range h {
		h[i].ID = uint(i + 1)
	}
	ids := func(h []Hit) []uint {
		var out []uint
		for _, x := range h {
			out = append(out, x.ID)
		}
		return out
	}
	if got := ids(capLinked(h, 1)); !reflect.DeepEqual(got, []uint{1, 2, 3, 5}) {
		t.Fatalf("capLinked 1 = %v", got)
	}
	if got := ids(capLinked(h, -1)); !reflect.DeepEqual(got, []uint{1, 3, 5}) {
		t.Fatalf("capLinked -1 = %v", got)
	}
	if got := ids(capLinked(h, 0)); len(got) != 5 {
		t.Fatalf("capLinked 0 harus tanpa batas: %v", got)
	}
	if got := ids(trimTail(append([]Hit(nil), h...), RankWeights{TailRatio: 0.75})); !reflect.DeepEqual(got, []uint{1, 2, 3}) {
		t.Fatalf("trimTail rasio = %v", got)
	}
	if got := ids(trimTail(append([]Hit(nil), h...), RankWeights{MinScore: 0.95})); !reflect.DeepEqual(got, []uint{1}) {
		t.Fatalf("butir teratas harus tetap ada: %v", got)
	}
}

func TestRankMaxHitsAndDefaults(t *testing.T) {
	db := testDB(t)
	for _, n := range []Note{
		{Title: "Validasi input handler", Content: "Validasi input handler sebelum dipakai."},
		{Title: "Validasi input form", Content: "Validasi input form di sisi server."},
		{Title: "Validasi input berkas", Content: "Validasi input berkas unggahan."},
	} {
		mustLearn(t, db, n)
	}
	ix := current(db)
	q := Query{Text: "validasi input", Limit: 10}
	if got := rank(db, ix, q, DefaultRankWeights, time.Now(), nil); len(got.Hits) != 3 {
		t.Fatalf("bawaan = %d butir", len(got.Hits))
	}
	w := DefaultRankWeights
	w.MaxHits = 2
	if got := rank(db, ix, q, w, time.Now(), nil); len(got.Hits) != 2 {
		t.Fatalf("MaxHits 2 = %d butir", len(got.Hits))
	}
}
