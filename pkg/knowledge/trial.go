package knowledge

import (
	"regexp"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Masa coba pustaka bersama. Butir bersama yang ditulis mesin (penilai QA, agent lewat MCP, riset celah pegawai)
// masuk tanpa bukti apa pun, jadi diperlakukan sebagai calon: bobot awalnya dibatasi, dan sesudah 30 hari butir yang
// belum pernah mendapat ✓ (kuat maupun lemah) dipensiunkan bila tidak pernah muncul di temu-kembali, sudah muncul
// berkali-kali tanpa ✓, atau sudah 90 hari. Butir yang baru muncul beberapa kali diberi waktu sampai vonisnya datang
// (vonis QA tidak mengalir setiap hari). Butir yang diajarkan manusia dan hasil promosi tidak ikut masa coba; butir
// asli bersama lainnya dipensiunkan bila lama tidak pernah dipakai sama sekali.
var trialSources = map[string]bool{
	models.KnowledgeSrcReview: true,
	models.KnowledgeSrcAgent:  true,
	models.KnowledgeSrcGap:    true,
}

const (
	sharedTrialWeight = 60                     // bobot awal maksimum butir bersama tulisan mesin
	sharedTrialAge    = 30 * 24 * time.Hour    // masa coba: harus mendapat ✓ dalam waktu ini
	sharedTrialMaxAge = 90 * 24 * time.Hour    // batas akhir masa coba walau baru sedikit muncul
	sharedTrialUses   = 5                      // muncul sebanyak ini tanpa ✓ = cukup kesempatan
	sharedIdleAge     = 60 * 24 * time.Hour    // butir asli bersama lain: pensiun bila tidak pernah dipakai selama ini
	trialReasonPrefix = "Masa coba 30 hari"    // status_reason pensiun masa coba
	idleReasonPrefix  = "Tidak pernah dipakai" // status_reason pensiun butir menganggur
)

// sharedLeakRe mengenali potongan laporan todo yang tidak boleh menjadi pustaka bersama: baris laporan berhuruf
// besar, todo bernomor, nomor baris "(L428)", atau path rumah.
var sharedLeakRe = regexp.MustCompile(`\b(PEGAWAI|KONSULTASI|BEDA|PELAJARAN):|PENGETAHUAN: *K\d|(?i:\btodo #\d+)|\(L\d+\)|(^|[\s(` + "`" + `"'])(/home/|/Users/|[A-Za-z]:\\)`)

// sharedTrial: catatan ini masuk pustaka bersama dengan masa coba.
func sharedTrial(n Note) bool { return n.OwnerEmployeeID == nil && trialSources[n.Source] }

// SharedLeak: teks ini potongan laporan todo, bukan pengetahuan lintas proyek.
func SharedLeak(title, content string) bool { return sharedLeakRe.MatchString(title + "\n" + content) }

// expireShared memensiunkan butir bersama yang gagal masa coba atau lama menganggur. Mengembalikan jumlahnya.
func expireShared(db *gorm.DB, now time.Time) int {
	srcs := make([]string, 0, len(trialSources))
	for s := range trialSources {
		srcs = append(srcs, s)
	}
	native := "(source_key IS NULL OR source_key = '')"
	trial := db.Model(&models.KnowledgeNode{}).
		Where("status = ? AND owner_employee_id IS NULL AND "+native+" AND source IN ? AND created_at < ? AND helpful = 0 AND weak_helpful = 0 AND (uses = 0 OR uses >= ? OR created_at < ?)",
			models.KnowledgeNodeActive, srcs, now.Add(-sharedTrialAge), sharedTrialUses, now.Add(-sharedTrialMaxAge)).
		Updates(map[string]interface{}{"status": models.KnowledgeNodeRetired,
			"status_reason": trialReasonPrefix + " lewat tanpa bukti berguna (belum pernah ✓)"})
	idle := db.Model(&models.KnowledgeNode{}).
		Where("status = ? AND owner_employee_id IS NULL AND "+native+" AND source NOT IN ? AND created_at < ? AND uses = 0 AND helpful = 0 AND harmful = 0 AND weak_helpful = 0 AND weak_harmful = 0",
			models.KnowledgeNodeActive, append(srcs, models.KnowledgeSrcManual, models.KnowledgeSrcPromoted), now.Add(-sharedIdleAge)).
		Updates(map[string]interface{}{"status": models.KnowledgeNodeRetired,
			"status_reason": idleReasonPrefix + " selama 60 hari dan tanpa bukti"})
	n := int(trial.RowsAffected + idle.RowsAffected)
	if n > 0 {
		invalidate()
	}
	return n
}
