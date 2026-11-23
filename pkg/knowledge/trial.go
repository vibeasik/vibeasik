package knowledge

import (
	"regexp"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Masa coba pustaka bersama. Butir bersama yang ditulis mesin (penilai QA, agent lewat MCP, riset celah pegawai)
// masuk tanpa bukti apa pun, jadi diperlakukan sebagai calon: bobot awalnya dibatasi sehingga kalah urutan dari butir
// yang sudah terbukti. Butir tidak pernah dipensiunkan hanya karena umur atau jarang dipakai — bisa jadi memang belum
// ada pekerjaan yang membutuhkannya. Yang dipensiunkan hanya butir yang terbukti keliru: butir masa coba yang belum
// pernah mendapat ✓ dipensiunkan sejak ✗ kuat kedua (lebih cepat daripada ambang umum prune untuk butir lain).
var trialSources = map[string]bool{
	models.KnowledgeSrcReview: true,
	models.KnowledgeSrcAgent:  true,
	models.KnowledgeSrcGap:    true,
}

const (
	sharedTrialWeight  = 60                           // bobot awal maksimum butir bersama tulisan mesin
	sharedTrialHarmful = 2                            // ✗ kuat sebanyak ini tanpa ✓ = terbukti keliru
	trialReasonPrefix  = "Masa coba: terbukti keliru" // status_reason pensiun masa coba
)

// sharedLeakRe mengenali potongan laporan todo yang tidak boleh menjadi pustaka bersama: baris laporan berhuruf
// besar, todo bernomor, nomor baris "(L428)", atau path rumah.
var sharedLeakRe = regexp.MustCompile(`\b(PEGAWAI|KONSULTASI|BEDA|PELAJARAN):|PENGETAHUAN: *K\d|(?i:\btodo #\d+)|\(L\d+\)|(^|[\s(` + "`" + `"'])(/home/|/Users/|[A-Za-z]:\\)`)

// sharedTrial: catatan ini masuk pustaka bersama dengan masa coba.
func sharedTrial(n Note) bool { return n.OwnerEmployeeID == nil && trialSources[n.Source] }

// SharedLeak: teks ini potongan laporan todo, bukan pengetahuan lintas proyek.
func SharedLeak(title, content string) bool { return sharedLeakRe.MatchString(title + "\n" + content) }

// expireShared memensiunkan butir masa coba yang terbukti keliru (✗ kuat berulang, belum pernah ✓). Mengembalikan
// jumlahnya.
func expireShared(db *gorm.DB) int {
	srcs := make([]string, 0, len(trialSources))
	for s := range trialSources {
		srcs = append(srcs, s)
	}
	var bad []models.KnowledgeNode
	db.Where("status = ? AND owner_employee_id IS NULL AND (source_key IS NULL OR source_key = '') AND source IN ? AND helpful = 0 AND harmful >= ?",
		models.KnowledgeNodeActive, srcs, sharedTrialHarmful).Find(&bad)
	for _, n := range bad {
		db.Model(&n).Updates(map[string]interface{}{"status": models.KnowledgeNodeRetired,
			"status_reason": trialReasonPrefix + " (✓0 ✗" + itoa(uint(n.Harmful)) + ")"})
	}
	if len(bad) > 0 {
		invalidate()
	}
	return len(bad)
}
