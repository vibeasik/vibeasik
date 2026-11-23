package diagnostics

import (
	"path/filepath"
	"testing"

	"github.com/vibeasik/vibeasik/pkg/models"
)

const playPubspec = "name: toko\ndependencies:\n  flutter:\n    sdk: flutter\n"

func TestPlayReadinessFlagsMissingObligations(t *testing.T) {
	dir := t.TempDir()
	pub := writeFixture(t, dir, "pubspec.yaml", playPubspec+"  midtrans_sdk: ^1.0.0\n")
	writeFixture(t, dir, "android/app/build.gradle.kts", "android {\n    defaultConfig {\n        targetSdk = 33\n    }\n}\n")
	writeFixture(t, dir, "lib/auth/register_page.dart", "class RegisterPage {}\n// Daftar akun baru\n")

	got := ruleSet(inspectPlayReadiness(pub, "mobile/pubspec.yaml"))
	for _, id := range []string{"PLAY-NO-ACCOUNT-DELETION", "PLAY-NO-PRIVACY-POLICY", "PLAY-EXTERNAL-PAYMENT", "PLAY-TARGET-SDK-OLD"} {
		if _, ok := got[id]; !ok {
			t.Errorf("expected %s, got %v", id, got)
		}
	}
	if f := got["PLAY-TARGET-SDK-OLD"]; f.Line != 3 || f.TargetFile != filepath.Join("mobile", "android", "app", "build.gradle.kts") {
		t.Errorf("target sdk finding points to %s:%d", f.TargetFile, f.Line)
	}
	if f := got["PLAY-EXTERNAL-PAYMENT"]; f.Line != 5 {
		t.Errorf("payment finding line = %d, want 5", f.Line)
	}
}

func TestPlayReadinessPassesCompliantApp(t *testing.T) {
	dir := t.TempDir()
	pub := writeFixture(t, dir, "pubspec.yaml", playPubspec+"  midtrans_sdk: ^1.0.0\n  in_app_purchase: ^3.0.0\n")
	writeFixture(t, dir, "android/app/build.gradle", "android {\n  defaultConfig {\n    targetSdkVersion flutter.targetSdkVersion\n  }\n}\n")
	writeFixture(t, dir, "lib/auth/register_page.dart", "class RegisterPage {}\n")
	writeFixture(t, dir, "lib/profile/settings_page.dart", "// Hapus akun\nvoid deleteAccount() {}\nconst privacyUrl = 'https://example.com/kebijakan-privasi';\n")

	if got := inspectPlayReadiness(pub, "pubspec.yaml"); len(got) != 0 {
		t.Fatalf("compliant app must have no findings, got %v", ruleSet(got))
	}
}

func TestPlayReadinessSkipsAppWithoutAccountsAndPackages(t *testing.T) {
	dir := t.TempDir()
	// Paket Flutter tanpa android/app (pustaka) tidak diperiksa.
	lib := writeFixture(t, dir, "pkg/pubspec.yaml", playPubspec)
	if got := inspectPlayReadiness(lib, "pkg/pubspec.yaml"); len(got) != 0 {
		t.Fatalf("package without android/app must be skipped, got %v", ruleSet(got))
	}
	// Aplikasi tanpa alur daftar tidak diwajibkan punya hapus akun.
	app := writeFixture(t, dir, "app/pubspec.yaml", playPubspec)
	writeFixture(t, dir, "app/android/app/build.gradle", "android {}\n")
	writeFixture(t, dir, "app/lib/main.dart", "// kalkulator; kebijakan privasi di menu\n")
	if got := ruleSet(inspectPlayReadiness(app, "app/pubspec.yaml")); len(got) != 0 {
		t.Fatalf("app without accounts must have no findings, got %v", got)
	}
}

func TestSEOReadiness(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "package.json", `{"dependencies":{"nuxt":"^4.0.0"}}`)
	writeFixture(t, dir, "nuxt.config.ts", "export default defineNuxtConfig({\n  ssr: false,\n})\n")
	writeFixture(t, dir, "app/pages/index.vue", "<template><h1>Halo</h1></template>\n")
	writeFixture(t, dir, "node_modules/x/index.js", "og:image application/ld+json llms.txt @nuxtjs/sitemap\n")
	comp := &models.Component{Role: models.RoleFrontendClient, AbsolutePath: dir, RelativePath: "web", Framework: "nuxt"}

	got := ruleSet(inspectSEOReadiness(comp))
	for _, id := range []string{"SEO-SPA-ONLY", RuleShareMissing, "SEO-NO-ROBOTS", "SEO-NO-SITEMAP", "SEO-NO-STRUCTURED-DATA", "SEO-NO-LLMS-TXT"} {
		f, ok := got[id]
		if !ok {
			t.Errorf("expected %s", id)
			continue
		}
		if id != RuleShareMissing && f.TargetFile != filepath.Join("web", "nuxt.config.ts") {
			t.Errorf("%s: target = %s", id, f.TargetFile)
		}
	}

	writeFixture(t, dir, "nuxt.config.ts", "export default defineNuxtConfig({\n  modules: ['@nuxtjs/sitemap'],\n})\n")
	writeFixture(t, dir, "app/app.vue", "<script setup>\nuseSeoMeta({ ogImage: 'https://example.com/og.jpg' })\nuseHead({ script: [{ type: 'application/ld+json', innerHTML: '{}' }] })\n</script>\n")
	writeFixture(t, dir, "public/robots.txt", "User-agent: *\nAllow: /\n")
	writeFixture(t, dir, "public/llms.txt", "# Situs\n")
	writeFixture(t, dir, "app/pages/index.vue", completeNuxtPage)
	writeBrandIcons(t, dir)
	if got := inspectSEOReadiness(comp); len(got) != 0 {
		t.Fatalf("ready website must have no findings, got %v", ruleSet(got))
	}

	admin := &models.Component{Role: models.RoleFrontendAdmin, AbsolutePath: t.TempDir()}
	writeFixture(t, admin.AbsolutePath, "package.json", "{}")
	if got := inspectSEOReadiness(admin); len(got) != 0 {
		t.Fatalf("admin panel must be skipped, got %v", ruleSet(got))
	}
}
