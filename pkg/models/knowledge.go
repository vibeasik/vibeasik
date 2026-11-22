package models

import "time"

// Pusat Pengetahuan (satu pintu): semua patokan & pengetahuan UMUM pekerja AI vibeasik — rubrik guideline,
// checklist audit, pelajaran vonis audit, keahlian pegawai, hasil belajar teknologi baru — disimpan sebagai
// node yang bisa dicari (vektor TF-IDF), bertaut (link), berkelompok (klaster), dan belajar dari hasil kerja
// (penghitung helpful/harmful). Fakta khusus satu proyek tetap milik dewan eksekutif (ExecutiveKnowledge).

// Jenis node (tipe memori ala CoALA: semantik = rule/concept, prosedural = procedure, episodik → lesson).
const (
	KnowledgeKindRule      = "rule"      // patokan normatif: yang wajib/dilarang
	KnowledgeKindProcedure = "procedure" // cara mengerjakan: langkah, pola, perintah
	KnowledgeKindLesson    = "lesson"    // pelajaran dari pengalaman (berhasil/gagal, audit keliru/benar)
	KnowledgeKindConcept   = "concept"   // pengetahuan dasar: bahasa, framework, metode
)

// KnowledgeKinds: jenis node yang sah.
var KnowledgeKinds = []string{KnowledgeKindRule, KnowledgeKindProcedure, KnowledgeKindLesson, KnowledgeKindConcept}

// Asal node.
const (
	KnowledgeSrcGuideline = "guideline"       // berkas guidelines/*.json (sumber kebenaran tetap berkasnya)
	KnowledgeSrcChecklist = "audit_checklist" // checklist review audit LLM per ekosistem
	KnowledgeSrcVerdict   = "audit_verdict"   // pelajaran dari vonis audit keliru/benar
	KnowledgeSrcEmployee  = "employee"        // cermin pengetahuan pribadi pegawai (employee_knowledge)
	KnowledgeSrcPromoted  = "promoted"        // pengetahuan pribadi yang terbukti berguna lalu dibagikan ke kantor
	KnowledgeSrcGap       = "gap_research"    // hasil pegawai mempelajari teknologi/metode baru (celah pengetahuan)
	KnowledgeSrcReview    = "qa_review"       // best practice dari penilai QA (model ≥ Opus 5.5) atas hasil kerja
	KnowledgeSrcDesign    = "design_library"  // cermin Galeri Desain: kesimpulan per jenis website, pola teramati, referensi
	KnowledgeSrcTheory    = "design_theory"   // teori desain & teori warna (designlib/theory.json) bertaut ke contoh di Galeri Desain
	KnowledgeSrcLesson    = "design_lesson"   // pelajaran desain before/after (gambar awal, gambar akhir, narasi konten)
	KnowledgeSrcHub       = "hub"             // paket Pusat Pengetahuan vibeasik (ditarik dari server, bisa diperbarui)
	KnowledgeSrcManual    = "manual"          // diajarkan manusia
	KnowledgeSrcAgent     = "agent"           // dicatat agent coding lewat MCP
)

// AuthoritativeSources: sumber yang berkedudukan di atas pengetahuan umum LLM dan pegawai. Galeri Desain adalah
// pengetahuan hasil mempelajari website nyata, jadi bukti ✓/✗ dari nilai QA pegawai tidak mengubah bobotnya dan
// temu-kembali memberinya utilitas penuh. Yang dinilai QA adalah kepatuhan pegawai pada galeri, bukan galerinya.
var AuthoritativeSources = []string{KnowledgeSrcDesign}

// KnowledgeAuthoritative: node dari sumber ini tidak ikut dinilai oleh hasil kerja pegawai.
func KnowledgeAuthoritative(source string) bool {
	for _, s := range AuthoritativeSources {
		if s == source {
			return true
		}
	}
	return false
}

// Status node.
const (
	KnowledgeNodeActive  = "active"
	KnowledgeNodeMerged  = "merged"  // dilebur ke node lain yang hampir sama
	KnowledgeNodeRetired = "retired" // terbukti merugikan / sumbernya hilang / dihapus manusia
)

// KnowledgeNode adalah satu butir pengetahuan di Pusat Pengetahuan.
type KnowledgeNode struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Key: identitas di sumbernya (guideline:SEC-001, audit:go:2, employee:15, promoted:40); kosong = node asli pusat.
	Key     string   `gorm:"column:source_key;size:191;index" json:"key,omitempty"` // `key` kata tercadang MySQL
	Kind    string   `gorm:"size:16;index" json:"kind"`
	Domain  string   `gorm:"size:48;index" json:"domain"` // ekosistem (go, web, flutter, …) atau area (security, testing, …)
	Tags    []string `gorm:"type:text;serializer:json" json:"tags"`
	Title   string   `gorm:"size:191" json:"title"`
	Content string   `gorm:"type:text" json:"content"`
	Source  string   `gorm:"size:32;index" json:"source"`
	// OwnerEmployeeID: pengetahuan pribadi pegawai; nil = pustaka bersama seluruh kantor.
	OwnerEmployeeID *uint  `gorm:"index" json:"owner_employee_id,omitempty"`
	OwnerName       string `gorm:"size:128" json:"owner_name,omitempty"`
	// Weight: kepentingan awal 0–100. Helpful/Harmful: bukti KUAT — vonis per node (penilai QA, umpan balik,
	// vonis audit) atau node yang dikutip agent di baris PENGETAHUAN todo yang lulus. WeakHelpful/WeakHarmful:
	// bukti LEMAH — node hanya ikut disisipkan ke pekerjaan yang hasilnya bagus/buruk (lihat KnowledgeWeakWeight).
	Weight       int        `json:"weight"`
	Helpful      int        `json:"helpful"`
	Harmful      int        `json:"harmful"`
	WeakHelpful  int        `gorm:"default:0" json:"weak_helpful"`
	WeakHarmful  int        `gorm:"default:0" json:"weak_harmful"`
	Uses         int        `json:"uses"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	ClusterID    uint       `gorm:"index" json:"cluster_id"`
	MergedIntoID *uint      `json:"merged_into_id,omitempty"`
	// NeedsReview: patokan bersumber berkas yang berkali-kali terbukti keliru — berkasnya perlu ditinjau manusia/agent.
	NeedsReview  bool   `gorm:"index" json:"needs_review"`
	Status       string `gorm:"size:16;index;default:'active'" json:"status"`
	StatusReason string `gorm:"type:text" json:"status_reason,omitempty"`
	// Masa berlaku. Node yang digantikan atau lewat masa berlakunya tetap berstatus aktif dan tersimpan utuh,
	// hanya tidak dikirim temu-kembali. AppliesTo: batas berlaku dalam teks pendek (go>=1.26, mcp 2025-06-18),
	// ditampilkan ke agent. ValidUntil: sesudah waktu ini node tidak dikirim dan kurator menandainya perlu
	// ditinjau. SupersededByID: node pengganti (selalu ujung rantai); berlaku selama penggantinya aktif.
	AppliesTo      string     `gorm:"size:64" json:"applies_to,omitempty"`
	ValidUntil     *time.Time `json:"valid_until,omitempty"`
	SupersededByID *uint      `gorm:"index" json:"superseded_by_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Expired: masa berlaku node sudah lewat pada waktu now.
func (n KnowledgeNode) Expired(now time.Time) bool {
	return n.ValidUntil != nil && !now.Before(*n.ValidUntil)
}

// KnowledgeWeakWeight: bobot satu bukti lemah terhadap satu bukti kuat di Utility. Satu nilai todo dibagikan ke
// belasan sampai puluhan node yang disisipkan, padahal yang benar-benar diikuti agent biasanya hanya beberapa;
// 0.25 berarti empat hasil todo yang "ikut dinikmati" setara satu vonis per node. Cukup untuk membedakan node
// yang selalu hadir di pekerjaan bagus dari yang selalu hadir di pekerjaan buruk, tetapi tidak bisa menyusul
// node yang punya vonis eksplisit.
const KnowledgeWeakWeight = 0.25

// Utility: rata-rata posterior Beta(helpful+1+w·weak_helpful, harmful+1+w·weak_harmful), w = KnowledgeWeakWeight —
// 0.5 untuk node yang belum teruji. Tanpa bukti lemah nilainya sama dengan StrongUtility.
func (n KnowledgeNode) Utility() float64 {
	if n.WeakHelpful == 0 && n.WeakHarmful == 0 {
		return n.StrongUtility()
	}
	a := float64(n.Helpful+1) + KnowledgeWeakWeight*float64(n.WeakHelpful)
	b := float64(n.Harmful+1) + KnowledgeWeakWeight*float64(n.WeakHarmful)
	return a / (a + b)
}

// StrongUtility: rata-rata posterior Beta(helpful+1, harmful+1) atas bukti kuat saja — dasar promosi & pensiun.
func (n KnowledgeNode) StrongUtility() float64 {
	return float64(n.Helpful+1) / float64(n.Helpful+n.Harmful+2)
}

// KnowledgeLink: tautan antar node yang isinya berdekatan (gaya Zettelkasten / A-MEM), dipakai memperluas pencarian.
type KnowledgeLink struct {
	ID       uint    `gorm:"primaryKey" json:"id"`
	FromID   uint    `gorm:"index" json:"from_id"`
	ToID     uint    `gorm:"index" json:"to_id"`
	Strength float64 `json:"strength"` // kemiripan kosinus 0–1
}

// KnowledgeCluster: kelompok node yang membahas konsep yang sama (dihitung ulang tiap kurasi).
type KnowledgeCluster struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Label     string    `gorm:"size:191" json:"label"`
	Domain    string    `gorm:"size:48" json:"domain"`
	Terms     []string  `gorm:"type:text;serializer:json" json:"terms"`
	Size      int       `json:"size"`
	Utility   float64   `json:"utility"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Hasil pemakaian node.
const (
	KnowledgeOutcomeHelpful = "helpful"
	KnowledgeOutcomeHarmful = "harmful"
	KnowledgeOutcomeNeutral = "neutral"
)

// Asal bukti sebuah pemakaian (KnowledgeUse.Evidence). Verdict & cited = bukti kuat; outcome = bukti lemah.
const (
	KnowledgeEvidenceVerdict = "verdict" // vonis per node dari penilai (Judge)
	KnowledgeEvidenceCited   = "cited"   // dikutip agent di baris PENGETAHUAN catatan penyelesaian todo yang lulus
	KnowledgeEvidenceOutcome = "outcome" // hanya ikut hasil pekerjaan tempat node itu disisipkan
)

// KnowledgeUse: node yang disisipkan ke prompt sebuah pekerjaan; diberi nilai saat hasil kerjanya diketahui.
type KnowledgeUse struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	NodeID     uint   `gorm:"index" json:"node_id"`
	Context    string `gorm:"size:191;index" json:"context"` // todo:12, audit:3:path/berkas.go, plan:4
	EmployeeID *uint  `json:"employee_id,omitempty"`
	Outcome    string `gorm:"size:16;index" json:"outcome,omitempty"`
	// Evidence: asal bukti outcome (KnowledgeEvidence*); kosong = belum dinilai, ditutup netral, atau catatan lama.
	Evidence  string    `gorm:"size:16;index;default:''" json:"evidence,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// KnowledgeAdoption: pegawai memakai pengalaman pribadi pegawai lain lewat rujukan (bukan salinan), sehingga bukti
// dari pekerjaan penerima kembali ke butir yang sama. Penerima menjalani masa coba lalu mempertahankan atau membuang.
type KnowledgeAdoption struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	NodeID     uint   `gorm:"uniqueIndex:idx_knowledge_adoption_node_emp" json:"node_id"`
	EmployeeID uint   `gorm:"uniqueIndex:idx_knowledge_adoption_node_emp;index" json:"employee_id"`
	Status     string `gorm:"size:16;index" json:"status"` // KnowledgeAdoption*
	// Uses: berapa kali disodorkan ke penerima; Cited: dikutip di catatan penyelesaian penerima;
	// Helpful/Harmful: dikutip di todo penerima yang lulus / ditandai KELIRU atau dinilai keliru oleh penilai.
	Uses      int        `json:"uses"`
	Cited     int        `json:"cited"`
	Helpful   int        `json:"helpful"`
	Harmful   int        `json:"harmful"`
	Reason    string     `gorm:"size:255" json:"reason,omitempty"`
	DecidedAt *time.Time `json:"decided_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Status adopsi pengalaman.
const (
	KnowledgeAdoptionTrial   = "trial"   // ditawarkan dan sedang dicoba penerima
	KnowledgeAdoptionKept    = "kept"    // terbukti cocok untuk penerima
	KnowledgeAdoptionDropped = "dropped" // dibuang penerima (tidak cocok, keliru, atau tidak pernah dipakai)
)

// KnowledgeVerdict: vonis terakhir penilai atas satu node di satu konteks. Satu pekerjaan memberi paling banyak
// satu bukti kuat per node walaupun dinilai berkali-kali (putaran QA gate, review berkala); vonis yang berubah
// memindahkan buktinya, bukan menambah. Tidak ikut dibuang bersama catatan pemakaian.
type KnowledgeVerdict struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Context   string    `gorm:"size:191;uniqueIndex:idx_knowledge_verdict_ctx_node" json:"context"`
	NodeID    uint      `gorm:"uniqueIndex:idx_knowledge_verdict_ctx_node" json:"node_id"`
	Helpful   bool      `json:"helpful"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Status celah pengetahuan.
const (
	KnowledgeGapOpen      = "open"      // terdeteksi, belum ada yang mempelajari
	KnowledgeGapLearning  = "learning"  // agenda belajar pegawai sudah dibuat
	KnowledgeGapLearned   = "learned"   // node baru sudah masuk pusat pengetahuan
	KnowledgeGapDismissed = "dismissed" // diabaikan manusia / gagal berulang
)

// KnowledgeGap: teknologi/metode yang muncul di pekerjaan tetapi belum punya pengetahuan di kantor.
type KnowledgeGap struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Domain       string     `gorm:"size:48;index" json:"domain"`
	Topic        string     `gorm:"size:191" json:"topic"`
	Sample       string     `gorm:"type:text" json:"sample,omitempty"` // contoh konteks tempat celah terlihat
	Hits         int        `json:"hits"`
	Auto         bool       `json:"auto"` // terdeteksi otomatis: ekosistem yang muncul di pekerjaan tanpa satu node pun
	Attempts     int        `json:"attempts"`
	Status       string     `gorm:"size:16;index;default:'open'" json:"status"`
	EmployeeID   *uint      `json:"employee_id,omitempty"`
	EmployeeName string     `gorm:"size:128" json:"employee_name,omitempty"`
	AgendaID     *uint      `json:"agenda_id,omitempty"`
	NodesLearned int        `json:"nodes_learned"`
	Note         string     `gorm:"type:text" json:"note,omitempty"`
	LearnedAt    *time.Time `json:"learned_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
