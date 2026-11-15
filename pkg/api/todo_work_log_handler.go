package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/vibeasik/vibeasik/pkg/models"
)

// getTodoWorkLogs: riwayat pengerjaan todo — kapan mulai, selesai, atau dibuka lagi, dan oleh agent apa.
func (s *Server) getTodoWorkLogs(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid todo id"})
	}
	var n int64
	if s.db.Model(&models.Todo{}).Where("id = ?", id).Count(&n); n == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "todo not found"})
	}
	return c.JSON(fiber.Map{"logs": models.TodoWorkLogs(s.db.WithContext(c.Context()), uint(id))})
}
