// Package knowledge adalah Pusat Pengetahuan vibeasik — SATU PINTU untuk semua patokan & pengetahuan umum pekerja
// AI (audit, rapat perencanaan, pegawai). Rancangannya merangkum riset memori agent:
//
//   - Node bergaya Zettelkasten yang saling bertaut & berevolusi (A-MEM, Xu dkk., NeurIPS 2025).
//   - Skor temu-kembali = relevansi + kepentingan + kebaruan (Generative Agents, Park dkk., 2023).
//   - Setiap butir punya penghitung helpful/harmful yang diperbarui dari hasil kerja; butir yang terbukti
//     merugikan dipangkas, pembaruan kecil (delta) alih-alih menulis ulang (ACE, Zhang dkk., ICLR 2026;
//     ExpeL, Zhao dkk., 2024).
//   - Pustaka keahlian yang tumbuh saat menemui hal baru: celah pengetahuan → agenda belajar → node baru
//     (Voyager, Wang dkk., 2023).
//   - Klaster konsep untuk navigasi & keberagaman hasil (RAPTOR/GraphRAG, versi ringan tanpa LLM).
//
// Vektornya TF-IDF murni Go (lihat vector.go) sehingga jalan tanpa layanan embedding. Fakta khusus satu proyek
// TIDAK disimpan di sini — itu milik dewan eksekutif proyek (staff.RecordProjectFact / ExecutiveKnowledge).
package knowledge

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Note adalah butir pengetahuan yang akan disimpan (Learn) atau dicerminkan dari sumbernya (sync).
type Note struct {
	Key             string   `json:"key,omitempty"`
	Kind            string   `json:"kind"`
	Domain          string   `json:"domain"`
	Title           string   `json:"title"`
	Content         string   `json:"content"`
	Source          string   `json:"source"`
	Tags            []string `json:"tags,omitempty"`
	OwnerEmployeeID *uint    `json:"owner_employee_id,omitempty"`
	OwnerName       string   `json:"owner_name,omitempty"`
	Weight          int      `json:"weight"`
}

const (
	maxTitle    = 180
	maxContent  = 2000
	dupMin      = 0.8  // kemiripan minimal untuk dianggap butir yang sama (diperkuat, bukan digandakan)
	linkMin     = 0.25 // kemiripan minimal untuk menautkan dua node
	linksPerNew = 3
)

func (n *Note) normalize() error {
	n.Title = clip(n.Title, maxTitle)
	n.Content = clip(n.Content, maxContent)
	if n.Title == "" || n.Content == "" {
		return errors.New("judul dan isi pengetahuan wajib diisi")
	}
	n.Kind = strings.ToLower(strings.TrimSpace(n.Kind))
	if !validKind(n.Kind) {
		n.Kind = models.KnowledgeKindLesson
	}
	n.Domain = CanonicalDomain(n.Domain)
	if n.Domain == "" {
		n.Domain = primaryDomain(n.Title + "\n" + n.Content)
	}
	if n.Source == "" {
		n.Source = models.KnowledgeSrcManual
	}
	if n.Weight <= 0 {
		n.Weight = 60
	}
	n.Weight = clamp(n.Weight, 1, 100)
	return nil
}

func validKind(k string) bool {
	for _, x := range models.KnowledgeKinds {
		if x == k {
			return true
		}
	}
	return false
}

func ownerOf(p *uint) uint {
	if p == nil {
		return 0
	}
	return *p
}

// Learn adalah pintu tulis node asli pusat. Bila di lingkup yang sama (pustaka bersama, atau pengetahuan pribadi
// pegawai yang sama) sudah ada butir yang hampir sama, butir itu diperkuat (bukti +1, bobot naik, isi dilengkapi)
// alih-alih digandakan. Node baru langsung ditautkan ke tetangga terdekatnya & mengisi celah domainnya.
// Mengembalikan node yang tersimpan dan apakah ia peleburan ke node lama.
func Learn(db *gorm.DB, n Note) (*models.KnowledgeNode, bool, error) {
	if db == nil {
		return nil, false, errors.New("database belum siap")
	}
	n.Key = ""
	if err := n.normalize(); err != nil {
		return nil, false, err
	}
	ix := current(db)
	v := ix.noteVector(n.Title, n.Content, n.Domain, n.Tags)
	owner := ownerOf(n.OwnerEmployeeID)
	dups := ix.nearest(v, 1, dupMin, func(e *entry) bool { return e.owner != owner })
	if len(dups) > 0 {
		var old models.KnowledgeNode
		if db.First(&old, dups[0].e.id).Error == nil {
			return reinforce(db, &old, n), true, nil
		}
	}
	node := models.KnowledgeNode{
		Kind: n.Kind, Domain: n.Domain, Tags: n.Tags, Title: n.Title, Content: n.Content, Source: n.Source,
		OwnerEmployeeID: n.OwnerEmployeeID, OwnerName: clip(n.OwnerName, 120), Weight: n.Weight, Status: models.KnowledgeNodeActive,
	}
	if err := db.Create(&node).Error; err != nil {
		return nil, false, err
	}
	var links []models.KnowledgeLink
	for _, nb := range ix.nearest(v, linksPerNew, linkMin, func(e *entry) bool { return e.owner != 0 && e.owner != owner }) {
		links = append(links, models.KnowledgeLink{FromID: node.ID, ToID: nb.e.id, Strength: round3(nb.sim)})
	}
	if len(links) > 0 {
		db.Create(&links)
	}
	invalidate()
	fillGaps(db, node.Domain)
	return &node, false, nil
}

// reinforce: butir yang muncul lagi = bukti tambahan. Node bersumber berkas (guideline, checklist, pegawai) hanya
// diberi bukti; isinya tetap milik sumbernya.
func reinforce(db *gorm.DB, old *models.KnowledgeNode, n Note) *models.KnowledgeNode {
	updates := map[string]interface{}{"helpful": gorm.Expr("helpful + 1")}
	old.Helpful++
	if native(old.Key) {
		old.Weight = clamp(max(old.Weight, n.Weight)+3, 1, 99)
		old.Content = synthesize(old.Content, n.Content)
		updates["weight"], updates["content"] = old.Weight, old.Content
		db.Model(old).Updates(updates)
		invalidate()
		return old
	}
	db.Model(old).UpdateColumns(updates)
	return old
}

// native: node milik pusat sendiri (bukan cermin berkas/pegawai) — boleh dilebur, diperkuat isinya, dipangkas.
func native(key string) bool { return key == "" || strings.HasPrefix(key, "promoted:") }

func synthesize(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	la, lb := strings.ToLower(a), strings.ToLower(b)
	switch {
	case b == "" || strings.Contains(la, lb):
		return a
	case a == "" || strings.Contains(lb, la):
		return b
	}
	if !strings.HasSuffix(a, ".") {
		a += "."
	}
	return clip(a+" "+b, maxContent)
}

// upsertKey menyimpan node bersumber (key tetap) tanpa menyentuh penghitung buktinya. Node yang sudah
// dilebur/dipensiunkan tidak dihidupkan lagi di sini.
func upsertKey(db *gorm.DB, n Note) (*models.KnowledgeNode, bool, error) {
	if err := n.normalize(); err != nil {
		return nil, false, err
	}
	var node models.KnowledgeNode
	if db.Where("source_key = ?", n.Key).Limit(1).Find(&node).Error == nil && node.ID != 0 {
		return &node, false, nil
	}
	node = models.KnowledgeNode{
		Key: n.Key, Kind: n.Kind, Domain: n.Domain, Tags: n.Tags, Title: n.Title, Content: n.Content, Source: n.Source,
		OwnerEmployeeID: n.OwnerEmployeeID, OwnerName: clip(n.OwnerName, 120), Weight: n.Weight, Status: models.KnowledgeNodeActive,
	}
	if err := db.Create(&node).Error; err != nil {
		return nil, false, err
	}
	invalidate()
	fillGaps(db, node.Domain)
	return &node, true, nil
}

// syncSet mencerminkan satu sumber (scope = node milik sumber itu) ke pusat: butir baru dibuat, butir yang
// teksnya berubah diperbarui, butir yang hilang dari sumber dipensiunkan, butir yang kembali dihidupkan lagi.
// Penghitung bukti (helpful/harmful/uses) tidak pernah ditimpa. Mengembalikan jumlah node yang berubah.
func syncSet(db *gorm.DB, scope func(*gorm.DB) *gorm.DB, notes []Note) int {
	var existing []models.KnowledgeNode
	scope(db.Model(&models.KnowledgeNode{})).Find(&existing)
	byKey := make(map[string]models.KnowledgeNode, len(existing))
	for _, e := range existing {
		byKey[e.Key] = e
	}
	changed := 0
	seen := map[string]bool{}
	for _, n := range notes {
		if n.Key == "" || seen[n.Key] || n.normalize() != nil {
			continue
		}
		seen[n.Key] = true
		old, ok := byKey[n.Key]
		if !ok {
			row := models.KnowledgeNode{
				Key: n.Key, Kind: n.Kind, Domain: n.Domain, Tags: n.Tags, Title: n.Title, Content: n.Content, Source: n.Source,
				OwnerEmployeeID: n.OwnerEmployeeID, OwnerName: clip(n.OwnerName, 120), Weight: n.Weight, Status: models.KnowledgeNodeActive,
			}
			if db.Create(&row).Error == nil {
				changed++
				fillGaps(db, row.Domain)
			}
			continue
		}
		updates := map[string]interface{}{}
		if old.Title != n.Title {
			updates["title"] = n.Title
		}
		if old.Content != n.Content {
			updates["content"] = n.Content
		}
		if old.Domain != n.Domain {
			updates["domain"] = n.Domain
		}
		if old.Kind != n.Kind {
			updates["kind"] = n.Kind
		}
		if old.Weight != n.Weight {
			updates["weight"] = n.Weight
		}
		if old.OwnerName != clip(n.OwnerName, 120) {
			updates["owner_name"] = clip(n.OwnerName, 120)
		}
		if strings.Join(old.Tags, ",") != strings.Join(n.Tags, ",") {
			updates["tags"] = models.JSONValue(n.Tags)
		}
		if old.Status == models.KnowledgeNodeRetired && strings.HasPrefix(old.StatusReason, retiredBySource) {
			updates["status"], updates["status_reason"] = models.KnowledgeNodeActive, ""
		}
		if len(updates) > 0 {
			old.Tags = n.Tags
			db.Model(&old).Updates(updates)
			changed++
		}
	}
	for key, old := range byKey {
		if !seen[key] && old.Status == models.KnowledgeNodeActive {
			db.Model(&old).Updates(map[string]interface{}{"status": models.KnowledgeNodeRetired, "status_reason": retiredBySource + " (butir tidak ada lagi di sumbernya)"})
			changed++
		}
	}
	if changed > 0 {
		invalidate()
	}
	return changed
}

const retiredBySource = "Sumber berubah"

// Retire memensiunkan node (dipakai manusia/agent bila butir keliru). Node bersumber berkas akan dihidupkan lagi
// oleh sinkronisasi selama butirnya masih ada di sumber — ubah berkasnya untuk menghapus permanen.
func Retire(db *gorm.DB, id uint, reason string) error {
	reason = clip(reason, 500)
	if reason == "" {
		reason = "Dipensiunkan manual"
	}
	res := db.Model(&models.KnowledgeNode{}).Where("id = ? AND status = ?", id, models.KnowledgeNodeActive).
		Updates(map[string]interface{}{"status": models.KnowledgeNodeRetired, "status_reason": reason})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("node %d tidak ditemukan atau sudah tidak aktif", id)
	}
	invalidate()
	return nil
}

// Feedback menambah bukti langsung pada satu node (manusia di studio atau agent lewat MCP).
func Feedback(db *gorm.DB, id uint, helpful bool) (*models.KnowledgeNode, error) {
	col := "harmful"
	if helpful {
		col = "helpful"
	}
	res := db.Model(&models.KnowledgeNode{}).Where("id = ?", id).UpdateColumn(col, gorm.Expr(col+" + 1"))
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, fmt.Errorf("node %d tidak ditemukan", id)
	}
	var n models.KnowledgeNode
	db.First(&n, id)
	return &n, nil
}

// recordUse mencatat node yang disisipkan ke sebuah pekerjaan (konteks yang sama tidak dicatat dua kali selama
// belum dinilai) dan menaikkan hitungan pakai & waktu terakhir dipakai.
func recordUse(db *gorm.DB, ctx string, employeeID uint, ids []uint) {
	if ctx == "" || len(ids) == 0 {
		return
	}
	var pending []uint
	db.Model(&models.KnowledgeUse{}).Where("context = ? AND outcome = ?", ctx, "").Pluck("node_id", &pending)
	had := map[uint]bool{}
	for _, id := range pending {
		had[id] = true
	}
	var emp *uint
	if employeeID > 0 {
		emp = &employeeID
	}
	var rows []models.KnowledgeUse
	var fresh []uint
	for _, id := range ids {
		if !had[id] {
			rows = append(rows, models.KnowledgeUse{NodeID: id, Context: clip(ctx, 190), EmployeeID: emp})
			fresh = append(fresh, id)
		}
	}
	if len(rows) == 0 {
		return
	}
	db.Create(&rows)
	db.Model(&models.KnowledgeNode{}).Where("id IN ?", fresh).
		UpdateColumns(map[string]interface{}{"uses": gorm.Expr("uses + 1"), "last_used_at": time.Now()})
}

// Reward menilai node yang dipakai di sebuah konteks begitu hasil kerjanya diketahui (nilai QA 0–100, vonis audit):
// ≥75 = membantu, <50 = merugikan, di antaranya netral. Untuk konteks todo:<id>, node yang dikutip agent di baris
// "PENGETAHUAN: K<id>" catatan penyelesaian todo itu ikut dibaca (lihat RewardCited). Mengembalikan jumlah
// pemakaian yang dinilai.
func Reward(db *gorm.DB, ctx string, score int) int {
	if db == nil || ctx == "" {
		return 0
	}
	return RewardCited(db, ctx, score, TodoCitations(db, ctx))
}

// RewardCited menilai pemakaian yang belum dinilai di ctx dengan membedakan kekuatan buktinya:
//   - hasil bagus: node yang disisipkan DAN dikutip (cited) mendapat ✓ kuat (sekali per konteks); node lain yang
//     hanya ikut disisipkan mendapat ✓ lemah;
//   - hasil buruk: semua node yang disisipkan hanya mendapat ✗ lemah. Nilai todo yang rendah tidak menunjukkan
//     node mana yang keliru (bisa karena build, alat, lingkungan, atau cara kerja agent), jadi ✗ kuat hanya datang
//     dari vonis eksplisit (Judge, Feedback);
//   - node yang sudah divonis penilai di ctx (Judge) tidak dinilai lagi: pemakaiannya mengikuti vonis itu.
//
// Kutipan atas node yang tidak disisipkan ke ctx diabaikan: tidak ada bukti node itu benar-benar ditampilkan.
// Node sumber berkedudukan tinggi (models.AuthoritativeSources, mis. Galeri Desain) tetap tercatat hasilnya, tetapi
// penghitungnya tidak berubah: hasil kerja pegawai tidak boleh menggeser pengetahuan di atasnya.
func RewardCited(db *gorm.DB, ctx string, score int, cited []uint) int {
	if db == nil || ctx == "" {
		return 0
	}
	outcome := scoreOutcome(score)
	var ids []uint
	db.Model(&models.KnowledgeUse{}).Where("context = ? AND outcome = ?", ctx, "").Pluck("node_id", &ids)
	if len(ids) == 0 {
		return 0
	}
	judged := verdictsIn(db, ctx, ids)
	var strongDone []uint
	db.Model(&models.KnowledgeUse{}).Where("context = ? AND node_id IN ? AND evidence IN ?", ctx, ids,
		[]string{models.KnowledgeEvidenceVerdict, models.KnowledgeEvidenceCited}).Pluck("node_id", &strongDone)
	done := toSet(strongDone)
	isCited := toSet(cited)
	var strong, weak, judgedOK, judgedBad []uint
	for _, id := range ids {
		switch v, ok := judged[id]; {
		case ok && v:
			judgedOK = append(judgedOK, id)
		case ok:
			judgedBad = append(judgedBad, id)
		case outcome == models.KnowledgeOutcomeHelpful && isCited[id] && !done[id]:
			strong = append(strong, id)
		default:
			weak = append(weak, id)
		}
	}
	mark := func(nodes []uint, outcome, evidence string) {
		if len(nodes) > 0 {
			db.Model(&models.KnowledgeUse{}).Where("context = ? AND outcome = ? AND node_id IN ?", ctx, "", nodes).
				Updates(map[string]interface{}{"outcome": outcome, "evidence": evidence})
		}
	}
	mark(judgedOK, models.KnowledgeOutcomeHelpful, models.KnowledgeEvidenceVerdict)
	mark(judgedBad, models.KnowledgeOutcomeHarmful, models.KnowledgeEvidenceVerdict)
	mark(strong, outcome, models.KnowledgeEvidenceCited)
	mark(weak, outcome, models.KnowledgeEvidenceOutcome)
	switch outcome {
	case models.KnowledgeOutcomeHelpful:
		bump(db, strong, "helpful", 1)
		bump(db, weak, "weak_helpful", 1)
	case models.KnowledgeOutcomeHarmful:
		bump(db, weak, "weak_harmful", 1)
	}
	return len(ids)
}

// TodoCitations: node yang dikutip di baris "PENGETAHUAN: K<id>" catatan penyelesaian todo (todos.rationale)
// untuk konteks todo:<id>; konteks lain atau todo yang tidak terbaca → kosong.
func TodoCitations(db *gorm.DB, ctx string) []uint {
	if db == nil || !strings.HasPrefix(ctx, "todo:") {
		return nil
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(ctx, "todo:"), 10, 64)
	if err != nil || id == 0 {
		return nil
	}
	var t models.Todo
	if db.Select("id", "rationale").Where("id = ?", id).Limit(1).Find(&t).Error != nil || t.ID == 0 {
		return nil
	}
	return CitedNodes(t.Rationale)
}

func scoreOutcome(score int) string {
	switch {
	case score >= 75:
		return models.KnowledgeOutcomeHelpful
	case score < 50:
		return models.KnowledgeOutcomeHarmful
	}
	return models.KnowledgeOutcomeNeutral
}

// bump menambah (delta > 0) atau mengurangi (delta < 0, tidak sampai negatif) penghitung bukti node; node sumber
// berkedudukan tinggi tidak berubah.
func bump(db *gorm.DB, ids []uint, col string, delta int) {
	if len(ids) == 0 || delta == 0 {
		return
	}
	q := db.Model(&models.KnowledgeNode{}).Where("id IN ? AND source NOT IN ?", ids, models.AuthoritativeSources)
	if delta < 0 {
		q = q.Where(col+" >= ?", -delta)
	}
	q.UpdateColumn(col, gorm.Expr(col+" + ?", delta))
}

func toSet(ids []uint) map[uint]bool {
	out := make(map[uint]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// verdictsIn: vonis terakhir penilai di ctx untuk node ids (true = ✓). Tabel belum ada → kosong.
func verdictsIn(db *gorm.DB, ctx string, ids []uint) map[uint]bool {
	out := map[uint]bool{}
	if ctx == "" || len(ids) == 0 {
		return out
	}
	var rows []models.KnowledgeVerdict
	if db.Select("node_id", "helpful").Where("context = ? AND node_id IN ?", ctx, ids).Find(&rows).Error != nil {
		return out
	}
	for _, r := range rows {
		out[r.NodeID] = r.Helpful
	}
	return out
}

// Judge mencatat penilaian eksplisit per node (peran "Reflector" ACE: penilai menunjuk butir mana yang tepat
// dan mana yang keliru). true → ✓ kuat, false → ✗ kuat. Satu konteks memberi paling banyak satu vonis per node:
// penilaian ulang yang sama (putaran QA gate berikutnya, review berkala) tidak menambah bukti, vonis yang berubah
// memindahkan buktinya. Pemakaian node itu di ctx ditandai berasal dari vonis, sehingga Reward tidak menghitungnya
// lagi. Node yang tidak dipakai di ctx tetap dinilai. Tanpa ctx setiap panggilan adalah bukti baru. Node sumber
// berkedudukan tinggi (models.AuthoritativeSources) tidak berubah. Mengembalikan jumlah vonis baru atau berubah.
func Judge(db *gorm.DB, ctx string, verdicts map[uint]bool) int {
	if db == nil || len(verdicts) == 0 {
		return 0
	}
	ids := make([]uint, 0, len(verdicts))
	for id := range verdicts {
		ids = append(ids, id)
	}
	var valid []uint
	if db.Model(&models.KnowledgeNode{}).Where("id IN ? AND source NOT IN ?", ids, models.AuthoritativeSources).Pluck("id", &valid).Error != nil {
		return 0
	}
	prev := verdictsIn(db, ctx, valid)
	var incOK, incBad, decOK, decBad []uint
	var fresh []models.KnowledgeVerdict
	for _, id := range valid {
		ok := verdicts[id]
		old, had := prev[id]
		if ctx != "" && had && old == ok {
			continue
		}
		if ok {
			incOK = append(incOK, id)
		} else {
			incBad = append(incBad, id)
		}
		switch {
		case ctx == "":
		case !had:
			fresh = append(fresh, models.KnowledgeVerdict{Context: clip(ctx, 190), NodeID: id, Helpful: ok})
		case old:
			decOK = append(decOK, id)
		default:
			decBad = append(decBad, id)
		}
	}
	bump(db, incOK, "helpful", 1)
	bump(db, incBad, "harmful", 1)
	bump(db, decOK, "helpful", -1)
	bump(db, decBad, "harmful", -1)
	if ctx == "" {
		return len(incOK) + len(incBad)
	}
	if len(fresh) > 0 {
		db.Create(&fresh)
	}
	flip := func(nodes []uint, ok bool) {
		if len(nodes) == 0 {
			return
		}
		db.Model(&models.KnowledgeVerdict{}).Where("context = ? AND node_id IN ?", ctx, nodes).
			Updates(map[string]interface{}{"helpful": ok, "updated_at": time.Now()})
	}
	flip(decBad, true) // ✗ → ✓
	flip(decOK, false) // ✓ → ✗
	mark := func(nodes []uint, outcome string) {
		if len(nodes) == 0 {
			return
		}
		db.Model(&models.KnowledgeUse{}).
			Where("context = ? AND node_id IN ? AND (outcome = ? OR evidence = ?)", ctx, nodes, "", models.KnowledgeEvidenceVerdict).
			Updates(map[string]interface{}{"outcome": outcome, "evidence": models.KnowledgeEvidenceVerdict})
	}
	mark(incOK, models.KnowledgeOutcomeHelpful)
	mark(incBad, models.KnowledgeOutcomeHarmful)
	return len(incOK) + len(incBad)
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }

// Utilities mengembalikan utilitas Beta (KnowledgeNode.Utility) node aktif menurut kunci sumbernya. Kunci yang belum ada
// sebagai node tidak muncul di hasil (pemanggil memakai 0.5 sebagai prior). Tabel belum ada → hasil kosong.
func Utilities(db *gorm.DB, keys []string) map[string]float64 {
	out := map[string]float64{}
	if db == nil || len(keys) == 0 {
		return out
	}
	var nodes []models.KnowledgeNode
	if db.Select("source_key", "helpful", "harmful", "weak_helpful", "weak_harmful").Where("source_key IN ? AND status = ?", keys, models.KnowledgeNodeActive).Find(&nodes).Error != nil {
		return out
	}
	for _, n := range nodes {
		out[n.Key] = n.Utility()
	}
	return out
}

// RecordUseByKeys mencatat node (menurut kunci sumber) yang dipakai sebuah pekerjaan, sama seperti Recall dengan
// Context: nilai QA todo itu kelak memberi bukti ✓/✗ lewat Reward. Mengembalikan jumlah node yang dicatat.
func RecordUseByKeys(db *gorm.DB, ctx string, employeeID uint, keys []string) int {
	if db == nil || ctx == "" || len(keys) == 0 {
		return 0
	}
	var ids []uint
	if db.Model(&models.KnowledgeNode{}).Where("source_key IN ? AND status = ?", keys, models.KnowledgeNodeActive).Pluck("id", &ids).Error != nil {
		return 0
	}
	recordUse(db, ctx, employeeID, ids)
	return len(ids)
}
