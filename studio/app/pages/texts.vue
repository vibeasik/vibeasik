<script setup lang="ts">
import type { TextEntry, TextFacets, TextSource } from '~/types'

definePageMeta({
  fluid: true
})

// Teks Website: website mengirim semua teks yang tampil ke vibeasik (website → vibeasik). Di sini pemilik
// menelusuri teks per halaman/section lalu menjadikan satu teks todo "ubah teks" — mis. agar tulisan bawaan AI
// terasa lebih manusiawi.
const ALL = '__all'
const PAGE_SIZES = [25, 50, 100]

const route = useRoute()
const toast = useToast()
const { activeProjectId, fetchProjects, selectActiveProject } = useActiveProject()
const { listSources, updateSource, deleteSource, getFacets, listEntries } = useTexts()

const sources = ref<TextSource[]>([])
const sourceId = ref<number | null>(null)
const tab = ref<'texts' | 'setup'>('texts')
const entries = ref<TextEntry[]>([])
const total = ref(0)
const facets = ref<TextFacets>({ pages: [], sections: [] })
const loading = ref(false)
const showCreate = ref(false)
const confirmDelete = ref(false)
const autoRefresh = ref(true)
const copiedId = ref<number | null>(null)

const search = ref('')
const pageFilter = ref(ALL)
const sectionFilter = ref(ALL)
const todoFilter = ref<'' | 'has' | 'none'>('')
const page = ref(1)
const perPage = ref(PAGE_SIZES[0]!)

const todoEntry = ref<TextEntry | null>(null)
const showTodo = ref(false)

let timer: ReturnType<typeof setInterval> | null = null
let searchTimer: ReturnType<typeof setTimeout> | null = null

const source = computed(() => sources.value.find(s => s.id === sourceId.value) || null)
const sourceItems = computed(() => sources.value.map(s => ({ label: `${s.name}${s.enabled ? '' : ' (jeda)'}`, value: s.id })))
const pageItems = computed(() => [{ label: 'Semua halaman', value: ALL }, ...facets.value.pages.map(p => ({ label: `${p.name} (${p.count})`, value: p.name }))])
const sectionItems = computed(() => [{ label: 'Semua section', value: ALL }, ...facets.value.sections.map(s => ({ label: `${s.name} (${s.count})`, value: s.name }))])
const rangeStart = computed(() => (total.value ? (page.value - 1) * perPage.value + 1 : 0))
const rangeEnd = computed(() => Math.min(page.value * perPage.value, total.value))
const filtered = computed(() => !!search.value.trim() || pageFilter.value !== ALL || sectionFilter.value !== ALL || !!todoFilter.value)

const connection = computed(() => {
  const s = source.value
  if (!s) return null
  if (!s.last_ingest_at) return { cls: 'text-neutral-500', icon: 'i-lucide-circle-dashed', text: 'Menunggu kiriman pertama dari website — pasang skripnya di tab Pemasangan.' }
  const age = (Date.now() - new Date(s.last_ingest_at).getTime()) / 1000
  const from = s.last_origin ? ` dari ${s.last_origin}` : ''
  if (!s.enabled) return { cls: 'text-neutral-500', icon: 'i-lucide-pause-circle', text: `Dijeda · kiriman terakhir ${fmtAgo(s.last_ingest_at)}${from}` }
  if (age > 3600) return { cls: 'text-amber-600 dark:text-amber-400', icon: 'i-lucide-circle-alert', text: `Kiriman terakhir ${fmtAgo(s.last_ingest_at)}${from} — buka website agar teksnya terkirim lagi.` }
  return { cls: 'text-emerald-600 dark:text-emerald-400', icon: 'i-lucide-circle-check', text: `Tersambung · kiriman terakhir ${fmtAgo(s.last_ingest_at)}${from}` }
})

const fail = (title: string, err: any) => toast.add({ title, description: err?.data?.error || err?.message, icon: 'i-lucide-alert-circle', color: 'error' })

const loadSources = async () => {
  sources.value = await listSources(activeProjectId.value || undefined)
  if (!sources.value.some(s => s.id === sourceId.value)) sourceId.value = sources.value[0]?.id ?? null
}

const loadFacets = async () => {
  if (!sourceId.value) return
  try {
    facets.value = await getFacets(sourceId.value, pageFilter.value === ALL ? undefined : pageFilter.value)
  } catch { /* filter tetap bisa dipakai tanpa facet */ }
}

const loadEntries = async (quiet = false) => {
  if (!sourceId.value) {
    entries.value = []
    total.value = 0
    return
  }
  if (!quiet) loading.value = true
  const id = sourceId.value
  try {
    const res = await listEntries(id, {
      q: search.value.trim(),
      page: pageFilter.value === ALL ? '' : pageFilter.value,
      section: sectionFilter.value === ALL ? '' : sectionFilter.value,
      todo: todoFilter.value,
      limit: perPage.value,
      offset: (page.value - 1) * perPage.value
    })
    if (id !== sourceId.value) return
    entries.value = res.items
    total.value = res.total
  } catch (err: any) {
    if (!quiet) fail('Gagal memuat teks', err)
  } finally {
    loading.value = false
  }
}

const refreshAll = async (quiet = false) => {
  try {
    await loadSources()
  } catch (err: any) {
    if (!quiet) fail('Gagal memuat sumber teks', err)
  }
  await Promise.all([loadEntries(quiet), loadFacets()])
}

const restartTimer = () => {
  if (timer) clearInterval(timer)
  timer = null
  if (autoRefresh.value && import.meta.client) timer = setInterval(() => { if (!showTodo.value && tab.value === 'texts') refreshAll(true) }, 10000)
}

watch(sourceId, () => {
  confirmDelete.value = false
  pageFilter.value = ALL
  sectionFilter.value = ALL
  page.value = 1
  loadEntries()
  loadFacets()
})
watch(pageFilter, () => {
  sectionFilter.value = ALL
  page.value = 1
  loadFacets()
  loadEntries()
})
watch([sectionFilter, todoFilter, perPage], () => {
  page.value = 1
  loadEntries()
})
watch(page, () => loadEntries())
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    loadEntries()
  }, 300)
})
watch(autoRefresh, restartTimer)
watch(activeProjectId, async () => {
  try {
    await loadSources()
  } catch (err: any) {
    fail('Gagal memuat sumber teks', err)
  }
})

const handleCreated = async (src: TextSource) => {
  await loadSources()
  sourceId.value = src.id
  tab.value = 'setup'
}

const patchSource = async (body: { enabled?: boolean }) => {
  if (!source.value) return
  try {
    const updated = await updateSource(source.value.id, body)
    const i = sources.value.findIndex(s => s.id === updated.id)
    if (i >= 0) sources.value[i] = { ...sources.value[i]!, ...updated }
  } catch (err: any) {
    fail('Gagal menyimpan', err)
  }
}

const removeSource = async () => {
  if (!source.value) return
  try {
    await deleteSource(source.value.id)
    toast.add({ title: 'Sumber teks dihapus', icon: 'i-lucide-trash-2', color: 'success' })
    confirmDelete.value = false
    await loadSources()
    await Promise.all([loadEntries(), loadFacets()])
  } catch (err: any) {
    fail('Gagal menghapus', err)
  }
}

const openTodo = (e: TextEntry) => {
  todoEntry.value = e
  showTodo.value = true
}

const onTodoCreated = (entryId: number, todoId: number) => {
  const i = entries.value.findIndex(e => e.id === entryId)
  if (i >= 0) entries.value[i] = { ...entries.value[i]!, todo_id: todoId, todo_status: 'pending' }
  loadEntries(true)
}

const copyText = async (e: TextEntry) => {
  await navigator.clipboard.writeText(e.text)
  copiedId.value = e.id
  setTimeout(() => { if (copiedId.value === e.id) copiedId.value = null }, 1500)
}

const resetFilters = () => {
  search.value = ''
  pageFilter.value = ALL
  sectionFilter.value = ALL
  todoFilter.value = ''
}

const todoFilters = [
  { value: '', label: 'Semua' },
  { value: 'none', label: 'Belum ada todo' },
  { value: 'has', label: 'Sudah ada todo' }
] as const

onMounted(async () => {
  await fetchProjects()
  const queryPId = route.query.project_id ? Number(route.query.project_id) : null
  if (queryPId) selectActiveProject(queryPId)
  try {
    await loadSources()
  } catch (err: any) {
    fail('Teks Website tidak tersedia', err)
  }
  await Promise.all([loadEntries(), loadFacets()])
  if (source.value && !source.value.last_ingest_at) tab.value = 'setup'
  restartTimer()
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
  if (searchTimer) clearTimeout(searchTimer)
})
</script>

<template>
  <div class="space-y-5 w-full max-w-[1400px] mx-auto px-4 sm:px-6 lg:px-8 pt-6 pb-16">
    <AppPageHeader
      title="Teks Website"
      description="Semua teks yang tampil ke pengunjung dikirim website ke sini. Telusuri per halaman & section, lalu jadikan satu teks todo untuk diubah — misalnya agar tulisan bawaan AI terasa lebih manusiawi."
      icon="i-lucide-type"
    >
      <template #actions>
        <UButton size="sm" color="neutral" icon="i-lucide-plus" @click="showCreate = true">
          Hubungkan website
        </UButton>
      </template>
    </AppPageHeader>

    <!-- Belum ada sumber -->
    <section v-if="!sources.length" class="p-12 rounded-xl border border-dashed border-neutral-300 dark:border-neutral-700 text-center space-y-4">
      <div class="size-12 rounded-full bg-sky-50 dark:bg-sky-950/40 text-sky-600 dark:text-sky-400 grid place-items-center mx-auto">
        <UIcon name="i-lucide-type" class="size-6" />
      </div>
      <div class="space-y-1">
        <h2 class="text-base font-bold text-neutral-900 dark:text-white">
          Belum ada website yang terhubung
        </h2>
        <p class="text-xs text-neutral-500 max-w-md mx-auto leading-relaxed">
          Pasang satu skrip kecil di website dev/staging. Skrip mengirim teks yang tampil ke vibeasik, jadi daftar teks selalu mengikuti isi website — tanpa vibeasik mengambil apa pun dari website.
        </p>
      </div>
      <UButton color="primary" icon="i-lucide-plus" @click="showCreate = true">
        Hubungkan website
      </UButton>
    </section>

    <template v-else>
      <!-- Sumber & kontrol -->
      <section class="p-4 bg-white dark:bg-[#04111f] border border-neutral-200 dark:border-white/[0.08] rounded-xl space-y-3">
        <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-3">
          <div class="flex flex-wrap items-center gap-2">
            <USelect :model-value="sourceId ?? undefined" :items="sourceItems" size="sm" class="w-56" icon="i-lucide-globe" @update:model-value="v => sourceId = Number(v)" />
            <span v-if="source?.project_name" class="text-xs text-neutral-500">{{ source.project_name }}<template v-if="source.component_name"> · {{ source.component_name }}</template></span>
          </div>
          <div v-if="source" class="flex flex-wrap items-center gap-2">
            <USwitch v-model="autoRefresh" size="sm" label="Segarkan otomatis" @update:model-value="restartTimer" />
            <UButton size="sm" color="neutral" variant="ghost" :icon="source.enabled ? 'i-lucide-pause' : 'i-lucide-play'" @click="patchSource({ enabled: !source.enabled })">
              {{ source.enabled ? 'Jeda' : 'Lanjutkan' }}
            </UButton>
            <UButton v-if="!confirmDelete" size="sm" color="error" variant="ghost" icon="i-lucide-trash-2" aria-label="Hapus sumber" @click="confirmDelete = true" />
            <template v-else>
              <span class="text-xs text-rose-600 dark:text-rose-400">Hapus sumber & semua teksnya? (todo tetap ada)</span>
              <UButton size="sm" color="error" @click="removeSource">
                Hapus
              </UButton>
              <UButton size="sm" color="neutral" variant="ghost" @click="confirmDelete = false">
                Batal
              </UButton>
            </template>
          </div>
        </div>
        <div v-if="source && connection" class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
          <span class="flex items-center gap-1.5 min-w-0" :class="connection.cls">
            <UIcon :name="connection.icon" class="size-4 shrink-0" /><span class="break-all">{{ connection.text }}</span>
          </span>
          <span class="text-neutral-500">{{ fmtNum(source.texts) }} teks · {{ fmtNum(source.pages) }} halaman</span>
        </div>
      </section>

      <!-- Tab -->
      <div class="flex gap-1 border-b border-neutral-200 dark:border-white/[0.07]">
        <button
          v-for="t in [{ v: 'texts', label: `Teks (${fmtNum(source?.texts || 0)})`, icon: 'i-lucide-list' }, { v: 'setup', label: 'Pemasangan', icon: 'i-lucide-plug' }]"
          :key="t.v"
          type="button"
          class="relative flex items-center gap-1.5 px-3 py-2 text-sm font-medium cursor-pointer"
          :class="tab === t.v ? 'text-neutral-950 dark:text-white' : 'text-neutral-500 hover:text-neutral-800 dark:hover:text-slate-200'"
          @click="tab = t.v as 'texts' | 'setup'"
        >
          <UIcon :name="t.icon" class="size-4" /> {{ t.label }}
          <span v-if="tab === t.v" class="absolute inset-x-2 -bottom-px h-0.5 rounded-full bg-sky-500" />
        </button>
      </div>

      <TextsTextSetupPanel v-if="tab === 'setup' && source" :source="source" @rotated="loadSources" />

      <template v-else>
        <!-- Filter -->
        <section class="flex flex-col lg:flex-row lg:items-center gap-2">
          <UInput v-model="search" size="sm" icon="i-lucide-search" placeholder="Cari teks…" class="lg:w-72" :ui="{ base: 'w-full' }" />
          <USelect v-model="pageFilter" :items="pageItems" size="sm" class="lg:w-56" icon="i-lucide-file-text" title="Filter halaman" />
          <USelect v-model="sectionFilter" :items="sectionItems" size="sm" class="lg:w-56" icon="i-lucide-layout-panel-top" :disabled="pageFilter === ALL" :title="pageFilter === ALL ? 'Pilih halaman dulu untuk memfilter section' : 'Filter section'" />
          <div class="flex flex-wrap gap-1">
            <UButton
              v-for="f in todoFilters"
              :key="f.value"
              size="xs"
              :color="todoFilter === f.value ? 'primary' : 'neutral'"
              :variant="todoFilter === f.value ? 'soft' : 'ghost'"
              class="whitespace-nowrap"
              :title="`Klik untuk memfilter: ${f.label.toLowerCase()}`"
              @click="todoFilter = f.value"
            >
              {{ f.label }}
            </UButton>
          </div>
          <UButton v-if="filtered" size="xs" color="neutral" variant="ghost" icon="i-lucide-x" @click="resetFilters">
            Reset
          </UButton>
        </section>

        <!-- Memuat -->
        <div v-if="loading && !entries.length" class="flex items-center justify-center gap-2 py-16 text-sm text-neutral-500">
          <UIcon name="i-lucide-loader-2" class="size-8 animate-spin" /> Memuat teks website…
        </div>

        <!-- Kosong -->
        <section v-else-if="!entries.length" class="p-12 text-center rounded-xl border border-neutral-200 dark:border-white/[0.08] space-y-3">
          <div class="size-12 rounded-full bg-sky-50 dark:bg-sky-950/40 text-sky-600 dark:text-sky-400 grid place-items-center mx-auto">
            <UIcon :name="filtered ? 'i-lucide-search-x' : 'i-lucide-inbox'" class="size-6" />
          </div>
          <h3 class="text-base font-bold text-neutral-900 dark:text-white">
            {{ filtered ? 'Tidak ada teks yang cocok' : 'Belum ada teks masuk' }}
          </h3>
          <p class="text-xs text-neutral-500 max-w-md mx-auto leading-relaxed">
            {{ filtered ? 'Longgarkan filter atau cari kata lain.' : 'Buka website yang sudah dipasangi skrip; teks tiap halaman terkirim otomatis dalam beberapa detik.' }}
          </p>
          <UButton v-if="filtered" size="sm" color="neutral" variant="soft" @click="resetFilters">
            Reset filter
          </UButton>
          <UButton v-else size="sm" color="neutral" variant="soft" icon="i-lucide-plug" @click="tab = 'setup'">
            Lihat cara memasang
          </UButton>
        </section>

        <!-- Daftar -->
        <section v-else id="text-list" class="space-y-2">
          <article
            v-for="e in entries"
            :key="e.id"
            class="p-4 rounded-xl border border-neutral-200 dark:border-white/[0.08] bg-white dark:bg-[#04111f] flex flex-col md:flex-row md:items-start gap-3"
          >
            <div class="min-w-0 flex-1 space-y-2">
              <p class="text-sm leading-relaxed text-neutral-900 dark:text-white whitespace-pre-wrap break-words">
                {{ e.text }}
              </p>
              <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-neutral-500">
                <span class="flex items-center gap-1 font-mono" title="Halaman"><UIcon name="i-lucide-file-text" class="size-3.5" />{{ e.page }}</span>
                <span class="flex items-center gap-1" title="Section"><UIcon name="i-lucide-layout-panel-top" class="size-3.5" />{{ e.section }}</span>
                <UBadge v-if="e.tag" size="sm" color="neutral" variant="subtle" class="font-mono" :title="e.selector || undefined">
                  {{ e.tag }}
                </UBadge>
                <span :title="`Pertama terlihat ${e.first_seen_at}`">terlihat {{ fmtAgo(e.last_seen_at) }}</span>
              </div>
            </div>
            <div class="flex items-center gap-2 shrink-0">
              <UBadge
                v-if="e.todo_id"
                size="sm"
                :color="todoStatusLabel[e.todo_status || '']?.color || 'neutral'"
                variant="soft"
                icon="i-lucide-list-checks"
                :title="`Todo #${e.todo_id} — klik untuk membuka halaman Todo`"
              >
                <NuxtLink to="/todos">
                  Todo #{{ e.todo_id }} · {{ todoStatusLabel[e.todo_status || '']?.label || e.todo_status || '—' }}
                </NuxtLink>
              </UBadge>
              <UButton size="xs" color="neutral" variant="ghost" :icon="copiedId === e.id ? 'i-lucide-check' : 'i-lucide-copy'" aria-label="Salin teks" title="Salin teks" @click="copyText(e)" />
              <UButton size="sm" color="primary" :variant="e.todo_id ? 'soft' : 'solid'" icon="i-lucide-pencil-line" title="Buka form: tulis perubahan yang diinginkan" @click="openTodo(e)">
                Jadikan todo ubah teks
              </UButton>
            </div>
          </article>

          <AppListPager
            v-model:page="page"
            v-model:per-page="perPage"
            :total="total"
            :per-page-options="PAGE_SIZES"
            :range-start="rangeStart"
            :range-end="rangeEnd"
            noun="teks"
            scroll-target="text-list"
          />
        </section>
      </template>
    </template>

    <TextsTextSourceFormModal v-model:open="showCreate" @created="handleCreated" />
    <TextsTextTodoModal v-model:open="showTodo" :entry="todoEntry" :origin="source?.last_origin" @created="onTodoCreated" />
  </div>
</template>
