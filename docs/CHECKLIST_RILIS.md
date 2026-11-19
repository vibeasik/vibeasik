# Checklist Rilis: Google Play, SEO, Pratinjau Bagikan, dan GEO

vibeasik membawa dua checklist yang dipakai sejak aplikasi dirancang, bukan baru saat akan rilis:

| Checklist | Patokan | Berkas | Berlaku untuk |
| :--- | :--- | :--- | :--- |
| Kewajiban Google Play | `PLAY-001` … `PLAY-014` | `guidelines/13_mobile_kepatuhan_toko_aplikasi.json` | Aplikasi Android/Flutter |
| SEO, pratinjau media sosial, GEO | `SEO-001` … `SEO-013` | `guidelines/14_frontend_seo_sosial_dan_geo.json` | Website publik |

Butirnya otomatis masuk Pusat Pengetahuan (`guideline:<ID>`), sehingga muncul saat agent memanggil
`vibeasik_knowledge_recall` untuk tugas mobile atau website. Kebijakan Google berubah berkala: sebelum rilis,
cocokkan dengan Pusat Kebijakan Developer Google Play.

## Google Play

| Kewajiban | Patokan | Dicek audit |
| :--- | :--- | :--- |
| Hapus akun di aplikasi **dan** lewat halaman web | PLAY-001 | `PLAY-NO-ACCOUNT-DELETION` |
| Akun demo untuk peninjau (tanpa OTP/2FA, selalu aktif) | PLAY-002 | — |
| Barang/layanan digital lewat Google Play Billing | PLAY-003 | `PLAY-EXTERNAL-PAYMENT` |
| Syarat memakai pembayaran lain (barang fisik, P2P, program resmi) | PLAY-004 | `PLAY-EXTERNAL-PAYMENT` |
| Langganan: syarat jelas, mudah dibatalkan | PLAY-005 | — |
| Kebijakan privasi di web dan di dalam aplikasi | PLAY-006 | `PLAY-NO-PRIVACY-POLICY` |
| Formulir Keamanan Data sesuai perilaku aplikasi dan SDK | PLAY-007 | — |
| Izin minimal; izin sensitif berdeklarasi | PLAY-008 | — |
| Target API terbaru, AAB, Play App Signing | PLAY-009 | `PLAY-TARGET-SDK-OLD` |
| Uji tertutup 12 penguji × 14 hari (akun pribadi baru) | PLAY-010 | — |
| Deklarasi konten: rating, target usia, iklan | PLAY-011 | — |
| Konten buatan pengguna: lapor, blokir, moderasi | PLAY-012 | — |
| Listing toko jujur, aset sesuai ukuran | PLAY-013 | — |
| Fungsi minimum: stabil, lengkap, bukan pembungkus web | PLAY-014 | — |

## SEO, pratinjau bagikan, dan GEO

| Kewajiban | Patokan | Dicek audit |
| :--- | :--- | :--- |
| Judul dan deskripsi unik per halaman | SEO-001 | — |
| Isi publik ter-render di HTML server | SEO-002 | `SEO-SPA-ONLY` |
| Open Graph lengkap, gambar 1200×630 | SEO-003 | `SEO-NO-OG-IMAGE` |
| Gambar bagikan khusus halaman konten | SEO-004 | — |
| Canonical, bahasa, URL bersih | SEO-005 | — |
| `robots.txt` dan `sitemap.xml` | SEO-006 | `SEO-NO-ROBOTS`, `SEO-NO-SITEMAP` |
| Data terstruktur JSON-LD | SEO-007 | `SEO-NO-STRUCTURED-DATA` |
| Satu H1, heading berurutan, alt gambar, 404 sungguhan | SEO-008 | — |
| Perayap AI diizinkan di robots.txt dan WAF/hosting | SEO-009 | — |
| `llms.txt` | SEO-010 | `SEO-NO-LLMS-TXT` |
| Konten mudah dikutip: jawaban dulu, fakta berupa teks, FAQ | SEO-011 | — |
| Favicon, manifest, warna tema | SEO-012 | — |
| Verifikasi pasca-deploy (Search Console, Bing, uji bagikan, akses AI) | SEO-013 | — |

## Cara kerja audit

Audit statis (`pkg/diagnostics/readiness_inspector.go`) menandai tanda yang **tidak ditemukan** di kode, jadi
temuannya adalah ajakan memeriksa, bukan bukti pelanggaran:

- Aturan `PLAY-*` berjalan pada aplikasi Flutter yang punya `android/app`. Hapus akun hanya dituntut bila
  aplikasi punya alur daftar/login.
- Aturan `SEO-*` berjalan pada komponen berperan `frontend_client` (website publik). Panel admin tidak diperiksa.

Butir tanpa pemeriksaan otomatis (akun demo, formulir Play Console, verifikasi pasca-deploy) dikerjakan
manusia atau agent memakai patokannya.
