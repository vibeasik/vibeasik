package knowledge

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Masa berlaku node (fakta lama dibatalkan, tidak dihapus — gaya graf dua garis waktu Zep/Graphiti):
//
//   - digantikan: SupersededByID menunjuk node pengganti. Selama penggantinya aktif, node lama tidak dikirim
//     temu-kembali; bila penggantinya dipensiunkan/dilebur node lama terlihat lagi dan kurator menandainya.
//   - kedaluwarsa: ValidUntil sudah lewat. Node tidak dikirim, kurator menandainya perlu ditinjau (tidak
//     dipensiunkan otomatis): perpanjang, gantikan, atau pensiunkan.
//
// Status node tetap "active": isi, bukti, tautan, dan kunci sumbernya tidak berubah, dan semua pemakai status
// (sinkron sumber, ekspor, paket) tetap melihatnya sebagai node tersimpan. Pertentangan isi tidak dideteksi
// otomatis; penggantian selalu lewat jalur eksplisit (Supersede, Note.Supersedes, CLI).

const maxAppliesTo = 64

// inForce: node ini boleh dikirim temu-kembali pada waktu now (tidak digantikan node aktif, belum kedaluwarsa).
func (e *entry) inForce(now time.Time) bool {
	return e.replacedBy == 0 && (e.until.IsZero() || now.Before(e.until))
}

// loadValidity mengisi masa berlaku entri indeks. Kolomnya dibaca terpisah dari kueri pembangun indeks agar
// database yang belum dimigrasikan (perintah baca-saja tanpa migrasi) tetap mendapat indeks lengkap.
func loadValidity(db *gorm.DB, ix *index) {
	var rows []models.KnowledgeNode
	if db.Select("id", "superseded_by_id", "valid_until").
		Where("status = ? AND (superseded_by_id IS NOT NULL OR valid_until IS NOT NULL)", models.KnowledgeNodeActive).
		Find(&rows).Error != nil {
		return
	}
	for _, r := range rows {
		e := ix.byID[r.ID]
		if e == nil {
			continue
		}
		if r.SupersededByID != nil && *r.SupersededByID != r.ID && ix.byID[*r.SupersededByID] != nil {
			e.replacedBy = *r.SupersededByID
		}
		if r.ValidUntil != nil {
			e.until = *r.ValidUntil
		}
	}
}

// NodeInForce: versi DB dari entry.inForce untuk node yang sudah dimuat (penggantinya dianggap aktif).
func NodeInForce(n models.KnowledgeNode, now time.Time) bool {
	return n.SupersededByID == nil && !n.Expired(now)
}

func normAppliesTo(s string) string {
	return clip(strings.Join(strings.Fields(s), " "), maxAppliesTo)
}

// checkSupersede memeriksa apakah node old boleh digantikan node yang pemilik & sumbernya seperti itu.
func checkSupersede(old *models.KnowledgeNode, newOwner *uint, newSource string) error {
	if old.Status != models.KnowledgeNodeActive {
		return fmt.Errorf("K%d tidak aktif (%s); hanya node aktif yang bisa digantikan", old.ID, old.Status)
	}
	// Pengganti pribadi milik orang lain (atau pengganti pribadi untuk pustaka bersama) akan menyembunyikan node
	// dari pegawai yang tidak bisa melihat penggantinya.
	if newOwner != nil && ownerOf(newOwner) != ownerOf(old.OwnerEmployeeID) {
		return fmt.Errorf("pengganti K%d harus pustaka bersama atau milik pegawai yang sama", old.ID)
	}
	if models.KnowledgeAuthoritative(old.Source) && newSource != old.Source {
		return fmt.Errorf("K%d bersumber %s (berkedudukan tinggi); hanya node dari sumber yang sama yang boleh menggantikannya", old.ID, old.Source)
	}
	return nil
}

// supersedeProposal mengawali status_reason node yang penggantiannya diusulkan tetapi belum diterapkan. Selama
// awalan ini ada, kurator tidak melepas tanda perlu ditinjau walau bukti node itu baik.
const supersedeProposal = "Usulan pengganti K"

// AgentMaySupersede: node old boleh langsung digantikan lewat jalur agent (tool MCP) oleh butir milik newOwner
// (nil = pustaka bersama). Yang boleh hanya node yang memang tulisan agent: catatan agent, hasil belajar celah, atau
// pengetahuan pribadi pegawai yang sama. Patokan & hasil kurasi (guideline, checklist audit, playbook, promosi,
// penilaian QA, vonis audit, galeri/teori desain, paket, ajaran manusia) hanya bisa diusulkan untuk diganti —
// tanpa batas ini satu agent bisa menyembunyikan patokan bersama dari temu-kembali semua pekerja.
func AgentMaySupersede(old *models.KnowledgeNode, newOwner *uint) bool {
	switch old.Source {
	case models.KnowledgeSrcAgent, models.KnowledgeSrcGap:
		return true
	}
	return old.OwnerEmployeeID != nil && newOwner != nil && *old.OwnerEmployeeID == *newOwner
}

// proposeSupersede: lanjutan Learn dengan Note.ProposeSupersede. Node lama tetap dikirim temu-kembali; ia ditandai
// perlu ditinjau dengan alasan yang menyebut usulan penggantinya. Penggantian sungguhan lewat Supersede (CLI).
func proposeSupersede(db *gorm.DB, oldID uint, repl *models.KnowledgeNode) error {
	reason := supersedeProposal + itoa(repl.ID) + " (" + clip(repl.Title, 80) + "), dicatat agent; " +
		"menunggu tinjauan — node ini belum disembunyikan"
	err := db.Model(&models.KnowledgeNode{}).Where("id = ?", oldID).
		Updates(map[string]interface{}{"needs_review": true, "status_reason": reason}).Error
	if err != nil {
		return fmt.Errorf("K%d tersimpan, tetapi usulan penggantian K%d tidak bisa dicatat: %w", repl.ID, oldID, err)
	}
	return nil
}

// Supersede menandai node oldID digantikan node newID. Bila newID sendiri sudah digantikan, penunjuknya diarahkan
// ke ujung rantai (node aktif terakhir); node lain yang sebelumnya digantikan oldID ikut diarahkan ke ujung itu,
// sehingga rantai selalu sedalam satu langkah. Rantai melingkar ditolak. Node lama tetap aktif dan isinya tidak
// berubah; hanya penunjuk & alasan yang ditulis. Mengembalikan id ujung rantai.
func Supersede(db *gorm.DB, oldID, newID uint, reason string) (uint, error) {
	if db == nil {
		return 0, errors.New("database belum siap")
	}
	if oldID == 0 || newID == 0 || oldID == newID {
		return 0, errors.New("node lama dan pengganti harus dua node berbeda")
	}
	var old models.KnowledgeNode
	if db.Where("id = ?", oldID).Limit(1).Find(&old).Error != nil || old.ID == 0 {
		return 0, fmt.Errorf("node %d tidak ditemukan", oldID)
	}
	head, err := chainHead(db, oldID, newID)
	if err != nil {
		return 0, err
	}
	if err := checkSupersede(&old, head.OwnerEmployeeID, head.Source); err != nil {
		return 0, err
	}
	reason = clip(reason, 400)
	if reason == "" {
		reason = "tanpa alasan"
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.KnowledgeNode{}).Where("id = ?", oldID).Updates(map[string]interface{}{
			"superseded_by_id": head.ID, "status_reason": "Digantikan K" + itoa(head.ID) + ": " + reason,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&models.KnowledgeNode{}).Where("superseded_by_id = ? AND id <> ?", oldID, head.ID).
			Updates(map[string]interface{}{"superseded_by_id": head.ID}).Error
	})
	if err != nil {
		return 0, err
	}
	invalidate()
	return head.ID, nil
}

// chainHead mengikuti penunjuk pengganti dari newID sampai node aktif terakhir. Bertemu oldID = rantai melingkar.
func chainHead(db *gorm.DB, oldID, newID uint) (*models.KnowledgeNode, error) {
	var head *models.KnowledgeNode
	seen := map[uint]bool{}
	for id := newID; id != 0; {
		if id == oldID {
			return nil, fmt.Errorf("rantai melingkar: K%d sudah (lewat rantai) digantikan K%d", newID, oldID)
		}
		if seen[id] {
			break // data lama sudah melingkar tanpa melewati oldID: berhenti di node terakhir yang sah
		}
		seen[id] = true
		var n models.KnowledgeNode
		if db.Where("id = ?", id).Limit(1).Find(&n).Error != nil || n.ID == 0 || n.Status != models.KnowledgeNodeActive {
			if head == nil {
				return nil, fmt.Errorf("pengganti K%d tidak ditemukan atau tidak aktif", newID)
			}
			break
		}
		head = &n
		id = 0
		if n.SupersededByID != nil {
			id = *n.SupersededByID
		}
	}
	return head, nil
}

// Restore membatalkan penggantian: node kembali dikirim temu-kembali (bila belum kedaluwarsa).
func Restore(db *gorm.DB, id uint) error {
	res := db.Model(&models.KnowledgeNode{}).Where("id = ? AND superseded_by_id IS NOT NULL", id).
		Updates(map[string]interface{}{"superseded_by_id": nil, "status_reason": ""})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("node %d tidak ditemukan atau tidak sedang digantikan", id)
	}
	invalidate()
	return nil
}

// SetValidity menulis keterangan berlaku-untuk dan batas berlaku node (until nil = tanpa batas). Isi node tidak
// berubah. Batas yang diperpanjang melepas tanda perlu ditinjau pada kurasi berikutnya bila buktinya tidak buruk.
func SetValidity(db *gorm.DB, id uint, appliesTo string, until *time.Time) error {
	res := db.Model(&models.KnowledgeNode{}).Where("id = ?", id).
		Updates(map[string]interface{}{"applies_to": normAppliesTo(appliesTo), "valid_until": until})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("node %d tidak ditemukan", id)
	}
	invalidate()
	return nil
}

// outdatedNodes: node aktif yang perlu ditinjau karena masa berlakunya: kedaluwarsa pada now, atau digantikan node
// yang kini tidak aktif lagi (node lama terlihat lagi tanpa ada yang memutuskan). Nilai = alasan.
func outdatedNodes(db *gorm.DB, now time.Time) map[uint]string {
	out := map[uint]string{}
	var rows []models.KnowledgeNode
	if db.Select("id", "superseded_by_id", "valid_until").
		Where("status = ? AND (superseded_by_id IS NOT NULL OR valid_until IS NOT NULL)", models.KnowledgeNodeActive).
		Find(&rows).Error != nil {
		return out
	}
	var targets []uint
	for _, r := range rows {
		if r.SupersededByID != nil {
			targets = append(targets, *r.SupersededByID)
		}
	}
	active := map[uint]bool{}
	if len(targets) > 0 {
		var ids []uint
		db.Model(&models.KnowledgeNode{}).Where("id IN ? AND status = ?", targets, models.KnowledgeNodeActive).Pluck("id", &ids)
		active = toSet(ids)
	}
	for _, r := range rows {
		switch {
		case r.Expired(now):
			out[r.ID] = "kedaluwarsa"
		case r.SupersededByID != nil && !active[*r.SupersededByID]:
			out[r.ID] = "pengganti tidak aktif"
		}
	}
	return out
}

// flagOutdated menandai perlu ditinjau node yang masa berlakunya bermasalah (lihat outdatedNodes). Node tidak
// dipensiunkan: yang memutuskan manusia/agent. Mengembalikan jumlah tanda baru.
func flagOutdated(db *gorm.DB, now time.Time) int {
	ids := make([]uint, 0)
	for id := range outdatedNodes(db, now) {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return 0
	}
	res := db.Model(&models.KnowledgeNode{}).Where("id IN ? AND needs_review = ?", ids, false).UpdateColumn("needs_review", true)
	return int(res.RowsAffected)
}
