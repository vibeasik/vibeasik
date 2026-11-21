# Standar kunci agent AI (AI-003)

Setiap aplikasi yang punya backend atau panel admin **wajib** menyediakan *kunci agent AI*: jalur
mesin-ke-mesin tempat AI coding agent milik pemilik (Claude Code, Antigravity, Cline, Cursor, …) membaca
data operasional aplikasi — laporan galat, masukan pengguna, rekap agregat — lalu menandainya setelah
diperbaiki. Jalur ini **bisa dinyalakan atau dimatikan** pemilik kapan saja dan **wajib terdokumentasi**.

Patokan resminya ada di `guidelines/07_ai_dan_mcp_integration.json` (butir `AI-003`). vibeasik
menyarankan todo AI-003 untuk setiap proyek yang punya komponen backend tetapi belum punya
`docs/KUNCI_AGENT.md` (atau `docs/AGENT_KEY.md`) di root proyek.

## Kenapa bukan sandi panel atau token?

| Pilihan | Masalah |
| --- | --- |
| Sandi panel di konfigurasi agent | Akses penuh (hapus, data pribadi, kelola sandi); bocor bersama berkas konfigurasi |
| Token API panjang | Rahasia tersimpan di disk & di server; sulit diaudit per agent |
| **Kunci Ed25519 + sakelar** | Kunci privat tidak pernah meninggalkan laptop/ssh-agent; server hanya menyimpan kunci publik; tiap kunci punya nama & izin; dicabut atau dimatikan seketika |

## Syarat wajib

1. **Sakelar utama** di panel admin: *Jalur agent AI — Aktif/Nonaktif*.
   - Bawaan **nonaktif**; pemilik menyalakannya sendiri.
   - Nonaktif → semua rute agent dijawab `403` dengan kode `agent_disabled`, sebelum tanda tangan diperiksa.
   - Pengaturan tidak terbaca (galat DB) = nonaktif (*fail closed*).
2. **Kelola kunci** di panel (butuh sesi admin): tambah kunci publik Ed25519 (`ssh-ed25519 …`) dengan nama &
   izin, cabut kapan saja. Kunci dibaca ulang tiap request agar pencabutan berlaku seketika. Tolak RSA/ECDSA
   dan teks kunci privat.
3. **Izin per kunci**: `baca` (hanya melihat) dan `tandai` (baca + ubah status/catatan). Kunci `baca` yang
   mencoba mengubah mendapat `403` dengan kode `read_only`.
4. **Tanda tangan per request**. Amplop kanonis (dipisah `\n`), satu paket kode dipakai server dan klien:

   ```text
   <namespace-aplikasi>-agent-v1
   <METHOD>
   <path relatif akar panel, mis. /agent/errors>
   <query mentah>
   <unix timestamp detik>
   <nonce hex acak>
   <sha256(body) hex>
   ```

   Header: sidik jari kunci (`SHA256:…`), timestamp, nonce, tanda tangan (base64). Server menolak
   timestamp di luar ±2 menit, nonce yang dipakai ulang (dicatat **setelah** tanda tangan sah), dan request
   yang dibuat sebelum proses server mulai bila cache nonce hanya di memori.
5. **Cakupan sempit**: tidak ada rute hapus, kelola kunci/sandi, pembayaran, atau daftar akun. Respons tanpa
   id/nama/email pengguna; teks bebas (pesan galat, stack, konteks, masukan) diredaksi: email, token, header
   `Authorization`/`Cookie`, blob base64 panjang, kunci konteks rahasia.
6. **Pintu terpisah**: cookie panel tidak berlaku di jalur agent dan tanda tangan agent tidak berlaku di panel.
7. **Rate limit & jejak**: batasi request agent per IP (tanda tangan gagal ikut dihitung); log tiap request
   agent (diterima/ditolak, nama kunci, izin), pendaftaran/pencabutan kunci, dan perubahan sakelar.
8. **Server MCP stdio**: CLI aplikasi punya subperintah `mcp` yang berbicara JSON-RPC 2.0 (satu pesan per
   baris; stdout hanya protokol, log ke stderr) dan meneruskan tiap tool ke jalur agent dengan request
   bertanda tangan. Daftarkan di `.mcp.json` root repo (Claude Code) dan beri contoh konfigurasi agent lain.
9. **Katalog tool satu sumber**: nama, deskripsi, izin, dan rute tiap tool ditulis sekali dan dipakai oleh
   server (`GET /agent/me`), panel (tabel tool), dan klien MCP (`tools/list`).
10. **Dokumentasi**:
    - `docs/KUNCI_AGENT.md` di root proyek: tabel tool (nama, perintah CLI, rute, izin, isi), sakelar & izin,
      model keamanan, tutorial pemasangan, tabel pesan galat & perbaikannya, alur kerja agent, berkas kode.
    - Halaman **Kunci agent** di panel admin: kartu sakelar, tabel kunci, tabel kemampuan MCP, dan tutorial
      langkah demi langkah dengan blok kode bertombol salin.

## Tool minimal

| Tool | Izin | Isi |
| --- | --- | --- |
| `status` | baca | Identitas kunci (sidik jari, nama, izin), lingkungan, katalog tool |
| `ringkasan` | baca | Rekap agregat tanpa data pribadi |
| `galat_daftar` / `galat_detail` | baca | Laporan galat teredaksi (daftar ringkas tanpa stack; detail lengkap) |
| `galat_tandai` | tandai | Status `baru`/`ditangani`/`selesai`/`abaikan` + catatan perbaikan |

Tambahkan tool lain sesuai aplikasi (mis. masukan pengguna) dengan aturan cakupan yang sama.

## Tutorial pemasangan (isi minimal di dokumen & panel)

1. Deploy backend yang berisi grup rute `/agent`.
2. Buat kunci di laptop: `ssh-keygen -t ed25519 -a 100 -f ~/.ssh/<aplikasi>_agent` (izin 600).
3. Tempel kunci publik di panel → **Kunci agent** → **Tambah kunci**, pilih izin.
4. Panel → **Aktifkan** jalur agent.
5. Cek: `<cli> ping` → `OK — kunci SHA256:… diterima`.
6. Daftarkan MCP: Claude Code membaca `.mcp.json`; agent lain memakai path absolut:

   ```json
   { "mcpServers": { "<aplikasi>": { "command": "/path/ke/repo/<cli>", "args": ["mcp"] } } }
   ```

7. Minta agent: *"Cek galat baru, perbaiki yang paling sering, lalu tandai selesai."*

Untuk keamanan tambahan, beri passphrase dan muat kunci ke ssh-agent; klien wajib memakai ssh-agent
bila kuncinya tersedia di sana.

## Susunan rute yang disarankan

```text
/<panel>/api/agent-keys          GET, POST        sesi admin — daftar & tambah kunci (+ enabled, tools)
/<panel>/api/agent-keys/revoke   POST             sesi admin — cabut
/<panel>/api/agent-settings      POST {enabled}   sesi admin — sakelar utama
/<panel>/agent/*                 rate limit → sakelar → verifikasi tanda tangan → handler
  GET   /agent/me
  GET   /agent/overview
  GET   /agent/errors            PATCH /agent/errors/:id     (izin tandai)
```

## Tes wajib

- Tanpa tanda tangan, kunci tak terdaftar, body/path/method diubah, timestamp kedaluwarsa/masa depan, nonce
  rusak, tanda tangan kunci lain → `401`, data tidak berubah.
- Request diputar ulang → `401`. Kunci dicabut → `401` seketika.
- Sakelar bawaan/mati → `403 agent_disabled`; galat DB saat membaca sakelar → bukan `200`.
- Kunci `baca` → `403 read_only` pada semua rute ubah. Tidak ada rute hapus.
- Respons agent tidak memuat email, nama, id pengguna, token, atau ciphertext.
- Ujung ke ujung: `<cli> mcp` (stdin/stdout) → HTTP bertanda tangan → backend sungguhan (initialize,
  tools/list, tiap tool, notifikasi tanpa jawaban, metode/tool tak dikenal).
- Mutan "verifikasi dimatikan" dan "sakelar dilewati" harus membuat tes gagal.
