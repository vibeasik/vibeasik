package api

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/localauth"
	"github.com/vibeasik/vibeasik/pkg/models"
)

func textReq(t *testing.T, s *Server, method, path, body string, auth bool) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	if auth {
		req.Header.Set(localauth.Header, testLocalToken)
	}
	resp, err := s.app.Test(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestTextIngestAndTodoFlow(t *testing.T) {
	s := newGuardTestServer(t)
	if err := s.db.AutoMigrate(&models.TextSource{}, &models.TextEntry{}); err != nil {
		t.Fatal(err)
	}
	s.SetEnqueueAssignment(func(_ *gorm.DB, _ *models.Todo) error { return nil })
	var proj models.Project
	s.db.First(&proj)

	// Sumber dibuat dari Studio (butuh token lokal).
	if code, _ := textReq(t, s, "POST", "/api/texts/sources", `{"name":"Web","project_id":`+itoa(proj.ID)+`}`, false); code != 401 {
		t.Fatalf("buat sumber tanpa token = %d, want 401", code)
	}
	code, created := textReq(t, s, "POST", "/api/texts/sources", `{"name":"Web","project_id":`+itoa(proj.ID)+`}`, true)
	if code != 201 {
		t.Fatalf("buat sumber = %d %v", code, created)
	}
	var src models.TextSource
	s.db.First(&src)
	if !strings.HasPrefix(src.IngestKey, "vtx_") {
		t.Fatalf("kunci = %q", src.IngestKey)
	}

	// Ingest tanpa token lokal tapi dengan kunci sumber; kunci salah ditolak.
	if code, _ := textReq(t, s, "POST", "/api/texts/ingest", `{"key":"salah","texts":[]}`, false); code != 401 {
		t.Fatalf("ingest kunci salah = %d, want 401", code)
	}
	body := `{"key":"` + src.IngestKey + `","origin":"http://localhost:3000/x?y=1","texts":[` +
		`{"page":"/","section":"Hero","tag":"h1","selector":"main > h1","text":"Solusi terbaik untuk bisnis Anda"},` +
		`{"page":"/harga","section":"Paket harga","tag":"p","text":"Mulai dari Rp99.000"}]}`
	code, res := textReq(t, s, "POST", "/api/texts/ingest", body, false)
	if code != 202 || res["created"].(float64) != 2 {
		t.Fatalf("ingest = %d %v", code, res)
	}
	s.db.First(&src, src.ID)
	if src.LastOrigin != "http://localhost:3000" || src.LastIngestAt == nil {
		t.Fatalf("sumber tidak diperbarui: %+v", src)
	}

	// Daftar & filter.
	code, list := textReq(t, s, "GET", "/api/texts/sources/"+itoa(src.ID)+"/entries?page=/harga&q=Rp99", "", true)
	items, _ := list["items"].([]any)
	if code != 200 || len(items) != 1 {
		t.Fatalf("daftar = %d %v", code, list)
	}
	entryID := uint(items[0].(map[string]any)["id"].(float64))

	// Jadikan todo: instruksi wajib.
	if code, _ := textReq(t, s, "POST", "/api/texts/entries/"+itoa(entryID)+"/todo", `{"instruction":"  "}`, true); code != 400 {
		t.Fatalf("todo tanpa instruksi = %d, want 400", code)
	}
	code, td := textReq(t, s, "POST", "/api/texts/entries/"+itoa(entryID)+"/todo", `{"instruction":"Ganti jadi lebih santai, tanpa kata 'Mulai dari'"}`, true)
	if code != 201 || td["already_exists"] != false {
		t.Fatalf("buat todo = %d %v", code, td)
	}
	todo := td["todo"].(map[string]any)
	desc := todo["description"].(string)
	for _, want := range []string{"Mulai dari Rp99.000", "`/harga`", "Paket harga", "lebih santai"} {
		if !strings.Contains(desc, want) {
			t.Errorf("deskripsi todo tanpa %q:\n%s", want, desc)
		}
	}
	if todo["source"] != "text_edit" || todo["category"] != "ui_ux" {
		t.Errorf("todo = %v", todo)
	}

	// Kedua kali: todo terbuka yang sama dikembalikan, tidak ganda.
	code, again := textReq(t, s, "POST", "/api/texts/entries/"+itoa(entryID)+"/todo", `{"instruction":"lain"}`, true)
	if code != 200 || again["already_exists"] != true {
		t.Fatalf("todo ganda = %d %v", code, again)
	}
	var n int64
	s.db.Model(&models.Todo{}).Where("source = ?", "text_edit").Count(&n)
	if n != 1 {
		t.Fatalf("jumlah todo = %d, want 1", n)
	}

	// Daftar menampilkan status todo.
	_, list = textReq(t, s, "GET", "/api/texts/sources/"+itoa(src.ID)+"/entries?todo=has", "", true)
	items, _ = list["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["todo_status"] == "" {
		t.Fatalf("daftar todo=has = %v", list)
	}

	// Sumber dijeda: ingest diterima tapi tidak disimpan.
	textReq(t, s, "PUT", "/api/texts/sources/"+itoa(src.ID), `{"enabled":false}`, true)
	code, res = textReq(t, s, "POST", "/api/texts/ingest", `{"key":"`+src.IngestKey+`","texts":[{"page":"/","text":"Baru sekali"}]}`, false)
	if code != 202 || res["paused"] != true {
		t.Fatalf("ingest saat jeda = %d %v", code, res)
	}
}

func itoa(id uint) string { return strconv.FormatUint(uint64(id), 10) }
