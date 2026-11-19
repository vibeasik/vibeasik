<script setup lang="ts">
/**
 * Editor bagian Dokumen Proyek yang ditulis tangan: identitas dokumen, di luar lingkup, kebutuhan non-fungsional,
 * keputusan teknis (ADR), glosarium, dan riwayat versi. Disimpan sekaligus (PUT mengganti seluruh isi).
 */
import type { ProjectDocContent } from '~/utils/projectDocLabels'
import { DOC_ADR_STATUS, emptyDocContent } from '~/utils/projectDocLabels'

const props = defineProps<{
  content: ProjectDocContent
  saving: boolean
  save: (c: ProjectDocContent) => Promise<void>
  generate?: () => Promise<{ content: ProjectDocContent, model: string }>
}>()
const open = defineModel<boolean>('open', { default: false })
const tab = defineModel<string>('tab', { default: 'umum' })

const TABS = [
  { value: 'umum', label: 'Umum', icon: 'i-lucide-file-text' },
  { value: 'lingkup', label: 'Lingkup & mutu', icon: 'i-lucide-target' },
  { value: 'adr', label: 'Keputusan', icon: 'i-lucide-git-branch' },
  { value: 'glosarium', label: 'Glosarium', icon: 'i-lucide-book-a' },
  { value: 'riwayat', label: 'Riwayat', icon: 'i-lucide-history' }
]
const ADR_OPTIONS = Object.entries(DOC_ADR_STATUS).map(([value, v]) => ({ value, label: v.label }))

const draft = ref<ProjectDocContent>(emptyDocContent())
const error = ref('')
// Tanggal lokal (bukan UTC) agar entri dini hari tidak tercatat tanggal kemarin.
const today = () => new Date().toLocaleDateString('sv-SE')

// Salinan kerja dibuat tiap kali editor dibuka, jadi "Batal" tidak mengubah dokumen.
watch(open, (v) => {
  if (!v) return
  draft.value = JSON.parse(JSON.stringify({ ...emptyDocContent(), ...props.content }))
  error.value = ''
  aiNote.value = ''
}, { immediate: true })

// Isi otomatis: model aktif di navbar menyusun draf dari bukti proyek; hasilnya hanya masuk ke salinan kerja.
const { selectedModel, modelLabel } = useLLMState()
const activeModel = computed(() => selectedModel.value ? modelLabel(selectedModel.value) : '')
const generating = ref(false)
const aiNote = ref('')

const key = (v: string) => v.trim().toLowerCase()
// Gabungkan daftar tanpa duplikat (berdasarkan judul/istilah); mengembalikan jumlah butir baru.
function mergeList<T>(into: T[], add: T[], keyOf: (x: T) => string) {
  const seen = new Set(into.map(x => key(keyOf(x))))
  let n = 0
  for (const x of add) {
    const k = key(keyOf(x))
    if (!k || seen.has(k)) continue
    seen.add(k)
    into.push(x)
    n++
  }
  return n
}

const autofill = async () => {
  if (!props.generate) return
  generating.value = true
  error.value = ''
  aiNote.value = ''
  try {
    const { content: g, model } = await props.generate()
    const d = draft.value
    let added = 0
    // Latar belakang tulisan pemilik tidak ditimpa: draf hanya mengisi yang masih kosong.
    if (g.background && !d.background.trim()) {
      d.background = g.background
      added++
    }
    added += mergeList(d.out_of_scope, g.out_of_scope || [], x => x)
    added += mergeList(d.non_functional, g.non_functional || [], x => x.name)
    added += mergeList(d.decisions, (g.decisions || []).map(x => ({ ...x, date: x.date || today() })), x => x.title)
    added += mergeList(d.glossary, g.glossary || [], x => x.term)
    aiNote.value = added
      ? `${added} isian baru dari ${modelLabel(model) || 'LLM'}. Periksa tiap tab, lalu Simpan.`
      : `${modelLabel(model) || 'LLM'} tidak menemukan isian baru di luar yang sudah ada.`
  } catch (err: any) {
    error.value = err?.statusCode === 402 || err?.status === 402
      ? 'Isi otomatis dengan AI butuh paket Pro (atau proyek ini belum masuk pengawasan penuh).'
      : err?.data?.error || err?.message || 'LLM gagal menyusun draf'
  } finally {
    generating.value = false
  }
}

const addDecision = () => draft.value.decisions.push({ title: '', date: today(), status: 'proposed', context: '', decision: '', consequences: '' })
const addChange = () => draft.value.changelog.unshift({ version: draft.value.version || '', date: today(), changes: '' })

const submit = async () => {
  error.value = ''
  try {
    await props.save(draft.value)
    open.value = false
  } catch (err: any) {
    error.value = err?.data?.error || err?.message || 'Gagal menyimpan'
  }
}

const LABEL = 'block text-xs font-medium text-neutral-600 dark:text-slate-400 mb-1'
const CARD = 'rounded-xl border border-neutral-200 dark:border-white/[0.08] p-3 space-y-2'
</script>

<template>
  <USlideover
    v-model:open="open"
    title="Edit catatan dokumen"
    description="Bagian yang ditulis sendiri: tidak bisa dirakit otomatis dari kode."
    :ui="{ content: 'max-w-2xl' }"
  >
    <template #body>
      <div class="space-y-4 text-sm">
        <div v-if="generate" class="flex flex-wrap items-center gap-2 rounded-xl border border-neutral-200 dark:border-white/[0.08] px-3 py-2">
          <UIcon name="i-lucide-sparkles" class="size-4 text-neutral-500" />
          <p class="flex-1 min-w-0 text-xs text-neutral-600 dark:text-slate-400">
            Susun draf latar belakang, lingkup, mutu, keputusan, dan glosarium dari kode proyek<template v-if="activeModel">
              dengan <span class="font-medium text-neutral-900 dark:text-white">{{ activeModel }}</span>
            </template>.
          </p>
          <UButton size="xs" color="neutral" icon="i-lucide-sparkles" :loading="generating" :disabled="saving" @click="autofill">
            {{ generating ? 'Menyusun…' : 'Isi dengan AI' }}
          </UButton>
        </div>
        <UAlert v-if="aiNote" color="info" variant="subtle" icon="i-lucide-info" :description="aiNote" />

        <div class="flex gap-1 overflow-x-auto no-scrollbar pb-3 border-b border-neutral-200 dark:border-white/[0.07]">
          <button
            v-for="t in TABS"
            :key="t.value"
            type="button"
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium whitespace-nowrap"
            :class="tab === t.value ? 'bg-neutral-900 text-white dark:bg-white dark:text-neutral-900' : 'text-neutral-600 dark:text-slate-400 hover:bg-neutral-100 dark:hover:bg-white/[0.05]'"
            @click="tab = t.value"
          >
            <UIcon :name="t.icon" class="size-3.5" />{{ t.label }}
          </button>
        </div>

        <!-- Umum -->
        <div v-if="tab === 'umum'" class="space-y-3">
          <div class="grid sm:grid-cols-2 gap-3">
            <label><span :class="LABEL">Versi dokumen</span><UInput v-model="draft.version" size="sm" placeholder="mis. 1.0" class="w-full" /></label>
            <label><span :class="LABEL">Penyusun</span><UInput v-model="draft.authors" size="sm" placeholder="Nama penyusun / tim" class="w-full" /></label>
          </div>
          <label class="block">
            <span :class="LABEL">Tambahan latar belakang</span>
            <UTextarea v-model="draft.background" :rows="6" autoresize placeholder="Konteks bisnis, sejarah sistem lama, alasan proyek dimulai…" class="w-full" />
          </label>
        </div>

        <!-- Lingkup & mutu -->
        <div v-else-if="tab === 'lingkup'" class="space-y-5">
          <div class="space-y-2">
            <div class="flex items-center justify-between">
              <span class="text-xs font-semibold">Di luar lingkup</span>
              <UButton size="xs" color="neutral" variant="outline" icon="i-lucide-plus" @click="draft.out_of_scope.push('')">
                Tambah
              </UButton>
            </div>
            <p v-if="!draft.out_of_scope.length" class="text-xs text-neutral-500">
              Hal yang sengaja tidak dikerjakan, agar batas proyek jelas.
            </p>
            <div v-for="(_, i) in draft.out_of_scope" :key="i" class="flex gap-2">
              <UInput v-model="draft.out_of_scope[i]" size="sm" class="flex-1" placeholder="mis. Aplikasi mobile native" />
              <UButton size="sm" color="neutral" variant="ghost" icon="i-lucide-trash-2" title="Hapus butir" @click="draft.out_of_scope.splice(i, 1)" />
            </div>
          </div>
          <div class="space-y-2">
            <div class="flex items-center justify-between">
              <span class="text-xs font-semibold">Kebutuhan non-fungsional</span>
              <UButton size="xs" color="neutral" variant="outline" icon="i-lucide-plus" @click="draft.non_functional.push({ name: '', target: '' })">
                Tambah
              </UButton>
            </div>
            <p v-if="!draft.non_functional.length" class="text-xs text-neutral-500">
              Target mutu terukur: performa, ketersediaan, keamanan, kapasitas.
            </p>
            <div v-for="(r, i) in draft.non_functional" :key="i" class="flex gap-2">
              <UInput v-model="r.name" size="sm" class="w-40" placeholder="Performa" />
              <UInput v-model="r.target" size="sm" class="flex-1" placeholder="p95 API < 300 ms pada 100 pengguna" />
              <UButton size="sm" color="neutral" variant="ghost" icon="i-lucide-trash-2" title="Hapus butir" @click="draft.non_functional.splice(i, 1)" />
            </div>
          </div>
        </div>

        <!-- ADR -->
        <div v-else-if="tab === 'adr'" class="space-y-3">
          <div class="flex items-center justify-between">
            <p class="text-xs text-neutral-500">
              Satu keputusan = konteks → keputusan → akibat. Keputusan lama tidak dihapus; tandai "Digantikan".
            </p>
            <UButton size="xs" color="neutral" variant="outline" icon="i-lucide-plus" @click="addDecision">
              Tambah keputusan
            </UButton>
          </div>
          <div v-for="(d, i) in draft.decisions" :key="i" :class="CARD">
            <div class="flex gap-2">
              <UInput v-model="d.title" size="sm" class="flex-1" placeholder="Judul keputusan, mis. Pakai MySQL" />
              <UInput v-model="d.date" size="sm" type="date" class="w-36" />
              <USelect v-model="d.status" :items="ADR_OPTIONS" size="sm" class="w-36" />
              <UButton size="sm" color="neutral" variant="ghost" icon="i-lucide-trash-2" title="Hapus keputusan" @click="draft.decisions.splice(i, 1)" />
            </div>
            <UTextarea v-model="d.context" :rows="2" autoresize placeholder="Konteks: masalah dan batasan yang memaksa keputusan" class="w-full" />
            <UTextarea v-model="d.decision" :rows="2" autoresize placeholder="Keputusan: apa yang dipilih" class="w-full" />
            <UTextarea v-model="d.consequences" :rows="2" autoresize placeholder="Akibat: untung, rugi, dan hal yang perlu diwaspadai" class="w-full" />
          </div>
        </div>

        <!-- Glosarium -->
        <div v-else-if="tab === 'glosarium'" class="space-y-2">
          <div class="flex items-center justify-between">
            <p class="text-xs text-neutral-500">
              Istilah bisnis & teknis yang perlu dipahami pembaca.
            </p>
            <UButton size="xs" color="neutral" variant="outline" icon="i-lucide-plus" @click="draft.glossary.push({ term: '', definition: '' })">
              Tambah istilah
            </UButton>
          </div>
          <div v-for="(g, i) in draft.glossary" :key="i" class="flex gap-2">
            <UInput v-model="g.term" size="sm" class="w-40" placeholder="Istilah" />
            <UInput v-model="g.definition" size="sm" class="flex-1" placeholder="Arti" />
            <UButton size="sm" color="neutral" variant="ghost" icon="i-lucide-trash-2" title="Hapus istilah" @click="draft.glossary.splice(i, 1)" />
          </div>
        </div>

        <!-- Riwayat -->
        <div v-else class="space-y-3">
          <div class="flex items-center justify-between">
            <p class="text-xs text-neutral-500">
              Catat perubahan tiap kali dokumen diserahkan ulang (terbaru di atas).
            </p>
            <UButton size="xs" color="neutral" variant="outline" icon="i-lucide-plus" @click="addChange">
              Tambah versi
            </UButton>
          </div>
          <div v-for="(ch, i) in draft.changelog" :key="i" :class="CARD">
            <div class="flex gap-2">
              <UInput v-model="ch.version" size="sm" class="w-28" placeholder="Versi" />
              <UInput v-model="ch.date" size="sm" type="date" class="w-36" />
              <span class="flex-1" />
              <UButton size="sm" color="neutral" variant="ghost" icon="i-lucide-trash-2" title="Hapus versi" @click="draft.changelog.splice(i, 1)" />
            </div>
            <UTextarea v-model="ch.changes" :rows="2" autoresize placeholder="Apa yang berubah" class="w-full" />
          </div>
        </div>

        <UAlert v-if="error" color="error" variant="subtle" icon="i-lucide-alert-circle" :description="error" />
      </div>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton size="sm" color="neutral" variant="outline" @click="open = false">
          Batal
        </UButton>
        <UButton size="sm" color="neutral" icon="i-lucide-save" :loading="saving" @click="submit">
          Simpan
        </UButton>
      </div>
    </template>
  </USlideover>
</template>
