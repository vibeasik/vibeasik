package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/vibeasik/vibeasik/pkg/config"
	"github.com/vibeasik/vibeasik/pkg/models"
)

// schemaRevision dinaikkan bila langkah migrasi di luar definisi model berubah (tuneMySQLStorage, backfill),
// agar cap skema lama tidak lagi dianggap cocok.
const schemaRevision = 1

// SchemaStamp mencatat sidik skema terakhir yang berhasil dimigrasi InitDB pada database ini.
// Proses singkat (mis. `vibeasik mcp`) membacanya untuk melewati AutoMigrate, ALTER, dan backfill yang
// bisa tertahan kunci tabel puluhan detik saat engine sedang bekerja.
type SchemaStamp struct {
	ID          uint   `gorm:"primaryKey"`
	Fingerprint string `gorm:"size:64"`
	UpdatedAt   time.Time
}

func (SchemaStamp) TableName() string { return "vibeasik_schema_stamps" }

const schemaStampID = 1

var (
	fingerprintOnce sync.Once
	fingerprintVal  string
)

// schemaFingerprint meringkas nama, tipe, dan tag setiap kolom model inti; berubah bila model berubah.
func schemaFingerprint() string {
	fingerprintOnce.Do(func() {
		h := sha256.New()
		fmt.Fprintf(h, "rev=%d\n", schemaRevision)
		seen := map[reflect.Type]bool{}
		for _, m := range schemaModels() {
			writeTypeShape(h, reflect.TypeOf(m), seen)
		}
		fingerprintVal = hex.EncodeToString(h.Sum(nil))
	})
	return fingerprintVal
}

func writeTypeShape(h interface{ Write([]byte) (int, error) }, t reflect.Type, seen map[reflect.Type]bool) {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	fmt.Fprintf(h, "type %s.%s\n", t.PkgPath(), t.Name())
	// Hanya struct milik proyek & gorm yang dirinci; isi internal pustaka lain (time.Time, ...) tidak memengaruhi skema.
	if t.Kind() != reflect.Struct || seen[t] || !(strings.HasPrefix(t.PkgPath(), "github.com/vibeasik/") || strings.HasPrefix(t.PkgPath(), "gorm.io/")) {
		return
	}
	seen[t] = true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fmt.Fprintf(h, "  %s %s `%s`\n", f.Name, f.Type.String(), f.Tag)
		// Struct bertingkat (embedded, relasi, ...) ikut diringkas; seen mencegah putaran.
		writeTypeShape(h, f.Type, seen)
	}
}

func schemaStampMatches(db *gorm.DB) bool {
	var stamp SchemaStamp
	// Tabel belum ada (database lama/baru) → galat → migrasi penuh. Logger dibungkam agar galat itu tidak tercetak.
	err := db.Session(&gorm.Session{Logger: logger.Discard}).Where("id = ?", schemaStampID).Limit(1).Find(&stamp).Error
	return err == nil && stamp.Fingerprint == schemaFingerprint()
}

func writeSchemaStamp(db *gorm.DB) {
	_ = db.Save(&SchemaStamp{ID: schemaStampID, Fingerprint: schemaFingerprint()}).Error
}

// attachRuntime memasang koneksi global dan hook model yang dibutuhkan semua jalur kode saat runtime.
func attachRuntime(db *gorm.DB) {
	DB = db
	// Todo gabungan (Aturan 43): sub todo mengikuti status induknya di semua jalur update/hapus todo.
	models.RegisterSubTodoSettle(db)
	models.SetLLMHealthDB(db)
}

// InitDBFast seperti InitDB, tetapi melewati migrasi skema dan backfill data bila database sudah dimigrasi
// dengan skema yang sama (cap SchemaStamp cocok). Untuk proses berumur pendek yang harus cepat siap, seperti
// `vibeasik mcp` (klien MCP memutus koneksi bila initialize tidak dijawab dalam ~30 detik). `vibeasik serve`
// tetap memakai InitDB agar migrasi dan backfill berjalan setiap engine start.
func InitDBFast(dbPathOverride string) (*gorm.DB, error) {
	db, err := openDatabaseConnection(dbPathOverride, config.GetConfig(), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	if schemaStampMatches(db) {
		attachRuntime(db)
		return db, nil
	}
	return prepareDB(db)
}
