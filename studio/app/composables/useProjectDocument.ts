import type { Ref } from 'vue'
import type { ProjectDocContent } from '~/utils/projectDocLabels'
import { emptyDocContent } from '~/utils/projectDocLabels'
import type {
  BaseKnowledgeRoute, CodeMapOverview, CodeMapOverviewFile, CodeMapSearchHit, DiagnosticReportResponse, Project, ProjectBaseKnowledge,
  ProjectProfile, RuntimeSimSession, SecurityTestReport
} from '~/types'

/**
 * Dokumen Proyek: satu dokumen teknis yang dirakit dari data yang sudah dimiliki engine (profil, Base Knowledge,
 * peta kode, uji runtime, uji keamanan, audit). Tiap sumber dimuat sendiri-sendiri; sumber yang gagal atau
 * terkunci Pro (402) hanya mengosongkan bagiannya, tidak menggagalkan seluruh dokumen.
 */

export interface DocSectionMeta {
  id: string
  no: string
  title: string
  children?: { id: string, no: string, title: string }[]
}

// Kerangka dokumen. Urutan mengikuti dokumen teknis yang umum (ringkas → konteks → ruang lingkup → solusi →
// operasional → mutu → risiko), dari yang dibaca pemilik/klien sampai yang dibaca pengembang.
export const DOC_SECTIONS: DocSectionMeta[] = [
  { id: 'keterangan', no: '1', title: 'Keterangan proyek' },
  { id: 'latar-belakang', no: '2', title: 'Latar belakang & masalah' },
  { id: 'tujuan', no: '3', title: 'Tujuan & pengguna sasaran' },
  { id: 'lingkup', no: '4', title: 'Ruang lingkup & fitur' },
  { id: 'stack', no: '5', title: 'Stack teknologi' },
  { id: 'arsitektur', no: '6', title: 'Arsitektur & aplikasi' },
  { id: 'data', no: '7', title: 'Model data' },
  { id: 'api', no: '8', title: 'Antarmuka: API & halaman' },
  { id: 'konfigurasi', no: '9', title: 'Konfigurasi & environment' },
  { id: 'menjalankan', no: '10', title: 'Menjalankan, build & deploy' },
  { id: 'kode', no: '11', title: 'Struktur kode & contoh' },
  {
    id: 'pengujian',
    no: '12',
    title: 'Pengujian & mutu',
    children: [
      { id: 'uji-strategi', no: '12.1', title: 'Strategi & cakupan' },
      { id: 'uji-spesifik', no: '12.2', title: 'Test spesifik' },
      { id: 'uji-runtime', no: '12.3', title: 'Uji runtime' },
      { id: 'uji-keamanan', no: '12.4', title: 'Uji keamanan' },
      { id: 'uji-audit', no: '12.5', title: 'Audit kode statis' }
    ]
  },
  { id: 'risiko', no: '13', title: 'Risiko, roadmap & pertanyaan terbuka' },
  { id: 'keputusan', no: '14', title: 'Keputusan teknis (ADR)' },
  { id: 'glosarium', no: '15', title: 'Glosarium' },
  { id: 'riwayat', no: '16', title: 'Riwayat versi dokumen' }
]

export type DocSourceKey = 'profile' | 'knowledge' | 'codemap' | 'runtime' | 'security' | 'audit' | 'notes'

export interface DocSourceState {
  key: DocSourceKey
  label: string
  // ok = ada data; empty = belum pernah dibuat; locked = fitur Pro; error = gagal dimuat
  state: 'ok' | 'empty' | 'locked' | 'error'
  hint: string
  fixTo?: string
}

export interface DocTestFile {
  path: string
  component: string
  lang: string
  lines: number
  summary?: string
  cases: string[]
}

// Dokumen bisa dibagikan sebagai PDF: alat token/seed/kredensial tidak dijadikan contoh kode, dan email serta
// nilai rahasia yang tertulis di kode disamarkan.
const SENSITIVE_PATH = /(token|secret|seed|passw|credential|cred|apikey|api_key|private|\.env)/i
const redactSnippet = (code: string) => code
  .replace(/[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/g, '<email>')
  .replace(/((?:pass(?:word)?|secret|token|api[_-]?key|private[_-]?key|dsn)\w*["']?\s*[:=]+\s*)(["'`])[^"'`\n]{3,}\2/gi, '$1$2•••$2')

const statusOf = (err: any): number => err?.statusCode ?? err?.status ?? err?.response?.status ?? 0

// Nama fungsi/kasus test dari simbol peta kode (Go TestX, JS describe/it tidak menjadi simbol, jadi dipakai apa adanya).
const testCases = (f: CodeMapOverviewFile): string[] =>
  (f.symbols || []).filter(s => /^(Test|Benchmark|Fuzz|Example|test_|it_|should)/.test(s) || f.lang !== 'go')

export function useProjectDocument(projectId: Ref<number | null>) {
  const api = useApi()
  const codeMap = useCodeMap()
  const security = useSecurityTest()

  const project = ref<Project | null>(null)
  const profile = ref<ProjectProfile | null>(null)
  const knowledge = ref<ProjectBaseKnowledge | null>(null)
  const overview = ref<CodeMapOverview | null>(null)
  const runtime = ref<RuntimeSimSession | null>(null)
  const securityReport = ref<SecurityTestReport | null>(null)
  const audit = ref<DiagnosticReportResponse | null>(null)
  const entrySnippets = ref<CodeMapSearchHit[]>([])
  const componentRoutes = ref<BaseKnowledgeRoute[]>([])
  // Bagian yang ditulis tangan (ADR, di luar lingkup, glosarium, riwayat versi).
  const notes = ref<ProjectDocContent>(emptyDocContent())
  const notesUpdatedAt = ref<string | null>(null)
  const savingNotes = ref(false)
  const notesUrl = (pid: number) => `${api.apiBase}/api/projects/${pid}/doc-notes`
  const sources = ref<DocSourceState[]>([])
  const loading = ref(false)
  const error = ref('')
  const loadedAt = ref<Date | null>(null)

  const note = (key: DocSourceKey, label: string, state: DocSourceState['state'], hint: string, fixTo?: string) => {
    sources.value.push({ key, label, state, hint, fixTo })
  }

  // Satu sumber: hasil null bila gagal, dengan status sumbernya dicatat untuk panel "Sumber data".
  const settle = async <T>(key: DocSourceKey, label: string, fixTo: string, run: () => Promise<T | null>, emptyHint: string): Promise<T | null> => {
    try {
      const v = await run()
      if (v) note(key, label, 'ok', 'Termuat')
      else note(key, label, 'empty', emptyHint, fixTo)
      return v
    } catch (err: any) {
      const code = statusOf(err)
      if (code === 402) note(key, label, 'locked', 'Fitur edisi Pro', '/upgrade')
      else if (code === 404) note(key, label, 'empty', emptyHint, fixTo)
      else note(key, label, 'error', err?.data?.error || err?.message || 'Gagal dimuat', fixTo)
      return null
    }
  }

  const load = async () => {
    const id = projectId.value
    if (!id) return
    loading.value = true
    error.value = ''
    sources.value = []
    try {
      project.value = await api.getProject(id)
    } catch (err: any) {
      error.value = err?.data?.error || err?.message || 'Proyek tidak dapat dimuat'
      loading.value = false
      return
    }
    const pid = id
    const [prof, bk, cm, rt, sec, aud, nt] = await Promise.all([
      settle('profile', 'Profil proyek', `/projects/${pid}`, () => api.getProjectProfile(pid),
        'Belum disusun — buka Ringkasan Proyek lalu susun profil'),
      // Cache Base Knowledge yang belum dikompilasi berisi lingkup kosong: diperlakukan sebagai belum ada.
      settle('knowledge', 'Base Knowledge (rute, file, env)', `/projects/${pid}?tab=technical`,
        () => api.getProjectBaseKnowledge(pid).then(b => (b && (b.routers_scope?.total_routes || b.files_scope?.total_files || b.environment_scope?.detected_env_keys?.length) ? b : null)),
        'Kosong — kompilasi ulang di Rute, File & Env'),
      settle('codemap', 'Peta kode', '/code-map', () => codeMap.getOverview(pid, 'files').then(o => (o?.file_count ? o : null)),
        'Belum dibangun — buka Peta Kode'),
      settle('runtime', 'Uji runtime', '/runtime-test', () => api.getRuntimeSimulation(pid).then(r => r?.session || null),
        'Belum pernah dijalankan'),
      settle('security', 'Uji keamanan', '/security-test', () => security.getState(pid).then(r => r?.report || null),
        'Belum pernah dijalankan'),
      settle('audit', 'Audit & diagnostik', '/audit-report', () => api.getLatestProjectDiagnostic(pid).then(r => (r?.diagnostic ? r : null)),
        'Belum pernah diaudit'),
      settle('notes', 'Catatan tertulis (ADR, glosarium)', '', () => $fetch<{ content: ProjectDocContent, updated_at: string | null }>(notesUrl(pid))
        .then(r => (r?.updated_at ? r : null)), 'Belum ditulis — tekan Edit catatan')
    ])
    if (projectId.value !== pid) return
    profile.value = prof
    knowledge.value = bk
    overview.value = cm
    runtime.value = rt
    securityReport.value = sec
    audit.value = aud
    notes.value = { ...emptyDocContent(), ...(nt?.content || {}) }
    notesUpdatedAt.value = nt?.updated_at || null
    // Tanpa Base Knowledge, katalog rute diambil langsung dari rute tersinkron tiap aplikasi.
    componentRoutes.value = bk?.routers_scope?.routes?.length
      ? []
      : (await Promise.all((project.value?.components || []).map(c => api.getComponentRoutes(c.id)
          .then(rs => (rs || []).map(r => ({
            component_id: c.id, component_name: c.name, method: r.method, path: r.path, handler: r.handler,
            middleware: r.middleware || '', description: r.description, domain: r.domain, function_group: r.function_group
          } as BaseKnowledgeRoute)))
          .catch(() => [] as BaseKnowledgeRoute[])))).flat()
    // Potongan kode titik masuk untuk bagian "Struktur kode & contoh" (tidak wajib; abaikan bila gagal).
    entrySnippets.value = cm
      ? await codeMap.search(pid, 'main entry server router bootstrap', { kind: 'entry', limit: 8, snippet: true })
          .then(r => (r.hits || []).filter(h => h.snippet && !SENSITIVE_PATH.test(h.path)).slice(0, 3)
            .map(h => ({ ...h, snippet: redactSnippet(h.snippet || '') }))).catch(() => [])
      : []
    loadedAt.value = new Date()
    loading.value = false
  }

  watch(projectId, (id, old) => { if (id && id !== old) load() })

  // Simpan seluruh catatan (PUT mengganti isi). Galat dilempar ke pemanggil untuk ditampilkan di editor.
  const saveNotes = async (content: ProjectDocContent) => {
    const pid = projectId.value
    if (!pid) return
    savingNotes.value = true
    try {
      const r = await $fetch<{ content: ProjectDocContent, updated_at: string }>(notesUrl(pid), { method: 'PUT', body: content })
      notes.value = { ...emptyDocContent(), ...r.content }
      notesUpdatedAt.value = r.updated_at
      const src = sources.value.find(x => x.key === 'notes')
      if (src) Object.assign(src, { state: 'ok', hint: 'Termuat', fixTo: undefined })
    } finally {
      savingNotes.value = false
    }
  }

  // Draf catatan dari LLM aktif (fitur Pro). Tidak disimpan: hasilnya dimasukkan ke editor untuk diperiksa.
  const generateNotes = async (): Promise<{ content: ProjectDocContent, model: string }> => {
    const pid = projectId.value
    if (!pid) throw new Error('Pilih proyek dulu')
    return await $fetch<{ content: ProjectDocContent, model: string }>(`${notesUrl(pid)}/generate`, { method: 'POST' })
  }

  // ---------- Turunan untuk bagian dokumen ----------
  // Path aplikasi relatif terhadap folder proyek (relative_path sering kosong bila hanya absolute_path yang tercatat).
  const components = computed(() => {
    const root = (project.value?.root_path || '').replace(/\/+$/, '')
    return (project.value?.components || []).map((c) => {
      const abs = c.absolute_path || ''
      const rel = c.relative_path || (root && abs.startsWith(root + '/') ? abs.slice(root.length + 1) : '')
      return { ...c, relative_path: rel }
    })
  })

  const stack = computed(() => {
    const rows = new Map<string, { layer: string, items: Set<string> }>()
    const add = (layer: string, v?: string) => {
      const t = (v || '').trim()
      if (!t || t === 'unknown') return
      if (!rows.has(layer)) rows.set(layer, { layer, items: new Set() })
      rows.get(layer)!.items.add(t)
    }
    for (const c of components.value) {
      const layer = c.role.startsWith('frontend') ? 'Frontend' : c.role === 'backend' ? 'Backend' : c.role === 'worker' ? 'Worker' : 'Lainnya'
      add(layer, c.language)
      add(layer, c.framework)
      add('Alat build', c.package_manager)
    }
    const env = knowledge.value?.environment_scope
    add('Database', env?.database_type)
    add('Database', env?.orm_driver)
    for (const a of env?.auth_methods || []) add('Autentikasi', a)
    for (const s of env?.third_party_services || []) add('Layanan pihak ketiga', s)
    for (const s of profile.value?.external_services || []) add('Layanan pihak ketiga', s.name)
    return [...rows.values()].map(r => ({ layer: r.layer, items: [...r.items] }))
  })

  const testFiles = computed<DocTestFile[]>(() => {
    const out: DocTestFile[] = []
    for (const comp of overview.value?.components || []) {
      for (const dir of comp.dirs || []) {
        for (const f of dir.entries || []) {
          if (f.kind !== 'test') continue
          out.push({ path: f.path, component: comp.name, lang: f.lang, lines: f.lines, summary: f.summary, cases: testCases(f) })
        }
      }
    }
    return out.sort((a, b) => a.path.localeCompare(b.path))
  })

  const testCaseCount = computed(() => testFiles.value.reduce((n, f) => n + f.cases.length, 0))

  // Rasio baris test terhadap baris kode (perkiraan cakupan kasar, bukan coverage instrumen).
  const testRatio = computed(() => {
    const total = overview.value?.total_lines || 0
    const tests = testFiles.value.reduce((n, f) => n + f.lines, 0)
    return total ? Math.round((tests / total) * 1000) / 10 : 0
  })

  const routes = computed(() => knowledge.value?.routers_scope?.routes?.length ? knowledge.value.routers_scope.routes : componentRoutes.value)
  // Halaman = rute milik aplikasi frontend (kecuali /api) atau bermetode PAGE; sisanya endpoint API.
  const roleOf = (id: number) => components.value.find(c => c.id === id)?.role || ''
  const isPage = (r: BaseKnowledgeRoute) => !r.method || r.method.toUpperCase() === 'PAGE'
    || (roleOf(r.component_id).startsWith('frontend') && !r.path.startsWith('/api'))
  const backendRoutes = computed(() => routes.value.filter(r => !isPage(r)))
  const pageRoutes = computed(() => routes.value.filter(isPage))

  // Contoh pemanggilan API pertama yang aman (GET) untuk bagian kode.
  const curlExample = computed(() => {
    const be = components.value.find(c => c.role === 'backend' && (c.run_port || c.port))
    const r = backendRoutes.value.find(x => x.method.toUpperCase() === 'GET' && !x.path.includes(':')) || backendRoutes.value[0]
    if (!be || !r) return ''
    const port = be.run_port || be.port
    const path = r.path.replace(/:([a-zA-Z_]+)/g, '1').replace(/\{[^}]+\}/g, '1')
    const m = r.method.toUpperCase()
    return `curl -s -X ${m} http://localhost:${port}${path}${/auth|jwt|token/i.test(r.middleware || '') ? ' \\\n  -H "Authorization: Bearer <token>"' : ''}`
  })

  return {
    project, profile, knowledge, overview, runtime, securityReport, audit, entrySnippets, sources,
    notes, notesUpdatedAt, savingNotes, saveNotes, generateNotes,
    loading, error, loadedAt, load,
    components, stack, testFiles, testCaseCount, testRatio, routes, backendRoutes, pageRoutes, curlExample
  }
}

export type ProjectDocument = ReturnType<typeof useProjectDocument>
