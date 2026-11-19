<script setup lang="ts">
/**
 * Warna merek proyek: utama, pendamping, aksen. Bila pemilik belum memilih, engine mengusulkan warna dari kode
 * aplikasi (Nuxt UI, variabel CSS, Tailwind, tema Flutter, Android). Dipakai sebagai tema PDF Dokumen Proyek.
 */
import type { BrandColors } from '~/composables/useProjectBrand'

const props = defineProps<{ projectId: number }>()

const brand = useProjectBrand()
const toast = useToast()

const form = reactive<{ primary: string, secondary: string, accent: string }>({ primary: '', secondary: '', accent: '' })
const evidence = ref<string[]>([])
const source = ref<BrandColors['source']>('')
const saved = ref(false)
const loading = ref(false)
const busy = ref(false)
const showEvidence = ref(false)

const HEX = /^#[0-9a-f]{6}$/i
const fill = (c: BrandColors, isSaved: boolean) => {
  form.primary = c.primary || ''
  form.secondary = c.secondary || ''
  form.accent = c.accent || ''
  evidence.value = c.evidence || []
  source.value = c.source || ''
  saved.value = isSaved
}

const load = async (detect = false) => {
  loading.value = true
  try {
    const r = await brand.get(props.projectId, detect)
    fill(r.colors, detect ? false : r.saved)
  } catch (err: any) {
    toast.add({ title: 'Warna merek tidak dapat dimuat', description: err?.data?.error || err?.message, color: 'error', icon: 'i-lucide-alert-circle' })
  } finally {
    loading.value = false
  }
}
onMounted(() => load())
watch(() => props.projectId, () => load())

const valid = computed(() => HEX.test(form.primary) && (!form.secondary || HEX.test(form.secondary)) && (!form.accent || HEX.test(form.accent)))

const save = async () => {
  if (!valid.value) return
  busy.value = true
  try {
    const r = await brand.save(props.projectId, {
      primary: form.primary.toLowerCase(),
      secondary: form.secondary.toLowerCase() || undefined,
      accent: form.accent.toLowerCase() || undefined,
      evidence: evidence.value
    })
    fill(r.colors, true)
    toast.add({ title: 'Warna merek disimpan', icon: 'i-lucide-check-circle', color: 'success' })
  } catch (err: any) {
    toast.add({ title: 'Gagal menyimpan warna', description: err?.data?.error || err?.message, color: 'error', icon: 'i-lucide-alert-circle' })
  } finally {
    busy.value = false
  }
}

const reset = async () => {
  busy.value = true
  try {
    const r = await brand.reset(props.projectId)
    fill(r.colors, false)
  } finally {
    busy.value = false
  }
}

// Pratinjau tema dokumen: teks di atas bidang warna memakai putih atau tinta gelap, mana yang lebih kontras.
const lum = (hex: string) => {
  if (!HEX.test(hex)) return 1
  const ch = [1, 3, 5].map((i) => {
    const v = Number.parseInt(hex.slice(i, i + 2), 16) / 255
    return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * ch[0]! + 0.7152 * ch[1]! + 0.0722 * ch[2]!
}
const onColor = (hex: string) => (1.05 / (lum(hex) + 0.05) >= (lum(hex) + 0.05) / (lum('#111827') + 0.05) ? '#ffffff' : '#111827')
const mixBlack = (hex: string, t: number) => {
  if (!HEX.test(hex)) return '#0b2a45'
  return `#${[1, 3, 5].map(i => Math.round(Number.parseInt(hex.slice(i, i + 2), 16) * (1 - t)).toString(16).padStart(2, '0')).join('')}`
}
const preview = computed(() => {
  const primary = HEX.test(form.primary) ? form.primary : '#0b4f7c'
  const accent = HEX.test(form.accent) ? form.accent : primary
  // Sama dengan engine: aksen gelap yang kontras dengan warna utama menjadi warna footer bila pendamping kosong.
  const contrast = (a: string, b: string) => (Math.max(lum(a), lum(b)) + 0.05) / (Math.min(lum(a), lum(b)) + 0.05)
  const darkAccent = HEX.test(form.accent) && lum(accent) < 0.3 && contrast(accent, primary) >= 1.8
  const secondary = HEX.test(form.secondary) ? form.secondary : darkAccent ? accent : mixBlack(primary, 0.45)
  return { primary, secondary, accent, onPrimary: onColor(primary), onSecondary: onColor(secondary) }
})

const fields = [
  { key: 'primary' as const, label: 'Utama', hint: 'Navbar, tombol utama, sampul & header PDF' },
  { key: 'secondary' as const, label: 'Pendamping', hint: 'Footer / latar gelap (kosong = aksen gelap atau warna utama yang digelapkan)' },
  { key: 'accent' as const, label: 'Aksen', hint: 'Sorotan kecil, garis aksen' }
]

const statusLabel = computed(() => {
  if (saved.value) return { text: 'Tersimpan', color: 'success' as const }
  if (form.primary) return { text: 'Usulan dari kode', color: 'info' as const }
  return { text: 'Belum ada', color: 'neutral' as const }
})
</script>

<template>
  <section class="rounded-xl border border-neutral-200 dark:border-white/10 bg-white dark:bg-[#04111f] p-4 sm:p-5 space-y-4">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <h2 class="text-sm font-bold text-neutral-900 dark:text-white flex items-center gap-2">
          <UIcon name="i-lucide-palette" class="w-4 h-4 text-sky-500" />
          Warna merek
          <UBadge size="sm" variant="subtle" :color="statusLabel.color">
            {{ statusLabel.text }}
          </UBadge>
        </h2>
        <p class="mt-1 text-xs text-neutral-500 dark:text-slate-400">
          Warna tampilan website / aplikasi mobile proyek. Dipakai untuk sampul, header, dan footer PDF Dokumen Proyek.
        </p>
      </div>
      <div class="flex items-center gap-2">
        <UButton size="sm" color="neutral" variant="outline" icon="i-lucide-scan-search" :loading="loading" @click="load(true)">
          Deteksi dari kode
        </UButton>
        <UButton size="sm" color="neutral" icon="i-lucide-save" :loading="busy" :disabled="!valid" @click="save">
          Simpan
        </UButton>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_260px] gap-4">
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <label v-for="f in fields" :key="f.key" class="block space-y-1.5">
          <span class="text-xs font-medium text-neutral-700 dark:text-slate-300">{{ f.label }}</span>
          <span class="flex items-center gap-2">
            <input
              v-model="form[f.key]"
              type="color"
              class="size-9 shrink-0 cursor-pointer rounded-md border border-neutral-200 dark:border-white/10 bg-transparent p-0.5"
              :class="form[f.key] ? '' : 'opacity-25'"
              :aria-label="`Pilih warna ${f.label.toLowerCase()}`"
            >
            <UInput v-model="form[f.key]" size="sm" placeholder="#rrggbb" class="font-mono flex-1 min-w-0" :color="form[f.key] && !HEX.test(form[f.key]) ? 'error' : undefined" />
          </span>
          <span class="block text-[11px] text-neutral-500 dark:text-slate-500">{{ f.hint }}</span>
        </label>
      </div>

      <!-- Pratinjau halaman PDF -->
      <div class="rounded-lg border border-neutral-200 dark:border-white/10 overflow-hidden text-[10px] bg-white" aria-label="Pratinjau halaman PDF">
        <div class="flex items-center justify-between px-3 h-6 font-semibold" :style="{ background: preview.primary, color: preview.onPrimary }">
          <span>Judul proyek</span><span class="font-normal opacity-90">5  Stack teknologi</span>
        </div>
        <div class="h-0.5" :style="{ background: preview.accent }" />
        <div class="px-3 py-2.5 space-y-1.5">
          <div class="flex items-baseline gap-1.5 text-[12px] font-bold text-neutral-900">
            <span :style="{ color: mixBlack(preview.primary, lum(preview.primary) > 0.35 ? 0.35 : 0) }">5</span> Stack teknologi
          </div>
          <div class="h-px bg-neutral-200 relative">
            <div class="absolute left-0 -top-px h-[3px] w-8" :style="{ background: preview.primary }" />
          </div>
          <div class="h-1.5 w-11/12 rounded bg-neutral-200" />
          <div class="h-1.5 w-9/12 rounded bg-neutral-200" />
          <div class="h-4 rounded" :style="{ background: `${preview.primary}18` }" />
        </div>
        <div class="flex items-center justify-between px-3 h-5" :style="{ background: preview.secondary, color: preview.onSecondary }">
          <span>Dokumen teknis</span><span class="font-semibold">Halaman 5 dari 24</span>
        </div>
      </div>
    </div>

    <div v-if="evidence.length || saved" class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
      <UButton v-if="evidence.length" size="xs" color="neutral" variant="ghost" :icon="showEvidence ? 'i-lucide-chevron-up' : 'i-lucide-chevron-down'" class="-ml-2" @click="showEvidence = !showEvidence">
        Asal warna ({{ evidence.length }})
      </UButton>
      <UButton v-if="saved" size="xs" color="neutral" variant="ghost" icon="i-lucide-rotate-ccw" :loading="busy" @click="reset">
        Kembalikan ke deteksi kode
      </UButton>
    </div>
    <ul v-if="showEvidence" class="space-y-0.5 font-mono text-[11px] text-neutral-500 dark:text-slate-400 break-all">
      <li v-for="e in evidence" :key="e">
        {{ e }}
      </li>
    </ul>
    <p v-if="!form.primary && !loading" class="text-xs text-neutral-500">
      Warna merek tidak terbaca dari kode. Pilih warna utama secara manual; PDF memakai biru tua netral selama belum diisi.
    </p>
  </section>
</template>
