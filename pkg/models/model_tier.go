package models

import "regexp"

// Kelas model yang disarankan untuk mengerjakan sebuah todo. Model kelas menengah (setara Claude Sonnet) adalah
// model utama: dikawal pengetahuan vibeasik (rubrik, patokan, brief pegawai) ia cukup untuk hampir semua todo,
// termasuk yang berat — kerumitan dijawab dengan effort, bukan model yang lebih mahal. Kelas frontier (setara
// Claude Opus) hanya untuk todo kritis; kelas ringan (setara Claude Haiku) hanya untuk ekstraksi/olah data
// bervolume besar yang hasilnya pasti. Agent yang tidak punya model Claude memakai model setara dari daftarnya.
const (
	ModelTierMain     = "utama"     // setara Sonnet
	ModelTierCritical = "kritis"    // setara Opus
	ModelTierExtract  = "ekstraksi" // setara Haiku
)

// ExtractionTitle: judul todo ekstraksi/olah data massal yang hasilnya bisa dicek mesin.
var ExtractionTitle = regexp.MustCompile(`(?i)\b(ekstrak\w*|extract\w*|impor data|import data|ekspor data|export data|scrap\w*|kumpulkan data|inventaris\w*|konversi data|convert data|format ulang|reformat|terjemah\w*|translate|seed data|data dummy|isi data|migrasi data|rename massal|ganti nama massal|daftar(kan)? semua)\b`)

// TodoModelTier menentukan kelas model todo: kritis (kategori security atau prioritas critical), ekstraksi
// (judul ekstraksi/olah data massal yang tidak berat), atau utama.
func TodoModelTier(t *Todo) string {
	if t == nil {
		return ModelTierMain
	}
	if t.Category == CategorySecurity || t.Priority == TodoPriorityCritical {
		return ModelTierCritical
	}
	level := t.ComplexityLevel
	if level == "" {
		level = EstimateComplexity(t).ComplexityLevel
	}
	if level != ComplexityComplex && ExtractionTitle.MatchString(t.Title) {
		return ModelTierExtract
	}
	return ModelTierMain
}
