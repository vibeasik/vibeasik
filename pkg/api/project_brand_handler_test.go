package api

import (
	"bytes"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibeasik/vibeasik/pkg/localauth"
	"github.com/vibeasik/vibeasik/pkg/models"
)

func newBrandServer(t *testing.T) *Server {
	t.Helper()
	s := newGuardTestServer(t)
	if err := s.db.AutoMigrate(&models.ProjectBrand{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

// Tanpa warna tersimpan → usulan deteksi dari kode; simpan → terbaca kembali; hapus → kembali ke deteksi.
func TestProjectBrandColorsDetectSaveReset(t *testing.T) {
	s := newBrandServer(t)
	var proj models.Project
	s.db.First(&proj, 1)
	css := filepath.Join(proj.RootPath, "main.css")
	if err := os.WriteFile(css, []byte(":root{--color-primary:#7c3aed}"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := secTestRequest(t, s, "GET", "/api/projects/1/brand-colors", ``, testLocalToken)
	colors, _ := out["colors"].(map[string]interface{})
	if code != 200 || out["saved"] != false || colors["primary"] != "#7c3aed" {
		t.Fatalf("GET → %d %v", code, out)
	}
	if code, out := secTestRequest(t, s, "PUT", "/api/projects/1/brand-colors", `{"primary":"#0EA5E9","accent":"#f97316"}`, testLocalToken); code != 200 {
		t.Fatalf("PUT → %d %v", code, out)
	}
	code, out = secTestRequest(t, s, "GET", "/api/projects/1/brand-colors", ``, testLocalToken)
	colors, _ = out["colors"].(map[string]interface{})
	if out["saved"] != true || colors["primary"] != "#0ea5e9" || colors["source"] != "manual" {
		t.Fatalf("setelah simpan → %d %v", code, out)
	}
	if code, _ := secTestRequest(t, s, "PUT", "/api/projects/1/brand-colors", `{"primary":"biru"}`, testLocalToken); code != 400 {
		t.Errorf("warna tidak sah → %d, ingin 400", code)
	}
	if code, _ := secTestRequest(t, s, "PUT", "/api/projects/999/brand-colors", `{"primary":"#000000"}`, testLocalToken); code != 404 {
		t.Errorf("proyek tak dikenal → %d, ingin 404", code)
	}
	code, out = secTestRequest(t, s, "DELETE", "/api/projects/1/brand-colors", ``, testLocalToken)
	colors, _ = out["colors"].(map[string]interface{})
	if code != 200 || out["saved"] != false || colors["primary"] != "#7c3aed" {
		t.Fatalf("DELETE → %d %v", code, out)
	}
}

func TestProjectDocPDFExport(t *testing.T) {
	s := newBrandServer(t)
	body := `{"title":"Demo App","subtitle":"Aplikasi contoh","sections":[{"no":"1","title":"Keterangan","blocks":[{"type":"p","text":"Isi **tebal**"},{"type":"table","head":["A","B"],"rows":[["x",{"t":"GET","tone":"get"}]]}]}]}`
	req := httptest.NewRequest("POST", "/api/projects/1/doc-pdf", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "localhost"
	req.Header.Set(localauth.Header, testLocalToken)
	resp, err := s.app.Test(req, 20000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(raw, []byte("%PDF-")) {
		t.Fatalf("status %d, tipe %q, awal %q", resp.StatusCode, resp.Header.Get("Content-Type"), raw[:min(20, len(raw))])
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "Dokumen-Teknis-Demo-App-") {
		t.Errorf("nama berkas %q", cd)
	}
	if code, _ := secTestRequest(t, s, "POST", "/api/projects/999/doc-pdf", body, testLocalToken); code != 404 {
		t.Errorf("proyek tak dikenal → %d, ingin 404", code)
	}
	if code, _ := secTestRequest(t, s, "POST", "/api/projects/1/doc-pdf", body, ""); code != 401 {
		t.Errorf("tanpa token → %d, ingin 401", code)
	}
}
