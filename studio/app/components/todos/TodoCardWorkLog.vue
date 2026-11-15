<script setup lang="ts">
import type { Todo, TodoWorkLog } from '~/types'

/**
 * Riwayat pengerjaan todo: kapan mulai, selesai, atau dibuka lagi, dan oleh agent apa. Dimuat saat dibuka
 * saja agar daftar todo tidak menambah satu permintaan per kartu.
 */
const props = defineProps<{ todo: Todo }>()

const { getTodoWorkLogs } = useApi()
const { formatDateTime } = useFormatters()
const { modelName } = useModelNames()

const open = ref(false)
const loading = ref(false)
const loadError = ref<string | null>(null)
const logs = ref<TodoWorkLog[] | null>(null)

const eventMeta: Record<string, { label: string, icon: string, dot: string }> = {
  started: { label: 'Mulai dikerjakan', icon: 'i-lucide-play', dot: 'bg-sky-500' },
  completed: { label: 'Selesai', icon: 'i-lucide-check', dot: 'bg-emerald-500' },
  reopened: { label: 'Dibuka lagi', icon: 'i-lucide-rotate-ccw', dot: 'bg-amber-500' }
}
const viaLabel: Record<string, string> = {
  mcp: 'lewat MCP',
  studio: 'lewat Studio',
  orchestrator: 'dikirim engine',
  review: 'hasil pemeriksaan'
}
const metaOf = (event: string) => eventMeta[event] ?? { label: event, icon: 'i-lucide-circle', dot: 'bg-neutral-400' }

const load = async () => {
  loading.value = true
  loadError.value = null
  try {
    logs.value = await getTodoWorkLogs(props.todo.id)
  } catch (err) {
    const e = err as { data?: { error?: string }, message?: string }
    loadError.value = e?.data?.error || e?.message || 'Gagal menghubungi engine'
  } finally {
    loading.value = false
  }
}

const toggle = () => {
  open.value = !open.value
  if (open.value) load()
}

// Status berubah selagi terbuka (mis. agent baru menyelesaikan todo) → riwayat dimuat ulang.
watch(() => props.todo.status, () => {
  if (open.value) load()
})
</script>

<template>
  <div class="text-xs">
    <button
      type="button"
      class="inline-flex items-center gap-1 font-medium text-neutral-600 dark:text-neutral-400 hover:underline cursor-pointer"
      title="Klik untuk melihat kapan dan oleh agent apa todo ini dikerjakan"
      :aria-expanded="open"
      @click="toggle"
    >
      <UIcon :name="open ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'" class="size-3.5" />
      <UIcon name="i-lucide-history" class="size-3.5" />
      Riwayat pengerjaan<template v-if="logs"> ({{ logs.length }})</template>
    </button>

    <div
      v-if="open"
      class="mt-2 rounded-lg border border-neutral-200 dark:border-white/[0.08] bg-neutral-50/70 dark:bg-neutral-800/40 p-2.5"
    >
      <p v-if="loading && !logs" class="text-neutral-500 dark:text-neutral-400">
        Memuat riwayat pengerjaan…
      </p>
      <UAlert
        v-else-if="loadError"
        color="error"
        variant="subtle"
        :title="loadError"
        description="Pastikan engine berjalan, lalu buka lagi riwayat ini."
      />
      <p v-else-if="!logs?.length" class="text-neutral-500 dark:text-neutral-400">
        Belum ada catatan. Riwayat mulai dicatat sejak todo ditandai dikerjakan atau selesai.
      </p>
      <ol v-else class="space-y-1.5">
        <li v-for="log in logs" :key="log.id" class="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          <span class="size-1.5 rounded-full shrink-0" :class="metaOf(log.event).dot" />
          <span class="tabular-nums text-neutral-500 dark:text-neutral-400">{{ formatDateTime(log.created_at) }}</span>
          <span class="font-medium text-neutral-800 dark:text-neutral-200">{{ metaOf(log.event).label }}</span>
          <span v-if="log.agent" class="inline-flex items-center gap-1 text-neutral-700 dark:text-neutral-300">
            <UIcon name="i-lucide-bot" class="size-3.5 text-neutral-400" />
            {{ log.agent }}
          </span>
          <span v-if="log.model" class="font-mono text-neutral-500 dark:text-neutral-400">{{ modelName(log.model) }}</span>
          <span v-if="log.employee_name" class="text-neutral-500 dark:text-neutral-400">· pegawai {{ log.employee_name }}</span>
          <span v-if="log.via && viaLabel[log.via]" class="text-neutral-400 dark:text-neutral-500">· {{ viaLabel[log.via] }}</span>
        </li>
      </ol>
    </div>
  </div>
</template>
