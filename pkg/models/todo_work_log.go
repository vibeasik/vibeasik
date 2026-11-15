package models

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Riwayat pengerjaan todo: setiap kali todo mulai dikerjakan, selesai, atau dibuka lagi, satu baris dicatat
// beserta waktu dan agent-nya. Kolom completed_at / completed_by_agent di Todo hanya menyimpan penyelesaian
// TERAKHIR dan dikosongkan saat todo dibuka lagi; riwayat ini menyimpan semua percobaan, sehingga bisa dipakai
// sebagai bahan pertimbangan (siapa pernah mengerjakan, berapa lama, berapa kali diulang).

// Peristiwa riwayat pengerjaan.
const (
	TodoWorkStarted   = "started"   // todo menjadi in_progress
	TodoWorkCompleted = "completed" // todo ditandai selesai
	TodoWorkReopened  = "reopened"  // todo yang sedang/selesai dikerjakan kembali ke pending
)

// Jalur yang mengubah status todo (kolom Via).
const (
	TodoWorkViaMCP          = "mcp"          // agent coding lewat tool update_todo
	TodoWorkViaStudio       = "studio"       // pengguna lewat Studio / REST
	TodoWorkViaOrchestrator = "orchestrator" // run agent yang dikirim engine
	TodoWorkViaReview       = "review"       // dibuka lagi karena hasil pemeriksaan
)

// TodoWorkLog adalah satu peristiwa pengerjaan todo.
type TodoWorkLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TodoID       uint      `gorm:"index;not null" json:"todo_id"`
	ProjectID    *uint     `gorm:"index" json:"project_id,omitempty"`
	Event        string    `gorm:"size:32;index" json:"event"`
	Agent        string    `gorm:"size:128;index" json:"agent,omitempty"` // mis. Claude Code, Codex, developer
	Model        string    `gorm:"size:128" json:"model,omitempty"`
	EmployeeName string    `gorm:"size:128" json:"employee_name,omitempty"` // pegawai yang ditugaskan saat itu
	Via          string    `gorm:"size:32" json:"via,omitempty"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// RecordTodoWork mencatat satu peristiwa pengerjaan todo. Peristiwa yang sama oleh agent yang sama
// berturut-turut (mis. in_progress dikirim dua kali) tidak digandakan. Kegagalan mencatat tidak
// menggagalkan perubahan status: riwayat adalah catatan tambahan, bukan syarat.
func RecordTodoWork(db *gorm.DB, todoID uint, event, agent, model, via string) {
	if db == nil || todoID == 0 || event == "" {
		return
	}
	var t Todo
	if err := db.Select("id", "project_id", "employee_name").First(&t, todoID).Error; err != nil {
		return
	}
	agent, model = clipRunes(strings.TrimSpace(agent), 128), clipRunes(strings.TrimSpace(model), 128)
	var last TodoWorkLog
	if db.Where("todo_id = ?", todoID).Order("id DESC").First(&last).Error == nil {
		if last.Event == event && (agent == "" || strings.EqualFold(last.Agent, agent)) {
			return
		}
		// "Dibuka lagi" hanya bermakna setelah todo pernah dikerjakan.
		if event == TodoWorkReopened && last.Event == TodoWorkReopened {
			return
		}
	} else if event == TodoWorkReopened {
		return
	}
	db.Create(&TodoWorkLog{TodoID: todoID, ProjectID: t.ProjectID, Event: event, Agent: agent, Model: model, EmployeeName: t.EmployeeName, Via: via})
}

// RecordTodoStatusChange menerjemahkan perpindahan status todo menjadi peristiwa riwayat.
func RecordTodoStatusChange(db *gorm.DB, todoID uint, oldStatus, newStatus, agent, model, via string) {
	if oldStatus == newStatus {
		return
	}
	switch newStatus {
	case TodoStatusInProgress:
		RecordTodoWork(db, todoID, TodoWorkStarted, agent, model, via)
	case TodoStatusCompleted:
		RecordTodoWork(db, todoID, TodoWorkCompleted, agent, model, via)
	case TodoStatusPending:
		if oldStatus == TodoStatusInProgress || oldStatus == TodoStatusCompleted {
			RecordTodoWork(db, todoID, TodoWorkReopened, agent, model, via)
		}
	}
}

// TodoWorkLogs mengembalikan riwayat pengerjaan sebuah todo, urut lama → baru.
func TodoWorkLogs(db *gorm.DB, todoID uint) []TodoWorkLog {
	logs := []TodoWorkLog{}
	db.Where("todo_id = ?", todoID).Order("id ASC").Find(&logs)
	return logs
}

var todoWorkLabels = map[string]string{
	TodoWorkStarted:   "mulai",
	TodoWorkCompleted: "selesai",
	TodoWorkReopened:  "dibuka lagi",
}

// TodoWorkSummary merangkum riwayat pengerjaan sebagai teks pendek untuk prompt (≤ max baris terakhir);
// "" bila todo belum pernah dikerjakan.
func TodoWorkSummary(db *gorm.DB, todoID uint, max int) string {
	logs := TodoWorkLogs(db, todoID)
	if len(logs) == 0 {
		return ""
	}
	if max > 0 && len(logs) > max {
		logs = logs[len(logs)-max:]
	}
	var b strings.Builder
	for _, l := range logs {
		label := todoWorkLabels[l.Event]
		if label == "" {
			label = l.Event
		}
		fmt.Fprintf(&b, "- %s — %s", l.CreatedAt.Format("2 Jan 2006 15:04"), label)
		if l.Agent != "" {
			b.WriteString(" oleh " + l.Agent)
		}
		if l.Model != "" {
			b.WriteString(" (" + l.Model + ")")
		}
		if l.EmployeeName != "" {
			b.WriteString(", pegawai " + l.EmployeeName)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// TodoWorkViaBackfill menandai baris yang disusun dari data lama (sebelum riwayat dicatat langsung).
const TodoWorkViaBackfill = "backfill"

// BackfillTodoWorkLogs mengisi riwayat todo yang dikerjakan sebelum riwayat dicatat: satu baris "selesai"
// per catatan penyelesaian di Rationale (siapa, kapan), ditambah penyelesaian terakhir dari completed_at /
// completed_by_agent bila belum terwakili. Waktu mulai tidak pernah disimpan, jadi tidak dikarang.
// Hanya menyentuh todo yang belum punya riwayat sama sekali, sehingga aman dijalankan tiap engine menyala.
func BackfillTodoWorkLogs(db *gorm.DB) int {
	var todos []Todo
	db.Select("id", "project_id", "employee_name", "status", "rationale", "completed_at", "completed_by_agent", "completed_by_model").
		Where("(completed_at IS NOT NULL OR rationale LIKE ?) AND id NOT IN (?)", "%"+completionNoteMark+"%",
			db.Model(&TodoWorkLog{}).Select("todo_id")).
		Find(&todos)
	var rows []TodoWorkLog
	for _, t := range todos {
		rows = append(rows, backfillRows(t)...)
	}
	if len(rows) == 0 {
		return 0
	}
	if err := db.CreateInBatches(rows, 200).Error; err != nil {
		return 0
	}
	return len(rows)
}

// backfillRows menyusun baris riwayat satu todo lama, urut waktu.
func backfillRows(t Todo) []TodoWorkLog {
	row := func(at time.Time, agent, model string) TodoWorkLog {
		return TodoWorkLog{TodoID: t.ID, ProjectID: t.ProjectID, Event: TodoWorkCompleted, Agent: clipRunes(agent, 128), Model: clipRunes(model, 128),
			EmployeeName: t.EmployeeName, Via: TodoWorkViaBackfill, CreatedAt: at}
	}
	var rows []TodoWorkLog
	_, notes := SplitRationale(t.Rationale)
	for _, n := range notes {
		if at, err := time.ParseInLocation("2 Jan 2006 15:04", n.When, time.Local); err == nil {
			rows = append(rows, row(at, n.Who, ""))
		}
	}
	if t.Status != TodoStatusCompleted || t.CompletedAt == nil {
		return rows
	}
	// Catatan penyelesaian terakhir biasanya ditulis bersamaan dengan penyelesaian itu sendiri: satukan.
	if n := len(rows); n > 0 {
		if d := t.CompletedAt.Sub(rows[n-1].CreatedAt); d > -10*time.Minute && d < 10*time.Minute {
			rows[n-1].CreatedAt = *t.CompletedAt
			if t.CompletedByAgent != "" {
				rows[n-1].Agent = t.CompletedByAgent
			}
			rows[n-1].Model = t.CompletedByModel
			return rows
		}
	}
	return append(rows, row(*t.CompletedAt, t.CompletedByAgent, t.CompletedByModel))
}
