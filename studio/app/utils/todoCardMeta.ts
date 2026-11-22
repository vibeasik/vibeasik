import type { Todo } from '~/types'

// Metadata tampilan kartu todo (label, warna, ikon) yang diturunkan dari satu todo.
export const todoLevelMeta = (todo: Todo) => {
  switch (todo.level) {
    case 'global':
      return { label: 'Global (Platform)', color: 'neutral', icon: 'i-lucide-globe', badgeClass: 'bg-violet-100 text-violet-800 dark:bg-violet-900/40 dark:text-violet-300 border-violet-200 dark:border-violet-800' }
    case 'project':
      return { label: 'Proyek (Bundle)', color: 'info', icon: 'i-lucide-folder-kanban', badgeClass: 'bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300 border-blue-200 dark:border-blue-800' }
    case 'component':
      return { label: 'Komponen (Module)', color: 'success', icon: 'i-lucide-layers', badgeClass: 'bg-sky-100 text-sky-800 dark:bg-sky-900/40 dark:text-sky-300 border-sky-200 dark:border-sky-800' }
    default:
      return { label: todo.level, color: 'neutral', icon: 'i-lucide-check-square', badgeClass: 'bg-neutral-100 text-neutral-800 dark:bg-neutral-800 dark:text-neutral-300 border-neutral-200 dark:border-neutral-700' }
  }
}

export const todoPriorityMeta = (todo: Todo) => {
  switch (todo.priority) {
    case 'critical':
      return { label: 'Kritis', class: 'text-red-700 bg-red-50 dark:bg-red-950/40 dark:text-red-300 border-red-200 dark:border-red-800' }
    case 'high':
      return { label: 'Tinggi', class: 'text-amber-700 bg-amber-50 dark:bg-amber-950/40 dark:text-amber-300 border-amber-200 dark:border-amber-800' }
    case 'medium':
      return { label: 'Sedang', class: 'text-blue-700 bg-blue-50 dark:bg-blue-950/40 dark:text-blue-300 border-blue-200 dark:border-blue-800' }
    default:
      return { label: 'Rendah', class: 'text-neutral-600 bg-neutral-50 dark:bg-neutral-800/40 dark:text-neutral-400 border-neutral-200 dark:border-neutral-700' }
  }
}

export const todoStatusMeta = (todo: Todo) => {
  switch (todo.status) {
    case 'suggested':
      return { label: 'Saran AI Menunggu Approval', class: 'text-amber-700 bg-amber-50 dark:bg-amber-950/30 dark:text-amber-300 border-amber-300 dark:border-amber-700' }
    case 'in_progress':
      return { label: 'Sedang Dikerjakan', class: 'text-sky-700 bg-sky-50 dark:bg-sky-950/30 dark:text-sky-300 border-sky-300 dark:border-sky-700' }
    case 'completed':
      return { label: 'Selesai', class: 'text-emerald-700 bg-emerald-50 dark:bg-emerald-950/30 dark:text-emerald-300 border-emerald-300 dark:border-emerald-700' }
    case 'rejected':
      return { label: 'Ditolak', class: 'text-neutral-500 bg-neutral-100 dark:bg-neutral-800 dark:text-neutral-400 border-neutral-200 dark:border-neutral-700' }
    case 'merged':
      return { label: 'Dilebur ke Todo Gabungan', class: 'text-violet-700 bg-violet-50 dark:bg-violet-950/30 dark:text-violet-300 border-violet-300 dark:border-violet-700' }
    case 'split':
      return { label: 'Dipecah per Aplikasi', class: 'text-indigo-700 bg-indigo-50 dark:bg-indigo-950/30 dark:text-indigo-300 border-indigo-300 dark:border-indigo-700' }
    default:
      return { label: 'Menunggu (Pending)', class: 'text-neutral-700 bg-neutral-100 dark:bg-neutral-800 dark:text-neutral-300 border-neutral-200 dark:border-neutral-700' }
  }
}

export const todoCategoryMeta = (todo: Pick<Todo, 'category'>) => {
  const cat = todo.category || 'general'
  switch (cat) {
    case 'security':
      return { label: 'Keamanan (Security)', icon: 'i-lucide-shield-check', class: 'bg-rose-100 text-rose-800 dark:bg-rose-950/40 dark:text-rose-300 border-rose-200 dark:border-rose-800' }
    case 'ui_ux':
      return { label: 'UI / UX', icon: 'i-lucide-palette', class: 'bg-fuchsia-100 text-fuchsia-800 dark:bg-fuchsia-950/40 dark:text-fuchsia-300 border-fuchsia-200 dark:border-fuchsia-800' }
    case 'feature':
      return { label: 'Fitur', icon: 'i-lucide-sparkles', class: 'bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-300 border-amber-200 dark:border-amber-800' }
    case 'architecture':
      return { label: 'Arsitektur', icon: 'i-lucide-network', class: 'bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300 border-blue-200 dark:border-blue-800' }
    case 'performance':
      return { label: 'Performa & Port', icon: 'i-lucide-zap', class: 'bg-sky-100 text-sky-800 dark:bg-sky-950/40 dark:text-sky-300 border-sky-200 dark:border-sky-800' }
    case 'backend':
      return { label: 'Backend & API', icon: 'i-lucide-server', class: 'bg-sky-100 text-sky-800 dark:bg-sky-950/40 dark:text-sky-300 border-sky-200 dark:border-sky-800' }
    case 'devops':
      return { label: 'DevOps / CI-CD', icon: 'i-lucide-git-branch', class: 'bg-violet-100 text-violet-800 dark:bg-violet-950/40 dark:text-violet-300 border-violet-200 dark:border-violet-800' }
    default:
      return { label: 'Umum', icon: 'i-lucide-tag', class: 'bg-neutral-100 text-neutral-800 dark:bg-neutral-800 dark:text-neutral-300 border-neutral-200 dark:border-neutral-700' }
  }
}

// todoCategoriesMeta: lencana semua kategori todo (todo gabungan lintas kategori membawa beberapa), kategori utama dulu.
export const todoCategoriesMeta = (todo: Pick<Todo, 'category' | 'categories'>) => {
  const cats = [todo.category || 'general', ...(todo.categories || '').split(',')]
    .map(c => c.trim())
    .filter((c, i, all) => c && all.indexOf(c) === i)
  return cats.map(c => ({ key: c, ...todoCategoryMeta({ category: c }) }))
}

export const todoTargetAppsDisplay = (todo: Todo) => {
  if (todo.target_apps) return todo.target_apps
  if (todo.component_name) return `${todo.component_name} [1 App]`
  if (todo.project_name) return `Seluruh Aplikasi (${todo.project_name})`
  return 'Global Platform'
}

export type ModelSuggestion = {
  label: string
  badgeClass: string
  icon: string
  title: string
}

/**
 * Kelas model todo — sama dengan aturan engine (models.TodoModelTier). Model kelas menengah (setara Claude Sonnet)
 * adalah model utama: dikawal pengetahuan vibeasik ia cukup untuk hampir semua todo, termasuk yang berat.
 * Opus hanya untuk todo kritis; Haiku hanya untuk ekstraksi/olah data massal yang hasilnya pasti.
 */
export type ModelTier = 'utama' | 'kritis' | 'ekstraksi'

/** Model setara satu kelas untuk satu agent coding yang terpasang (GET /api/orchestrator/model-tiers). */
export type TierChoice = { agent: string, agent_name: string, model: string }
export type TierModels = Partial<Record<ModelTier, TierChoice[]>>

export const MODEL_SUGGESTIONS: Record<ModelTier, ModelSuggestion> = {
  kritis: {
    label: 'Saran: Opus',
    badgeClass: 'bg-violet-100/80 text-violet-800 dark:bg-violet-950/40 dark:text-violet-300 border-violet-200 dark:border-violet-800',
    icon: 'i-lucide-shield-alert',
    title: 'Todo kritis (keamanan / prioritas critical): model kelas Opus'
  },
  ekstraksi: {
    label: 'Saran: Haiku',
    badgeClass: 'bg-emerald-100/80 text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800',
    icon: 'i-lucide-zap',
    title: 'Ekstraksi / olah data massal yang hasilnya pasti: cukup model kelas Haiku'
  },
  utama: {
    label: 'Saran: Sonnet',
    badgeClass: 'bg-blue-100/80 text-blue-800 dark:bg-blue-950/40 dark:text-blue-300 border-blue-200 dark:border-blue-800',
    icon: 'i-lucide-sparkles',
    title: 'Model utama vibeasik: kelas Sonnet, dikawal pengetahuan vibeasik — termasuk todo berat (effort menyesuaikan)'
  }
}

const EXTRACTION_TITLE = /\b(ekstrak\w*|extract\w*|impor data|import data|ekspor data|export data|scrap\w*|kumpulkan data|inventaris\w*|konversi data|convert data|format ulang|reformat|terjemah\w*|translate|seed data|data dummy|isi data|migrasi data|rename massal|ganti nama massal|daftar(kan)? semua)\b/i

export const isCriticalTask = (todo: Todo): boolean =>
  todo.category === 'security' || todo.priority === 'critical'

export const isExtractionTask = (todo: Todo): boolean =>
  todo.complexity_level !== 'complex' && EXTRACTION_TITLE.test(todo.title || '')

export const todoModelTier = (todo: Todo): ModelTier => {
  if (isCriticalTask(todo)) return 'kritis'
  if (isExtractionTask(todo)) return 'ekstraksi'
  return 'utama'
}

/**
 * Lencana saran model. Dengan `tiers` (model setara per agent terpasang di komputer ini) label memakai model yang
 * benar-benar tersedia — Claude Code dulu, lalu agent lain (mis. Antigravity claude-sonnet-4-6) — dan tooltip
 * menyebut semua pilihan setara.
 */
export const todoRecommendedModelMeta = (todo: Todo, tiers?: TierModels | null): ModelSuggestion | null => {
  if (todo.status === 'completed' || todo.status === 'rejected') return null
  const tier = todoModelTier(todo)
  const base = MODEL_SUGGESTIONS[tier]
  const choices = tiers?.[tier]
  if (!tiers) return base
  if (!choices?.length) {
    return { ...base, title: `${base.title}. Belum ada agent terpasang dengan model setara kelas ini.` }
  }
  const first = choices[0]!
  const label = first.agent === 'claude' ? base.label : `Saran: ${first.model}`
  const list = choices.map(c => `${c.agent_name}: ${c.model}`).join(' · ')
  return { ...base, label, title: `${base.title}. Di komputer ini: ${list}` }
}

export interface TodoOriginMetaResult {
  type: string
  label: string
  sublabel: string
  badgeClass: string
  icon: string
  title: string
}

export const isExecutiveOrigin = (todo: Todo): boolean => {
  const src = (todo.source || '').toLowerCase()
  const creator = (todo.creator_name || '').toLowerCase()
  const cType = (todo.creator_type || '').toLowerCase()
  return src === 'autonomous_executive' || src === 'executive_chat' || creator.includes('eksekutif') || cType === 'executive'
}

export const isAuditOrigin = (todo: Todo): boolean => {
  const src = (todo.source || '').toLowerCase()
  const auditKey = (todo as any).audit_key || ''
  const guideline = todo.guideline_ref || ''
  return src === 'diagnostic_audit' || src === 'audit' || auditKey !== '' || guideline !== ''
}

export const isPmOrigin = (todo: Todo): boolean => {
  const src = (todo.source || '').toLowerCase()
  const creator = (todo.creator_name || '').toLowerCase()
  return src === 'pm_agent' || src === 'planner' || creator.includes('project-manager') || creator.includes('pm')
}

export const isAiSuggestionOrigin = (todo: Todo): boolean => {
  const src = (todo.source || '').toLowerCase()
  const creator = (todo.creator_name || '').toLowerCase()
  return Boolean(todo.is_ai_suggestion) || creator.includes('advisor') || src === 'ai_template' || src === 'template'
}

export const isAgentOrigin = (todo: Todo): boolean => {
  const creator = (todo.creator_name || '').toLowerCase()
  const cType = (todo.creator_type || '').toLowerCase()
  return cType === 'ai_agent' || creator.includes('claude') || creator.includes('antigravity') || creator.includes('opencode') || creator.includes('agent')
}

export const createExecutiveOriginMeta = (todo: Todo): TodoOriginMetaResult => ({
  type: 'executive',
  label: '🏛️ Dewan Eksekutif',
  sublabel: todo.creator_name || 'Dewan Eksekutif',
  badgeClass: 'bg-amber-100 text-amber-900 dark:bg-amber-950/60 dark:text-amber-300 border-amber-300 dark:border-amber-800 font-semibold',
  icon: 'i-lucide-crown',
  title: 'Dirumuskan melalui musyawarah Dewan Eksekutif (CEO, CTO, Product Owner, CMO)'
})

export const createAuditOriginMeta = (todo: Todo): TodoOriginMetaResult => {
  const guideline = todo.guideline_ref || ''
  const modelOrEngine = todo.creator_name || 'Engine'
  return {
    type: 'audit',
    label: '🔍 Optimasi Audit',
    sublabel: guideline ? `${guideline}` : modelOrEngine,
    badgeClass: 'bg-sky-100 text-sky-900 dark:bg-sky-950/60 dark:text-sky-300 border-sky-300 dark:border-sky-800 font-medium',
    icon: 'i-lucide-shield-alert',
    title: `Temuan dari hasil audit kualitas/arsitektur: ${guideline || modelOrEngine}`
  }
}

export const createPmOriginMeta = (_todo: Todo): TodoOriginMetaResult => ({
  type: 'pm',
  label: '📋 Project Manager',
  sublabel: 'PM Planner',
  badgeClass: 'bg-indigo-100 text-indigo-900 dark:bg-indigo-950/60 dark:text-indigo-300 border-indigo-300 dark:border-indigo-800 font-medium',
  icon: 'i-lucide-clipboard-list',
  title: 'Dirumuskan oleh AI Project Manager / Product Planner'
})

export const createAiSuggestionOriginMeta = (todo: Todo): TodoOriginMetaResult => ({
  type: 'ai_suggestion',
  label: '💡 Rekomendasi AI',
  sublabel: todo.creator_name || 'AI Advisor',
  badgeClass: 'bg-fuchsia-100 text-fuchsia-900 dark:bg-fuchsia-950/60 dark:text-fuchsia-300 border-fuchsia-300 dark:border-fuchsia-800 font-medium',
  icon: 'i-lucide-sparkles',
  title: 'Saran perbaikan/fitur dari Vibeasik AI Advisor'
})

export const createAgentOriginMeta = (todo: Todo): TodoOriginMetaResult => ({
  type: 'agent',
  label: '💻 Agent Coding',
  sublabel: todo.creator_name || 'AI Agent',
  badgeClass: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-950/60 dark:text-emerald-300 border-emerald-300 dark:border-emerald-800 font-medium',
  icon: 'i-lucide-bot',
  title: `Dibuat oleh AI Coding Agent: ${todo.creator_name}`
})

export const createHumanOriginMeta = (todo: Todo): TodoOriginMetaResult => ({
  type: 'human',
  label: '👤 Developer (Manual)',
  sublabel: todo.creator_name || 'Developer',
  badgeClass: 'bg-neutral-100 text-neutral-800 dark:bg-neutral-800 dark:text-neutral-300 border-neutral-300 dark:border-neutral-700 font-medium',
  icon: 'i-lucide-user',
  title: `Dibuat manual oleh developer: ${todo.creator_name || 'developer'}`
})

export const todoOriginMeta = (todo: Todo): TodoOriginMetaResult => {
  if (isExecutiveOrigin(todo)) return createExecutiveOriginMeta(todo)
  if (isAuditOrigin(todo)) return createAuditOriginMeta(todo)
  if (isPmOrigin(todo)) return createPmOriginMeta(todo)
  if (isAiSuggestionOrigin(todo)) return createAiSuggestionOriginMeta(todo)
  if (isAgentOrigin(todo)) return createAgentOriginMeta(todo)
  return createHumanOriginMeta(todo)
}
