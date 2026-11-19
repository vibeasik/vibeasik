// Label bahasa Indonesia untuk nilai mentah di Dokumen Proyek.
import type { UnwrapNestedRefs } from 'vue'
import type { ProjectDocument } from '~/composables/useProjectDocument'

export type DocData = UnwrapNestedRefs<ProjectDocument>

export const DOC_ROLE_LABEL: Record<string, string> = {
  backend: 'Backend / API',
  frontend_admin: 'Frontend admin',
  frontend_client: 'Frontend klien',
  frontend_mobile: 'Aplikasi mobile',
  cli: 'CLI',
  worker: 'Worker',
  docs: 'Dokumentasi',
  other: 'Lainnya'
}
export const docRole = (r?: string) => DOC_ROLE_LABEL[r || ''] || r || '—'

export const DOC_FEATURE_STATUS: Record<string, { label: string, color: 'success' | 'warning' | 'neutral' }> = {
  done: { label: 'Selesai', color: 'success' },
  partial: { label: 'Sebagian', color: 'warning' },
  planned: { label: 'Direncanakan', color: 'neutral' }
}

export const DOC_MATURITY: Record<string, string> = {
  idea: 'Ide', prototype: 'Prototipe', mvp: 'MVP', beta: 'Beta', production: 'Produksi'
}

export const DOC_STATUS: Record<string, string> = {
  active: 'Aktif', hold: 'Ditahan', done: 'Selesai', deprecated: 'Usang', paused: 'Dijeda', archived: 'Diarsipkan'
}

export const DOC_HORIZON: Record<string, string> = { now: 'Sekarang', next: 'Berikutnya', later: 'Nanti' }

export const DOC_SEVERITY: Record<string, { label: string, color: 'error' | 'warning' | 'neutral' }> = {
  high: { label: 'Tinggi', color: 'error' },
  medium: { label: 'Sedang', color: 'warning' },
  low: { label: 'Rendah', color: 'neutral' },
  CRITICAL: { label: 'Kritis', color: 'error' },
  WARNING: { label: 'Peringatan', color: 'warning' },
  INFO: { label: 'Info', color: 'neutral' }
}

export const docDate = (v?: string | Date | null) => {
  if (!v) return '—'
  const d = typeof v === 'string' ? new Date(v) : v
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
}

export const docNum = (n?: number | null) => (n ?? 0).toLocaleString('id-ID')

// Kelas tabel dokumen (satu sumber agar semua tabel sama padatnya).
export const DOC_TABLE = 'w-full text-xs tabular-nums border-collapse'
export const DOC_TH = 'text-left font-medium text-neutral-500 dark:text-slate-400 py-2 pr-3 border-b border-neutral-200 dark:border-white/[0.08]'
export const DOC_TD = 'py-2 pr-3 align-top border-b border-neutral-100 dark:border-white/[0.05]'

// Markdown sebaris (tebal, miring, kode) → HTML aman. Teks di-escape dulu, jadi hanya tag yang dibuat di sini yang lolos.
export const docInline = (text?: string | null) => (text || '')
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
  .replace(/`([^`]+)`/g, '<code class="font-mono text-[0.85em] rounded bg-neutral-100 dark:bg-white/[0.06] px-1">$1</code>')
  .replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>')
  .replace(/(^|[^*])\*([^*\n]+)\*/g, '$1<i>$2</i>')

// Berkas hasil build/vendor tidak mewakili kode proyek (mis. bundel _nuxt lama, .min.js, dist/).
export const isGeneratedPath = (p: string) => /(^|\/)(_nuxt|dist|build|vendor|node_modules|\.output|\.nuxt)\/|\.min\.(js|css)$/.test(p)

// ---------- Bagian yang ditulis tangan (GET/PUT /api/projects/:id/doc-notes) ----------
export interface ProjectDocDecision {
  title: string
  date: string
  status: '' | 'proposed' | 'accepted' | 'superseded' | 'deprecated'
  context: string
  decision: string
  consequences: string
}

export interface ProjectDocContent {
  version: string
  authors: string
  background: string
  out_of_scope: string[]
  non_functional: { name: string, target: string }[]
  decisions: ProjectDocDecision[]
  glossary: { term: string, definition: string }[]
  changelog: { version: string, date: string, changes: string }[]
}

export const emptyDocContent = (): ProjectDocContent => ({
  version: '', authors: '', background: '', out_of_scope: [], non_functional: [], decisions: [], glossary: [], changelog: []
})

export const DOC_ADR_STATUS: Record<string, { label: string, color: 'success' | 'warning' | 'neutral' | 'error' }> = {
  proposed: { label: 'Diusulkan', color: 'warning' },
  accepted: { label: 'Diterima', color: 'success' },
  superseded: { label: 'Digantikan', color: 'neutral' },
  deprecated: { label: 'Tidak berlaku', color: 'error' }
}
