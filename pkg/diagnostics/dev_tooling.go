package diagnostics

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/vibeasik/vibeasik/pkg/util"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Default proyek vibeasik (Aturan 45): dev.sh menjalankan semua komponen sekaligus, deploy.sh memasang
// .env.<tujuan> lalu selalu mengembalikan .env.local, FE/mobile menunjuk port BE yang benar saat dev,
// dan tidak ada duplicate import (warning Nuxt/unimport maupun import ganda dalam satu berkas).

// effectivePort = port yang benar-benar dipakai saat dev: run_port vibeasik, atau port deklarasi kode.
func effectivePort(c models.Component) int {
	if c.RunPort > 0 {
		return c.RunPort
	}
	return c.Port
}

func isMobileComponent(c models.Component) bool {
	fw := strings.ToLower(c.Framework)
	return c.Role == models.RoleFrontendMobile || c.Language == "dart" ||
		strings.Contains(fw, "flutter") || strings.Contains(fw, "react-native") || strings.Contains(fw, "expo")
}

// inspectProjectTooling memeriksa skrip wajib di root proyek: dev.sh & deploy.sh.
func inspectProjectTooling(project *models.Project) []models.Finding {
	root := project.RootPath
	if root == "" || root == "-" {
		return nil
	}
	var findings []models.Finding
	findings = append(findings, inspectDevScript(project)...)
	findings = append(findings, inspectDevLANAccess(project)...)
	findings = append(findings, inspectMobileDevScript(project)...)
	findings = append(findings, inspectDeployScript(project)...)
	return findings
}

func inspectDevScript(project *models.Project) []models.Finding {
	devPath := filepath.Join(project.RootPath, "dev.sh")
	info, err := os.Stat(devPath)
	if err != nil {
		return []models.Finding{{
			Category:     "deployment",
			Severity:     "WARNING",
			RuleID:       "DEV-SCRIPT-MISSING",
			Title:        "dev.sh Belum Ada di Root Proyek",
			Description:  "Proyek wajib punya dev.sh di root yang menyalakan semua komponen (BE, FE, mobile) dengan satu perintah, memakai port dari blok proyek di PORTS.md, env FE→BE yang menunjuk port BE, dan listen di 0.0.0.0 agar bisa dibuka dari LAN.",
			Suggestion:   "Buat dev.sh: set PORT tiap komponen dari blok PORTS.md, jalankan BE & FE dengan HOST=0.0.0.0, ekspor URL API BE (IP LAN) ke FE/mobile dan CORS_ORIGIN ke BE, cetak URL LAN, jalankan semua komponen paralel, dan hentikan semuanya dengan trap saat Ctrl+C.",
			TargetFile:   "dev.sh",
			TheoryBasis:  "ARCH-002 Unified Local Orchestration (Dev Runner).",
			SuggestedFix: devScriptTemplate,
		}}
	}

	var findings []models.Finding
	content := readSmallFile(devPath)
	if info.Mode()&0o111 == 0 {
		findings = append(findings, models.Finding{
			Category:    "deployment",
			Severity:    "INFO",
			RuleID:      "DEV-SCRIPT-NOT-EXECUTABLE",
			Title:       "dev.sh Belum Executable",
			Description: "dev.sh ada tetapi tidak punya izin eksekusi, sehingga ./dev.sh gagal.",
			Suggestion:  "Jalankan chmod +x dev.sh lalu commit (git menyimpan bit eksekusi).",
			TargetFile:  "dev.sh",
		})
	}
	if missing := findMissingDevComponents(project.Components, content); len(missing) > 0 {
		findings = append(findings, models.Finding{
			Category:    "deployment",
			Severity:    "WARNING",
			RuleID:      "DEV-SCRIPT-INCOMPLETE",
			Title:       "dev.sh Tidak Menjalankan Semua Komponen",
			Description: fmt.Sprintf("dev.sh tidak menyebut folder komponen: %s. Semua komponen proyek wajib berjalan dari dev.sh.", strings.Join(missing, ", ")),
			Suggestion:  "Tambahkan perintah dev komponen tersebut ke dev.sh (cd ke foldernya, PORT dari blok PORTS.md).",
			TargetFile:  "dev.sh",
		})
	}
	return findings
}

func findMissingDevComponents(components []models.Component, content string) []string {
	var missing []string
	for _, c := range components {
		rel := strings.Trim(filepath.ToSlash(c.RelativePath), "/.")
		if rel == "" {
			continue
		}
		if !strings.Contains(content, rel) && !strings.Contains(content, filepath.Base(rel)) {
			missing = append(missing, fmt.Sprintf("%s (%s)", c.Name, rel))
		}
	}
	return missing
}

// isHTTPComponent: komponen yang melayani HTTP saat dev (backend atau frontend web). Mobile, CLI, docs, dan
// library dilewati karena tidak membuka port yang dibuka peramban.
func isHTTPComponent(c models.Component) bool {
	if isMobileComponent(c) {
		return false
	}
	return c.Role == models.RoleBackend || strings.HasPrefix(c.Role, "frontend")
}

// componentDir: folder komponen di disk, atau "" bila tidak bisa ditentukan / tidak ada.
func componentDir(project *models.Project, c models.Component) string {
	dir := c.AbsolutePath
	if dir == "" {
		rel := strings.Trim(filepath.ToSlash(c.RelativePath), "/")
		if rel == "" || rel == "." {
			return ""
		}
		dir = filepath.Join(project.RootPath, rel)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// shellCode: isi skrip tanpa baris komentar penuh (# ...), termasuk shebang.
func shellCode(content string) string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

var (
	// alamat semua-antarmuka: 0.0.0.0 atau [::] (bukan [::1])
	lanAddrRe = regexp.MustCompile(`0\.0\.0\.0|\[::\]`)
	// flag host CLI dev server (Nuxt/Vite --host, Next --hostname) beserta nilainya bila ada; Next -H hanya
	// dihitung bersama 0.0.0.0 (lanAddrRe) karena -H juga dipakai perintah lain (mis. curl -H header)
	hostFlagRe = regexp.MustCompile(`--host(?:name)?\b(?:[ =]+([^\s;&|)]+))?`)
	// host: true / host: '0.0.0.0' / host: '::' di nuxt.config / vite.config (devServer atau server)
	configLANHostRe = regexp.MustCompile(`\bhost\s*:\s*(?:true\b|['"](?:0\.0\.0\.0|::)['"])`)
	loopbackHostRe  = regexp.MustCompile(`^["']?(?:127\.0\.0\.1|localhost|::1|\[::1\])(?::\d+)?["']?$`)
)

// hasLANBindMarker: kode shell/skrip memuat tanda bind LAN: 0.0.0.0, [::], atau flag --host/--hostname yang
// tidak diarahkan ke loopback (Vite: --host tanpa nilai = semua antarmuka).
func hasLANBindMarker(code string) bool {
	if lanAddrRe.MatchString(code) {
		return true
	}
	for _, m := range hostFlagRe.FindAllStringSubmatch(code, -1) {
		if m[1] == "" || !loopbackHostRe.MatchString(m[1]) {
			return true
		}
	}
	return false
}

// componentBindsLAN: konfigurasi dev komponen sendiri jelas bind LAN, sehingga dev.sh tidak perlu menimpanya.
// Keterbatasan: yang dibaca hanya nuxt.config.*, vite.config.*, dan skrip "dev" package.json. Bind di kode
// (mis. Go http.ListenAndServe(":"+port), yang sebenarnya semua antarmuka) tidak dikenali, jadi backend tetap
// dianggap belum terbukti LAN sampai dev.sh menyatakan HOST=0.0.0.0.
func componentBindsLAN(dir string) bool {
	for _, name := range []string{"nuxt.config.ts", "nuxt.config.js", "nuxt.config.mjs", "vite.config.ts", "vite.config.js", "vite.config.mjs", "vite.config.mts"} {
		if configLANHostRe.MatchString(readSmallFile(filepath.Join(dir, name))) {
			return true
		}
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal([]byte(readSmallFile(filepath.Join(dir, "package.json"))), &pkg) == nil {
		if dev, ok := pkg.Scripts["dev"]; ok && hasLANBindMarker(" "+dev) {
			return true
		}
	}
	return false
}

// devScriptCode: isi dev.sh tanpa komentar, ok=false bila dev.sh tidak ada (sudah dilaporkan DEV-SCRIPT-MISSING).
func devScriptCode(project *models.Project) (string, bool) {
	path := filepath.Join(project.RootPath, "dev.sh")
	if !util.FileExists(path) {
		return "", false
	}
	return shellCode(readSmallFile(path)), true
}

// inspectDevLANAccess (NET-002): semua komponen HTTP wajib bisa dibuka dari LAN saat dev.
func inspectDevLANAccess(project *models.Project) []models.Finding {
	code, ok := devScriptCode(project)
	if !ok {
		return nil
	}
	var httpComps []models.Component
	var dirs []string
	for _, c := range project.Components {
		if !isHTTPComponent(c) {
			continue
		}
		if dir := componentDir(project, c); dir != "" {
			httpComps = append(httpComps, c)
			dirs = append(dirs, dir)
		}
	}
	if len(httpComps) == 0 {
		return nil
	}
	var findings []models.Finding
	if !hasLANBindMarker(code) {
		var notLAN []string
		for i, c := range httpComps {
			if !componentBindsLAN(dirs[i]) {
				notLAN = append(notLAN, fmt.Sprintf("%s (%s)", c.Name, strings.Trim(filepath.ToSlash(c.RelativePath), "/")))
			}
		}
		if len(notLAN) > 0 {
			findings = append(findings, models.Finding{
				Category:     "deployment",
				Severity:     "WARNING",
				RuleID:       "DEV-LAN-NOT-ACCESSIBLE",
				Title:        "Komponen Proyek Tidak Bisa Diakses dari LAN saat Dev",
				Description:  fmt.Sprintf("dev.sh tidak menjalankan komponen HTTP di 0.0.0.0 (tidak ada HOST=0.0.0.0, --host, atau -H 0.0.0.0), dan konfigurasi dev komponen berikut juga tidak bind LAN: %s. Akibatnya proyek tidak bisa dibuka dari HP atau komputer lain lewat http://<IP-LAN>:<port>.", strings.Join(notLAN, ", ")),
				Suggestion:   "Jalankan BE & FE di dev.sh dengan HOST=0.0.0.0 (Nuxt/Vite: --host 0.0.0.0, Next: -H 0.0.0.0), arahkan URL API FE ke http://$LAN_IP:$BE_PORT, beri BE CORS_ORIGIN yang memuat http://$LAN_IP:$FE_PORT (tanpa *), dan cetak URL LAN-nya. Hanya untuk dev; produksi tetap bind loopback di belakang reverse proxy.",
				TargetFile:   "dev.sh",
				TheoryBasis:  "NET-002 Semua Komponen Wajib Bisa Diakses dari LAN saat Development.",
				SuggestedFix: "LAN_IP=\"$(hostname -I 2>/dev/null | awk '{print $1}')\"; LAN_IP=\"${LAN_IP:-127.0.0.1}\"\n(cd \"$ROOT/backend\" && HOST=0.0.0.0 PORT=$BE_PORT CORS_ORIGIN=\"http://localhost:$FE_PORT,http://$LAN_IP:$FE_PORT\" go run .) &\n(cd \"$ROOT/frontend\" && HOST=0.0.0.0 PORT=$FE_PORT NUXT_PUBLIC_API_BASE=http://$LAN_IP:$BE_PORT bun run dev) &\necho \"LAN: http://$LAN_IP:$FE_PORT\"",
			})
		}
	}
	for i, c := range httpComps {
		findings = append(findings, inspectLoopbackHostEnv(c, dirs[i], code)...)
	}
	return findings
}

// envHostLineRe: kunci host bind di env dev bernilai loopback. Grup 1 = kunci, grup 2 = nilai.
var envHostLineRe = regexp.MustCompile(`^(?:export\s+)?(HOST|NUXT_HOST|NITRO_HOST|HOSTNAME|BIND_ADDR|LISTEN_ADDR)\s*=\s*(["']?(?:127\.0\.0\.1|localhost|::1|\[::1\])(?::\d+)?["']?)\s*$`)

// devScriptOverridesHost: dev.sh menimpa kunci host itu dengan 0.0.0.0 / [::] (KEY=0.0.0.0, KEY="0.0.0.0").
func devScriptOverridesHost(code, key string) bool {
	re := regexp.MustCompile(`(?m)(?:^|[\s;(&|])` + regexp.QuoteMeta(key) + `=["']?(?:0\.0\.0\.0|\[::\]|::(?:[^0-9a-fA-F:]|$))`)
	return re.MatchString(code)
}

// inspectLoopbackHostEnv: env dev komponen mengunci bind ke loopback dan dev.sh tidak menimpanya.
// Snippet hanya memuat kunci=nilai baris itu, tidak pernah baris env lain.
func inspectLoopbackHostEnv(c models.Component, dir, devCode string) []models.Finding {
	var findings []models.Finding
	for _, envName := range []string{".env.local", ".env", ".env.development"} {
		content := readSmallFile(filepath.Join(dir, envName))
		if content == "" {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(c.RelativePath, envName))
		for i, line := range strings.Split(content, "\n") {
			m := envHostLineRe.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil || devScriptOverridesHost(devCode, m[1]) {
				continue
			}
			findings = append(findings, models.Finding{
				Category:     "deployment",
				Severity:     "WARNING",
				RuleID:       "DEV-LAN-HOST-LOOPBACK",
				Title:        fmt.Sprintf("Host Dev '%s' Terkunci ke Loopback", c.Name),
				Description:  fmt.Sprintf("%s di %s membuat '%s' hanya listen di loopback, dan dev.sh tidak menimpanya dengan 0.0.0.0. Komponen tidak bisa dibuka dari HP atau komputer lain di jaringan yang sama.", m[1], rel, c.Name),
				Suggestion:   fmt.Sprintf("Ubah %s menjadi 0.0.0.0 di env dev, atau jalankan komponen dari dev.sh dengan %s=0.0.0.0. Produksi tetap bind loopback lewat .env.server.", m[1], m[1]),
				TargetFile:   rel,
				Line:         i + 1,
				Snippet:      m[1] + "=" + m[2],
				TheoryBasis:  "NET-002 Semua Komponen Wajib Bisa Diakses dari LAN saat Development.",
				SuggestedFix: m[1] + "=0.0.0.0",
			})
		}
	}
	return findings
}

// deployEnvOnServerRe: penanda eksplisit di deploy.sh bahwa env produksi dibaca di server, bukan disalin dari laptop.
var (
	deployEnvOnServerRe = regexp.MustCompile(`(?i)vibeasik:\s*env-on-server`)
	deployCopiesEnvRe   = regexp.MustCompile(`\bcp\b[^\n]*\.env`)
)

// deployEnvOnServer: deploy.sh menyatakan env-on-server dan memang tidak menyalin berkas .env apa pun.
func deployEnvOnServer(content string) bool {
	return deployEnvOnServerRe.MatchString(content) && !deployCopiesEnvRe.MatchString(content)
}

func inspectDeployScript(project *models.Project) []models.Finding {
	deployPath := filepath.Join(project.RootPath, "deploy.sh")
	if !util.FileExists(deployPath) {
		return []models.Finding{{
			Category:     "deployment",
			Severity:     "WARNING",
			RuleID:       "DEPLOY-SCRIPT-MISSING",
			Title:        "deploy.sh Belum Ada di Root Proyek",
			Description:  "Proyek wajib punya deploy.sh: pengguna cukup mengisi .env.server (atau .env.<nama> pilihannya), skrip memasangnya sebagai env aktif, build & upload ke server, lalu mengembalikan .env.local walau deploy gagal.",
			Suggestion:   "Buat deploy.sh dengan argumen nama env (default server), salin .env.<nama> ke berkas env aktif, trap EXIT untuk memulihkan .env.local, dan pastikan .gitignore memuat .env / .env.* / !.env.example.",
			TargetFile:   "deploy.sh",
			TheoryBasis:  "SEC-002 & DEV-007: hanya .env.example yang di-commit; env server dipasang saat deploy saja.",
			SuggestedFix: deployScriptTemplate,
		}}
	}

	var findings []models.Finding
	content := readSmallFile(deployPath)
	if deployEnvOnServer(content) {
		// Rahasia produksi hanya ada di server dan deploy tidak menyalin .env lokal: tidak ada env yang perlu
		// dipilih atau dikembalikan, jadi kedua aturan env di bawah tidak berlaku.
		return nil
	}
	if !strings.Contains(content, ".env.local") || !strings.Contains(content, "trap") {
		findings = append(findings, models.Finding{
			Category:    "deployment",
			Severity:    "WARNING",
			RuleID:      "DEPLOY-ENV-NOT-RESTORED",
			Title:       "deploy.sh Tidak Mengembalikan .env.local",
			Description: "deploy.sh wajib mengembalikan env aktif ke .env.local setelah deploy, termasuk saat deploy gagal (trap EXIT). Tanpa itu laptop dev tertinggal memakai env server.",
			Suggestion:  "Tambahkan trap 'cp .env.local .env' EXIT (per folder komponen) sebelum menyalin .env.<tujuan>.",
			TargetFile:  "deploy.sh",
		})
	}
	if !deployEnvSelectRe.MatchString(content) {
		findings = append(findings, models.Finding{
			Category:    "deployment",
			Severity:    "INFO",
			RuleID:      "DEPLOY-ENV-NOT-SELECTABLE",
			Title:       "deploy.sh Tidak Memakai .env.<tujuan>",
			Description: "deploy.sh tidak terlihat memasang .env.server / .env.<nama> pilihan pengguna, sehingga konfigurasi server tidak cukup diatur lewat satu berkas env.",
			Suggestion:  "Terima argumen nama env (./deploy.sh server-baru → .env.server-baru, default .env.server) lalu salin sebagai env aktif selama build & upload.",
			TargetFile:  "deploy.sh",
		})
	}
	return findings
}

// isFlutterComponent: komponen mobile Flutter, yang dijalankan ke HP lewat dev-mobile.sh (flutter-win).
func isFlutterComponent(c models.Component) bool {
	return isMobileComponent(c) && (c.Language == "dart" || strings.Contains(strings.ToLower(c.Framework), "flutter"))
}

// inspectMobileDevScript (Aturan 49): proyek dengan komponen Flutter wajib punya dev-mobile.sh di root yang
// menjalankan aplikasi ke HP/emulator di PC Windows (flutter-win, default) atau laptop, dengan API_BASE_URL ke
// IP LAN laptop. Patokan: nemudijalan/dev-mobile.sh.
func inspectMobileDevScript(project *models.Project) []models.Finding {
	var mobile *models.Component
	for i := range project.Components {
		if isFlutterComponent(project.Components[i]) {
			mobile = &project.Components[i]
			break
		}
	}
	if mobile == nil {
		return nil
	}
	path := filepath.Join(project.RootPath, "dev-mobile.sh")
	info, err := os.Stat(path)
	if err != nil {
		return []models.Finding{{
			Category:     "deployment",
			Severity:     "WARNING",
			RuleID:       "MOBILE-DEV-SCRIPT-MISSING",
			Title:        "dev-mobile.sh Belum Ada di Root Proyek",
			Description:  "Proyek dengan aplikasi mobile Flutter wajib punya dev-mobile.sh di root agar aplikasi bisa langsung dijalankan ke HP: default di PC Windows lewat flutter-win (sync kode via SSH lalu flutter run), atau './dev-mobile.sh laptop', dengan API_BASE_URL menunjuk IP LAN laptop dan port BE dari PORTS.md.",
			Suggestion:   "Salin contoh di bawah ke dev-mobile.sh (chmod +x), sesuaikan folder mobile, port BE, path API & berkas env dev Flutter bila berbeda, lalu commit.",
			TargetFile:   "dev-mobile.sh",
			TheoryBasis:  "ARCH-002 Unified Local Orchestration (Dev Runner), patokan nemudijalan/dev-mobile.sh.",
			SuggestedFix: mobileDevScript(project, mobile),
		}}
	}
	var findings []models.Finding
	if info.Mode()&0o111 == 0 {
		findings = append(findings, models.Finding{
			Category:    "deployment",
			Severity:    "INFO",
			RuleID:      "MOBILE-DEV-SCRIPT-NOT-EXECUTABLE",
			Title:       "dev-mobile.sh Belum Executable",
			Description: "dev-mobile.sh ada tetapi tidak punya izin eksekusi, sehingga ./dev-mobile.sh gagal.",
			Suggestion:  "Jalankan chmod +x dev-mobile.sh lalu commit (git menyimpan bit eksekusi).",
			TargetFile:  "dev-mobile.sh",
		})
	}
	content := readSmallFile(path)
	rel := strings.Trim(filepath.ToSlash(mobile.RelativePath), "/.")
	var missing []string
	for _, want := range []string{rel, "flutter-win", "API_BASE_URL"} {
		if want != "" && !strings.Contains(content, want) {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		findings = append(findings, models.Finding{
			Category:     "deployment",
			Severity:     "WARNING",
			RuleID:       "MOBILE-DEV-SCRIPT-INCOMPLETE",
			Title:        "dev-mobile.sh Tidak Sesuai Standar",
			Description:  fmt.Sprintf("dev-mobile.sh tidak menyebut: %s. Skrip wajib menjalankan folder mobile lewat flutter-win (PC Windows, default) atau laptop, dengan API_BASE_URL ke IP LAN laptop.", strings.Join(missing, ", ")),
			Suggestion:   "Samakan dengan contoh standar (patokan nemudijalan/dev-mobile.sh).",
			TargetFile:   "dev-mobile.sh",
			SuggestedFix: mobileDevScript(project, mobile),
		})
	}
	return findings
}

//go:embed dev_mobile.sh.tmpl
var mobileDevScriptTemplate string

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// mobileDevScript mengisi template dev-mobile.sh: folder mobile, port BE proyek, & folder unik di PC Windows.
func mobileDevScript(project *models.Project, mobile *models.Component) string {
	rel := strings.Trim(filepath.ToSlash(mobile.RelativePath), "/.")
	if rel == "" {
		rel = "mobile"
	}
	bePort := 4105
	for _, c := range project.Components {
		if c.Role == models.RoleBackend && effectivePort(c) > 0 {
			bePort = effectivePort(c)
			break
		}
	}
	remote := strings.Trim(nonSlugRe.ReplaceAllString(strings.ToLower(project.Name), "_"), "_")
	if remote == "" {
		remote = "app"
	}
	return strings.NewReplacer("__MOBILE_DIR__", rel, "__BE_PORT__", strconv.Itoa(bePort),
		"__REMOTE_NAME__", remote+"_"+filepath.Base(rel)).Replace(mobileDevScriptTemplate)
}

var deployEnvSelectRe = regexp.MustCompile(`\.env\.(server|production|prod|\$|"\$)`)

func readSmallFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 256*1024 {
		return ""
	}
	return string(b)
}

// Port lintas proyek: setiap proyek memakai blok PORTS.md sendiri, jadi port sama = bentrok saat dijalankan serentak.
func crossProjectPortFindings(project *models.Project, allProjects []models.Project) []models.Finding {
	var findings []models.Finding
	reported := map[int]bool{}
	for _, comp := range project.Components {
		port := effectivePort(comp)
		if port <= 0 || reported[port] {
			continue
		}
		var shared []string
		for _, other := range allProjects {
			if other.ID == project.ID {
				continue
			}
			for _, oc := range other.Components {
				if effectivePort(oc) == port {
					shared = append(shared, fmt.Sprintf("'%s' (%s)", other.Name, oc.Name))
				}
			}
		}
		if len(shared) == 0 {
			continue
		}
		reported[port] = true
		findings = append(findings, models.Finding{
			ID:          fmt.Sprintf("shared_port_%d", port),
			RuleID:      fmt.Sprintf("PORT-SHARED-%d", port),
			Category:    "port",
			Severity:    "WARNING",
			Title:       fmt.Sprintf("Port Bentrok Antar Proyek: :%d", port),
			Description: fmt.Sprintf("Komponen '%s' memakai port %d yang juga dipakai %s. Proyek tidak bisa dijalankan serentak (address already in use).", comp.Name, port, strings.Join(shared, ", ")),
			Suggestion:  "Pindahkan ke port blok proyek di PORTS.md (Aturan 35) lewat vibeasik_assign_port, perbarui env PORT, dev.sh, dan URL API FE/mobile dalam satu commit.",
			TargetFile:  comp.Name,
			Line:        port,
			TheoryBasis: "NET-001 Alokasi Port per Blok Proyek.",
		})
	}
	return findings
}

var (
	envURLLineRe = regexp.MustCompile(`^(?:export\s+)?([A-Za-z_][A-Za-z0-9_.]*)\s*=\s*["']?(https?://[^\s"']+)`)
	localURLRe   = regexp.MustCompile(`https?://(localhost|127\.0\.0\.1|0\.0\.0\.0)(?::(\d+))?`)
	// kunci URL BE milik proyek sendiri (NUXT_PUBLIC_API_BASE, VITE_API_URL, API_BASE_URL, BACKEND_URL, ...);
	// URL layanan luar (OLLAMA_BASE_URL, SYSTEM_ONE_URL) sengaja tidak cocok
	apiKeyHint = regexp.MustCompile(`(?i)^(?:[A-Z]+_PUBLIC_|VITE_|REACT_APP_|PUBLIC_)?(?:API|BACKEND|BE|SERVER)(?:_BASE)?(?:_URL|_HOST|_ORIGIN)?$`)
)

func collectBackendPorts(project *models.Project) (map[int]bool, []string) {
	bePorts := map[int]bool{}
	var bePortList []string
	for _, c := range project.Components {
		if c.Role == models.RoleBackend && effectivePort(c) > 0 {
			bePorts[effectivePort(c)] = true
			bePortList = append(bePortList, strconv.Itoa(effectivePort(c)))
		}
	}
	return bePorts, bePortList
}

func isInspectableFEComponent(c models.Component, only uint) bool {
	if only != 0 && c.ID != only {
		return false
	}
	if c.Role == models.RoleBackend {
		return false
	}
	return strings.HasPrefix(c.Role, "frontend") || isMobileComponent(c)
}

func fePortMismatchFinding(c models.Component, rel string, lineNo int, key, url string, port int, bePortList []string) models.Finding {
	return models.Finding{
		Category:    "architecture",
		Severity:    "WARNING",
		RuleID:      "FE-API-PORT-MISMATCH",
		Title:       fmt.Sprintf("URL API '%s' Tidak Menunjuk Port Backend", c.Name),
		Description: fmt.Sprintf("%s di %s menunjuk port %d, sedangkan backend proyek berjalan di port %s. Saat dev, data BE gagal dimuat.", key, rel, port, strings.Join(bePortList, "/")),
		Suggestion:  "Samakan URL API dengan port BE dari blok PORTS.md, dan biarkan dev.sh mengekspor URL ini agar selalu sinkron dengan PORT backend.",
		TargetFile:  rel,
		Line:        lineNo,
		Snippet:     key + "=" + url,
	}
}

func inspectComponentEnvURLs(c models.Component, bePorts map[int]bool, bePortList []string) []models.Finding {
	var findings []models.Finding
	mobile := isMobileComponent(c)
	for _, envName := range []string{".env.local", ".env", ".env.development"} {
		path := filepath.Join(c.AbsolutePath, envName)
		rel := filepath.ToSlash(filepath.Join(c.RelativePath, envName))
		for _, e := range envURLs(path) {
			if !apiKeyHint.MatchString(e.key) {
				continue
			}
			m := localURLRe.FindStringSubmatch(e.url)
			if m == nil {
				continue
			}
			if mobile {
				findings = append(findings, mobileLocalhostFinding(c, rel, e.line, e.key+"="+e.url))
			}
			if port, _ := strconv.Atoi(m[2]); port > 0 && len(bePorts) > 0 && !bePorts[port] {
				findings = append(findings, fePortMismatchFinding(c, rel, e.line, e.key, e.url, port, bePortList))
			}
		}
	}
	return findings
}

// inspectFrontendBackendLink memastikan FE & mobile memuat data BE yang benar saat dev: URL API di env
// menunjuk port BE proyek ini, dan aplikasi mobile tidak memakai localhost (emulator/HP tidak bisa menjangkaunya).
// only != 0 membatasi pemeriksaan ke satu komponen (AnalyzeComponent).
func inspectFrontendBackendLink(project *models.Project, only uint) []models.Finding {
	bePorts, bePortList := collectBackendPorts(project)
	var findings []models.Finding
	for _, c := range project.Components {
		if !isInspectableFEComponent(c, only) {
			continue
		}
		findings = append(findings, inspectComponentEnvURLs(c, bePorts, bePortList)...)
		if isMobileComponent(c) {
			findings = append(findings, mobileSourceLocalhost(c)...)
		}
	}
	return findings
}

type envURL struct {
	line     int
	key, url string
}

// envURLs mengembalikan setiap baris env bernilai URL.
func envURLs(path string) []envURL {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []envURL
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		if m := envURLLineRe.FindStringSubmatch(strings.TrimSpace(sc.Text())); m != nil {
			out = append(out, envURL{n, m[1], m[2]})
		}
	}
	return out
}

func mobileLocalhostFinding(c models.Component, rel string, line int, snippet string) models.Finding {
	return models.Finding{
		Category:     "mobile",
		Severity:     "WARNING",
		RuleID:       "MOBILE-API-LOCALHOST",
		Title:        fmt.Sprintf("Aplikasi Mobile '%s' Memanggil BE lewat localhost", c.Name),
		Description:  "Di emulator Android localhost adalah emulator itu sendiri, dan di HP fisik adalah HP itu, sehingga data BE tidak pernah termuat saat dev.",
		Suggestion:   "Ambil URL API dari env/--dart-define yang diisi dev.sh dengan IP LAN laptop (atau 10.0.2.2 khusus emulator Android); pastikan BE listen di 0.0.0.0 (NET-002) dan izinkan cleartext HTTP hanya untuk build debug.",
		TargetFile:   rel,
		Line:         line,
		Snippet:      snippet,
		SuggestedFix: "const apiBase = String.fromEnvironment('API_BASE_URL', defaultValue: 'http://10.0.2.2:4105');\n// dev.sh: flutter run --dart-define=API_BASE_URL=http://$LAN_IP:$BE_PORT",
	}
}

func isMobileSourceFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".dart" || ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx"
}

func findFirstLocalhostLine(content string) (int, string, bool) {
	for i, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "//") || !localURLRe.MatchString(trim) {
			continue
		}
		return i + 1, trim, true
	}
	return 0, "", false
}

// mobileSourceLocalhost mencari URL localhost tertanam di kode mobile (lib/, src/, app/), maksimal satu temuan per berkas.
func mobileSourceLocalhost(c models.Component) []models.Finding {
	var findings []models.Finding
	scanned := 0
	for _, sub := range []string{"lib", "src", "app"} {
		dir := filepath.Join(c.AbsolutePath, sub)
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if ignoredDirNames[d.Name()] || strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !isMobileSourceFile(path) {
				return nil
			}
			if scanned++; scanned > 600 || len(findings) >= 5 {
				return filepath.SkipAll
			}
			if lineNo, trim, ok := findFirstLocalhostLine(readSmallFile(path)); ok {
				rel, _ := filepath.Rel(c.AbsolutePath, path)
				findings = append(findings, mobileLocalhostFinding(c, filepath.ToSlash(filepath.Join(c.RelativePath, rel)), lineNo, trim))
			}
			return nil
		})
	}
	return findings
}

var (
	// import ... from 'x' / import 'x' (baris tunggal), dipisah antara import type dan import nilai
	importNamespaceRe = regexp.MustCompile(`^\s*import\s+(type\s+)?\*\s+as\s`)
	importFromRe      = regexp.MustCompile(`^\s*import\s+(type\s+)?(?:[^'";]*?\s+from\s+)?['"]([^'"]+)['"]`)
	// ekspor bernama tingkat atas yang dibaca unimport (auto-import Nuxt) dari composables/ & utils/
	exportNameRe = regexp.MustCompile(`^export\s+(?:async\s+)?(?:const|let|var|function\*?|class)\s+([A-Za-z_$][\w$]*)`)
)

// inspectDuplicateImports: modul yang sama diimpor lebih dari sekali dalam satu berkas JS/TS/Vue.
func inspectDuplicateImports(relPath string, lines []string) []models.Finding {
	seen := map[string]int{}
	var findings []models.Finding
	for i, line := range lines {
		m := importFromRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := m[1] + m[2]
		if importNamespaceRe.MatchString(line) {
			// `import * as vue from 'vue'` tidak bisa digabung dengan `import { h } from 'vue'` dalam satu
			// pernyataan, jadi bukan import ganda; hanya namespace yang sama dua kali yang dihitung.
			key = "*" + key
		}
		first, dup := seen[key]
		if !dup {
			seen[key] = i + 1
			continue
		}
		findings = append(findings, models.Finding{
			Category:     "frontend",
			Severity:     "WARNING",
			RuleID:       "FE-DUPLICATE-IMPORT",
			Title:        fmt.Sprintf("Duplicate Import: '%s'", m[2]),
			Description:  fmt.Sprintf("'%s' diimpor lagi di baris %d padahal sudah diimpor di baris %d.", m[2], i+1, first),
			Suggestion:   "Gabungkan menjadi satu pernyataan import untuk modul tersebut.",
			TargetFile:   relPath,
			Line:         i + 1,
			Snippet:      strings.TrimSpace(line),
			SuggestedFix: "import { a, b } from 'modul'",
		})
	}
	return findings
}

func isNuxtComponent(c models.Component) bool {
	return strings.Contains(strings.ToLower(c.Framework), "nuxt") ||
		util.FileExists(filepath.Join(c.AbsolutePath, "nuxt.config.ts")) || util.FileExists(filepath.Join(c.AbsolutePath, "nuxt.config.js"))
}

func extractFileExports(filePath, fileName string) []string {
	var names []string
	seen := map[string]bool{}
	for _, line := range strings.Split(readSmallFile(filePath), "\n") {
		if m := exportNameRe.FindStringSubmatch(line); m != nil {
			if !seen[m[1]] {
				seen[m[1]] = true
				names = append(names, m[1])
			}
		} else if strings.HasPrefix(line, "export default") {
			name := camelFromFile(fileName)
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

func scanNuxtAutoImportDir(c models.Component, dirRel string, owners map[string][]string) {
	dir := filepath.Join(c.AbsolutePath, dirRel)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if e.IsDir() || (ext != ".ts" && ext != ".js") || strings.Contains(e.Name(), ".test.") || strings.Contains(e.Name(), ".spec.") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(c.RelativePath, dirRel, e.Name()))
		for _, name := range extractFileExports(filepath.Join(dir, e.Name()), e.Name()) {
			owners[name] = append(owners[name], rel)
		}
	}
}

func findDuplicateAutoImports(owners map[string][]string, deps map[string]bool) []models.Finding {
	for n, files := range owners {
		if builtinFor(n, deps) {
			owners[n] = append(files, "#imports (bawaan Nuxt/Vue)")
		}
	}
	var names []string
	for n, files := range owners {
		if len(files) > 1 {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var findings []models.Finding
	for _, n := range names {
		files := owners[n]
		sort.Strings(files)
		findings = append(findings, models.Finding{
			Category:    "frontend",
			Severity:    "WARNING",
			RuleID:      "FE-DUPLICATE-IMPORT",
			Title:       fmt.Sprintf("Duplicated Auto-Import Nuxt: '%s'", n),
			Description: fmt.Sprintf("'%s' diekspor dari %s. Nuxt (unimport) menampilkan WARN Duplicated imports dan hanya memakai salah satunya, jadi kode bisa memanggil fungsi yang salah.", n, strings.Join(files, " dan ")),
			Suggestion:  "Sisakan satu definisi (atau ganti nama salah satunya) lalu perbarui pemanggilnya; warning dev server harus bersih.",
			TargetFile:  userFile(files),
		})
	}
	return findings
}

// inspectNuxtDuplicateAutoImports meniru warning unimport "Duplicated imports" pada Nuxt: nama yang sama
// diekspor dari lebih dari satu berkas di composables/ atau utils/, sehingga salah satunya diabaikan diam-diam.
func inspectNuxtDuplicateAutoImports(c models.Component) []models.Finding {
	if !isNuxtComponent(c) {
		return nil
	}
	owners := map[string][]string{}
	for _, dirRel := range []string{"composables", "utils", "app/composables", "app/utils", "shared/utils"} {
		scanNuxtAutoImportDir(c, dirRel, owners)
	}
	return findDuplicateAutoImports(owners, componentDeps(c))
}

// moduleAutoImports: auto-import yang hanya ada bila modul Nuxt tertentu terpasang — useToast bukan bawaan Nuxt,
// tetapi milik Nuxt UI; composable useToast buatan sendiri tanpa modul itu tidak bentrok dengan apa pun.
var moduleAutoImports = map[string][]string{
	"useToast":     {"@nuxt/ui", "@nuxt/ui-pro"},
	"useColorMode": {"@nuxt/ui", "@nuxt/ui-pro", "@nuxtjs/color-mode"},
}

// componentDeps: nama paket di dependencies & devDependencies package.json komponen.
func componentDeps(c models.Component) map[string]bool {
	var pkg struct {
		Deps    map[string]string `json:"dependencies"`
		DevDeps map[string]string `json:"devDependencies"`
	}
	_ = json.Unmarshal([]byte(readSmallFile(filepath.Join(c.AbsolutePath, "package.json"))), &pkg)
	out := map[string]bool{}
	for n := range pkg.Deps {
		out[n] = true
	}
	for n := range pkg.DevDeps {
		out[n] = true
	}
	return out
}

// builtinFor: nama ini auto-import bawaan (Nuxt/Vue, atau modul yang terpasang).
func builtinFor(name string, deps map[string]bool) bool {
	mods, needsModule := moduleAutoImports[name]
	if !needsModule {
		return builtinAutoImports[name]
	}
	for _, m := range mods {
		if deps[m] {
			return true
		}
	}
	return false
}

// builtinAutoImports: auto-import bawaan Nuxt/Vue/Nuxt UI yang sering tertimpa composable buatan sendiri.
var builtinAutoImports = map[string]bool{
	"ref": true, "computed": true, "reactive": true, "watch": true, "watchEffect": true, "nextTick": true,
	"onMounted": true, "onUnmounted": true, "onBeforeUnmount": true, "toRef": true, "toRefs": true,
	"useFetch": true, "useLazyFetch": true, "useAsyncData": true, "useLazyAsyncData": true, "useState": true,
	"useRoute": true, "useRouter": true, "navigateTo": true, "useHead": true, "useSeoMeta": true,
	"useCookie": true, "useRuntimeConfig": true, "useAppConfig": true, "useNuxtApp": true, "useError": true,
	"createError": true, "showError": true, "clearError": true, "useRequestHeaders": true, "definePageMeta": true,
	"defineNuxtPlugin": true, "defineNuxtRouteMiddleware": true,
}

func userFile(files []string) string {
	for _, f := range files {
		if !strings.HasPrefix(f, "#") {
			return f
		}
	}
	return files[0]
}

func camelFromFile(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	parts := strings.FieldsFunc(stem, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

const devScriptTemplate = `#!/usr/bin/env bash
# Jalankan semua komponen proyek sekaligus di 0.0.0.0 (LAN, NET-002). Port dari blok PORTS.md (Aturan 35).
set -e
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BE_PORT="${BE_PORT:-4105}"; FE_PORT="${FE_PORT:-4100}"
LAN_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"; LAN_IP="${LAN_IP:-127.0.0.1}"
trap 'kill 0' EXIT INT TERM
(cd "$ROOT/backend" && HOST=0.0.0.0 PORT=$BE_PORT CORS_ORIGIN="http://localhost:$FE_PORT,http://$LAN_IP:$FE_PORT" go run .) &
(cd "$ROOT/frontend" && HOST=0.0.0.0 PORT=$FE_PORT NUXT_PUBLIC_API_BASE=http://$LAN_IP:$BE_PORT bun run dev) &
# (cd "$ROOT/mobile" && flutter run --dart-define=API_BASE_URL=http://$LAN_IP:$BE_PORT) &
echo "Lokal: http://localhost:$FE_PORT  LAN: http://$LAN_IP:$FE_PORT"; wait`

const deployScriptTemplate = `#!/usr/bin/env bash
# ./deploy.sh [nama-env]  → pakai .env.<nama-env> (default: server), lalu kembali ke .env.local.
set -e
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="${1:-server}"
DIRS=(backend frontend)
for d in "${DIRS[@]}"; do
  [ -f "$ROOT/$d/.env.$TARGET" ] || { echo "Isi dulu $d/.env.$TARGET (salin dari .env.example)"; exit 1; }
done
# deploy pertama: simpan env dev sekarang sebagai .env.local agar tidak pernah hilang
for d in "${DIRS[@]}"; do [ -f "$ROOT/$d/.env.local" ] || [ ! -f "$ROOT/$d/.env" ] || cp "$ROOT/$d/.env" "$ROOT/$d/.env.local"; done
restore() { for d in "${DIRS[@]}"; do [ ! -f "$ROOT/$d/.env.local" ] || cp "$ROOT/$d/.env.local" "$ROOT/$d/.env"; done; }
trap restore EXIT
for d in "${DIRS[@]}"; do cp "$ROOT/$d/.env.$TARGET" "$ROOT/$d/.env"; done
# build + upload; host & path server dibaca dari .env.$TARGET (DEPLOY_HOST, DEPLOY_PATH)
set -a; . "$ROOT/backend/.env.$TARGET"; set +a
(cd "$ROOT/backend" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o app .)
(cd "$ROOT/frontend" && bun run build)
rsync -az --delete "$ROOT/backend/app" "$ROOT/frontend/.output" "$DEPLOY_HOST:$DEPLOY_PATH/"`
