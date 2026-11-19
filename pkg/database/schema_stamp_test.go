package database

import (
	"path/filepath"
	"testing"

	"github.com/vibeasik/vibeasik/pkg/models"
)

func TestSchemaFingerprint_Stable(t *testing.T) {
	fp := schemaFingerprint()
	if len(fp) != 64 {
		t.Fatalf("fingerprint length = %d, want 64", len(fp))
	}
	if fp != schemaFingerprint() {
		t.Fatal("fingerprint must be deterministic")
	}
}

func TestInitDBFast_SkipsMigrationWhenStampMatches(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fast.db")

	// Database baru tanpa cap: InitDBFast memigrasi penuh dan mencatat cap.
	db, err := InitDBFast(dbPath)
	if err != nil {
		t.Fatalf("InitDBFast (fresh) failed: %v", err)
	}
	if !db.Migrator().HasTable(&models.Todo{}) || !schemaStampMatches(db) {
		t.Fatal("fresh database must be migrated and stamped")
	}

	// Baris yang hanya diperbaiki backfill: jalur cepat tidak menyentuhnya.
	if err := db.Exec("INSERT INTO todos (title, level, creator_type) VALUES ('t', '', '')").Error; err != nil {
		t.Fatalf("insert todo: %v", err)
	}
	if _, err := InitDBFast(dbPath); err != nil {
		t.Fatalf("InitDBFast (stamped) failed: %v", err)
	}
	var level string
	db.Raw("SELECT level FROM todos WHERE title = 't'").Scan(&level)
	if level != "" {
		t.Fatalf("fast path must skip backfill, level = %q", level)
	}
	if DB == nil {
		t.Fatal("fast path must set the global DB")
	}

	// Cap usang (skema berubah): kembali ke migrasi + backfill penuh.
	if err := db.Exec("UPDATE vibeasik_schema_stamps SET fingerprint = 'old'").Error; err != nil {
		t.Fatalf("stale stamp: %v", err)
	}
	db2, err := InitDBFast(dbPath)
	if err != nil {
		t.Fatalf("InitDBFast (stale) failed: %v", err)
	}
	db2.Raw("SELECT level FROM todos WHERE title = 't'").Scan(&level)
	if level != models.TodoLevelGlobal {
		t.Fatalf("stale stamp must run backfill, level = %q", level)
	}
	if !schemaStampMatches(db2) {
		t.Fatal("full migration must refresh the stamp")
	}
}
