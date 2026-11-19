package models

import "time"

// ProjectDocNotes menyimpan bagian Dokumen Proyek yang ditulis tangan (bukan hasil rakitan otomatis):
// keputusan teknis (ADR), batas di luar lingkup, kebutuhan non-fungsional, glosarium, dan riwayat versi.
// Satu baris per proyek; isinya JSON ProjectDocContent agar bentuknya bisa bertambah tanpa migrasi kolom.
type ProjectDocNotes struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ProjectID   uint      `gorm:"uniqueIndex;not null" json:"project_id"`
	ContentJSON string    `gorm:"column:content_json;type:longtext" json:"-"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProjectDocContent adalah isi yang bisa diedit di halaman Dokumen Proyek.
type ProjectDocContent struct {
	Version       string                  `json:"version"`        // versi dokumen, mis. "1.2"
	Authors       string                  `json:"authors"`        // penyusun / pemilik dokumen
	Background    string                  `json:"background"`     // tambahan latar belakang dari pemilik
	OutOfScope    []string                `json:"out_of_scope"`   // yang sengaja tidak dikerjakan
	NonFunctional []ProjectDocRequirement `json:"non_functional"` // kebutuhan mutu: performa, keamanan, ketersediaan
	Decisions     []ProjectDocDecision    `json:"decisions"`      // catatan keputusan arsitektur (ADR)
	Glossary      []ProjectDocTerm        `json:"glossary"`
	Changelog     []ProjectDocChange      `json:"changelog"`
}

// ProjectDocRequirement: satu kebutuhan non-fungsional dengan target terukur.
type ProjectDocRequirement struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

// ProjectDocDecision mengikuti format ADR ringkas: konteks → keputusan → akibat.
type ProjectDocDecision struct {
	Title        string `json:"title"`
	Date         string `json:"date"`   // YYYY-MM-DD
	Status       string `json:"status"` // proposed | accepted | superseded | deprecated
	Context      string `json:"context"`
	Decision     string `json:"decision"`
	Consequences string `json:"consequences"`
}

type ProjectDocTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}

type ProjectDocChange struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	Changes string `json:"changes"`
}

// FillEmpty mengganti slice nil dengan slice kosong agar JSON selalu berisi [] (bukan null) untuk klien.
func (c *ProjectDocContent) FillEmpty() {
	if c.OutOfScope == nil {
		c.OutOfScope = []string{}
	}
	if c.NonFunctional == nil {
		c.NonFunctional = []ProjectDocRequirement{}
	}
	if c.Decisions == nil {
		c.Decisions = []ProjectDocDecision{}
	}
	if c.Glossary == nil {
		c.Glossary = []ProjectDocTerm{}
	}
	if c.Changelog == nil {
		c.Changelog = []ProjectDocChange{}
	}
}
