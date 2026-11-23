package diagnostics

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/vibeasik/vibeasik/pkg/models"
	"github.com/vibeasik/vibeasik/pkg/util"
)

// Kesiapan rilis: kewajiban Google Play untuk aplikasi Flutter (patokan PLAY-001…PLAY-014) dan kesiapan
// SEO / pratinjau bagikan / GEO untuk website publik (patokan SEO-001…SEO-013). Semua pemeriksaan di sini
// bersifat "tidak ditemukan tanda X di kode": hasilnya ajakan memeriksa, bukan bukti pelanggaran.

// playMinTargetSDK: batas bawah targetSdk yang diterima Google Play untuk aplikasi baru & update.
// Google menaikkannya tiap 31 Agustus; naikkan angka ini mengikuti pengumuman resminya.
const playMinTargetSDK = 35

const (
	readinessMaxFiles    = 3000
	readinessMaxFileSize = 512 * 1024
)

var (
	playSignupRegex   = regexp.MustCompile(`(?i)sign[\s_\-]?up|\bregist(?:er|rasi)|\bdaftar\b|createUser|create[\s_]?account|buat[\s_]akun|GoogleSignIn|signInWith(?:Credential|Google|Apple|Popup)`)
	playDeleteRegex   = regexp.MustCompile(`(?i)(?:delete|remove|hapus|close|tutup)[\s_\-]*(?:my[\s_\-]*|own[\s_\-]*)?(?:account|akun)|account[\s_/\-]*delet|penghapusan[\s_]akun`)
	privacyRegex      = regexp.MustCompile(`(?i)privacy|privasi`)
	playGatewayRegex  = regexp.MustCompile(`(?im)^\s{2,}(\w*(?:midtrans|xendit|stripe|razorpay|paypal|duitku|doku|tripay|ipaymu|flutterwave|paystack)\w*)\s*:`)
	playBillingRegex  = regexp.MustCompile(`(?im)^\s{2,}(?:in_app_purchase\w*|purchases_flutter|flutter_inapp_purchase|purchases_ui_flutter)\s*:`)
	playTargetSDKExpr = regexp.MustCompile(`\btargetSdk(?:Version)?\s*(?:=|\s)\s*(\d+)\b`)

	seoOGRegex      = regexp.MustCompile(`og:image|ogImage|openGraph|nuxt-og-image|@nuxtjs/seo`)
	seoRobotsRegex  = regexp.MustCompile(`@nuxtjs/robots|@nuxtjs/seo|nuxt-simple-robots|next-sitemap|astro-robots`)
	seoSitemapRegex = regexp.MustCompile(`@nuxtjs/sitemap|@nuxtjs/seo|nuxt-simple-sitemap|next-sitemap|@astrojs/sitemap|vite-plugin-sitemap`)
	seoLLMSRegex    = regexp.MustCompile(`nuxt-llms|llms\.txt`)
	seoJSONLDRegex  = regexp.MustCompile(`application/ld\+json|useSchemaOrg|schema-org|@nuxtjs/seo`)
	seoSPARegex     = regexp.MustCompile(`(?m)^\s*ssr\s*:\s*false\b`)
)

var (
	playSourceExts = map[string]bool{".dart": true, ".arb": true, ".json": true}
	seoSourceExts  = map[string]bool{".vue": true, ".ts": true, ".js": true, ".mjs": true, ".tsx": true, ".jsx": true,
		".html": true, ".astro": true, ".svelte": true, ".json": true}
)

// sourceSignals memindai berkas sumber di bawah roots (ekstensi exts) dan melaporkan, per regex, apakah ada yang
// cocok; names memuat nama berkas (huruf kecil) yang dilewati, untuk pemeriksaan keberadaan berkas.
func sourceSignals(roots []string, exts map[string]bool, res ...*regexp.Regexp) (hits []bool, names map[string]bool) {
	hits = make([]bool, len(res))
	names = map[string]bool{}
	scanned := 0
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if d.IsDir() {
				if path != root && (ignoredDirNames[name] || strings.HasPrefix(name, ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			names[strings.ToLower(name)] = true
			if scanned >= readinessMaxFiles || !exts[strings.ToLower(filepath.Ext(name))] {
				return nil
			}
			info, err := d.Info()
			if err != nil || info.Size() > readinessMaxFileSize {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			scanned++
			for i, re := range res {
				if !hits[i] && re.Match(data) {
					hits[i] = true
				}
			}
			return nil
		})
	}
	return hits, names
}

func hasNamePrefix(names map[string]bool, prefix string) bool {
	for n := range names {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

// inspectPlayReadiness memeriksa aplikasi Flutter ber-target Android (ada android/app di samping pubspec.yaml)
// terhadap kewajiban Google Play yang bisa dikenali dari kode.
func inspectPlayReadiness(pubspecPath string, relPubspec string) []models.Finding {
	pubspec, err := os.ReadFile(pubspecPath)
	if err != nil || !flutterPubspecRegex.Match(pubspec) {
		return nil
	}
	dir := filepath.Dir(pubspecPath)
	if !util.PathExists(filepath.Join(dir, "android", "app")) {
		return nil
	}
	relDir := filepath.Dir(relPubspec)
	hits, _ := sourceSignals([]string{filepath.Join(dir, "lib"), filepath.Join(dir, "assets")}, playSourceExts,
		playSignupRegex, playDeleteRegex, privacyRegex)
	hasSignup, hasDelete, hasPrivacy := hits[0], hits[1], hits[2]

	var findings []models.Finding
	if hasSignup && !hasDelete {
		findings = append(findings, models.Finding{
			Category:    "mobile",
			Severity:    "WARNING",
			RuleID:      "PLAY-NO-ACCOUNT-DELETION",
			Title:       "Aplikasi Ber-akun Tanpa Fitur Hapus Akun (Google Play)",
			Description: "Aplikasi ini punya alur daftar/login, tetapi di lib/ dan assets/ tidak ditemukan fitur hapus akun (delete account / hapus akun). Google Play mewajibkan aplikasi yang bisa membuat akun menyediakan hapus akun di dalam aplikasi DAN lewat halaman web, serta URL-nya diisi di formulir Keamanan Data.",
			Suggestion:  "Tambahkan menu Hapus akun di Profil/Pengaturan yang memanggil endpoint penghapusan akun beserta datanya, dan sediakan halaman web publik untuk meminta penghapusan. Lihat patokan PLAY-001.",
			TargetFile:  filepath.Join(relDir, "pubspec.yaml"),
			Line:        1,
			TheoryBasis: "Google Play Developer Policy — User Data: Account deletion requirement.",
		})
	}
	if !hasPrivacy {
		findings = append(findings, models.Finding{
			Category:    "mobile",
			Severity:    "WARNING",
			RuleID:      "PLAY-NO-PRIVACY-POLICY",
			Title:       "Tautan Kebijakan Privasi Tidak Ditemukan di Aplikasi (Google Play)",
			Description: "Di lib/ dan assets/ tidak ditemukan rujukan kebijakan privasi (privacy / privasi). Google Play mewajibkan kebijakan privasi pada URL publik yang dicantumkan di Play Console dan bisa dibuka dari dalam aplikasi.",
			Suggestion:  "Buat halaman kebijakan privasi di website proyek dan tautkan dari layar daftar/login serta menu pengaturan. Lihat patokan PLAY-006.",
			TargetFile:  filepath.Join(relDir, "pubspec.yaml"),
			Line:        1,
			TheoryBasis: "Google Play Developer Policy — User Data: Privacy policy requirement.",
		})
	}
	if m := playGatewayRegex.FindSubmatch(pubspec); m != nil && !playBillingRegex.Match(pubspec) {
		findings = append(findings, models.Finding{
			Category:    "mobile",
			Severity:    "INFO",
			RuleID:      "PLAY-EXTERNAL-PAYMENT",
			Title:       "Pembayaran Lewat Gateway Tanpa Google Play Billing",
			Description: fmt.Sprintf("pubspec.yaml memakai SDK pembayaran '%s' tanpa in_app_purchase. Ini sah hanya bila semua yang dijual adalah barang/layanan fisik atau pengecualian lain; barang dan layanan digital (langganan, fitur premium, koin, konten) wajib lewat Google Play Billing atau program resmi Google yang mengizinkan cara bayar lain.", string(m[1])),
			Suggestion:  "Golongkan tiap produk digital/fisik. Item digital dipindahkan ke Play Billing (in_app_purchase) dengan verifikasi purchase token di server; gateway hanya untuk item fisik. Lihat patokan PLAY-003 dan PLAY-004.",
			TargetFile:  filepath.Join(relDir, "pubspec.yaml"),
			Line:        lineOfMatch(pubspec, playGatewayRegex),
			TheoryBasis: "Google Play Developer Policy — Payments.",
		})
	}
	return append(findings, playTargetSDKFinding(dir, relDir)...)
}

// lineOfMatch: nomor baris (mulai 1) kecocokan pertama re di data; 1 bila tidak ada.
func lineOfMatch(data []byte, re *regexp.Regexp) int {
	loc := re.FindSubmatchIndex(data)
	if loc == nil {
		return 1
	}
	at := loc[0]
	if len(loc) >= 4 && loc[2] >= 0 {
		at = loc[2]
	}
	return strings.Count(string(data[:at]), "\n") + 1
}

// playTargetSDKFinding: targetSdk yang ditulis sebagai angka di bawah batas Google Play. Nilai yang mengikuti
// Flutter (flutter.targetSdkVersion) dilewati karena naik bersama SDK Flutter.
func playTargetSDKFinding(dir, relDir string) []models.Finding {
	for _, name := range []string{"build.gradle.kts", "build.gradle"} {
		rel := filepath.Join("android", "app", name)
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			continue
		}
		m := playTargetSDKExpr.FindSubmatch(data)
		if m == nil {
			return nil
		}
		sdk, _ := strconv.Atoi(string(m[1]))
		if sdk >= playMinTargetSDK {
			return nil
		}
		line := lineOfMatch(data, playTargetSDKExpr)
		return []models.Finding{{
			Category:     "mobile",
			Severity:     "WARNING",
			RuleID:       "PLAY-TARGET-SDK-OLD",
			Title:        fmt.Sprintf("targetSdk %d di Bawah Syarat Google Play", sdk),
			Description:  fmt.Sprintf("%s menetapkan targetSdk %d, di bawah API %d. Google Play menolak unggahan aplikasi baru dan update yang menargetkan API lebih lama dari yang disyaratkan (batasnya naik tiap 31 Agustus).", rel, sdk, playMinTargetSDK),
			Suggestion:   "Naikkan targetSdk ke API yang disyaratkan atau ikuti flutter.targetSdkVersion, lalu uji ulang izin dan perilaku yang berubah di versi Android itu. Lihat patokan PLAY-009.",
			TargetFile:   filepath.Join(relDir, rel),
			Line:         line,
			Snippet:      strings.TrimSpace(strings.Split(string(data), "\n")[line-1]),
			TheoryBasis:  "Google Play — Target API level requirements.",
			SuggestedFix: "targetSdk = flutter.targetSdkVersion",
		}}
	}
	return nil
}

// inspectSEOReadiness memeriksa website publik (peran frontend_client, proyek ber-package.json) terhadap
// kesiapan SEO, pratinjau bagikan, dan GEO. Panel admin dan aplikasi mobile tidak diperiksa.
func inspectSEOReadiness(comp *models.Component) []models.Finding {
	if comp.Role != models.RoleFrontendClient || isMobileComponent(*comp) ||
		!util.FileExists(filepath.Join(comp.AbsolutePath, "package.json")) {
		return nil
	}
	hits, names := sourceSignals([]string{comp.AbsolutePath}, seoSourceExts,
		seoOGRegex, seoRobotsRegex, seoSitemapRegex, seoLLMSRegex, seoJSONLDRegex)
	target := "package.json"
	spa := false
	for _, cfg := range []string{"nuxt.config.ts", "nuxt.config.js"} {
		if data := readSmallFile(filepath.Join(comp.AbsolutePath, cfg)); data != "" {
			target = cfg
			spa = seoSPARegex.MatchString(data)
			break
		}
	}
	target = filepath.Join(comp.RelativePath, target)

	type check struct {
		missing                                  bool
		id, severity, title, description, advice string
	}
	checks := []check{
		{spa, "SEO-SPA-ONLY", "WARNING", "Website Publik Dirender di Klien Saja (ssr: false)",
			"Konfigurasi mematikan SSR, sehingga HTML yang diterima perayap kosong sampai JavaScript berjalan. Perayap media sosial dan sebagian besar perayap AI tidak menjalankan JavaScript: pratinjau tautan gagal dan isi halaman tidak bisa dikutip.",
			"Aktifkan SSR atau prerender untuk rute publik (routeRules { prerender: true }); biarkan hanya area di balik login yang dirender di klien. Lihat patokan SEO-002."},
		{!hits[0], "SEO-NO-OG-IMAGE", "WARNING", "Tanpa Open Graph: Tautan Tampil Polos Saat Dibagikan",
			"Tidak ditemukan og:image/ogImage di sumber website ini. Tautan yang dibagikan ke WhatsApp, Facebook, LinkedIn, Telegram, atau X akan tampil tanpa gambar dan ringkasan.",
			"Sediakan gambar bagikan 1200x630 (di bawah ±300 KB, URL absolut https) dan set og:title, og:description, og:image, og:url, serta twitter:card summary_large_image di app.vue; halaman konten menimpanya dengan datanya. Lihat patokan SEO-003 dan SEO-004."},
		{!hits[1] && !hasNamePrefix(names, "robots."), "SEO-NO-ROBOTS", "INFO", "robots.txt Tidak Ditemukan",
			"Tidak ada robots.txt maupun modul pembuatnya. Tanpa itu tidak ada tempat menyatakan halaman yang ditutup (admin, api), lokasi sitemap, dan izin untuk perayap pencarian serta AI.",
			"Tambahkan public/robots.txt (atau modul robots) dengan baris Sitemap: dan aturan Allow untuk perayap pencarian dan AI. Lihat patokan SEO-006 dan SEO-009."},
		{!hits[2] && !hasNamePrefix(names, "sitemap"), "SEO-NO-SITEMAP", "INFO", "sitemap.xml Tidak Ditemukan",
			"Tidak ada sitemap maupun modul/rute pembuatnya. Halaman dinamis yang tidak tertaut dari beranda lambat atau tidak pernah ditemukan mesin pencari.",
			"Pasang modul sitemap atau rute server /sitemap.xml yang dibangun dari data, lalu kirim ke Google Search Console dan Bing Webmaster Tools setelah deploy. Lihat patokan SEO-006 dan SEO-013."},
		{!hits[4], "SEO-NO-STRUCTURED-DATA", "INFO", "Tanpa Data Terstruktur JSON-LD",
			"Tidak ditemukan application/ld+json atau schema.org di sumber website ini. Mesin pencari dan asisten AI kehilangan fakta pasti tentang organisasi, produk, artikel, dan tanya-jawab di situs.",
			"Tambahkan JSON-LD mulai dari Organization + WebSite di beranda, lalu Product/Article/FAQPage sesuai jenis halaman. Lihat patokan SEO-007."},
		{!hits[3] && !hasNamePrefix(names, "llms"), "SEO-NO-LLMS-TXT", "INFO", "llms.txt Tidak Ditemukan (GEO)",
			"Tidak ada /llms.txt. Berkas ringkasan ini memberi asisten AI peta situs yang ringkas: siapa pemiliknya, apa yang ditawarkan, dan tautan halaman terpenting.",
			"Buat public/llms.txt berformat Markdown: judul, satu paragraf ringkasan, dan daftar tautan halaman utama dengan keterangan satu baris. Lihat patokan SEO-010."},
	}
	pageFindings, scanned := inspectSharePages(comp)
	var findings []models.Finding
	for _, c := range checks {
		// Temuan per halaman menggantikan temuan per situs untuk masalah yang sama (tanpa temuan dobel).
		if !c.missing || (c.id == "SEO-NO-OG-IMAGE" && scanned > 0) {
			continue
		}
		findings = append(findings, models.Finding{
			Category:    "frontend",
			Severity:    c.severity,
			RuleID:      c.id,
			Title:       c.title,
			Description: c.description,
			Suggestion:  c.advice,
			TargetFile:  target,
			Line:        1,
			TheoryBasis: "Open Graph protocol, Google Search Central (SEO Starter Guide, structured data) & konvensi llms.txt.",
		})
	}
	findings = append(findings, pageFindings...)
	return append(findings, inspectSiteIcons(comp)...)
}
