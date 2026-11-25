# Teks Website

Semua teks yang tampil ke pengunjung website dikirim **oleh website ke vibeasik** (bukan sebaliknya), lalu bisa
ditelusuri per halaman dan section di Studio (`/texts`). Satu teks bisa dijadikan todo "ubah teks" — misalnya
agar tulisan bawaan AI terasa lebih manusiawi.

## Alur

```
website + vibeasik-texts.js ──POST /api/texts/ingest (kunci sumber)──▶ engine ──▶ Studio /texts
Studio: [Jadikan todo ubah teks] ─▶ popup (teks asli + halaman + section, tulis perubahan) ─▶ todo
```

Skrip membaca teks yang tampil (judul, paragraf, tombol, tautan, label), `placeholder`, `alt`, `aria-label`,
judul halaman, dan meta description. Halaman yang berubah (SPA, konten dimuat belakangan) dipantau lewat
`MutationObserver`, jadi daftar di Studio mengikuti isi website.

## Memasang

1. Studio → **Teks Website** → **Hubungkan website** (pilih proyek & komponen).
2. Tab **Pemasangan**: **Tulis ke komponen** menyalin `public/vibeasik-texts.js`, lalu muat di halaman:

```html
<script src="/vibeasik-texts.js" data-url="http://localhost:8765/api/texts/ingest" data-key="KUNCI" defer></script>
```

Pasang **hanya di dev/staging**. Studio menampilkan potongan siap salin untuk HTML/Vite, Nuxt, dan Next.js.

## Privasi

- Dikirim: teks, path halaman (tanpa query/hash), nama section, selector pendek, asal (`skema://host`).
- Tidak dikirim: nilai `input`/`textarea`/`select`, isi `contenteditable`, cookie, header, storage.
- Dilewati: `script`, `style`, `code`/`pre`, elemen tersembunyi, dan elemen ber-atribut `data-vibeasik-ignore`
  (pakai untuk data pengguna/pelanggan).
- Kunci terlihat di browser, tetapi hanya bisa **mengirim** teks; tidak bisa membaca apa pun dari vibeasik.
  Ganti kunci kapan saja dari tab Pemasangan.

## Nama section

Tanpa konfigurasi: `Header`, `Footer`, `Navigasi`, `Konten utama`, `Formulir`, `Dialog`, atau judul/`id` dari
`<section>`. Beri nama sendiri dengan `data-vibeasik-section="Hero"` pada elemen pembungkus (mengalahkan nama otomatis).

## Todo ubah teks

`POST /api/texts/entries/:id/todo` `{ "instruction": "...", "priority": "low|medium|high" }` membuat todo
kategori `ui_ux` berisi teks asli, halaman, section, selector, dan permintaan pemilik. Satu teks hanya punya satu
todo terbuka; permintaan kedua mengembalikan todo yang sama (`already_exists: true`).

## API

| Rute | Fungsi |
|---|---|
| `POST /api/texts/ingest` | Dipanggil skrip website; kunci di header `X-Vibeasik-Key` atau field `key` badan (tanpa token lokal) |
| `GET/POST /api/texts/sources`, `PUT/DELETE /api/texts/sources/:id` | Kelola sumber |
| `POST /api/texts/sources/:id/rotate-key` | Ganti kunci |
| `GET /api/texts/sources/:id/sdk`, `POST …/sdk/install` | Panduan & penulisan skrip ke komponen |
| `GET /api/texts/sources/:id/facets`, `GET …/entries` | Halaman/section dan daftar teks (`page`, `section`, `q`, `todo`, `limit`, `offset`) |
| `POST /api/texts/entries/:id/todo` | Jadikan todo ubah teks |

Teks yang sama pada satu halaman menjadi satu entri. Teks yang diedit tampil sebagai entri baru; entri lama berhenti
diperbarui (`last_seen_at` tidak maju). Batas: 1000 teks per kiriman, 20.000 entri per sumber, 2000 karakter per teks.
