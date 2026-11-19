<script setup lang="ts">
/**
 * Dokumen Proyek: dokumen teknis proyek aktif yang dirakit otomatis dari data engine — keterangan, latar belakang,
 * ruang lingkup, stack, arsitektur, API, konfigurasi, cara menjalankan, struktur kode, dan pengujian.
 * Kiri isi dokumen bernomor, kanan daftar isi + lembar teknis. Ekspor PDF memakai cetak browser (tata letak cetak
 * khusus: sampul, tanpa navigasi, mode terang), jadi teksnya tetap bisa dicari & disalin di PDF.
 */
import { DOC_SECTIONS, useProjectDocument } from '~/composables/useProjectDocument'
import type { ProjectDocContent } from '~/utils/projectDocLabels'
import { docDate, docInline } from '~/utils/projectDocLabels'

definePageMeta({ fluid: true })

const route = useRoute()
const { activeProjectId, projects, fetchProjects, selectActiveProject } = useActiveProject()
const doc = reactive(useProjectDocument(activeProjectId))

// ?project=<id> membuka dokumen proyek tertentu (tautan yang dibagikan) dan menjadikannya proyek aktif.
onMounted(async () => {
  await fetchProjects()
  const wanted = Number(route.query.project)
  if (wanted > 0 && wanted !== activeProjectId.value && projects.value.some(p => p.id === wanted)) {
    selectActiveProject(wanted)
    return // watch di useProjectDocument memuat dokumennya
  }
  doc.load()
})

// ---------- Penanda bagian yang sedang dibaca (untuk daftar isi) ----------
const activeId = ref(DOC_SECTIONS[0]!.id)
let observer: IntersectionObserver | null = null
const observeSections = () => {
  observer?.disconnect()
  observer = new IntersectionObserver((entries) => {
    const visible = entries.filter(e => e.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
    if (visible[0]) activeId.value = visible[0].target.id
  }, { rootMargin: '-80px 0px -65% 0px' })
  const ids = DOC_SECTIONS.flatMap(s => [s.id, ...(s.children || []).map(c => c.id)])
  for (const id of ids) {
    const el = document.getElementById(id)
    if (el) observer.observe(el)
  }
}
watch(() => doc.loading, async (busy) => {
  if (busy) return
  await nextTick()
  observeSections()
})
onBeforeUnmount(() => observer?.disconnect())

// ---------- Ekspor PDF ----------
// Cetak selalu dalam mode terang dan judul tab menjadi nama berkas PDF bawaan; keduanya dipulihkan sesudahnya.
const colorMode = useColorMode()
const exporting = ref(false)
const exportPdf = async () => {
  if (exporting.value || !doc.project) return
  exporting.value = true
  const prevMode = colorMode.preference
  const prevTitle = document.title
  const restore = () => {
    colorMode.preference = prevMode
    document.title = prevTitle
    exporting.value = false
    window.removeEventListener('afterprint', restore)
  }
  window.addEventListener('afterprint', restore)
  colorMode.preference = 'light'
  document.title = `Dokumen Teknis - ${doc.project.name} - ${new Date().toISOString().slice(0, 10)}`
  await nextTick()
  // Beri waktu kelas tema terang terpasang sebelum dialog cetak memotret halaman.
  setTimeout(() => window.print(), 250)
}

// ---------- Editor catatan tertulis ----------
const editorOpen = ref(false)
const editorTab = ref('umum')
const openEditor = (tab = 'umum') => {
  editorTab.value = tab
  editorOpen.value = true
}
const toast = useToast()
const saveNotes = async (content: ProjectDocContent) => {
  await doc.saveNotes(content)
  toast.add({ title: 'Catatan dokumen disimpan', icon: 'i-lucide-check-circle', color: 'success' })
}

const sourcesMissing = computed(() => doc.sources.filter(s => s.state !== 'ok').length)
</script>

<template>
  <div class="w-full max-w-[1400px] mx-auto px-4 sm:px-6 lg:px-8 pt-6 pb-16 space-y-6 print:p-0 print:max-w-none">
    <AppPageHeader
      class="print:hidden"
      title="Dokumen Proyek"
      icon="i-lucide-book-text"
      description="Dokumen teknis proyek aktif, disusun otomatis dari profil, katalog rute, konfigurasi, peta kode, dan hasil pengujian. Bisa diekspor ke PDF."
    >
      <template v-if="doc.project" #meta>
        <span class="flex items-center gap-1"><UIcon name="i-lucide-folder" class="size-3.5" />{{ doc.project.name }}</span>
        <span class="flex items-center gap-1"><UIcon name="i-lucide-clock" class="size-3.5" />Disusun {{ docDate(doc.loadedAt) }}</span>
        <span v-if="sourcesMissing" class="rounded-md bg-amber-500/10 px-1.5 text-amber-700 dark:text-amber-400">{{ sourcesMissing }} sumber belum ada</span>
      </template>
      <template #actions>
        <UButton size="sm" color="neutral" variant="outline" icon="i-lucide-refresh-cw" :loading="doc.loading" @click="doc.load()">
          Muat ulang
        </UButton>
        <UButton size="sm" color="neutral" variant="outline" icon="i-lucide-pencil" :disabled="!doc.project" @click="openEditor('umum')">
          Edit catatan
        </UButton>
        <UButton size="sm" color="neutral" icon="i-lucide-file-down" :loading="exporting" :disabled="!doc.project || doc.loading" @click="exportPdf">
          Ekspor PDF
        </UButton>
      </template>
    </AppPageHeader>

    <UAlert
      v-if="!activeProjectId"
      color="neutral"
      variant="subtle"
      icon="i-lucide-folder-search"
      title="Pilih proyek aktif"
      description="Pilih proyek lewat pemilih proyek di navbar atas untuk menyusun dokumennya."
    />

    <UAlert
      v-else-if="doc.error"
      color="error"
      variant="subtle"
      icon="i-lucide-alert-circle"
      title="Dokumen tidak dapat disusun"
      :description="`${doc.error}. Pastikan engine berjalan lalu tekan Muat ulang.`"
    />

    <div v-else-if="doc.loading && !doc.project" class="py-16 flex flex-col items-center gap-3 text-sm text-neutral-500">
      <UIcon name="i-lucide-loader-circle" class="size-8 animate-spin" />
      Menyusun dokumen dari profil, rute, peta kode, dan hasil uji…
    </div>

    <div v-else-if="doc.project" class="grid lg:grid-cols-[minmax(0,1fr)_300px] gap-6 items-start print:block">
      <article
        class="min-w-0 rounded-xl border border-neutral-200 dark:border-white/[0.08] bg-white dark:bg-[#04111f] px-5 py-6 sm:px-8 sm:py-8 print:border-0 print:p-0"
        :class="doc.loading ? 'opacity-60' : ''"
      >
        <!-- Sampul & daftar isi (hanya saat dicetak) -->
        <header class="hidden print:flex flex-col min-h-[90vh] break-after-page">
          <div class="text-xs uppercase tracking-widest text-neutral-500">
            Dokumen teknis
          </div>
          <h1 class="mt-3 text-4xl font-semibold tracking-tight text-black">
            {{ doc.project.name }}
          </h1>
          <!-- eslint-disable-next-line vue/no-v-html -- docInline meng-escape teks lebih dulu -->
          <p class="mt-3 text-base text-neutral-700 max-w-xl" v-html="docInline(doc.profile?.short_description || doc.project.short_description || doc.project.description)" />
          <ol class="mt-12 space-y-1 text-sm">
            <li v-for="s in DOC_SECTIONS" :key="s.id" class="flex gap-3">
              <span class="font-mono w-6 text-neutral-500">{{ s.no }}</span>{{ s.title }}
            </li>
            <li class="flex gap-3">
              <span class="font-mono w-6 text-neutral-500">A</span>Lampiran: lembar teknis
            </li>
          </ol>
          <div class="mt-auto pt-12 text-xs text-neutral-500 space-y-0.5">
            <div v-if="doc.notes.version || doc.notes.authors" class="text-sm text-neutral-700">
              <template v-if="doc.notes.version">
                Versi {{ doc.notes.version }}
              </template><template v-if="doc.notes.version && doc.notes.authors">
                ·
              </template><template v-if="doc.notes.authors">
                {{ doc.notes.authors }}
              </template>
            </div>
            <div>Disusun {{ docDate(doc.loadedAt) }} oleh vibeasik Studio</div>
            <div v-if="doc.project.git_remote" class="font-mono">
              {{ doc.project.git_remote }}<template v-if="doc.project.git_branch">
                @ {{ doc.project.git_branch }}
              </template>
            </div>
          </div>
        </header>

        <ProjectDocsOverview :doc="doc" @edit="openEditor" />
        <ProjectDocsTechnical :doc="doc" />
        <ProjectDocsTesting :doc="doc" />
        <ProjectDocsRisks :doc="doc" />
        <ProjectDocsNotes :doc="doc" @edit="openEditor" />
      </article>

      <ProjectDocsSheet :doc="doc" :active-id="activeId" class="lg:sticky lg:top-20 lg:max-h-[calc(100dvh-6rem)] lg:overflow-y-auto no-scrollbar print:static print:max-h-none print:mt-0" />
    </div>

    <ProjectDocsNotesEditor
      v-model:open="editorOpen"
      v-model:tab="editorTab"
      :content="doc.notes"
      :saving="doc.savingNotes"
      :save="saveNotes"
      :generate="doc.generateNotes"
    />
  </div>
</template>
