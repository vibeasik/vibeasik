package models

import "time"

// ProjectBrand menyimpan warna merek proyek (utama, pendamping, aksen) yang dipilih pemilik atau hasil deteksi
// dari kode. Dipakai antara lain untuk tema PDF Dokumen Proyek. Satu baris per proyek; isinya JSON brandcolor.Colors.
type ProjectBrand struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ProjectID  uint      `gorm:"uniqueIndex;not null" json:"project_id"`
	ColorsJSON string    `gorm:"column:colors_json;type:text" json:"-"`
	UpdatedAt  time.Time `json:"updated_at"`
}
