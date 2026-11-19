/**
 * Mengubah Dokumen Proyek (data yang sama dengan tampilan layar) menjadi blok sederhana untuk renderer PDF di engine
 * (POST /api/projects/:id/doc-pdf). Engine yang menata halaman, sampul, daftar isi, header/footer, dan warna merek.
 */
import type { DocData } from '~/utils/projectDocLabels'
import {
  DOC_ADR_STATUS, DOC_FEATURE_STATUS, DOC_HORIZON, DOC_MATURITY, DOC_SEVERITY, DOC_STATUS,
  docDate, docNum, docRole, isGeneratedPath
} from '~/utils/projectDocLabels'

export type PdfCell = string | { t: string, sub?: string, tone?: string, mono?: boolean, bold?: boolean, align?: 'right' }
export interface PdfPair { k: string, v: string, tone?: string }
export interface PdfBlock {
  type: 'p' | 'lead' | 'muted' | 'h' | 'list' | 'olist' | 'table' | 'kv' | 'code' | 'stats' | 'note'
  text?: string
  items?: string[]
  head?: string[]
  rows?: PdfCell[][]
  widths?: number[]
  pairs?: PdfPair[]
  title?: string
  lang?: string
  tone?: string
}
export interface PdfSection { no: string, title: string, level?: number, blocks: PdfBlock[] }
export interface PdfDocument {
  title: string
  subtitle?: string
  kicker?: string
  version?: string
  authors?: string
  date?: string
  meta?: PdfPair[]
  sections: PdfSection[]
}

const ROUTE_LIMIT = 200
const p = (text?: string | null): PdfBlock[] => (text?.trim() ? [{ type: 'p', text }] : [])
const muted = (text: string): PdfBlock => ({ type: 'muted', text })
const h = (text: string): PdfBlock => ({ type: 'h', text })
const tone = (s?: string) => ({ success: 'success', warning: 'warning', error: 'error' } as Record<string, string>)[s || ''] || ''
const methodTone = (m: string) => ({ GET: 'get', POST: 'post', PUT: 'put', PATCH: 'patch', DELETE: 'delete' } as Record<string, string>)[m.toUpperCase()] || 'muted'
const statusTone = (code?: number) => !code ? 'muted' : code < 300 ? 'success' : code < 500 ? 'warning' : 'error'
const scoreTone = (n?: number | null) => n == null ? '' : n >= 80 ? 'success' : n >= 60 ? 'warning' : 'error'
const round = (n?: number | null) => (n == null ? '—' : String(Math.round(n)))

const testCommand = (lang: string, fw: string, pm: string) => {
  const l = `${lang} ${fw}`.toLowerCase()
  if (l.includes('go')) return 'go test ./...'
  if (l.includes('flutter') || l.includes('dart')) return 'flutter test'
  if (l.includes('python')) return 'pytest'
  if (l.includes('php') || l.includes('laravel')) return 'php artisan test'
  if (l.includes('rust')) return 'cargo test'
  if (/(ts|js|typescript|javascript|nuxt|vue|react|next|node)/.test(l)) return `${pm || 'npm'} ${pm === 'bun' ? 'test' : 'run test'}`
  return ''
}

const SEC_STATUS: Record<string, { label: string, tone: string }> = {
  pass: { label: 'Lulus', tone: 'success' },
  declared: { label: 'Dideklarasikan', tone: 'info' },
  warn: { label: 'Lemah', tone: 'warning' },
  fail: { label: 'Gagal', tone: 'error' },
  skipped: { label: 'Dilewati', tone: 'muted' }
}
const RISK_STATUS: Record<string, string> = {
  open: 'Terbuka', planned: 'Direncanakan', in_progress: 'Dikerjakan', handled: 'Ditangani', accepted: 'Diterima'
}

export function buildPdfDocument(doc: DocData): PdfDocument {
  const pr = doc.project!
  const prof = doc.profile
  const notes = doc.notes
  const env = doc.knowledge?.environment_scope
  const cm = doc.overview
  const rt = doc.runtime
  const sec = doc.securityReport
  const aud = doc.audit
  const S: PdfSection[] = []

  // 1. Keterangan
  S.push({
    no: '1',
    title: 'Keterangan proyek',
    blocks: [
      ...(prof?.short_description || pr.short_description ? [{ type: 'lead', text: prof?.short_description || pr.short_description } as PdfBlock] : []),
      ...p(prof?.full_description || pr.description),
      {
        type: 'kv',
        pairs: [
          { k: 'Nama proyek', v: pr.name },
          { k: 'Versi dokumen', v: notes.version || '—' },
          { k: 'Penyusun', v: notes.authors || '—' },
          { k: 'Jenis', v: pr.project_type || '—' },
          { k: 'Status', v: DOC_STATUS[pr.status] || pr.status },
          { k: 'Kematangan', v: prof?.maturity?.stage ? DOC_MATURITY[prof.maturity.stage] || prof.maturity.stage : '—' },
          { k: 'Repositori', v: pr.git_remote || '—' },
          { k: 'Branch', v: pr.git_branch || '—' },
          { k: 'Tag', v: pr.tags || '—' },
          { k: 'Dibuat', v: docDate(pr.created_at) },
          { k: 'Diperbarui', v: docDate(pr.updated_at) }
        ]
      }
    ]
  })

  // 2. Latar belakang
  const bg: PdfBlock[] = [...p(notes.background)]
  if (prof?.purpose?.problem) {
    bg.push(...p(prof.purpose.problem))
    if (prof.purpose.solution) bg.push({ type: 'note', title: 'Solusi yang ditawarkan', text: prof.purpose.solution })
    if (prof.maturity?.reason) bg.push(muted(`Kematangan saat ini: **${DOC_MATURITY[prof.maturity.stage] || prof.maturity.stage}** — ${prof.maturity.reason}`))
  }
  S.push({ no: '2', title: 'Latar belakang & masalah', blocks: bg.length ? bg : [muted('Belum ada uraian masalah.')] })

  // 3. Tujuan
  const goal: PdfBlock[] = []
  if (prof?.purpose?.value_proposition) goal.push({ type: 'p', text: `**Nilai utama:** ${prof.purpose.value_proposition}` })
  if (prof?.purpose?.target_users?.length) goal.push(h('Pengguna sasaran'), { type: 'list', items: prof.purpose.target_users })
  S.push({ no: '3', title: 'Tujuan & pengguna sasaran', blocks: goal.length ? goal : [muted('Tujuan dan pengguna sasaran belum tercatat di profil proyek.')] })

  // 4. Ruang lingkup
  const scope: PdfBlock[] = []
  const feats = prof?.features || []
  if (feats.length) {
    const n = (s: string) => feats.filter(f => f.status === s).length
    scope.push({ type: 'p', text: `Ruang lingkup memuat **${feats.length}** fitur: ${n('done')} selesai, ${n('partial')} sebagian, ${n('planned')} direncanakan. Fitur berstatus "direncanakan" belum termasuk rilis saat ini.` })
    const groups = new Map<string, typeof feats>()
    for (const f of feats) {
      const k = f.domain || 'Umum'
      if (!groups.has(k)) groups.set(k, [])
      groups.get(k)!.push(f)
    }
    for (const [domain, list] of groups) {
      scope.push(h(domain), {
        type: 'table',
        head: ['Fitur', 'Keterangan', 'Status'],
        widths: [0.26, 0.56, 0.18],
        rows: list.map(f => [
          { t: f.name, bold: true },
          { t: f.description || '—', sub: [...(f.pages || []), ...(f.endpoints || [])].slice(0, 6).join(' · ') },
          { t: DOC_FEATURE_STATUS[f.status]?.label || f.status, tone: tone(DOC_FEATURE_STATUS[f.status]?.color) || 'muted' }
        ])
      })
    }
  }
  if (notes.out_of_scope.length) scope.push(h('Di luar lingkup'), { type: 'list', items: notes.out_of_scope })
  if (notes.non_functional.length) {
    scope.push(h('Kebutuhan non-fungsional'), {
      type: 'table', head: ['Kebutuhan', 'Target'], widths: [0.3, 0.7],
      rows: notes.non_functional.map(r => [{ t: r.name, bold: true }, r.target])
    })
  }
  S.push({ no: '4', title: 'Ruang lingkup & fitur', blocks: scope.length ? scope : [muted('Daftar fitur belum tersusun.')] })

  // 5. Stack
  const stack: PdfBlock[] = []
  if (doc.stack.length) {
    stack.push({ type: 'table', head: ['Lapisan', 'Teknologi'], widths: [0.26, 0.74], rows: doc.stack.map(r => [{ t: r.layer, bold: true }, { t: r.items.join(' · '), mono: true }]) })
  }
  if (doc.components.length) {
    stack.push({
      type: 'table',
      head: ['Aplikasi', 'Peran', 'Bahasa / framework', 'Port'],
      widths: [0.32, 0.22, 0.34, 0.12],
      rows: doc.components.map(c => [
        { t: c.name, bold: true, sub: c.relative_path || '.' },
        docRole(c.role),
        { t: [c.language, c.framework].filter(Boolean).join(' · ') || '—', mono: true },
        { t: String(c.run_port || c.port || '—'), mono: true, align: 'right' }
      ])
    })
  }
  S.push({ no: '5', title: 'Stack teknologi', blocks: stack.length ? stack : [muted('Belum ada aplikasi terdaftar.')] })

  // 6. Arsitektur
  const arch: PdfBlock[] = [{ type: 'p', text: `Proyek terdiri dari **${doc.components.length}** aplikasi.` }]
  for (const c of doc.components) {
    const apps = (prof?.apps || []).filter(a => a.component_id === c.id)
    arch.push(h(`${c.name} — ${docRole(c.role)}`))
    arch.push(muted([[c.language, c.framework].filter(Boolean).join(' · '), c.run_port || c.port ? `port ${c.run_port || c.port}` : ''].filter(Boolean).join(' · ') || '—'))
    for (const a of apps) {
      arch.push(...p(a.summary))
      if (a.responsibilities?.length) arch.push({ type: 'list', items: a.responsibilities })
      if (a.talks_to?.length) arch.push(muted(`Berkomunikasi dengan: ${a.talks_to.join(', ')}`))
    }
  }
  if (prof?.integrations?.length) {
    arch.push(h('Integrasi'), {
      type: 'table', head: ['Dari', 'Ke', 'Melalui', 'Tujuan'], widths: [0.2, 0.2, 0.2, 0.4],
      rows: prof.integrations.map(i => [{ t: i.from, bold: true }, { t: i.to, bold: true }, { t: i.via, mono: true }, i.purpose])
    })
  }
  S.push({ no: '6', title: 'Arsitektur & aplikasi', blocks: arch })

  // 7. Model data
  const data: PdfBlock[] = []
  if (env?.database_type) {
    data.push({ type: 'p', text: `Basis data: **${env.database_type}**${env.orm_driver ? ` melalui \`${env.orm_driver}\`` : ''}${env.database_name ? `, nama \`${env.database_name}\`` : ''}.` })
  }
  if (prof?.data_entities?.length) {
    data.push({
      type: 'table', head: ['Entitas / tabel', 'Keterangan', 'Dipakai oleh'], widths: [0.26, 0.5, 0.24],
      rows: prof.data_entities.map(e => [{ t: e.name, mono: true, bold: true }, e.description || '—', (e.used_by || []).join(', ') || '—'])
    })
  }
  S.push({ no: '7', title: 'Model data', blocks: data.length ? data : [muted('Entitas data belum tercatat.')] })

  // 8. API & halaman
  const api: PdfBlock[] = []
  if (doc.routes.length) {
    api.push({ type: 'p', text: `Tercatat **${docNum(doc.backendRoutes.length)}** endpoint API dan **${docNum(doc.pageRoutes.length)}** halaman.${doc.backendRoutes.length > ROUTE_LIMIT ? ` Dokumen ini memuat ${ROUTE_LIMIT} endpoint pertama.` : ''}` })
    const groups = new Map<string, typeof doc.backendRoutes>()
    for (const r of doc.backendRoutes.slice(0, ROUTE_LIMIT)) {
      const k = r.domain || r.function_group || r.component_name || 'Umum'
      if (!groups.has(k)) groups.set(k, [])
      groups.get(k)!.push(r)
    }
    for (const [group, list] of [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0]))) {
      api.push(h(`${group} (${list.length})`), {
        type: 'table', head: ['Metode', 'Path', 'Keterangan'], widths: [0.11, 0.47, 0.42],
        rows: list.map(r => [
          { t: r.method.toUpperCase(), mono: true, bold: true, tone: methodTone(r.method) },
          { t: r.path, mono: true },
          { t: r.description || '—', sub: r.middleware || '' }
        ])
      })
    }
    if (doc.curlExample) api.push({ type: 'code', title: 'Contoh pemanggilan API', lang: 'bash', text: doc.curlExample })
    if (doc.pageRoutes.length) {
      api.push(h('Halaman antarmuka'), { type: 'code', lang: 'text', text: doc.pageRoutes.slice(0, 120).map(r => r.path).join('\n') })
    }
  }
  S.push({ no: '8', title: 'Antarmuka: API & halaman', blocks: api.length ? api : [muted('Katalog rute belum ada.')] })

  // 9. Konfigurasi
  const cfg: PdfBlock[] = []
  if (env?.detected_env_keys?.length) {
    cfg.push({ type: 'p', text: 'Nilai rahasia tidak pernah ditampilkan; kolom contoh memuat nilai aman untuk berkas `.env.example`.' })
    cfg.push({
      type: 'table', head: ['Variabel', 'Kegunaan', 'Wajib', 'Contoh'], widths: [0.3, 0.36, 0.1, 0.24],
      rows: env.detected_env_keys.map(k => [
        { t: k.key, mono: true, bold: true, sub: k.is_secret ? 'rahasia' : '' },
        k.purpose || '—',
        k.required ? 'Ya' : 'Tidak',
        { t: k.is_secret ? '••••••' : (k.safe_example || '—'), mono: true }
      ])
    })
    if (env.auth_methods?.length || env.login_endpoints?.length) {
      cfg.push({ type: 'p', text: `**Autentikasi:** ${(env.auth_methods || []).join(', ') || '—'}${env.login_endpoints?.length ? ` · endpoint masuk \`${env.login_endpoints.join(', ')}\`` : ''}` })
    }
  }
  S.push({ no: '9', title: 'Konfigurasi & environment', blocks: cfg.length ? cfg : [muted('Variabel environment belum terdeteksi.')] })

  // 10. Menjalankan
  const run: PdfBlock[] = []
  if (doc.components.length) {
    run.push({ type: 'p', text: 'Perintah tiap aplikasi, dijalankan dari folder aplikasinya masing-masing. Port mengikuti pengelola port engine.' })
    for (const c of doc.components.filter(x => x.dev_command || x.build_command)) {
      run.push({
        type: 'code',
        title: c.name,
        lang: 'bash',
        text: [
          `# ${c.name} (${docRole(c.role)})`,
          `cd ${c.relative_path || '.'}`,
          c.dev_command ? `${c.dev_command}        # mode pengembangan` : '',
          c.build_command ? `${c.build_command}        # build produksi` : '',
          c.health_url ? `curl -fsS ${c.health_url}   # cek kesehatan` : ''
        ].filter(Boolean).join('\n')
      })
    }
  }
  S.push({ no: '10', title: 'Menjalankan, build & deploy', blocks: run.length ? run : [muted('Belum ada aplikasi terdaftar.')] })

  // 11. Struktur kode
  const code: PdfBlock[] = []
  if (cm) {
    const langs = Object.entries(cm.languages || {}).sort((a, b) => b[1] - a[1])
    code.push({ type: 'stats', pairs: [{ k: 'Berkas', v: docNum(cm.file_count) }, { k: 'Baris kode', v: docNum(cm.total_lines) }, { k: 'Simbol', v: docNum(cm.symbol_count) }, { k: 'Bahasa', v: String(langs.length) }] })
    if (langs.length) code.push(muted(`Bahasa: ${langs.map(([l, n]) => `${l} (${n})`).join(', ')}.`))
    const core = (cm.core || []).filter(f => !isGeneratedPath(f.path)).slice(0, 12)
    if (core.length) {
      code.push(h('Modul inti (paling banyak dipakai)'), {
        type: 'table', head: ['Berkas', 'Dipakai', 'Baris'], widths: [0.7, 0.15, 0.15],
        rows: core.map(f => [{ t: f.path, mono: true, sub: f.note || '' }, { t: String(f.used_by), align: 'right' }, { t: docNum(f.lines), align: 'right' }])
      })
    }
    const hot = (cm.hotspots || []).filter(f => !isGeneratedPath(f.path)).slice(0, 6)
    if (hot.length) code.push({ type: 'note', tone: 'warn', title: 'Berkas besar', text: `Perlu dipecah bila terus tumbuh: ${hot.map(x => `\`${x.path}\` (${x.lines})`).join(', ')}.` })
  }
  let tree = doc.knowledge?.files_scope?.directory_tree || ''
  if (tree.split('\n').length < 3 && cm?.components?.length) {
    tree = cm.components.map(c => [
      `${c.path || c.name}/  (${c.files} berkas, ${c.lines} baris)`,
      ...(c.dirs || []).filter(d => !isGeneratedPath(`${d.path}/`)).slice(0, 25).map(d => `  ${d.path}/  (${d.files})`)
    ].join('\n')).join('\n')
  }
  if (tree) {
    const lines = tree.split('\n')
    code.push({ type: 'code', title: 'Struktur direktori', lang: 'text', text: lines.length > 60 ? [...lines.slice(0, 60), `… (${lines.length - 60} baris lagi)`].join('\n') : tree })
  }
  for (const s of doc.entrySnippets) {
    code.push({ type: 'code', title: s.snippet_start ? `${s.path}:${s.snippet_start}` : s.path, lang: s.lang, text: s.snippet || '' })
  }
  S.push({ no: '11', title: 'Struktur kode & contoh', blocks: code.length ? code : [muted('Peta kode belum dibangun.')] })

  // 12. Pengujian
  S.push({
    no: '12',
    title: 'Pengujian & mutu',
    blocks: [{ type: 'p', text: 'Mutu dibuktikan lewat empat lapis: test otomatis di repositori, uji runtime terhadap aplikasi yang berjalan, uji keamanan router & server, dan audit kode statis. Angka di bawah diambil dari hasil terakhir tiap lapis.' }]
  })
  const strat: PdfBlock[] = [
    {
      type: 'stats',
      pairs: [
        { k: 'Berkas test', v: docNum(doc.testFiles.length) },
        { k: 'Kasus test', v: docNum(doc.testCaseCount) },
        { k: 'Rasio baris test', v: `${doc.testRatio}%` },
        { k: 'Skor runtime / keamanan', v: `${rt?.score ?? '—'} / ${sec?.score ?? '—'}` }
      ]
    },
    muted('Rasio baris test = baris berkas test dibagi seluruh baris kode; perkiraan kasar, bukan coverage hasil instrumen.')
  ]
  const per = new Map<string, { files: number, cases: number, lines: number }>()
  for (const f of doc.testFiles) {
    const r = per.get(f.component) || { files: 0, cases: 0, lines: 0 }
    r.files++
    r.cases += f.cases.length
    r.lines += f.lines
    per.set(f.component, r)
  }
  if (per.size) {
    strat.push({
      type: 'table', head: ['Aplikasi', 'Berkas', 'Kasus', 'Baris'], widths: [0.52, 0.16, 0.16, 0.16],
      rows: [...per.entries()].map(([name, r]) => [{ t: name, bold: true }, { t: String(r.files), align: 'right' }, { t: String(r.cases), align: 'right' }, { t: docNum(r.lines), align: 'right' }])
    })
  }
  const testScript = doc.components
    .map(c => ({ c, cmd: testCommand(c.language, c.framework, c.package_manager) }))
    .filter(x => x.cmd)
    .map(x => `# ${x.c.name}\ncd ${x.c.relative_path || '.'} && ${x.cmd}`)
    .join('\n\n')
  if (testScript) strat.push({ type: 'code', title: 'Menjalankan test', lang: 'bash', text: testScript })
  S.push({ no: '12.1', title: 'Strategi & cakupan', level: 1, blocks: strat })

  S.push({
    no: '12.2',
    title: 'Test spesifik',
    level: 1,
    blocks: doc.testFiles.length
      ? [{
          type: 'table', head: ['Berkas test', 'Kasus yang diuji'], widths: [0.42, 0.58],
          rows: doc.testFiles.map(f => [
            { t: f.path, mono: true, sub: [f.component, `${f.lines} baris`, f.summary].filter(Boolean).join(' · ') },
            { t: f.cases.length ? f.cases.join(', ') : '—', mono: true }
          ])
        }]
      : [muted(doc.overview ? 'Peta kode tidak menemukan berkas test.' : 'Peta kode belum dibangun, jadi daftar test belum bisa disusun.')]
  })

  const rtb: PdfBlock[] = []
  if (rt) {
    rtb.push({ type: 'p', text: `Sesi ${docDate(rt.started_at)} — status **${rt.status}**, skor **${rt.score}**.${rt.summary ? ` ${rt.summary}` : ''}` })
    if (rt.components?.length) {
      rtb.push({
        type: 'table', head: ['Aplikasi', 'Keadaan', 'Boot (ms)', 'Memori (MB)', 'Baris galat'], widths: [0.3, 0.2, 0.16, 0.17, 0.17],
        rows: rt.components.map(c => [{ t: c.name, bold: true }, c.state, { t: round(c.boot_ms), align: 'right' }, { t: round(c.memory_mb), align: 'right' }, { t: String(c.error_lines), align: 'right' }])
      })
    }
    if (rt.probes?.length) {
      rtb.push({
        type: 'table', head: ['Endpoint', 'Status', 'Dingin (ms)', 'Hangat (ms)'], widths: [0.55, 0.13, 0.16, 0.16],
        rows: rt.probes.slice(0, 40).map(x => [{ t: `${x.method} ${x.path}`, mono: true }, { t: String(x.status_code || '—'), mono: true, align: 'right', tone: statusTone(x.status_code) }, { t: round(x.cold_ms), align: 'right' }, { t: round(x.warm_ms), align: 'right' }])
      })
    }
    if (rt.pages?.length) {
      rtb.push({
        type: 'table', head: ['Halaman', 'TTFB', 'FCP', 'Muat (ms)'], widths: [0.55, 0.15, 0.15, 0.15],
        rows: rt.pages.slice(0, 30).map(g => [{ t: g.path, mono: true, sub: g.auth === 'auth' ? 'login' : 'tamu' }, { t: round(g.ttfb_ms), align: 'right' }, { t: round(g.fcp_ms), align: 'right' }, { t: round(g.load_ms), align: 'right' }])
      })
    }
    if (rt.logins?.length) {
      rtb.push({
        type: 'table', head: ['Login', 'Hasil', 'Keterangan'], widths: [0.3, 0.14, 0.56],
        rows: rt.logins.map(l => [`${l.kind} ${l.component}`, { t: l.ok ? 'Berhasil' : 'Gagal', tone: l.ok ? 'success' : 'error' }, l.message || '—'])
      })
    }
  }
  S.push({ no: '12.3', title: 'Uji runtime', level: 1, blocks: rtb.length ? rtb : [muted('Uji runtime belum pernah dijalankan.')] })

  const secb: PdfBlock[] = []
  if (sec) {
    secb.push({ type: 'p', text: `Target \`${sec.target}\`, ${docDate(sec.started_at)} — nilai **${sec.grade}** (skor ${sec.score}). ${sec.summary.pass || 0} lulus, ${sec.summary.warn || 0} lemah, ${sec.summary.fail || 0} gagal; ${sec.routes_seen} rute dan ${sec.files_read} berkas diperiksa.` })
    secb.push({
      type: 'table', head: ['Pemeriksaan', 'Hasil', 'Tingkat'], widths: [0.64, 0.18, 0.18],
      rows: sec.checks.map(c => [
        { t: c.title, sub: c.suggestion && c.status !== 'pass' ? c.suggestion : '' },
        { t: SEC_STATUS[c.status]?.label || c.status, tone: SEC_STATUS[c.status]?.tone || '' },
        DOC_SEVERITY[c.severity]?.label || c.severity
      ])
    })
  }
  S.push({ no: '12.4', title: 'Uji keamanan', level: 1, blocks: secb.length ? secb : [muted('Uji keamanan belum pernah dijalankan.')] })

  const audb: PdfBlock[] = []
  if (aud) {
    const open = (aud.findings || []).filter(f => !f.status || f.status === 'open')
    const rank: Record<string, number> = { CRITICAL: 0, WARNING: 1, INFO: 2, OPTIMIZATION: 3 }
    const top = [...open].sort((a, b) => (rank[a.severity] ?? 9) - (rank[b.severity] ?? 9)).slice(0, 15)
    audb.push({ type: 'p', text: `Skor audit **${aud.score}**, ${open.length} temuan terbuka dari ${aud.total_findings} (${aud.severity_counts?.CRITICAL || 0} kritis, ${aud.severity_counts?.WARNING || 0} peringatan).` })
    if (top.length) {
      audb.push({
        type: 'table', head: ['Temuan terbuka teratas', 'Tingkat'], widths: [0.8, 0.2],
        rows: top.map(f => [
          { t: [f.rule_id, f.title].filter(Boolean).join(' · '), sub: f.target_file ? `${f.target_file}${f.line ? `:${f.line}` : ''}` : '' },
          { t: DOC_SEVERITY[f.severity]?.label || f.severity, tone: tone(DOC_SEVERITY[f.severity]?.color) }
        ])
      })
    }
  }
  S.push({ no: '12.5', title: 'Audit kode statis', level: 1, blocks: audb.length ? audb : [muted('Proyek belum diaudit.')] })

  // 13. Risiko
  const risk: PdfBlock[] = []
  if (prof?.risks?.length) {
    risk.push(h('Risiko'), {
      type: 'table', head: ['Risiko', 'Tingkat', 'Status'], widths: [0.64, 0.16, 0.2],
      rows: prof.risks.map(r => [
        { t: r.title, bold: true, sub: `${r.detail || ''}${r.todo_id ? ` · todo #${r.todo_id}` : ''}` },
        { t: DOC_SEVERITY[r.severity]?.label || r.severity, tone: tone(DOC_SEVERITY[r.severity]?.color) },
        RISK_STATUS[r.status || 'open'] || r.status || ''
      ])
    })
  }
  if (prof?.roadmap?.length) {
    risk.push(h('Roadmap'), {
      type: 'table', head: ['Waktu', 'Langkah'], widths: [0.18, 0.82],
      rows: prof.roadmap.map(r => [{ t: DOC_HORIZON[r.horizon] || r.horizon, bold: true }, { t: r.title, sub: r.rationale || '' }])
    })
  }
  if (prof?.open_questions?.length) risk.push(h('Pertanyaan terbuka'), { type: 'olist', items: prof.open_questions })
  S.push({ no: '13', title: 'Risiko, roadmap & pertanyaan terbuka', blocks: risk.length ? risk : [muted('Risiko dan roadmap belum tercatat di profil proyek.')] })

  // 14–16. Catatan tertulis
  const adr: PdfBlock[] = []
  notes.decisions.forEach((d, i) => {
    const st = DOC_ADR_STATUS[d.status]
    adr.push(h(`ADR-${String(i + 1).padStart(3, '0')} · ${d.title || 'Tanpa judul'}`))
    adr.push({
      type: 'kv',
      pairs: [
        { k: 'Status', v: st?.label || d.status || '—', tone: tone(st?.color) },
        ...(d.date ? [{ k: 'Tanggal', v: d.date }] : []),
        ...(d.context ? [{ k: 'Konteks', v: d.context }] : []),
        ...(d.decision ? [{ k: 'Keputusan', v: d.decision }] : []),
        ...(d.consequences ? [{ k: 'Akibat', v: d.consequences }] : [])
      ]
    })
  })
  S.push({ no: '14', title: 'Keputusan teknis (ADR)', blocks: adr.length ? adr : [muted('Belum ada keputusan teknis yang dicatat.')] })
  S.push({
    no: '15',
    title: 'Glosarium',
    blocks: notes.glossary.length
      ? [{ type: 'table', head: ['Istilah', 'Arti'], widths: [0.26, 0.74], rows: [...notes.glossary].sort((a, b) => a.term.localeCompare(b.term)).map(g => [{ t: g.term, bold: true }, g.definition]) }]
      : [muted('Belum ada istilah.')]
  })
  S.push({
    no: '16',
    title: 'Riwayat versi dokumen',
    blocks: notes.changelog.length
      ? [{ type: 'table', head: ['Versi', 'Tanggal', 'Perubahan'], widths: [0.14, 0.18, 0.68], rows: notes.changelog.map(c => [{ t: c.version || '—', mono: true, bold: true }, c.date || '—', c.changes]) }]
      : [muted('Belum ada riwayat versi.')]
  })

  // Lampiran A: lembar teknis.
  const sheet: PdfBlock[] = [
    {
      type: 'stats',
      pairs: [
        { k: 'Kesehatan', v: String(pr.health_score ?? '—'), tone: scoreTone(pr.health_score) },
        { k: 'Audit', v: String(aud?.score ?? '—'), tone: scoreTone(aud?.score) },
        { k: 'Runtime', v: String(rt?.score ?? '—'), tone: scoreTone(rt?.score) },
        { k: 'Keamanan', v: String(sec?.score ?? '—'), tone: scoreTone(sec?.score) }
      ]
    },
    {
      type: 'kv',
      pairs: [
        { k: 'Berkas', v: docNum(cm?.file_count ?? doc.knowledge?.files_scope?.total_files) },
        { k: 'Baris kode', v: docNum(cm?.total_lines) },
        { k: 'Endpoint API', v: docNum(doc.backendRoutes.length) },
        { k: 'Halaman', v: docNum(doc.pageRoutes.length) },
        { k: 'Berkas test', v: docNum(doc.testFiles.length) },
        { k: 'Kasus test', v: docNum(doc.testCaseCount) }
      ]
    }
  ]
  if (doc.components.length) {
    sheet.push(h('Aplikasi'), {
      type: 'table', head: ['Aplikasi', 'Peran', 'Perintah dev', 'Port'], widths: [0.24, 0.2, 0.44, 0.12],
      rows: doc.components.map(c => [
        { t: c.name, bold: true, sub: c.framework || c.language || '' },
        docRole(c.role),
        { t: c.dev_command || '—', mono: true },
        { t: String(c.run_port || c.port || '—'), mono: true, align: 'right' }
      ])
    })
  }
  S.push({ no: 'A', title: 'Lampiran: lembar teknis', blocks: sheet })

  const date = new Date().toLocaleDateString('id-ID', { day: 'numeric', month: 'long', year: 'numeric' })
  return {
    title: pr.name,
    subtitle: prof?.short_description || pr.short_description || '',
    kicker: 'Dokumen teknis',
    version: notes.version,
    authors: notes.authors,
    date,
    meta: [
      { k: 'Status', v: DOC_STATUS[pr.status] || pr.status || '' },
      { k: 'Kematangan', v: prof?.maturity?.stage ? DOC_MATURITY[prof.maturity.stage] || '' : '' },
      { k: 'Aplikasi', v: doc.components.length ? `${doc.components.length} aplikasi` : '' },
      { k: 'Stack', v: [...new Set(doc.stack.flatMap(s => s.items))].slice(0, 5).join(', ') },
      { k: 'Repositori', v: pr.git_remote || '' },
      { k: 'Branch', v: pr.git_branch || '' }
    ].filter(m => m.v),
    sections: S
  }
}
