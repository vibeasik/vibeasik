package models

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func workLogDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Todo{}, &TodoWorkLog{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTodoWorkLogKeepsEveryAttempt(t *testing.T) {
	db := workLogDB(t)
	pid := uint(7)
	todo := Todo{Title: "contoh", ProjectID: &pid, EmployeeName: "Laras"}
	db.Create(&todo)

	RecordTodoStatusChange(db, todo.ID, TodoStatusPending, TodoStatusPending, "x", "", TodoWorkViaMCP)      // bukan perpindahan
	RecordTodoStatusChange(db, todo.ID, TodoStatusSuggested, TodoStatusPending, "x", "", TodoWorkViaStudio) // belum pernah dikerjakan
	RecordTodoStatusChange(db, todo.ID, TodoStatusPending, TodoStatusInProgress, "Claude Code", "m1", TodoWorkViaMCP)
	RecordTodoWork(db, todo.ID, TodoWorkStarted, "claude code", "m1", TodoWorkViaOrchestrator) // ganda
	RecordTodoStatusChange(db, todo.ID, TodoStatusInProgress, TodoStatusCompleted, "Claude Code", "m1", TodoWorkViaMCP)
	RecordTodoStatusChange(db, todo.ID, TodoStatusCompleted, TodoStatusPending, "QA", "", TodoWorkViaReview)
	RecordTodoStatusChange(db, todo.ID, TodoStatusPending, TodoStatusInProgress, "Codex", "m2", TodoWorkViaOrchestrator)

	logs := TodoWorkLogs(db, todo.ID)
	var got []string
	for _, l := range logs {
		got = append(got, l.Event+":"+l.Agent)
	}
	want := "started:Claude Code,completed:Claude Code,reopened:QA,started:Codex"
	if strings.Join(got, ",") != want {
		t.Fatalf("riwayat = %v, mau %s", got, want)
	}
	if logs[0].ProjectID == nil || *logs[0].ProjectID != pid || logs[0].EmployeeName != "Laras" || logs[0].CreatedAt.IsZero() {
		t.Fatalf("baris pertama tidak lengkap: %+v", logs[0])
	}
	sum := TodoWorkSummary(db, todo.ID, 2)
	if strings.Count(sum, "\n") != 2 || !strings.Contains(sum, "mulai oleh Codex (m2), pegawai Laras") {
		t.Fatalf("ringkasan = %q", sum)
	}
	if TodoWorkSummary(db, 999, 5) != "" {
		t.Fatal("todo tanpa riwayat harus kosong")
	}
}
