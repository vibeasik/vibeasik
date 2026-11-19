package api

import (
	"strings"
	"testing"

	"github.com/vibeasik/vibeasik/pkg/models"
)

func newDocNotesServer(t *testing.T) *Server {
	t.Helper()
	s := newGuardTestServer(t)
	if err := s.db.AutoMigrate(&models.ProjectDocNotes{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

// Proyek tanpa catatan → isi kosong dengan slice [] (bukan null); proyek tak dikenal → 404; id buruk → 400.
func TestProjectDocNotesGetEmptyAndMissing(t *testing.T) {
	s := newDocNotesServer(t)
	code, out := secTestRequest(t, s, "GET", "/api/projects/1/doc-notes", ``, testLocalToken)
	if code != 200 {
		t.Fatalf("GET → %d %v", code, out)
	}
	content, _ := out["content"].(map[string]interface{})
	if d, ok := content["decisions"].([]interface{}); !ok || len(d) != 0 {
		t.Errorf("decisions ingin [] kosong, dapat %#v", content["decisions"])
	}
	if code, _ := secTestRequest(t, s, "GET", "/api/projects/999/doc-notes", ``, testLocalToken); code != 404 {
		t.Errorf("proyek tak dikenal → %d, ingin 404", code)
	}
	if code, _ := secTestRequest(t, s, "GET", "/api/projects/0/doc-notes", ``, testLocalToken); code != 400 {
		t.Errorf("id 0 → %d, ingin 400", code)
	}
	if code, _ := secTestRequest(t, s, "GET", "/api/projects/1/doc-notes", ``, ""); code != 401 {
		t.Errorf("tanpa token → %d, ingin 401", code)
	}
}

// Simpan dua kali (upsert satu baris), butir kosong dibuang, spasi dirapikan, lalu terbaca kembali.
func TestProjectDocNotesPutRoundTrip(t *testing.T) {
	s := newDocNotesServer(t)
	body := `{"version":" 1.0 ","out_of_scope":["Aplikasi mobile","  "],` +
		`"decisions":[{"title":"Pakai MySQL","status":"accepted","decision":"MySQL 8"},{"title":"","decision":""}],` +
		`"glossary":[{"term":"ADR","definition":"Catatan keputusan arsitektur"}]}`
	if code, out := secTestRequest(t, s, "PUT", "/api/projects/1/doc-notes", body, testLocalToken); code != 200 {
		t.Fatalf("PUT → %d %v", code, out)
	}
	body2 := strings.Replace(body, "1.0", "1.1", 1)
	if code, out := secTestRequest(t, s, "PUT", "/api/projects/1/doc-notes", body2, testLocalToken); code != 200 {
		t.Fatalf("PUT kedua → %d %v", code, out)
	}
	var n int64
	s.db.Model(&models.ProjectDocNotes{}).Where("project_id = ?", 1).Count(&n)
	if n != 1 {
		t.Fatalf("baris catatan = %d, ingin 1 (upsert)", n)
	}
	code, out := secTestRequest(t, s, "GET", "/api/projects/1/doc-notes", ``, testLocalToken)
	if code != 200 {
		t.Fatalf("GET → %d", code)
	}
	c := out["content"].(map[string]interface{})
	if c["version"] != "1.1" {
		t.Errorf("version = %v, ingin 1.1", c["version"])
	}
	if got := c["out_of_scope"].([]interface{}); len(got) != 1 || got[0] != "Aplikasi mobile" {
		t.Errorf("out_of_scope = %v", got)
	}
	if got := c["decisions"].([]interface{}); len(got) != 1 {
		t.Errorf("decisions = %v, ingin 1 butir", got)
	}
}

// Payload tidak sah ditolak 400 dan tidak menyimpan apa pun.
func TestProjectDocNotesPutRejects(t *testing.T) {
	s := newDocNotesServer(t)
	cases := []struct{ body, want string }{
		{``, "wajib"},
		{`{"decisions":[{"title":"x","status":"maybe"}]}`, "status"},
		{`{"version":"` + strings.Repeat("v", 300) + `"}`, "version"},
		{`{"out_of_scope":[` + strings.TrimSuffix(strings.Repeat(`"a",`, 101), ",") + `]}`, "out_of_scope"},
	}
	for _, tc := range cases {
		code, out := secTestRequest(t, s, "PUT", "/api/projects/1/doc-notes", tc.body, testLocalToken)
		if code != 400 || !strings.Contains(strings.ToLower(out["error"].(string)), tc.want) {
			t.Errorf("body %.40q → %d %v, ingin 400 berisi %q", tc.body, code, out["error"], tc.want)
		}
	}
	if code, _ := secTestRequest(t, s, "PUT", "/api/projects/999/doc-notes", `{"version":"1"}`, testLocalToken); code != 404 {
		t.Errorf("proyek tak dikenal → %d, ingin 404", code)
	}
	var n int64
	s.db.Model(&models.ProjectDocNotes{}).Count(&n)
	if n != 0 {
		t.Errorf("ada %d baris tersimpan dari payload ditolak", n)
	}
}
