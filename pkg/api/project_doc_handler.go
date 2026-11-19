package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Batas isi Dokumen Proyek yang ditulis tangan: cukup untuk dokumen nyata, tetapi tidak membebani DB/PDF.
const (
	maxDocItems = 100
	maxDocBody  = 512 * 1024
)

var adrStatuses = []string{"proposed", "accepted", "superseded", "deprecated"}

// normalizeDocContent merapikan spasi, membuang butir kosong, dan memeriksa batas panjang; pesan kosong = sah.
func normalizeDocContent(in *models.ProjectDocContent) string {
	in.Version = strings.TrimSpace(in.Version)
	in.Authors = strings.TrimSpace(in.Authors)
	in.Background = strings.TrimSpace(in.Background)

	scope := in.OutOfScope[:0]
	for _, s := range in.OutOfScope {
		if s = strings.TrimSpace(s); s != "" {
			scope = append(scope, s)
		}
	}
	in.OutOfScope = scope

	nf := in.NonFunctional[:0]
	for _, r := range in.NonFunctional {
		r.Name, r.Target = strings.TrimSpace(r.Name), strings.TrimSpace(r.Target)
		if r.Name != "" || r.Target != "" {
			nf = append(nf, r)
		}
	}
	in.NonFunctional = nf

	adr := in.Decisions[:0]
	for _, d := range in.Decisions {
		d.Title, d.Date, d.Status = strings.TrimSpace(d.Title), strings.TrimSpace(d.Date), strings.TrimSpace(d.Status)
		d.Context, d.Decision, d.Consequences = strings.TrimSpace(d.Context), strings.TrimSpace(d.Decision), strings.TrimSpace(d.Consequences)
		if d.Title != "" || d.Decision != "" {
			adr = append(adr, d)
		}
	}
	in.Decisions = adr

	terms := in.Glossary[:0]
	for _, t := range in.Glossary {
		t.Term, t.Definition = strings.TrimSpace(t.Term), strings.TrimSpace(t.Definition)
		if t.Term != "" {
			terms = append(terms, t)
		}
	}
	in.Glossary = terms

	log := in.Changelog[:0]
	for _, ch := range in.Changelog {
		ch.Version, ch.Date, ch.Changes = strings.TrimSpace(ch.Version), strings.TrimSpace(ch.Date), strings.TrimSpace(ch.Changes)
		if ch.Version != "" || ch.Changes != "" {
			log = append(log, ch)
		}
	}
	in.Changelog = log

	problems := []string{maxLen("version", in.Version, maxName), maxLen("authors", in.Authors, maxLabel), maxLen("background", in.Background, maxText)}
	counts := []struct {
		name string
		n    int
	}{{"out_of_scope", len(in.OutOfScope)}, {"non_functional", len(in.NonFunctional)}, {"decisions", len(in.Decisions)},
		{"glossary", len(in.Glossary)}, {"changelog", len(in.Changelog)}}
	for _, ct := range counts {
		if ct.n > maxDocItems {
			problems = append(problems, fmt.Sprintf("%s maksimal %d butir", ct.name, maxDocItems))
		}
	}
	for i, s := range in.OutOfScope {
		problems = append(problems, maxLen(fmt.Sprintf("out_of_scope[%d]", i), s, maxLabel))
	}
	for i, r := range in.NonFunctional {
		problems = append(problems, maxLen(fmt.Sprintf("non_functional[%d].name", i), r.Name, maxName), maxLen(fmt.Sprintf("non_functional[%d].target", i), r.Target, maxLabel))
	}
	for i, d := range in.Decisions {
		p := fmt.Sprintf("decisions[%d]", i)
		problems = append(problems, maxLen(p+".title", d.Title, maxLabel), maxLen(p+".date", d.Date, 32),
			oneOf(p+".status", d.Status, adrStatuses...), maxLen(p+".context", d.Context, maxText),
			maxLen(p+".decision", d.Decision, maxText), maxLen(p+".consequences", d.Consequences, maxText))
	}
	for i, t := range in.Glossary {
		problems = append(problems, maxLen(fmt.Sprintf("glossary[%d].term", i), t.Term, maxName), maxLen(fmt.Sprintf("glossary[%d].definition", i), t.Definition, maxLabel))
	}
	for i, ch := range in.Changelog {
		p := fmt.Sprintf("changelog[%d]", i)
		problems = append(problems, maxLen(p+".version", ch.Version, maxName), maxLen(p+".date", ch.Date, 32), maxLen(p+".changes", ch.Changes, maxText))
	}
	return firstProblem(problems...)
}

// projectExists membedakan 404 (proyek tidak ada) dari galat DB.
func (s *Server) projectExists(id uint) (bool, error) {
	var n int64
	if err := s.db.Model(&models.Project{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// getProjectDocNotes: GET /api/projects/:id/doc-notes — bagian Dokumen Proyek yang ditulis tangan.
// Proyek tanpa catatan mengembalikan isi kosong (bukan 404) agar editor langsung bisa dipakai.
func (s *Server) getProjectDocNotes(c *fiber.Ctx) error {
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
	var row models.ProjectDocNotes
	content := models.ProjectDocContent{}
	err = s.db.Where("project_id = ?", id).First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		content.FillEmpty()
		return c.JSON(fiber.Map{"project_id": id, "content": content, "updated_at": nil})
	case err != nil:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if row.ContentJSON != "" {
		if err := json.Unmarshal([]byte(row.ContentJSON), &content); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "isi catatan rusak: " + err.Error()})
		}
	}
	content.FillEmpty()
	return c.JSON(fiber.Map{"project_id": id, "content": content, "updated_at": row.UpdatedAt})
}

// putProjectDocNotes: PUT /api/projects/:id/doc-notes — simpan (ganti seluruhnya) bagian yang ditulis tangan.
func (s *Server) putProjectDocNotes(c *fiber.Ctx) error {
	id, msg := paramID(c, "id", "project")
	if msg != "" {
		return badPayload(c, msg)
	}
	if len(c.Body()) > maxDocBody {
		return badPayload(c, fmt.Sprintf("isi dokumen maksimal %d KB", maxDocBody/1024))
	}
	var in models.ProjectDocContent
	if msg := firstProblem(bindJSONRequired(c, &in), normalizeDocContent(&in)); msg != "" {
		return badPayload(c, msg)
	}
	ok, err := s.projectExists(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "proyek tidak ditemukan"})
	}
	in.FillEmpty()
	raw, err := json.Marshal(in)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	row := models.ProjectDocNotes{ProjectID: id, ContentJSON: string(raw)}
	// Upsert pada unique index project_id (AutoMigrate membuat index-nya di SQLite & MySQL).
	if err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"content_json", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	var saved models.ProjectDocNotes
	if err := s.db.Where("project_id = ?", id).First(&saved).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"project_id": id, "content": in, "updated_at": saved.UpdatedAt})
}
