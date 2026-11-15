package activity

import "github.com/vibeasik/vibeasik/pkg/models"

// WorkerIdentity mengembalikan nama tampilan dan model agent pada koneksi ini (mis. "Claude Code"),
// atau "" bila koneksinya belum dikenal. Dipakai untuk mencatat siapa yang mengerjakan sebuah todo.
func WorkerIdentity(c Caller) (name, model string) {
	d := getDB()
	if d == nil || c.Key == "" {
		return "", ""
	}
	var w models.AgentWorker
	if err := d.Select("name", "model").Where("worker_key = ?", c.Key).First(&w).Error; err != nil {
		return "", ""
	}
	return w.Name, w.Model
}
