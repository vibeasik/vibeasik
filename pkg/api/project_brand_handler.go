package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vibeasik/vibeasik/pkg/brandcolor"
	"github.com/vibeasik/vibeasik/pkg/docpdf"
	"github.com/vibeasik/vibeasik/pkg/models"
)

// Isi Dokumen Proyek untuk PDF bisa memuat ratusan rute dan blok kode; 4 MB cukup lapang.
const maxDocPDFBody = 4 << 20

// projectAppDirs: folder aplikasi proyek, frontend & mobile lebih dulu (tempat warna merek biasanya ditulis).
func (s *Server) projectAppDirs(id uint) ([]string, error) {
	var p models.Project
	if err := s.db.Preload("Components").First(&p, id).Error; err != nil {
		return nil, err
	}
	var front, rest []string
	for _, c := range p.Components {
		if c.AbsolutePath == "" {
			continue
		}
		if strings.HasPrefix(c.Role, "frontend") {
			front = append(front, c.AbsolutePath)
		} else {
			rest = append(rest, c.AbsolutePath)
		}
	}
	dirs := append(front, rest...)
	if len(dirs) == 0 && p.RootPath != "" && p.RootPath != "-" {
		dirs = append(dirs, p.RootPath)
	}
	return dirs, nil
}

// brandColors mengembalikan warna tersimpan; bila belum ada, hasil deteksi dari kode (tidak disimpan).
func (s *Server) brandColors(id uint) (colors brandcolor.Colors, saved bool, updatedAt *time.Time, err error) {
	var row models.ProjectBrand
	err = s.db.Where("project_id = ?", id).First(&row).Error
	switch {
	case err == nil && row.ColorsJSON != "":
		if jerr := json.Unmarshal([]byte(row.ColorsJSON), &colors); jerr == nil && !colors.Empty() {
			colors.Normalize()
			t := row.UpdatedAt
			return colors, true, &t, nil
		}
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		return colors, false, nil, err
	}
	dirs, derr := s.projectAppDirs(id)
	if derr != nil {
		return colors, false, nil, derr
	}
	return brandcolor.Detect(dirs), false, nil, nil
}

// getProjectBrandColors: GET /api/projects/:id/brand-colors — warna merek tersimpan, atau usulan hasil deteksi kode.
func (s *Server) getProjectBrandColors(c *fiber.Ctx) error {
	id, msg := paramID(c, "id", "project")
	if msg != "" {
		return badPayload(c, msg)
	}
	ok, err := s.projectExists(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "proyek tidak ditemukan"})
	}
	colors, saved, at, err := s.brandColors(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if c.Query("detect") == "1" && saved { // minta deteksi ulang walau sudah tersimpan (pratinjau, tidak menimpa)
		dirs, err := s.projectAppDirs(id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"project_id": id, "colors": brandcolor.Detect(dirs), "saved": false, "updated_at": nil})
	}
	return c.JSON(fiber.Map{"project_id": id, "colors": colors, "saved": saved, "updated_at": at})
}

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func checkHex(field, v string, required bool) string {
	v = strings.TrimSpace(v)
	if v == "" {
		if required {
			return field + " wajib diisi (#rrggbb)"
		}
		return ""
	}
	if !hexColorRe.MatchString(v) {
		return fmt.Sprintf("%s harus berformat #rrggbb", field)
	}
	return ""
}

// putProjectBrandColors: PUT /api/projects/:id/brand-colors — simpan warna merek pilihan pemilik.
func (s *Server) putProjectBrandColors(c *fiber.Ctx) error {
	id, msg := paramID(c, "id", "project")
	if msg != "" {
		return badPayload(c, msg)
	}
	var in brandcolor.Colors
	if msg := firstProblem(bindJSONRequired(c, &in), checkHex("primary", in.Primary, true),
		checkHex("secondary", in.Secondary, false), checkHex("accent", in.Accent, false)); msg != "" {
		return badPayload(c, msg)
	}
	ok, err := s.projectExists(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "proyek tidak ditemukan"})
	}
	in.Normalize()
	if in.Source != "detected" {
		in.Source = "manual"
	}
	raw, _ := json.Marshal(in)
	row := models.ProjectBrand{ProjectID: id, ColorsJSON: string(raw)}
	if err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"colors_json", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	var saved models.ProjectBrand
	if err := s.db.Where("project_id = ?", id).First(&saved).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"project_id": id, "colors": in, "saved": true, "updated_at": saved.UpdatedAt})
}

// deleteProjectBrandColors: DELETE /api/projects/:id/brand-colors — kembali ke warna hasil deteksi kode.
func (s *Server) deleteProjectBrandColors(c *fiber.Ctx) error {
	id, msg := paramID(c, "id", "project")
	if msg != "" {
		return badPayload(c, msg)
	}
	if err := s.db.Where("project_id = ?", id).Delete(&models.ProjectBrand{}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return s.getProjectBrandColors(c)
}

var unsafeFileRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// exportProjectDocPDF: POST /api/projects/:id/doc-pdf — render Dokumen Proyek (isi dari Studio) menjadi PDF
// bertema warna merek proyek.
func (s *Server) exportProjectDocPDF(c *fiber.Ctx) error {
	id, msg := paramID(c, "id", "project")
	if msg != "" {
		return badPayload(c, msg)
	}
	if len(c.Body()) > maxDocPDFBody {
		return badPayload(c, fmt.Sprintf("isi dokumen maksimal %d MB", maxDocPDFBody>>20))
	}
	var doc docpdf.Document
	if msg := bindJSONRequired(c, &doc); msg != "" {
		return badPayload(c, msg)
	}
	ok, err := s.projectExists(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "proyek tidak ditemukan"})
	}
	colors, _, _, err := s.brandColors(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	now := time.Now()
	out, err := docpdf.Render(doc, colors, now)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "gagal menyusun PDF: " + err.Error()})
	}
	name := strings.Trim(unsafeFileRe.ReplaceAllString(doc.Title, "-"), "-")
	if name == "" {
		name = "proyek"
	}
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="Dokumen-Teknis-%s-%s.pdf"`, name, now.Format("2006-01-02")))
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Send(out)
}
