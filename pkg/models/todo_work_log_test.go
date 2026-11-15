package models

import (
	"strings"
	"testing"
	"time"

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

func TestBackfillTodoWorkLogsFromOldData(t *testing.T) {
	db := workLogDB(t)
	done := time.Date(2026, 11, 2, 21, 28, 0, 0, time.Local)
	old := Todo{Title: "lama", Status: TodoStatusCompleted, CompletedAt: &done, CompletedByAgent: "Claude Code", CompletedByModel: "m9",
		Rationale: "Analisis.\n\nCatatan penyelesaian (Antigravity, 1 Nov 2026 04:05): gagal.\n\nCatatan penyelesaian (agent, 2 Nov 2026 21:27): beres."}
	plain := Todo{Title: "tanpa catatan", Status: TodoStatusCompleted, CompletedAt: &done, CompletedByAgent: "developer"}
	pending := Todo{Title: "belum"}
	logged := Todo{Title: "sudah punya riwayat", Status: TodoStatusCompleted, CompletedAt: &done}
	for _, x := range []*Todo{&old, &plain, &pending, &logged} {
		db.Create(x)
	}
	RecordTodoWork(db, logged.ID, TodoWorkCompleted, "Codex", "", TodoWorkViaMCP)

	if n := BackfillTodoWorkLogs(db); n != 3 {
		t.Fatalf("baris terisi = %d, mau 3", n)
	}
	if n := BackfillTodoWorkLogs(db); n != 0 {
		t.Fatalf("pengisian kedua harus kosong, dapat %d", n)
	}
	logs := TodoWorkLogs(db, old.ID)
	if len(logs) != 2 || logs[0].Agent != "Antigravity" || logs[0].CreatedAt.Day() != 1 ||
		logs[1].Agent != "Claude Code" || logs[1].Model != "m9" || !logs[1].CreatedAt.Equal(done) || logs[1].Via != TodoWorkViaBackfill {
		t.Fatalf("riwayat todo lama = %+v", logs)
	}
	if l := TodoWorkLogs(db, plain.ID); len(l) != 1 || l[0].Agent != "developer" {
		t.Fatalf("todo tanpa catatan = %+v", l)
	}
	if len(TodoWorkLogs(db, pending.ID)) != 0 || len(TodoWorkLogs(db, logged.ID)) != 1 {
		t.Fatal("todo belum dikerjakan / sudah punya riwayat tidak boleh disentuh")
	}
}
