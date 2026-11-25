<script setup lang="ts">
import type { TextSDKInfo, TextSource } from '~/types'

// Cara memasang skrip pengirim teks di website (hanya dev/staging): tulis ke komponen atau salin potongan kodenya.
const props = defineProps<{ source: TextSource }>()
const emit = defineEmits<{ (e: 'rotated'): void }>()

const toast = useToast()
const { getSDK, installSDK, rotateKey } = useTexts()

const info = ref<TextSDKInfo | null>(null)
const usageId = ref('html')
const installing = ref(false)
const rotating = ref(false)
const confirmRotate = ref(false)
const showKey = ref(false)

const load = async () => {
  try {
    info.value = await getSDK(props.source.id)
    if (!info.value.usages.some(u => u.id === usageId.value)) usageId.value = info.value.usages[0]?.id || 'html'
  } catch (err: any) {
    toast.add({ title: 'Gagal memuat panduan pemasangan', description: err?.data?.error || err?.message, color: 'error', icon: 'i-lucide-alert-circle' })
  }
}
watch(() => props.source.id, load, { immediate: true })

const usage = computed(() => info.value?.usages.find(u => u.id === usageId.value) || null)
const usageItems = computed(() => (info.value?.usages || []).map(u => ({ label: u.label, value: u.id })))
const mask = (s: string) => (info.value && !showKey.value ? s.replaceAll(info.value.ingest_key, info.value.ingest_key.slice(0, 8) + '••••••••••••') : s)

const copy = async (text: string, label: string) => {
  await navigator.clipboard.writeText(text)
  toast.add({ title: `${label} disalin`, icon: 'i-lucide-clipboard-check', color: 'success' })
}

const install = async (overwrite = false) => {
  installing.value = true
  try {
    const res = await installSDK(props.source.id, overwrite)
    toast.add({
      title: res.written ? 'Skrip ditulis ke komponen' : 'Skrip sudah ada',
      description: res.written ? res.path : `${res.path} tidak ditimpa — pakai "Timpa" untuk memperbarui.`,
      icon: 'i-lucide-file-check',
      color: res.written ? 'success' : 'neutral'
    })
  } catch (err: any) {
    toast.add({ title: 'Gagal menulis skrip', description: err?.data?.error || err?.message, icon: 'i-lucide-alert-circle', color: 'error' })
  } finally {
    installing.value = false
  }
}

const rotate = async () => {
  rotating.value = true
  try {
    await rotateKey(props.source.id)
    confirmRotate.value = false
    await load()
    emit('rotated')
    toast.add({ title: 'Kunci diganti', description: 'Perbarui data-key di website; kunci lama sudah ditolak.', icon: 'i-lucide-key-round', color: 'warning' })
  } catch (err: any) {
    toast.add({ title: 'Gagal mengganti kunci', description: err?.data?.error || err?.message, icon: 'i-lucide-alert-circle', color: 'error' })
  } finally {
    rotating.value = false
  }
}
</script>

<template>
  <div v-if="info" class="space-y-4">
    <div class="flex flex-wrap items-center gap-2 text-xs text-neutral-600 dark:text-neutral-300">
      <span class="px-2 py-1 rounded-lg bg-neutral-100 dark:bg-neutral-800 font-medium">Website + skrip</span>
      <UIcon name="i-lucide-arrow-right" class="size-3.5 text-neutral-400" />
      <span class="px-2 py-1 rounded-lg bg-neutral-100 dark:bg-neutral-800 font-medium">vibeasik <span class="font-mono font-normal text-neutral-500">{{ info.ingest_url }}</span></span>
    </div>

    <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
      <div class="space-y-2 min-w-0">
        <h4 class="text-xs font-bold text-neutral-800 dark:text-neutral-100">
          1 · Taruh skripnya di website
        </h4>
        <p class="text-xs text-neutral-500 leading-relaxed">
          Skrip kecil ini membaca teks yang tampil di tiap halaman (judul, paragraf, tombol, tautan, placeholder, alt) dan mengirimnya ke vibeasik. Halaman yang berubah (SPA, konten dimuat belakangan) ikut dipantau.
        </p>
        <div class="flex flex-wrap gap-2">
          <UButton size="xs" color="primary" icon="i-lucide-file-plus-2" :loading="installing" :disabled="!info.component_dir" @click="install(false)">
            Tulis ke {{ info.component_name || 'komponen' }} ({{ info.script_path }})
          </UButton>
          <UButton v-if="info.component_dir" size="xs" color="neutral" variant="ghost" icon="i-lucide-refresh-cw" :loading="installing" @click="install(true)">
            Timpa
          </UButton>
        </div>
        <p v-if="!info.component_dir" class="text-xs text-neutral-500">
          Sumber belum terhubung ke komponen berfolder di laptop ini — salin skripnya dari repo vibeasik (<span class="font-mono">pkg/texting/sdk/vibeasik-texts.js</span>) ke folder statis website.
        </p>
        <div class="flex items-center justify-between gap-2 pt-1">
          <span class="text-xs font-semibold text-neutral-600 dark:text-neutral-300">Lalu muat di halaman:</span>
          <USelect v-model="usageId" :items="usageItems" size="xs" class="w-60" />
        </div>
        <div v-if="usage" class="relative">
          <pre class="text-xs leading-relaxed font-mono bg-neutral-950 text-neutral-100 rounded-xl p-3 overflow-x-auto">{{ mask(usage.code) }}</pre>
          <UButton size="xs" color="neutral" variant="soft" icon="i-lucide-copy" class="absolute top-2 right-2" aria-label="Salin kode" @click="copy(usage.code, 'Kode')" />
        </div>
      </div>

      <div class="space-y-2 min-w-0">
        <div class="flex items-center justify-between gap-2">
          <h4 class="text-xs font-bold text-neutral-800 dark:text-neutral-100">
            2 · Aturan & keamanan
          </h4>
          <UButton size="xs" color="neutral" variant="ghost" :icon="showKey ? 'i-lucide-eye-off' : 'i-lucide-eye'" :aria-label="showKey ? 'Sembunyikan kunci' : 'Tampilkan kunci'" @click="showKey = !showKey" />
        </div>
        <ul class="text-xs text-neutral-500 space-y-1 leading-relaxed list-disc pl-4">
          <li>Pasang hanya di lingkungan dev/staging. Kuncinya terlihat di browser, tetapi hanya bisa mengirim teks — tidak bisa membaca apa pun dari vibeasik.</li>
          <li>Yang dikirim: teks, path halaman (tanpa query/hash), nama section, selector pendek. Nilai input/textarea, cookie, header, dan storage tidak pernah dikirim.</li>
          <li>Tandai bagian yang tidak boleh dibaca (data pengguna, nama pelanggan) dengan atribut <span class="font-mono">data-vibeasik-ignore</span>.</li>
          <li>Beri nama section sendiri dengan <span class="font-mono">data-vibeasik-section="Hero"</span> pada elemen pembungkus; tanpa itu dipakai header/footer/section + judulnya.</li>
          <li>Website harus bisa menjangkau engine ini (localhost atau LAN). Dari halaman https ke engine http lintas jaringan, browser memblokir kiriman.</li>
        </ul>
        <pre class="text-xs leading-relaxed font-mono bg-neutral-950 text-neutral-100 rounded-xl p-3 overflow-x-auto whitespace-pre-wrap break-all">{{ mask(info.curl_test) }}</pre>
        <div class="flex flex-wrap gap-2 pt-1">
          <UButton size="xs" color="neutral" variant="outline" icon="i-lucide-terminal" @click="copy(info.curl_test, 'Perintah uji curl')">
            Salin uji curl
          </UButton>
          <UButton v-if="!confirmRotate" size="xs" color="warning" variant="ghost" icon="i-lucide-key-round" @click="confirmRotate = true">
            Ganti kunci
          </UButton>
          <template v-else>
            <span class="text-xs text-amber-700 dark:text-amber-400 self-center">Kunci lama langsung ditolak. Lanjut?</span>
            <UButton size="xs" color="warning" :loading="rotating" @click="rotate">
              Ya, ganti
            </UButton>
            <UButton size="xs" color="neutral" variant="ghost" @click="confirmRotate = false">
              Batal
            </UButton>
          </template>
        </div>
      </div>
    </div>
  </div>
  <div v-else class="text-xs text-neutral-500 flex items-center gap-2">
    <UIcon name="i-lucide-loader-2" class="size-4 animate-spin" /> Memuat panduan…
  </div>
</template>
