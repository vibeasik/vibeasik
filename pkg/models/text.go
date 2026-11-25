package models

import "time"

// Teks Website: semua teks yang tampil ke pengunjung dikirim oleh website (skrip kecil di browser) ke engine ini,
// bukan sebaliknya. Pemilik bisa menelusuri teks per halaman/section lalu menjadikan satu teks sebagai todo
// "ubah teks" (mis. supaya tulisan bawaan AI terasa lebih manusiawi).

// TextSource = satu website/aplikasi yang mengirim teks, dengan kunci kirim sendiri.
type TextSource struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	ProjectID    *uint      `gorm:"index" json:"project_id,omitempty"`
	ComponentID  *uint      `gorm:"index" json:"component_id,omitempty"`
	Name         string     `gorm:"size:255;not null" json:"name"`
	IngestKey    string     `gorm:"size:128;index" json:"-"` // hanya tampil di endpoint pemasangan
	Enabled      bool       `gorm:"default:true" json:"enabled"`
	LastIngestAt *time.Time `json:"last_ingest_at"`
	LastOrigin   string     `gorm:"size:255" json:"last_origin"` // asal halaman pengirim terakhir (skema://host)
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TextEntry = satu teks unik pada satu halaman & section. Kunci unik (sumber, Hash): Hash dihitung dari
// halaman + teks yang dinormalkan, jadi teks yang diedit tampil sebagai entri baru dan yang lama
// berhenti diperbarui (LastSeenAt tidak maju lagi).
type TextEntry struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SourceID    uint      `gorm:"not null;uniqueIndex:idx_text_entry_key,priority:1;index:idx_text_entry_page,priority:1" json:"source_id"`
	Hash        string    `gorm:"size:40;not null;uniqueIndex:idx_text_entry_key,priority:2" json:"hash"`
	Page        string    `gorm:"size:255;index:idx_text_entry_page,priority:2" json:"page"`
	Section     string    `gorm:"size:255" json:"section"`
	Selector    string    `gorm:"size:512" json:"selector"`
	Tag         string    `gorm:"size:32" json:"tag"`
	Text        string    `gorm:"type:text;not null" json:"text"`
	Seen        int64     `gorm:"default:1" json:"seen"` // berapa kali dilaporkan (satu per muat halaman)
	TodoID      *uint     `gorm:"index" json:"todo_id,omitempty"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `gorm:"index" json:"last_seen_at"`
}
