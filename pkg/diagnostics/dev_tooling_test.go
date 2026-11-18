package diagnostics

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibeasik/vibeasik/pkg/util"

	"github.com/vibeasik/vibeasik/pkg/models"
)

func toolRuleIDs(fs []models.Finding) string {
	var ids []string
	for _, f := range fs {
		ids = append(ids, f.RuleID)
	}
	return strings.Join(ids, ",")
}

func TestInspectProjectTooling(t *testing.T) {
	root := t.TempDir()
	p := &models.Project{RootPath: root, Components: []models.Component{{Name: "api", RelativePath: "backend"}, {Name: "web", RelativePath: "frontend"}}}
	got := toolRuleIDs(inspectProjectTooling(p))
	if !strings.Contains(got, "DEV-SCRIPT-MISSING") || !strings.Contains(got, "DEPLOY-SCRIPT-MISSING") {
		t.Fatalf("missing scripts not reported: %s", got)
	}

	os.WriteFile(filepath.Join(root, "dev.sh"), []byte("cd backend && go run .\n"), 0o644)
	os.WriteFile(filepath.Join(root, "deploy.sh"), []byte("cp .env.server .env\n"), 0o755)
	got = toolRuleIDs(inspectProjectTooling(p))
	for _, want := range []string{"DEV-SCRIPT-NOT-EXECUTABLE", "DEV-SCRIPT-INCOMPLETE", "DEPLOY-ENV-NOT-RESTORED"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s, got %s", want, got)
		}
	}

	os.WriteFile(filepath.Join(root, "dev.sh"), []byte(devScriptTemplate), 0o755)
	os.Chmod(filepath.Join(root, "dev.sh"), 0o755)
	os.WriteFile(filepath.Join(root, "deploy.sh"), []byte(deployScriptTemplate), 0o755)
	if fs := inspectProjectTooling(p); len(fs) != 0 {
		t.Errorf("templates should pass their own audit, got %s", toolRuleIDs(fs))
	}
}

func TestDuplicateImports(t *testing.T) {
	lines := strings.Split("import { a } from 'vue'\nimport type { B } from 'vue'\nimport { c } from \"vue\"\nimport './x.css'", "\n")
	fs := inspectDuplicateImports("app/pages/x.vue", lines)
	if len(fs) != 1 || fs[0].Line != 3 {
		t.Fatalf("want one duplicate at line 3, got %+v", fs)
	}
}

func TestNuxtDuplicateAutoImports(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "nuxt.config.ts"), []byte("export default {}"), 0o644)
	os.MkdirAll(filepath.Join(dir, "app", "composables"), 0o755)
	os.MkdirAll(filepath.Join(dir, "app", "utils"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "composables", "useApi.ts"), []byte("export function useApi() {}\nexport const formatDate = 1\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "app", "utils", "format.ts"), []byte("export const formatDate = 2\n"), 0o644)
	fs := inspectNuxtDuplicateAutoImports(models.Component{AbsolutePath: dir})
	if len(fs) != 1 || !strings.Contains(fs[0].Title, "formatDate") {
		t.Fatalf("want formatDate duplicate, got %+v", fs)
	}

	os.WriteFile(filepath.Join(dir, "app", "composables", "useToast.ts"), []byte("export default function () {}\n"), 0o644)
	// useToast milik Nuxt UI: tanpa modul itu, composable useToast sendiri tidak bentrok.
	if fs = inspectNuxtDuplicateAutoImports(models.Component{AbsolutePath: dir}); len(fs) != 1 {
		t.Fatalf("useToast tanpa @nuxt/ui bukan duplikat, got %+v", fs)
	}
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"@nuxt/ui":"^3.0.0"}}`), 0o644)
	fs = inspectNuxtDuplicateAutoImports(models.Component{AbsolutePath: dir})
	if len(fs) != 2 || !strings.Contains(fs[1].Title, "useToast") || fs[1].TargetFile != "app/composables/useToast.ts" {
		t.Fatalf("want useToast clash with built-in, got %+v", fs)
	}
}

func TestFrontendBackendLink(t *testing.T) {
	fe, mob := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(fe, ".env.local"), []byte("NUXT_PUBLIC_API_BASE=http://localhost:3000\n"), 0o644)
	os.MkdirAll(filepath.Join(mob, "lib"), 0o755)
	os.WriteFile(filepath.Join(mob, "lib", "api.dart"), []byte("const base = 'http://127.0.0.1:4105';\n"), 0o644)
	p := &models.Project{Components: []models.Component{
		{ID: 1, Role: models.RoleBackend, RunPort: 4105},
		{ID: 2, Name: "web", Role: models.RoleFrontendClient, AbsolutePath: fe},
		{ID: 3, Name: "app", Role: models.RoleFrontendMobile, Language: "dart", AbsolutePath: mob},
	}}
	got := toolRuleIDs(inspectFrontendBackendLink(p, 0))
	if got != "FE-API-PORT-MISMATCH,MOBILE-API-LOCALHOST" {
		t.Fatalf("got %s", got)
	}
	if got := toolRuleIDs(inspectFrontendBackendLink(p, 3)); got != "MOBILE-API-LOCALHOST" {
		t.Fatalf("only filter: got %s", got)
	}
}

func TestStandardFindingsAndCodeTodos(t *testing.T) {
	root := t.TempDir()
	fe := filepath.Join(root, "frontend")
	os.MkdirAll(filepath.Join(fe, "app", "pages"), 0o755)
	os.WriteFile(filepath.Join(fe, ".env.local"), []byte("NUXT_PUBLIC_API_BASE=http://localhost:3000\n"), 0o644)
	os.WriteFile(filepath.Join(fe, "app", "pages", "index.vue"), []byte("<script setup lang=\"ts\">\nimport { ref } from 'vue'\nimport { computed } from 'vue'\n</script>\n"), 0o644)
	p := &models.Project{ID: 7, Name: "Toko", RootPath: root, Components: []models.Component{
		{ID: 1, Name: "api", Role: models.RoleBackend, RunPort: 4105, RelativePath: "backend"},
		{ID: 2, Name: "web", Role: models.RoleFrontendClient, AbsolutePath: fe, RelativePath: "frontend"},
	}}
	got := toolRuleIDs(StandardFindings(p))
	for _, want := range []string{"DEV-SCRIPT-MISSING", "DEPLOY-SCRIPT-MISSING", "ENV-EXAMPLE-MISSING"} {
		if !strings.Contains(got, want) {
			t.Errorf("standard findings missing %s: %s", want, got)
		}
	}
	refs := map[string]int{}
	for _, td := range CodeTodos(p) {
		refs[td.GuidelineRef]++
		if td.Status != models.TodoStatusPending || td.ComponentID == nil || *td.ProjectID != 7 {
			t.Errorf("bad todo %+v", td)
		}
		if td.GuidelineRef == "FE-015" && (!strings.Contains(td.Title, "index.vue") || !strings.Contains(td.Description, "File Target: `frontend/app/pages/index.vue`")) {
			t.Errorf("duplicate import todo must target the file: %+v", td)
		}
	}
	if refs["NET-010"] != 1 || refs["FE-015"] != 1 || len(refs) != 2 {
		t.Fatalf("unexpected code todos per guideline: %v", refs)
	}
}

func TestEnsureEnvFiles(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q")
	os.WriteFile(filepath.Join(dir, ".env"), []byte("PORT=3000\nNUXT_PUBLIC_API_BASE=http://localhost:8080\nOLLAMA_BASE_URL=http://localhost:11434\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".env.example"), []byte("PORT=3000\nNUXT_PUBLIC_API_BASE=http://localhost:8080\nAPP_NAME=toko\nJWT_SECRET=ganti\n"), 0o644)
	slot := 3
	p := &models.Project{ID: 1, PortSlot: &slot, Components: []models.Component{
		{ID: 1, Role: models.RoleFrontendClient, AbsolutePath: dir, RunPort: 4130},
		{ID: 2, Role: models.RoleBackend, AbsolutePath: t.TempDir(), RunPort: 4135},
	}}

	// .gitignore belum memuat .env.* → berkas env lokal/server tidak boleh dibuat (bisa ter-commit)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".env\n"), 0o644)
	EnsureEnvFiles(p)
	if util.FileExists(filepath.Join(dir, ".env.local")) || util.FileExists(filepath.Join(dir, ".env.server")) {
		t.Fatal("env files must not be created while git would not ignore them")
	}

	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(envGitignoreBlock+"\n"), 0o644)
	actions := EnsureEnvFiles(p)
	local, _ := os.ReadFile(filepath.Join(dir, ".env.local"))
	server, _ := os.ReadFile(filepath.Join(dir, ".env.server"))
	env, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if !strings.Contains(string(local), "PORT=4130") || !strings.Contains(string(local), "NUXT_PUBLIC_API_BASE=http://localhost:4135") {
		t.Errorf(".env.local must be a port-synced copy of .env:\n%s", local)
	}
	if !strings.Contains(string(env), "PORT=4130") || !strings.Contains(string(env), "OLLAMA_BASE_URL=http://localhost:11434") {
		t.Errorf(".env ports must follow the slot, external URLs untouched:\n%s", env)
	}
	if !strings.Contains(string(server), "APP_NAME=toko") || !strings.Contains(string(server), "JWT_SECRET=\n") || !strings.Contains(string(server), "NUXT_PUBLIC_API_BASE=\n") {
		t.Errorf(".env.server must keep plain defaults and blank secrets/localhost URLs:\n%s", server)
	}
	if len(actions) < 2 {
		t.Errorf("expected create actions, got %v", actions)
	}

	// Tidak pernah menimpa berkas yang sudah ada
	os.WriteFile(filepath.Join(dir, ".env.server"), []byte("DIISI=ya\n"), 0o600)
	EnsureEnvFiles(p)
	if server, _ = os.ReadFile(filepath.Join(dir, ".env.server")); string(server) != "DIISI=ya\n" {
		t.Errorf(".env.server overwritten: %s", server)
	}
}

func TestInspectMobileDevScript(t *testing.T) {
	root := t.TempDir()
	p := &models.Project{Name: "Nemu Dijalan", RootPath: root, Components: []models.Component{
		{Name: "api", RelativePath: "backend", Role: models.RoleBackend, RunPort: 8095},
		{Name: "app", RelativePath: "mobile", Role: models.RoleFrontendMobile, Language: "dart", Framework: "flutter"},
	}}
	fs := inspectMobileDevScript(p)
	if toolRuleIDs(fs) != "MOBILE-DEV-SCRIPT-MISSING" {
		t.Fatalf("want MOBILE-DEV-SCRIPT-MISSING, got %s", toolRuleIDs(fs))
	}
	script := fs[0].SuggestedFix
	for _, want := range []string{`"${ROOT_DIR}/mobile"`, "APP_PORT:-8095", `REMOTE_NAME="nemu_dijalan_mobile"`} {
		if !strings.Contains(script, want) {
			t.Errorf("template not filled: missing %s", want)
		}
	}
	if strings.Contains(script, "__MOBILE_DIR__") || strings.Contains(script, "__BE_PORT__") || strings.Contains(script, "__REMOTE_NAME__") {
		t.Error("placeholder left in template")
	}

	os.WriteFile(filepath.Join(root, "dev-mobile.sh"), []byte("cd mobile && flutter run\n"), 0o644)
	got := toolRuleIDs(inspectMobileDevScript(p))
	if got != "MOBILE-DEV-SCRIPT-NOT-EXECUTABLE,MOBILE-DEV-SCRIPT-INCOMPLETE" {
		t.Errorf("got %s", got)
	}

	os.WriteFile(filepath.Join(root, "dev-mobile.sh"), []byte(script), 0o755)
	os.Chmod(filepath.Join(root, "dev-mobile.sh"), 0o755)
	if fs := inspectMobileDevScript(p); len(fs) != 0 {
		t.Errorf("template should pass its own audit, got %s", toolRuleIDs(fs))
	}
	if out, err := exec.Command("bash", "-n", filepath.Join(root, "dev-mobile.sh")).CombinedOutput(); err != nil {
		t.Errorf("template bash syntax: %v %s", err, out)
	}

	p.Components = p.Components[:1]
	if fs := inspectMobileDevScript(p); len(fs) != 0 {
		t.Errorf("project without Flutter must not need dev-mobile.sh, got %s", toolRuleIDs(fs))
	}
}

func TestDevLANAccess(t *testing.T) {
	httpComps := []models.Component{
		{Name: "api", Role: models.RoleBackend, RelativePath: "backend"},
		{Name: "web", Role: models.RoleFrontendClient, RelativePath: "frontend"},
	}
	cases := []struct {
		name  string
		comps []models.Component
		files map[string]string // dev.sh tidak ditulis bila kuncinya tidak ada
		want  string
	}{
		{"no dev.sh", httpComps, map[string]string{"backend/.env": "HOST=127.0.0.1\n"}, ""},
		{"dev.sh without marker", httpComps, map[string]string{"dev.sh": "cd backend && go run . &\ncd frontend && bun run dev\n"}, "DEV-LAN-NOT-ACCESSIBLE"},
		{"marker only in comment", httpComps, map[string]string{"dev.sh": "# HOST=0.0.0.0 --host\ncd backend && go run .\n"}, "DEV-LAN-NOT-ACCESSIBLE"},
		{"HOST=0.0.0.0", httpComps, map[string]string{"dev.sh": "HOST=0.0.0.0 go run ./backend\n"}, ""},
		{"vite --host without value", httpComps, map[string]string{"dev.sh": "cd frontend && bunx vite --host\n"}, ""},
		{"--host loopback is not LAN", httpComps, map[string]string{"dev.sh": "cd frontend && nuxt dev --host 127.0.0.1\n"}, "DEV-LAN-NOT-ACCESSIBLE"},
		{"curl -H is not a host flag", httpComps, map[string]string{"dev.sh": "curl -H 'X-A: 1' http://x\n"}, "DEV-LAN-NOT-ACCESSIBLE"},
		{"next -H 0.0.0.0", httpComps, map[string]string{"dev.sh": "cd frontend && next dev -H 0.0.0.0\n"}, ""},
		{"IPv6 any", httpComps, map[string]string{"dev.sh": "BIND_ADDR=[::]:4105 go run ./backend\n"}, ""},
		{"only mobile and cli", []models.Component{
			{Name: "app", Role: models.RoleFrontendMobile, RelativePath: "mobile", Language: "dart"},
			{Name: "tool", Role: models.RoleCLI, RelativePath: "cli"},
		}, map[string]string{"dev.sh": "cd mobile && flutter run\n", "mobile/.env": "HOST=127.0.0.1\n", "cli/.env": "HOST=localhost\n"}, ""},
		{"component folder missing", httpComps[:1], map[string]string{"dev.sh": "go run .\n"}, ""},
		{"all components bind LAN in config", httpComps, map[string]string{
			"dev.sh":                  "cd backend && go run . &\ncd frontend && bun run dev\n",
			"frontend/nuxt.config.ts": "export default defineNuxtConfig({ devServer: { host: '0.0.0.0' } })\n",
			"backend/package.json":    `{"scripts":{"dev":"node server.js --host 0.0.0.0"}}`,
		}, ""},
		{"one component still loopback-only", httpComps, map[string]string{
			"dev.sh":                  "cd backend && go run . &\ncd frontend && bun run dev\n",
			"frontend/vite.config.ts": "export default { server: { host: true } }\n",
		}, "DEV-LAN-NOT-ACCESSIBLE"},
		{"env loopback not overridden", httpComps, map[string]string{
			"dev.sh":              "cd frontend && bunx vite --host\n",
			"backend/.env":        "DB_PASSWORD=rahasia\nHOST=127.0.0.1\n",
			"frontend/.env.local": "NUXT_HOST=\"localhost\"\n",
		}, "DEV-LAN-HOST-LOOPBACK,DEV-LAN-HOST-LOOPBACK"},
		{"env loopback overridden by dev.sh", httpComps, map[string]string{
			"dev.sh":              "export HOST=0.0.0.0\n(cd frontend && NUXT_HOST=\"0.0.0.0\" bun run dev)\n",
			"backend/.env":        "HOST=127.0.0.1\n",
			"frontend/.env.local": "NUXT_HOST=localhost\n",
		}, ""},
		{"override only in comment", httpComps, map[string]string{
			"dev.sh":       "# HOST=0.0.0.0\ncd frontend && bunx vite --host\n",
			"backend/.env": "LISTEN_ADDR=127.0.0.1:4105\n",
		}, "DEV-LAN-HOST-LOOPBACK"},
		{"override to ::1 is not LAN", httpComps, map[string]string{
			"dev.sh":       "HOST=::1 go run ./backend --host\n",
			"backend/.env": "HOST=::1\n",
		}, "DEV-LAN-HOST-LOOPBACK"},
		{"env already LAN", httpComps, map[string]string{
			"dev.sh":       "HOST=0.0.0.0 go run ./backend\n",
			"backend/.env": "HOST=0.0.0.0\nAPI_HOST=localhost\n",
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, c := range tc.comps {
				os.MkdirAll(filepath.Join(root, c.RelativePath), 0o755)
			}
			if tc.name == "component folder missing" {
				os.RemoveAll(filepath.Join(root, "backend"))
			}
			for rel, body := range tc.files {
				os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
				os.WriteFile(filepath.Join(root, rel), []byte(body), 0o755)
			}
			p := &models.Project{RootPath: root, Components: tc.comps}
			fs := inspectDevLANAccess(p)
			if got := toolRuleIDs(fs); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			for _, f := range fs {
				if f.RuleID != "DEV-LAN-HOST-LOOPBACK" {
					continue
				}
				if f.Line == 0 || f.SuggestedFix == "" || strings.Contains(f.Snippet, "rahasia") || strings.Count(f.Snippet, "=") != 1 {
					t.Errorf("bad loopback finding: %+v", f)
				}
				if !strings.HasPrefix(f.TargetFile, "backend/") && !strings.HasPrefix(f.TargetFile, "frontend/") {
					t.Errorf("TargetFile must be relative to project: %s", f.TargetFile)
				}
			}
		})
	}
}

func TestDevLANLoopbackFindingFields(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "backend"), 0o755)
	os.WriteFile(filepath.Join(root, "dev.sh"), []byte("go run ./backend --host\n"), 0o755)
	os.WriteFile(filepath.Join(root, "backend", ".env"), []byte("DB_PASSWORD=rahasia\nexport HOST='127.0.0.1'\n"), 0o644)
	p := &models.Project{RootPath: root, Components: []models.Component{{Name: "api", Role: models.RoleBackend, RelativePath: "backend"}}}
	fs := inspectDevLANAccess(p)
	if len(fs) != 1 {
		t.Fatalf("want one finding, got %+v", fs)
	}
	f := fs[0]
	if f.TargetFile != "backend/.env" || f.Line != 2 || f.Snippet != "HOST='127.0.0.1'" || f.SuggestedFix != "HOST=0.0.0.0" || !strings.HasPrefix(f.TheoryBasis, "NET-002") {
		t.Fatalf("unexpected finding fields: %+v", f)
	}
}

func TestDevScriptTemplateLAN(t *testing.T) {
	for _, want := range []string{"HOST=0.0.0.0 PORT=$BE_PORT", "HOST=0.0.0.0 PORT=$FE_PORT", `CORS_ORIGIN="http://localhost:$FE_PORT,http://$LAN_IP:$FE_PORT"`, `LAN: http://$LAN_IP:$FE_PORT"`, "NUXT_PUBLIC_API_BASE=http://$LAN_IP:$BE_PORT"} {
		if !strings.Contains(devScriptTemplate, want) {
			t.Errorf("dev.sh template missing %q", want)
		}
	}
	if strings.Contains(devScriptTemplate, "NUXT_PUBLIC_API_BASE=http://localhost") {
		t.Error("dev.sh template must not lock the browser API URL to localhost")
	}
	path := filepath.Join(t.TempDir(), "dev.sh")
	os.WriteFile(path, []byte(devScriptTemplate), 0o755)
	if out, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("dev.sh template is not valid bash: %v\n%s", err, out)
	}
	if !hasLANBindMarker(shellCode(devScriptTemplate)) {
		t.Error("dev.sh template must satisfy DEV-LAN-NOT-ACCESSIBLE")
	}
}
