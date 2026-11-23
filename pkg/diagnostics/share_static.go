package diagnostics

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/vibeasik/vibeasik/pkg/models"
	"github.com/vibeasik/vibeasik/pkg/util"
)

// Audit statis pratinjau bagikan per halaman (SEO-003, SEO-004, KONTEN-014): memindai berkas halaman publik
// (Nuxt pages/, Next app/ dan pages/, HTML statis) tanpa server berjalan. Satu temuan per halaman bermasalah.

// Rule ID ikon situs.
const (
	RuleFaviconStarter = "SEO-FAVICON-STARTER"
	RuleIconIncomplete = "SEO-ICON-INCOMPLETE"
)

var (
	sharePageNuxtRe    = regexp.MustCompile(`^(?:app/|src/)?pages/(.+)\.vue$`)
	sharePageNextAppRe = regexp.MustCompile(`^(?:src/)?app/(?:(.*)/)?page\.(?:tsx|jsx|js|ts)$`)
	sharePageNextPgRe  = regexp.MustCompile(`^(?:src/)?pages/(.+)\.(?:tsx|jsx)$`)
	sharePageHTMLRe    = regexp.MustCompile(`^(?:public/|static/)?([^/]+)\.html$`)

	sharePrivateSegs = map[string]bool{"admin": true, "login": true, "masuk": true, "register": true, "daftar": true,
		"signup": true, "signin": true, "logout": true, "keluar": true, "dashboard": true, "akun": true, "account": true,
		"auth": true, "api": true, "panel": true, "settings": true, "pengaturan": true, "404": true, "error": true}
	sharePrivateContentRe = regexp.MustCompile(`(?i)middleware\s*:\s*\[?\s*['"](?:auth|admin)|layout\s*:\s*['"](?:admin|dashboard)|noindex|robots\s*:\s*['"]?noindex`)

	shareHelperRe   = regexp.MustCompile(`\buse[A-Z]\w*(?:Seo|Share|Meta)\w*\s*\(`)
	shareAnyMetaRe  = regexp.MustCompile(`useSeoMeta|useServerSeoMeta|useHead|defineOgImage|generateMetadata|export\s+const\s+metadata|next/head|<Head|og:title|og:image|ogTitle|ogImage`)
	shareTitleRe    = regexp.MustCompile(`ogTitle|og:title|openGraph\s*:`)
	shareDescRe     = regexp.MustCompile(`ogDescription|og:description|openGraph\s*:`)
	shareImageRe    = regexp.MustCompile(`ogImage|og:image|openGraph\s*:[\s\S]*?images|defineOgImage|twitter-image`)
	shareTwitterRe  = regexp.MustCompile(`twitterCard|twitter:card|summary_large_image`)
	shareOGModuleRe = regexp.MustCompile(`"(?:@nuxtjs/seo|nuxt-og-image)"`)
	shareSEOUtilsRe = regexp.MustCompile(`"(?:@nuxtjs/seo|nuxt-seo-utils|nuxt-site-config)"`)
	shareValueRe    = map[string]*regexp.Regexp{
		"title":       regexp.MustCompile(`(?:ogTitle|og:title['"]?\s*,\s*content)\s*[:=]?\s*([^,\n}]+)`),
		"description": regexp.MustCompile(`(?:ogDescription|og:description['"]?\s*,\s*content)\s*[:=]?\s*([^,\n}]+)`),
		"image":       regexp.MustCompile(`(?:ogImage|og:image['"]?\s*,\s*content)\s*[:=]?\s*([^,\n}]+)`),
	}
	shareConstRe   = regexp.MustCompile("^(?:['\"`][^$'\"`]*['\"`]|[A-Z][A-Z0-9_]{2,}|site[A-Z]\\w*|config\\.\\w+)$")
	shareLocalImg  = regexp.MustCompile(`^['"` + "`" + `]?(/[^'"` + "`" + `\s]+\.(?:jpe?g|png|webp|gif))['"` + "`" + `]?$`)
	shareMetaLnRe  = regexp.MustCompile(`useSeoMeta|useServerSeoMeta|useHead|generateMetadata|export\s+const\s+metadata|<Head|og:title|ogTitle`)
	shareStaticDir = []string{"public", "static", "app/public", "src/public"}
)

// sharePage satu berkas halaman publik beserta rutenya.
type sharePage struct {
	rel, route string
	dynamic    bool
	src        string
	dir        string
}

// inspectSharePages memindai halaman publik komponen dan mengembalikan temuan serta jumlah halaman yang dipindai.
func inspectSharePages(comp *models.Component) ([]models.Finding, int) {
	root := comp.AbsolutePath
	pkg := readSmallFile(filepath.Join(root, "package.json"))
	pages := collectSharePages(root)
	if len(pages) == 0 {
		return nil, 0
	}
	globalTwitter := globalHasTwitterCard(root, pages)
	autoOG := shareOGModuleRe.MatchString(pkg)
	utils := shareSEOUtilsRe.MatchString(pkg)

	var findings []models.Finding
	for _, p := range pages {
		if f, ok := inspectSharePage(comp, p, root, autoOG, utils, globalTwitter || autoOG); ok {
			findings = append(findings, f)
		}
	}
	return findings, len(pages)
}

func collectSharePages(root string) []sharePage {
	var pages []sharePage
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (ignoredDirNames[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		route, ok := shareRouteOf(rel)
		if !ok || len(pages) >= readinessMaxFiles {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > readinessMaxFileSize {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || sharePrivateContentRe.Match(data) || isPrivateShareRoute(route) {
			return nil
		}
		pages = append(pages, sharePage{rel: rel, route: route, dynamic: strings.Contains(route, "["), src: string(data), dir: filepath.Dir(path)})
		return nil
	})
	sort.Slice(pages, func(i, j int) bool { return pages[i].rel < pages[j].rel })
	return pages
}

// shareRouteOf menurunkan path rute dari path berkas relatif; ok false bila bukan berkas halaman.
func shareRouteOf(rel string) (string, bool) {
	var r string
	switch {
	case sharePageNuxtRe.MatchString(rel):
		r = sharePageNuxtRe.FindStringSubmatch(rel)[1]
	case sharePageNextAppRe.MatchString(rel):
		r = sharePageNextAppRe.FindStringSubmatch(rel)[1]
	case sharePageNextPgRe.MatchString(rel):
		r = sharePageNextPgRe.FindStringSubmatch(rel)[1]
		if strings.HasPrefix(r, "_") || strings.HasPrefix(r, "api/") {
			return "", false
		}
	case sharePageHTMLRe.MatchString(rel):
		r = sharePageHTMLRe.FindStringSubmatch(rel)[1]
	default:
		return "", false
	}
	var segs []string
	for _, s := range strings.Split(r, "/") {
		if s == "" || s == "index" || (strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")")) {
			continue
		}
		segs = append(segs, s)
	}
	return "/" + strings.Join(segs, "/"), true
}

func isPrivateShareRoute(route string) bool {
	for _, s := range strings.Split(route, "/") {
		s = strings.ToLower(strings.Trim(s, "[]."))
		if sharePrivateSegs[s] || strings.HasPrefix(s, "_") {
			return true
		}
	}
	return false
}

// globalHasTwitterCard: apakah twitter:card summary_large_image diset di berkas non-halaman (app.vue, layout, config).
func globalHasTwitterCard(root string, pages []sharePage) bool {
	pageSet := map[string]bool{}
	for _, p := range pages {
		pageSet[p.rel] = true
	}
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			if path != root && (ignoredDirNames[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if pageSet[filepath.ToSlash(rel)] || !seoSourceExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		if info, err := d.Info(); err == nil && info.Size() <= readinessMaxFileSize {
			if data, err := os.ReadFile(path); err == nil && shareTwitterRe.Match(data) {
				found = true
			}
		}
		return nil
	})
	return found
}

func inspectSharePage(comp *models.Component, p sharePage, root string, autoOG, utils, twitterOK bool) (models.Finding, bool) {
	complete := shareHelperRe.MatchString(p.src) && !shareAnyMetaRe.MatchString(shareHelperRe.ReplaceAllString(p.src, ""))
	var problems []string
	rule := ""
	mark := func(r, msg string) {
		problems = append(problems, msg)
		if rule == "" {
			rule = r
		}
	}

	if !complete {
		if miss := missingShareParts(p, autoOG, utils); len(miss) > 0 {
			mark(RuleShareMissing, "tanpa "+strings.Join(miss, ", ")+" sendiri")
		}
	}
	if p.dynamic && !complete && !autoOG {
		if k := genericShareKeys(p.src); len(k) > 0 {
			mark(RuleShareGeneric, "halaman dinamis memakai teks/gambar konstanta untuk "+strings.Join(k, ", ")+", bukan dari datanya")
		}
	}
	if msg := badShareImage(p, root, autoOG || utils); msg != "" {
		mark(RuleShareImage, msg)
	}
	if !complete && !twitterOK && !shareTwitterRe.MatchString(p.src) {
		mark(RuleShareNoTwitter, "tanpa twitter:card summary_large_image (juga tidak di app.vue/layout/konfigurasi)")
	}
	if rule == "" {
		return models.Finding{}, false
	}
	line := shareMetaLine(p.src)
	advice := shareRuleAdvice(rule)
	return models.Finding{
		Category:     "frontend",
		Severity:     "WARNING",
		RuleID:       rule,
		Title:        shareRuleTitle(rule) + ": " + p.route,
		Description:  "Halaman " + p.route + " (" + p.rel + "): " + strings.Join(problems, "; ") + ". Tautan halaman ini tampil dengan pratinjau umum atau polos saat dibagikan ke WhatsApp, Messenger, Facebook, X, LinkedIn, dan Telegram.",
		Suggestion:   advice,
		SuggestedFix: advice,
		TargetFile:   filepath.Join(comp.RelativePath, filepath.FromSlash(p.rel)),
		Line:         line,
		Snippet:      shareLineText(p.src, line),
		TheoryBasis:  "Open Graph protocol; patokan SEO-003, SEO-004, dan KONTEN-014.",
	}, true
}

func missingShareParts(p sharePage, autoOG, utils bool) []string {
	titleRe, descRe := shareTitleRe, shareDescRe
	if utils || autoOG {
		titleRe = regexp.MustCompile(shareTitleRe.String() + `|\btitle\s*:|<title`)
		descRe = regexp.MustCompile(shareDescRe.String() + `|\bdescription\s*:|name=["']description`)
	}
	var miss []string
	if !titleRe.MatchString(p.src) {
		miss = append(miss, "og:title")
	}
	if !descRe.MatchString(p.src) {
		miss = append(miss, "og:description")
	}
	if !autoOG && !shareImageRe.MatchString(p.src) && !hasSiblingShareImage(p.dir) {
		miss = append(miss, "og:image")
	}
	return miss
}

func hasSiblingShareImage(dir string) bool {
	for _, n := range []string{"opengraph-image", "twitter-image"} {
		for _, e := range []string{".png", ".jpg", ".jpeg", ".tsx", ".ts", ".js"} {
			if _, err := os.Stat(filepath.Join(dir, n+e)); err == nil {
				return true
			}
		}
	}
	return false
}

// genericShareKeys mengembalikan kunci og yang nilainya konstanta literal (bukan dari data) di halaman dinamis.
func genericShareKeys(src string) []string {
	var keys []string
	for _, name := range []string{"title", "description", "image"} {
		for _, m := range shareValueRe[name].FindAllStringSubmatch(src, -1) {
			if shareConstRe.MatchString(strings.TrimSpace(m[1])) {
				keys = append(keys, "og:"+name)
				break
			}
		}
	}
	return keys
}

// badShareImage memeriksa nilai og:image literal: http, relatif, atau berkas lokal bukan 1200x630 / terlalu besar.
func badShareImage(p sharePage, root string, absolutized bool) string {
	for _, m := range shareValueRe["image"].FindAllStringSubmatch(p.src, -1) {
		v := strings.Trim(strings.TrimSpace(m[1]), "'\"`")
		switch {
		case strings.HasPrefix(v, "http://"):
			return fmt.Sprintf("og:image %q memakai http, wajib https absolut", v)
		case strings.HasPrefix(v, "https://") || v == "" || strings.ContainsAny(v, "$(") || !shareLooksLikePath(v):
			continue
		}
		if !strings.HasPrefix(v, "/") || !absolutized {
			return fmt.Sprintf("og:image %q relatif, wajib URL absolut https", v)
		}
		if msg := localShareImageProblem(root, v); msg != "" {
			return msg
		}
	}
	return ""
}

func shareLooksLikePath(v string) bool {
	return shareLocalImg.MatchString(v) || strings.HasPrefix(v, "/") || strings.HasPrefix(v, "./") || strings.HasPrefix(v, "~")
}

func localShareImageProblem(root, ref string) string {
	if !shareLocalImg.MatchString(ref) {
		return ""
	}
	for _, d := range shareStaticDir {
		data, err := os.ReadFile(filepath.Join(root, d, filepath.FromSlash(ref)))
		if err != nil {
			continue
		}
		if is := checkShareImageBytes(data); len(is) > 0 {
			return "berkas " + ref + ": " + is[0].Message
		}
		return ""
	}
	return fmt.Sprintf("berkas gambar bagikan %s tidak ditemukan di folder publik", ref)
}

func shareMetaLine(src string) int {
	if loc := shareMetaLnRe.FindStringIndex(src); loc != nil {
		return strings.Count(src[:loc[0]], "\n") + 1
	}
	return 1
}

func shareLineText(src string, line int) string {
	lines := strings.Split(src, "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[line-1])
}

// --- Ikon situs ---------------------------------------------------------------------------------------------

type starterIcon struct {
	name string
	size int
	sha  string // kosong = cocok hanya dari ukuran persis
}

// starterFavicons: sidik jari favicon.ico bawaan starter. Nuxt/Vue memakai berkas 4286 byte (SHA-256 terverifikasi
// dari berkas starter); Create React App (3870 byte) dan Next.js (25931 byte) dikenali dari ukuran persisnya.
var starterFavicons = []starterIcon{
	{"Nuxt/Vue", 4286, "1057b17aec08a7191d134000203947f195a8aa7c84c39f1164cee8d01279762a"},
	{"Create React App", 3870, ""},
	{"Next.js", 25931, ""},
}

// StarterFaviconName mengembalikan nama kerangka kerja bila data adalah favicon.ico bawaan starter.
func StarterFaviconName(data []byte) (string, bool) {
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	for _, s := range starterFavicons {
		if len(data) == s.size && (s.sha == "" || s.sha == hexSum) {
			return s.name, true
		}
	}
	return "", false
}

var (
	shareIconLinkRe = regexp.MustCompile(`(?is)rel\s*[:=]\s*['"][^'"]*icon[^'"]*['"][^}>]{0,200}?href\s*[:=]\s*['"](/[^'"]+)['"]`)
	shareIconSrcs   = []string{"nuxt.config.ts", "nuxt.config.js", "app.vue", "app/app.vue", "index.html", "public/index.html",
		"app/layout.tsx", "src/app/layout.tsx"}
)

// inspectSiteIcons memeriksa favicon bermerek dan kelengkapan set ikon (SEO-012).
func inspectSiteIcons(comp *models.Component) []models.Finding {
	root := comp.AbsolutePath
	staticDir := ""
	for _, d := range shareStaticDir {
		if util.PathExists(filepath.Join(root, d)) {
			staticDir = d
			break
		}
	}
	var out []models.Finding
	mk := func(rule, sev, title, desc, advice, target string) {
		out = append(out, models.Finding{Category: "frontend", Severity: sev, RuleID: rule, Title: title, Description: desc,
			Suggestion: advice, SuggestedFix: advice, TargetFile: filepath.Join(comp.RelativePath, target), Line: 1,
			TheoryBasis: "Patokan SEO-012 dan SEO-013 (ikon situs bermerek)."})
	}
	icoRel, ico := findFaviconICO(root)
	if ico != nil {
		if fw, ok := StarterFaviconName(ico); ok {
			mk(RuleFaviconStarter, "WARNING", "favicon.ico Adalah Ikon Bawaan Starter "+fw,
				fmt.Sprintf("%s (%d byte) identik dengan ikon bawaan starter %s. Perayap, Google, dan aplikasi yang meminta /favicon.ico langsung tetap menampilkan logo kerangka kerja walau <link rel=icon> menunjuk berkas lain.", icoRel, len(ico), fw),
				"Ganti dengan favicon.ico bermerek (16/32/48 px) dari logo situs. Uji: curl -sI https://<domain>/favicon.ico harus 200 dan isinya bukan ikon starter. Lihat SEO-012.", icoRel)
		}
	}
	if missing := missingSiteIcons(root, staticDir, icoRel != ""); len(missing) > 0 {
		mk(RuleIconIncomplete, "INFO", "Set Ikon Situs Tidak Lengkap",
			"Belum ada: "+strings.Join(missing, "; ")+".",
			"Sediakan /favicon.ico bermerek, SVG, PNG 32x32, apple-touch-icon 180x180, ikon manifest 192 dan 512, dan pastikan tiap <link rel=icon> menunjuk berkas yang ada. Lihat SEO-012.", firstNonEmpty(icoRel, staticDir, "package.json"))
	}
	return out
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func findFaviconICO(root string) (string, []byte) {
	for _, d := range append(append([]string{}, shareStaticDir...), "app", "src/app") {
		rel := filepath.ToSlash(filepath.Join(d, "favicon.ico"))
		if data, err := os.ReadFile(filepath.Join(root, rel)); err == nil {
			return rel, data
		}
	}
	return "", nil
}

func missingSiteIcons(root, staticDir string, hasICO bool) []string {
	var missing []string
	if !hasICO {
		missing = append(missing, "favicon.ico bermerek di folder publik")
	}
	has := func(patterns ...string) bool {
		for _, d := range append([]string{staticDir}, "app", "src/app") {
			if d == "" {
				continue
			}
			for _, pat := range patterns {
				if m, _ := filepath.Glob(filepath.Join(root, d, pat)); len(m) > 0 {
					return true
				}
			}
		}
		return false
	}
	if !has("*.svg", "icon.svg") {
		missing = append(missing, "ikon SVG")
	}
	if !has("*32*.png", "icon.png", "icon*.png") {
		missing = append(missing, "PNG 32x32")
	}
	if !has("apple-touch-icon*.png", "apple-icon*.png") {
		missing = append(missing, "apple-touch-icon 180x180")
	}
	if !has("*192*.png") || !has("*512*.png") {
		missing = append(missing, "ikon manifest 192 dan 512")
	}
	for _, rel := range shareIconSrcs {
		for _, m := range shareIconLinkRe.FindAllStringSubmatch(readSmallFile(filepath.Join(root, rel)), -1) {
			if !publicFileExists(root, m[1]) {
				missing = append(missing, fmt.Sprintf("berkas %s yang ditautkan di %s", m[1], rel))
			}
		}
	}
	return missing
}

func publicFileExists(root, ref string) bool {
	ref = strings.SplitN(ref, "?", 2)[0]
	for _, d := range shareStaticDir {
		if _, err := os.Stat(filepath.Join(root, d, filepath.FromSlash(ref))); err == nil {
			return true
		}
	}
	return false
}
