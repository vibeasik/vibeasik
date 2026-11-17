package knowledge

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Satu pemakaian per node per konteks. Node yang disisipkan lagi ke pekerjaan yang sama (prompt dibangun ulang,
// pengerjaan ulang, audit ulang berkas yang sama) tidak menambah baris maupun hitungan pakai. Bila pemakaiannya
// sudah dinilai, barisnya dibuka lagi agar penilaian berikutnya tetap tercatat, dengan bukti lama dititipkan di
// kolom Evidence (kode reopen*). Bukti lama tetap terhitung di penghitung node sampai penilaian baru datang:
//
//   - penilaian baru (RewardCited) mencabut bukti lama lalu memberi bukti baru, jadi satu konteks tetap
//     menyumbang paling banyak satu bukti per node, dan kutipan tetap dihitung sekali per konteks;
//   - bila penilaian baru tidak pernah datang (SettleStale), nilai lama dipulihkan tanpa mengubah penghitung.
//
// Pemakaian bervonis (Judge) tidak dibuka lagi: vonisnya disimpan per konteks di knowledge_verdicts dan vonis
// yang berubah sudah memindahkan buktinya sendiri.

// Kode Evidence untuk pemakaian yang dibuka lagi (outcome kosong): bukti yang sudah disumbangkan nilai lamanya.
const (
	reopenPrefix       = "re:"
	reopenCited        = "re:cited"   // ✓ kuat karena dikutip
	reopenWeakHelpful  = "re:weak+"   // ✓ lemah
	reopenWeakHarmful  = "re:weak-"   // ✗ lemah
	reopenLegacyHelped = "re:strong+" // ✓ catatan lama (sebelum asal bukti dicatat) = penghitung kuat
	reopenLegacyHarmed = "re:strong-" // ✗ catatan lama = penghitung kuat
)

// reopened: nilai lama & penghitung node yang disumbangkannya, per kode.
var reopened = map[string]struct{ outcome, evidence, column string }{
	reopenCited:        {models.KnowledgeOutcomeHelpful, models.KnowledgeEvidenceCited, "helpful"},
	reopenWeakHelpful:  {models.KnowledgeOutcomeHelpful, models.KnowledgeEvidenceOutcome, "weak_helpful"},
	reopenWeakHarmful:  {models.KnowledgeOutcomeHarmful, models.KnowledgeEvidenceOutcome, "weak_harmful"},
	reopenLegacyHelped: {models.KnowledgeOutcomeHelpful, "", "helpful"},
	reopenLegacyHarmed: {models.KnowledgeOutcomeHarmful, "", "harmful"},
}

// contributionCode: kode bukti yang sudah disumbangkan sebuah pemakaian bernilai ke penghitung node (tanpa
// memandang sumber node); "" = tidak menyumbang apa pun lewat Reward (netral, belum dinilai, vonis Judge).
func contributionCode(outcome, evidence string) string {
	if strings.HasPrefix(evidence, reopenPrefix) {
		if _, ok := reopened[evidence]; ok {
			return evidence
		}
		return ""
	}
	switch {
	case evidence == models.KnowledgeEvidenceCited && outcome == models.KnowledgeOutcomeHelpful:
		return reopenCited
	case evidence == models.KnowledgeEvidenceOutcome && outcome == models.KnowledgeOutcomeHelpful:
		return reopenWeakHelpful
	case evidence == models.KnowledgeEvidenceOutcome && outcome == models.KnowledgeOutcomeHarmful:
		return reopenWeakHarmful
	case evidence == "" && outcome == models.KnowledgeOutcomeHelpful:
		return reopenLegacyHelped
	case evidence == "" && outcome == models.KnowledgeOutcomeHarmful:
		return reopenLegacyHarmed
	}
	return ""
}

type useRow struct {
	ID       uint
	NodeID   uint
	Outcome  string
	Evidence string
}

// recordUse mencatat node yang disisipkan ke sebuah pekerjaan: satu baris per node per konteks selama barisnya
// masih ada (lihat catatan di atas). Node baru menaikkan hitungan pakai; node yang barisnya dibuka lagi hanya
// memperbarui waktu terakhir dipakai.
func recordUse(db *gorm.DB, ctx string, employeeID uint, ids []uint) {
	if ctx == "" || len(ids) == 0 {
		return
	}
	ctx = clip(ctx, 190)
	var existing []useRow
	db.Model(&models.KnowledgeUse{}).Select("id", "node_id", "outcome", "COALESCE(evidence, '') AS evidence").
		Where("context = ? AND node_id IN ?", ctx, ids).Order("id").Scan(&existing)
	pending := map[uint]bool{}
	latest := map[uint]useRow{} // pemakaian bernilai terakhir per node
	for _, u := range existing {
		if u.Outcome == "" {
			pending[u.NodeID] = true
		} else {
			latest[u.NodeID] = u
		}
	}
	var emp *uint
	if employeeID > 0 {
		emp = &employeeID
	}
	now := time.Now()
	var rows []models.KnowledgeUse
	var fresh, again []uint
	reopen := map[string][]uint{} // kode → id baris
	seen := map[uint]bool{}
	for _, id := range ids {
		if seen[id] || pending[id] {
			continue
		}
		seen[id] = true
		u, rated := latest[id]
		switch {
		case !rated:
			rows = append(rows, models.KnowledgeUse{NodeID: id, Context: ctx, EmployeeID: emp})
			fresh = append(fresh, id)
		case u.Evidence == models.KnowledgeEvidenceVerdict:
		default:
			code := contributionCode(u.Outcome, u.Evidence)
			reopen[code] = append(reopen[code], u.ID)
			again = append(again, id)
		}
	}
	if len(rows) > 0 {
		db.Create(&rows)
		db.Model(&models.KnowledgeNode{}).Where("id IN ?", fresh).
			UpdateColumns(map[string]interface{}{"uses": gorm.Expr("uses + 1"), "last_used_at": now})
	}
	if len(again) > 0 {
		// created_at ikut diperbarui: masa tunggu penilaian & masa simpan dihitung dari pemakaian terakhir.
		for code, rowIDs := range reopen {
			db.Model(&models.KnowledgeUse{}).Where("id IN ?", rowIDs).
				UpdateColumns(map[string]interface{}{"outcome": "", "evidence": code, "created_at": now})
		}
		db.Model(&models.KnowledgeNode{}).Where("id IN ?", again).UpdateColumn("last_used_at", now)
	}
}

// revokeReopened mencabut bukti lama pemakaian yang dibuka lagi di ctx untuk node ids, sebelum penilaian barunya
// ditulis. Node sumber berkedudukan tinggi tidak berubah (bump).
func revokeReopened(db *gorm.DB, ctx string, ids []uint) {
	var rows []useRow
	db.Model(&models.KnowledgeUse{}).Select("node_id", "evidence").
		Where("context = ? AND outcome = ? AND node_id IN ? AND evidence LIKE ?", ctx, "", ids, reopenPrefix+"%").Scan(&rows)
	byCol := map[string][]uint{}
	for _, r := range rows {
		if c, ok := reopened[r.Evidence]; ok {
			byCol[c.column] = append(byCol[c.column], r.NodeID)
		}
	}
	for col, nodes := range byCol {
		bump(db, nodes, col, -1)
	}
}

// closePending menutup pemakaian yang belum dinilai dalam scope: yang dibuka lagi dipulihkan ke nilai lamanya
// (penghitung node tidak berubah, bukti lamanya memang belum dicabut), sisanya ditutup netral.
func closePending(db *gorm.DB, scope func(*gorm.DB) *gorm.DB) (int, error) {
	n := 0
	for code, r := range reopened {
		res := scope(db.Model(&models.KnowledgeUse{})).Where("outcome = ? AND evidence = ?", "", code).
			Updates(map[string]interface{}{"outcome": r.outcome, "evidence": r.evidence})
		if res.Error != nil {
			return n, res.Error
		}
		n += int(res.RowsAffected)
	}
	res := scope(db.Model(&models.KnowledgeUse{})).Where("outcome = ?", "").
		Updates(map[string]interface{}{"outcome": models.KnowledgeOutcomeNeutral, "evidence": ""})
	if res.Error != nil {
		return n, res.Error
	}
	return n + int(res.RowsAffected), nil
}
