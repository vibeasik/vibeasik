# Traffic Analyzer: kunjungan halaman (pageview)

Selain request backend (rute, latensi, status), Traffic Analyzer menerima event `pageview` untuk menjawab
"halaman mana yang paling banyak dibuka". Cocok untuk situs yang berpindah halaman di peramban (SPA/statis),
sehingga server tidak melihat tiap perpindahan.

## Alur

```
peramban ──sendBeacon──▶ POST /_vibeasik/pv   (satu asal dengan situs, tanpa kunci di peramban)
server aplikasi (PageviewHandler) ──batch, X-Vibeasik-Key──▶ vibeasik live ──▶ engine ──▶ Studio /traffic
```

## Memasang (kit `pageview` di Studio)

```go
mux.Handle(vibeasiktraffic.PageviewPath, vibeasiktraffic.PageviewHandler())
```

```html
<script src="/vibeasik-pageview.js" data-endpoint="/_vibeasik/pv" defer></script>
```

Env server sama dengan SDK lain (`VIBEASIK_TRAFFIC_URL`, `VIBEASIK_TRAFFIC_KEY`, opsional
`VIBEASIK_TRAFFIC_HASH_IP=1`). SDK nonaktif: handler membalas 204 tanpa mencatat.

## Kontrak event

Bentuk `Event` yang sudah ada, tanpa field baru:

| field | isi |
|---|---|
| `kind` | `"pageview"` |
| `path` | pathname halaman, tanpa query/hash, maks 255 |
| `route` | host halaman (huruf kecil, tanpa port); kosong = situs satu host |
| `referer` | host perujuk luar saja; perujuk dari host yang sama dikosongkan |
| `ua`, `ip`, `country`, `ts` | seperti request biasa |
| `method`/`status` | `GET` / `200` (diabaikan engine) |

`POST /_vibeasik/pv`: badan maks 2 KB, JSON `{"p":"/path","r":"<document.referrer>"}` (Content-Type apa pun,
`sendBeacon` mengirim `text/plain`). Host diambil dari header `Host`, bukan dari badan. Ditolak 403 bila
`Origin` berbeda host atau `Sec-Fetch-Site: cross-site`; 400 bila badan atau path tidak sah; batas 60 kiriman
per menit per IP; sukses = 204.

## Pengolahan di engine

Pageview tidak masuk statistik rute/latensi dan tidak masuk dimensi `ip`, `gpath`, maupun ekor event live.
Event menambah `total_events` dan `last_event_at` sumber. Rangkuman disimpan di `traffic_dim_stats`:

| dim | gran | nilai |
|---|---|---|
| `page` | menit & jam | `host+path` bila host ada, selain itu `path`; bot tidak dihitung |
| `pref` | jam | host perujuk, atau `(langsung)` (termasuk perujuk dari host yang sama) |
| `pdev` | jam | kelas UA (`mobile`/`desktop`/`api`/`bot`/`unknown`); bot ikut di sini saja |
| `pvis` | jam | 12 hex pertama `sha256(ip|ua|tanggal UTC|salt sumber)`; tanpa IP mentah; bot tidak |

Retensi mengikuti `Prune` yang sudah ada (menit 48 jam, jam 90 hari).

## Laporan

`Report` bertambah:

- `totals.page_views`: kunjungan manusia (tanpa bot) dalam jendela.
- `totals.visitors`: pengunjung unik perkiraan; id berganti tiap hari UTC, jadi jendela multi-hari
  menghitung orang yang sama di hari berbeda sebagai pengunjung berbeda.
- `pages`: 50 teratas, `{page, host, path, views, share}`.
- `page_referers`, `page_devices`: daftar `{value, count, err, share}` (share terhadap seluruh nilai dimensi).
- `page_series`: `[{t, views}]` mengikuti granularitas dan langkah laporan; kosong bila tidak ada kunjungan.

Jendela menit (`15m`, `1h`, `6h`, `24h`) menghitung total, halaman, dan seri dari rangkuman per menit;
perujuk, perangkat, dan pengunjung unik memakai rangkuman per jam (selaras ke batas jam).

## Privasi

Tanpa cookie dan tanpa storage di peramban. Skrip berhenti bila `doNotTrack`, `globalPrivacyControl`, atau
`navigator.webdriver` aktif. Hanya path (tanpa query/hash) dan host perujuk yang dikirim.
