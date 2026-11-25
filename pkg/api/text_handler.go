package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
	"github.com/vibeasik/vibeasik/pkg/texting"
)

// Teks Website: website mengirim semua teks yang tampil ke engine ini (arah website → vibeasik, tidak sebaliknya).
// Pemilik menelusuri teks per halaman/section di Studio lalu menjadikan satu teks todo "ubah teks".

type textSourceView struct {
	models.TextSource
	ProjectName   string `json:"project_name,omitempty"`
	ComponentName string `json:"component_name,omitempty"`
	Texts         int64  `json:"texts"` // jumlah teks unik tersimpan
	Pages         int64  `json:"pages"` // jumlah halaman berbeda
}

type textEntryView struct {
	models.TextEntry
	TodoStatus string `json:"todo_status,omitempty"` // status todo terakhir untuk teks ini
}

func newTextKey() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return "vtx_" + hex.EncodeToString(b)
}

func (s *Server) textSource(c *fiber.Ctx) (*models.TextSource, error) {
	id, bad := paramID(c, "id", "text source")
	if bad != "" {
		return nil, badPayload(c, bad)
	}
	var src models.TextSource
	if err := s.db.First(&src, id).Error; err != nil {
		return nil, c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Sumber teks tidak ditemukan"})
	}
	return &src, nil
}

type textSourceRequest struct {
	Name        string `json:"name"`
	ProjectID   *uint  `json:"project_id"`
	ComponentID *uint  `json:"component_id"`
	Enabled     *bool  `json:"enabled"`
}

// ingestTexts: dipanggil skrip browser di website. Kunci di header X-Vibeasik-Key atau field "key" di badan
// (badan text/plain = tanpa preflight CORS). Kunci hanya bisa mengirim teks, tidak bisa membaca apa pun.
func (s *Server) ingestTexts(c *fiber.Ctx) error {
	c.Set(fiber.HeaderAccessControlAllowOrigin, "*")
	var req struct {
		Key    string         `json:"key"`
		Origin string         `json:"origin"`
		Texts  []texting.Item `json:"texts"`
	}
	parseErr := json.Unmarshal(c.Body(), &req)
	key := strings.TrimSpace(c.Get("X-Vibeasik-Key"))
	if key == "" {
		key = strings.TrimSpace(req.Key)
	}
	var src models.TextSource
	if key == "" || s.db.Where("ingest_key = ?", key).First(&src).Error != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "kunci teks tidak valid"})
	}
	if parseErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "badan harus JSON"})
	}
	if !src.Enabled {
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"accepted": 0, "paused": true})
	}
	if len(req.Texts) > texting.MaxTextsPerIngest {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"error": fmt.Sprintf("maksimal %d teks per kiriman", texting.MaxTextsPerIngest)})
	}
	now := time.Now()
	res, err := texting.Ingest(s.db, &src, req.Texts, now)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	upd := map[string]any{"last_ingest_at": now}
	if o := cleanOrigin(req.Origin); o != "" {
		upd["last_origin"] = o
	}
	s.db.Model(&src).Updates(upd)
	return c.Status(fiber.StatusAccepted).JSON(res)
}

// cleanOrigin hanya menyimpan skema://host (tanpa path/query).
func cleanOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	o := u.Scheme + "://" + strings.ToLower(u.Host)
	if len(o) > 255 {
		return ""
	}
	return o
}

func (s *Server) listTextSources(c *fiber.Ctx) error {
	q := s.db.Order("id ASC")
	if pid := c.QueryInt("project_id", 0); pid > 0 {
		q = q.Where("project_id = ?", pid)
	}
	var list []models.TextSource
	if err := q.Find(&list).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]textSourceView, 0, len(list))
	if len(list) == 0 {
		return c.JSON(out)
	}
	srcIDs := make([]uint, 0, len(list))
	projIDs, compIDs := []uint{}, []uint{}
	for _, src := range list {
		srcIDs = append(srcIDs, src.ID)
		if src.ProjectID != nil {
			projIDs = append(projIDs, *src.ProjectID)
		}
		if src.ComponentID != nil {
			compIDs = append(compIDs, *src.ComponentID)
		}
	}
	type agg struct {
		SourceID uint
		Texts    int64
		Pages    int64
	}
	var aggs []agg
	s.db.Model(&models.TextEntry{}).Select("source_id, COUNT(*) AS texts, COUNT(DISTINCT page) AS pages").
		Where("source_id IN ?", srcIDs).Group("source_id").Scan(&aggs)
	counts := map[uint]agg{}
	for _, a := range aggs {
		counts[a.SourceID] = a
	}
	pNames, cNames := map[uint]string{}, map[uint]string{}
	if len(projIDs) > 0 {
		var ps []models.Project
		s.db.Select("id, name").Where("id IN ?", projIDs).Find(&ps)
		for _, p := range ps {
			pNames[p.ID] = p.Name
		}
	}
	if len(compIDs) > 0 {
		var cs []models.Component
		s.db.Select("id, name").Where("id IN ?", compIDs).Find(&cs)
		for _, comp := range cs {
			cNames[comp.ID] = comp.Name
		}
	}
	for _, src := range list {
		v := textSourceView{TextSource: src, Texts: counts[src.ID].Texts, Pages: counts[src.ID].Pages}
		if src.ProjectID != nil {
			v.ProjectName = pNames[*src.ProjectID]
		}
		if src.ComponentID != nil {
			v.ComponentName = cNames[*src.ComponentID]
		}
		out = append(out, v)
	}
	return c.JSON(out)
}

func (s *Server) createTextSource(c *fiber.Ctx) error {
	var req textSourceRequest
	if msg := firstProblem(bindJSON(c, &req), maxLen("name", req.Name, maxName)); msg != "" {
		return badPayload(c, msg)
	}
	src := models.TextSource{
		Name: strings.TrimSpace(req.Name), ProjectID: req.ProjectID, ComponentID: req.ComponentID,
		IngestKey: newTextKey(), Enabled: true,
	}
	if src.ComponentID != nil {
		var comp models.Component
		if s.db.Select("id, project_id, name").First(&comp, *src.ComponentID).Error == nil {
			if src.ProjectID == nil {
				src.ProjectID = &comp.ProjectID
			}
			if src.Name == "" {
				src.Name = comp.Name
			}
		}
	}
	if src.Name == "" {
		src.Name = "Website"
	}
	if err := s.db.Create(&src).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(src)
}

func (s *Server) updateTextSource(c *fiber.Ctx) error {
	var req textSourceRequest
	if msg := firstProblem(bindJSONRequired(c, &req), maxLen("name", req.Name, maxName)); msg != "" {
		return badPayload(c, msg)
	}
	src, err := s.textSource(c)
	if src == nil {
		return err
	}
	updates := map[string]any{}
	if n := strings.TrimSpace(req.Name); n != "" {
		updates["name"] = n
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.ComponentID != nil {
		updates["component_id"] = *req.ComponentID
	}
	if len(updates) > 0 {
		if err := s.db.Model(src).Updates(updates).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}
	s.db.First(src, src.ID)
	return c.JSON(src)
}

func (s *Server) deleteTextSource(c *fiber.Ctx) error {
	src, err := s.textSource(c)
	if src == nil {
		return err
	}
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("source_id = ?", src.ID).Delete(&models.TextEntry{}).Error; err != nil {
			return err
		}
		return tx.Delete(src).Error
	})
	if txErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": txErr.Error()})
	}
	return c.JSON(fiber.Map{"deleted": true})
}

func (s *Server) rotateTextKey(c *fiber.Ctx) error {
	src, err := s.textSource(c)
	if src == nil {
		return err
	}
	if err := s.db.Model(src).Update("ingest_key", newTextKey()).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"rotated": true})
}

func (s *Server) getTextSDK(c *fiber.Ctx) error {
	src, err := s.textSource(c)
	if src == nil {
		return err
	}
	var comp models.Component
	hasComp := src.ComponentID != nil && s.db.First(&comp, *src.ComponentID).Error == nil
	ingest := c.BaseURL() + "/api/texts/ingest"
	return c.JSON(fiber.Map{
		"ingest_url":     ingest,
		"ingest_key":     src.IngestKey,
		"script_path":    texting.ScriptPath,
		"usages":         texting.Usages(ingest, src.IngestKey),
		"component_dir":  map[bool]string{true: comp.AbsolutePath, false: ""}[hasComp],
		"component_name": map[bool]string{true: comp.Name, false: ""}[hasComp],
		"curl_test": fmt.Sprintf("curl -X POST %s -H 'Content-Type: application/json' -H 'X-Vibeasik-Key: %s' -d '{\"origin\":\"http://localhost:3000\",\"texts\":[{\"page\":\"/\",\"section\":\"Hero\",\"tag\":\"h1\",\"text\":\"Teks uji\"}]}'",
			ingest, src.IngestKey),
	})
}

func (s *Server) installTextSDK(c *fiber.Ctx) error {
	src, err := s.textSource(c)
	if src == nil {
		return err
	}
	var req struct {
		Overwrite bool `json:"overwrite"`
	}
	if msg := bindJSON(c, &req); msg != "" {
		return badPayload(c, msg)
	}
	var comp models.Component
	if src.ComponentID == nil || s.db.First(&comp, *src.ComponentID).Error != nil || comp.AbsolutePath == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Sumber belum terhubung ke komponen yang punya folder di laptop ini"})
	}
	written, werr := texting.InstallScript(comp.AbsolutePath, req.Overwrite)
	if werr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": werr.Error()})
	}
	return c.JSON(fiber.Map{"written": written, "path": texting.ScriptPath, "dir": comp.AbsolutePath})
}

// getTextFacets: daftar halaman (dengan jumlah teks) dan, bila ?page diberikan, daftar section halaman itu.
func (s *Server) getTextFacets(c *fiber.Ctx) error {
	src, err := s.textSource(c)
	if src == nil {
		return err
	}
	type facet struct {
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	pages, sections := []facet{}, []facet{}
	s.db.Model(&models.TextEntry{}).Select("page AS name, COUNT(*) AS count").
		Where("source_id = ?", src.ID).Group("page").Order("page ASC").Limit(500).Scan(&pages)
	if page := c.Query("page"); page != "" {
		s.db.Model(&models.TextEntry{}).Select("section AS name, COUNT(*) AS count").
			Where("source_id = ? AND page = ?", src.ID, page).Group("section").Order("section ASC").Limit(200).Scan(&sections)
	}
	return c.JSON(fiber.Map{"pages": pages, "sections": sections})
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s *Server) listTextEntries(c *fiber.Ctx) error {
	src, err := s.textSource(c)
	if src == nil {
		return err
	}
	q := s.db.Model(&models.TextEntry{}).Where("source_id = ?", src.ID)
	if page := c.Query("page"); page != "" {
		q = q.Where("page = ?", page)
	}
	if section := c.Query("section"); section != "" {
		q = q.Where("section = ?", section)
	}
	if term := strings.TrimSpace(c.Query("q")); term != "" {
		q = q.Where("text LIKE ? ESCAPE '\\'", "%"+escapeLike(term)+"%")
	}
	switch c.Query("todo") {
	case "has":
		q = q.Where("todo_id IS NOT NULL")
	case "none":
		q = q.Where("todo_id IS NULL")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	limit := c.QueryInt("limit", 50)
	if limit < 1 || limit > 200 {
		limit = 50
	}
	offset := c.QueryInt("offset", 0)
	if offset < 0 {
		offset = 0
	}
	var rows []models.TextEntry
	if err := q.Order("last_seen_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	todoIDs := []uint{}
	for _, r := range rows {
		if r.TodoID != nil {
			todoIDs = append(todoIDs, *r.TodoID)
		}
	}
	statuses := map[uint]string{}
	if len(todoIDs) > 0 {
		var todos []models.Todo
		s.db.Select("id, status").Where("id IN ?", todoIDs).Find(&todos)
		for _, t := range todos {
			statuses[t.ID] = t.Status
		}
	}
	items := make([]textEntryView, 0, len(rows))
	for _, r := range rows {
		v := textEntryView{TextEntry: r}
		if r.TodoID != nil {
			v.TodoStatus = statuses[*r.TodoID]
		}
		items = append(items, v)
	}
	return c.JSON(fiber.Map{"items": items, "total": total, "limit": limit, "offset": offset})
}

var openTodoStatuses = []string{models.TodoStatusPending, models.TodoStatusInProgress, models.TodoStatusSuggested}

// textEditTodoDescription menyusun isi todo: teks asli, lokasi, dan permintaan pemilik.
func textEditTodoDescription(e *models.TextEntry, origin, instruction string) string {
	var b strings.Builder
	b.WriteString("**Teks asli (persis seperti tampil di website):**\n\n")
	for _, line := range strings.Split(e.Text, "\n") {
		b.WriteString("> " + line + "\n")
	}
	b.WriteString("\n**Lokasi:**\n")
	fmt.Fprintf(&b, "- Halaman: `%s`", e.Page)
	if origin != "" {
		fmt.Fprintf(&b, " (%s)", origin)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "- Bagian halaman: %s\n", e.Section)
	if e.Tag != "" {
		fmt.Fprintf(&b, "- Jenis elemen: `%s`\n", e.Tag)
	}
	if e.Selector != "" {
		fmt.Fprintf(&b, "- Selector: `%s`\n", e.Selector)
	}
	b.WriteString("\n**Perubahan yang diminta pemilik:**\n\n")
	b.WriteString(instruction)
	b.WriteString("\n\n**Cara mengerjakan:** cari teks asli di kode (komponen, berkas i18n/terjemahan, data/CMS), ubah di sumbernya ")
	b.WriteString("— bukan di hasil render — dan ganti di semua tempat teks yang sama dipakai bila memang satu kalimat. ")
	b.WriteString("Pertahankan makna, ejaan nama/angka/harga, dan panjang yang masih muat di tata letak. ")
	b.WriteString("Setelah selesai, muat ulang halaman: teks baru akan muncul di Teks Website.")
	return b.String()
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// createTextEditTodo: tombol "Jadikan todo ubah teks". Todo terbuka untuk teks yang sama dikembalikan apa adanya.
func (s *Server) createTextEditTodo(c *fiber.Ctx) error {
	id, bad := paramID(c, "id", "text entry")
	if bad != "" {
		return badPayload(c, bad)
	}
	var req struct {
		Instruction string `json:"instruction"`
		Priority    string `json:"priority"`
	}
	if msg := firstProblem(bindJSONRequired(c, &req), needText("instruction", req.Instruction, maxText), oneOf("priority", req.Priority, todoPriorities...)); msg != "" {
		return badPayload(c, msg)
	}
	var entry models.TextEntry
	if err := s.db.First(&entry, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Teks tidak ditemukan"})
	}
	var src models.TextSource
	if err := s.db.First(&src, entry.SourceID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Sumber teks tidak ditemukan"})
	}
	if src.ProjectID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Hubungkan sumber ke proyek dulu agar todo punya tempat"})
	}

	key := fmt.Sprintf("TEXT|%d|%s", src.ID, entry.Hash)
	var existing models.Todo
	if s.db.Where("audit_key = ? AND status IN ?", key, openTodoStatuses).First(&existing).Error == nil {
		return c.JSON(fiber.Map{"todo": existing, "already_exists": true})
	}

	priority := req.Priority
	if priority == "" {
		priority = models.TodoPriorityMedium
	}
	todo := models.Todo{
		Level: models.TodoLevelProject, ProjectID: src.ProjectID, ComponentID: src.ComponentID, Category: models.CategoryUIUX,
		Title:       clipString(fmt.Sprintf("Ubah teks di %s: \"%s\"", entry.Page, clipRunes(entry.Text, 70)), 500),
		Description: textEditTodoDescription(&entry, src.LastOrigin, strings.TrimSpace(req.Instruction)),
		Priority:    priority, CreatorType: models.CreatorHuman, CreatorName: "pemilik",
		Source: "text_edit", AuditKey: key,
	}
	applyTodoDefaults(&todo)
	s.resolveTodoEntities(c, &todo)
	finalizeTodoPayload(&todo)

	var assignErr error
	txErr := s.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&todo).Error; err != nil {
			return err
		}
		if err := s.safeEnqueueTodoAssignment(tx, &todo); err != nil {
			assignErr = err
			return err
		}
		return tx.Model(&models.TextEntry{}).Where("id = ?", entry.ID).Update("todo_id", todo.ID).Error
	})
	if txErr != nil {
		return respondCreateTodoError(c, txErr, assignErr)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"todo": todo, "already_exists": false})
}
