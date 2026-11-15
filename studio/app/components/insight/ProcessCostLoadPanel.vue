<script setup lang="ts">
import type { LoadProjection, LoadScenario } from '~/types'
import type { StatItem } from '~/components/AppStatStrip.vue'
import { COST_TRIGGERS, LOAD_SIGNALS, LOAD_STATUS, ROUTE_OPS, costScaleInfo, costTriggerInfo, fmtCount, fmtMs, fmtTicks } from '~/utils/processCost'

// Proyeksi beban: rumus biaya tiap rute diterjemahkan menjadi beban server pada sekian pengguna aktif dan sekian
// baris data. Semua asumsi bisa diubah; hasilnya perkiraan orde besaran, bukan pengukuran.
const props = defineProps<{ projectId: number, reloadKey?: string }>()

const { getProcessCostProjection } = useApi()
const STORE = 'vibeasik.processCost.loadForm'
const form = reactive({ users: '10, 100, 1000, 10000', rows: '1000, 100000, 1000000', req_per_user_min: 6, cores: 2, db_conns: 20, tick_ns: 50, query_ms: 2, small_items: 20, budget_ms: 300 })
const more = ref(false)
const data = shallowRef<LoadProjection | null>(null)
const loading = ref(false)
const failed = ref('')
let ctl: AbortController | null = null

const load = async () => {
  ctl?.abort()
  const mine = ctl = new AbortController()
  loading.value = true
  failed.value = ''
  try {
    const res = await getProcessCostProjection(props.projectId, { ...form, users: form.users.replace(/\s/g, ''), rows: form.rows.replace(/\s/g, '') }, mine.signal)
    if (ctl === mine) data.value = res
    try { localStorage.setItem(STORE, JSON.stringify(form)) } catch { /* penyimpanan lokal tidak tersedia */ }
  } catch (err: any) {
    if (mine.signal.aborted) return
    failed.value = err?.data?.error || err?.message || 'Gagal menghitung proyeksi beban'
  } finally {
    if (ctl === mine) loading.value = false
  }
}
onMounted(() => {
  try { Object.assign(form, JSON.parse(localStorage.getItem(STORE) || '{}')) } catch { /* abaikan isi yang rusak */ }
  load()
})
watch(() => [props.projectId, props.reloadKey], load)
onBeforeUnmount(() => ctl?.abort())

const BOTTLENECK = { 'cpu': 'CPU', 'db': 'database', '': '' }
const limits = computed<StatItem[]>(() => (data.value?.limits || []).map((l) => {
  const low = l.max_users < (data.value?.params.users[0] || 0)
  return {
    label: `Kapasitas pada ${fmtTicks(l.rows)} baris`,
    value: `±${fmtCount(l.max_users)}`,
    hint: `pengguna aktif · penentu: ${BOTTLENECK[l.bottleneck]}`,
    title: `Satu server menampung ±${fmtCount(l.max_users)} pengguna aktif pada ${fmtTicks(l.rows)} baris data (CPU: ${fmtCount(l.by_cpu)}, database: ${fmtCount(l.by_db)})`,
    tone: low ? 'text-rose-600 dark:text-rose-400' : undefined
  }
}))

// Skenario disusun sebagai tabel: baris = pengguna aktif, kolom = baris data.
const grid = computed(() => {
  const d = data.value
  if (!d) return []
  return d.params.users.map(users => ({
    users,
    cells: d.params.rows.map(rows => d.scenarios.find(s => s.users === users && s.rows === rows)).filter((s): s is LoadScenario => !!s)
  }))
})
const cellTitle = (s: LoadScenario) => `${fmtCount(s.rps)} permintaan/dtk · ${fmtCount(s.qps)} query/dtk · CPU ${Math.round(s.cpu_use * 100)}% dari ${data.value?.params.cores} inti · database ${Math.round(s.db_use * 100)}% dari ${data.value?.params.db_conns} koneksi · rata-rata ${fmtMs(s.avg_ms)}`

const layers = computed(() => {
  const t = data.value?.func_triggers || {}
  return Object.keys(COST_TRIGGERS).filter(k => t[k]).map(k => ({ key: k, n: t[k]!, ...costTriggerInfo(k) }))
})
const METHOD_TONE: Record<string, string> = {
  GET: 'text-emerald-700 dark:text-emerald-400', POST: 'text-sky-700 dark:text-sky-400', PUT: 'text-amber-700 dark:text-amber-400',
  PATCH: 'text-amber-700 dark:text-amber-400', DELETE: 'text-rose-700 dark:text-rose-400'
}
const field = 'space-y-1 text-xs font-medium text-neutral-600 dark:text-neutral-300'
const th = 'font-medium px-3 py-2.5'
</script>

<template>
  <section class="min-w-0 rounded-xl border border-neutral-200 dark:border-white/10 bg-white dark:bg-[#04111f] p-5 space-y-4">
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div class="space-y-1">
        <h3 class="text-sm font-semibold text-neutral-900 dark:text-white">
          Proyeksi beban
        </h3>
        <p class="text-sm text-neutral-600 dark:text-neutral-300 max-w-2xl">
          Rumus tiap rute dikalikan pengguna aktif dan ukuran data: berapa inti CPU dan koneksi database yang terpakai, dan kapan satu server jenuh. Perkiraan orde besaran, bukan pengukuran.
        </p>
      </div>
      <span v-if="data" class="text-xs text-neutral-500" :title="data.assumptions[0]">
        Porsi trafik: {{ data.mix === 'traffic' ? `trafik nyata (${fmtTicks(data.mix_requests || 0)} permintaan)` : 'asumsi per jenis operasi' }}
      </span>
    </div>

    <form class="flex flex-wrap items-end gap-3" @submit.prevent="load">
      <label :class="field">
        <span class="block">Pengguna aktif</span>
        <UInput v-model="form.users" size="sm" class="w-44" placeholder="10, 100, 1000" />
      </label>
      <label :class="field">
        <span class="block">Baris data</span>
        <UInput v-model="form.rows" size="sm" class="w-48" placeholder="1000, 100000" />
      </label>
      <label :class="field">
        <span class="block">Permintaan / pengguna / menit</span>
        <UInput v-model.number="form.req_per_user_min" type="number" min="0.1" step="any" size="sm" class="w-28" />
      </label>
      <label :class="field">
        <span class="block">Inti CPU</span>
        <UInput v-model.number="form.cores" type="number" min="0.5" step="any" size="sm" class="w-20" />
      </label>
      <label :class="field">
        <span class="block">Koneksi database</span>
        <UInput v-model.number="form.db_conns" type="number" min="1" size="sm" class="w-24" />
      </label>
      <template v-if="more">
        <label :class="field" title="Harga satu tick (satu pernyataan) dalam nanodetik CPU">
          <span class="block">Tick (ns)</span>
          <UInput v-model.number="form.tick_ns" type="number" min="1" step="any" size="sm" class="w-20" />
        </label>
        <label :class="field" title="Harga satu query berindeks dalam milidetik">
          <span class="block">Query (md)</span>
          <UInput v-model.number="form.query_ms" type="number" min="0.1" step="any" size="sm" class="w-20" />
        </label>
        <label :class="field" title="Banyaknya putaran loop yang tidak memutari baris data: satu halaman, isi permintaan, koleksi di memori">
          <span class="block">Koleksi kecil</span>
          <UInput v-model.number="form.small_items" type="number" min="1" size="sm" class="w-24" />
        </label>
        <label :class="field" title="Di atas ini respons dianggap lambat">
          <span class="block">Batas nyaman (md)</span>
          <UInput v-model.number="form.budget_ms" type="number" min="10" size="sm" class="w-24" />
        </label>
      </template>
      <UButton type="submit" size="sm" color="neutral" :loading="loading" icon="i-lucide-calculator">
        Hitung
      </UButton>
      <UButton size="sm" color="neutral" variant="ghost" :aria-pressed="more" :title="more ? 'Sembunyikan harga tick, query, dan batas' : 'Ubah harga tick, query, ukuran koleksi kecil, dan batas nyaman'" @click="more = !more">
        {{ more ? 'Lebih sedikit' : 'Asumsi lain' }}
      </UButton>
    </form>

    <UAlert v-if="failed" color="error" variant="subtle" :title="failed" description="Periksa isian angka lalu tekan Hitung lagi." />
    <div v-else-if="!data" class="h-40 rounded-xl border border-neutral-200 dark:border-white/[0.08] animate-pulse" />

    <template v-else>
      <AppStatStrip :items="limits" :class="loading ? 'opacity-60' : ''" />

      <div class="overflow-x-auto rounded-xl border border-neutral-200 dark:border-white/[0.08]" :class="loading ? 'opacity-60' : ''">
        <table class="w-full min-w-[560px] text-sm tabular-nums">
          <thead>
            <tr class="text-left text-xs text-neutral-500 border-b border-neutral-200 dark:border-white/[0.08]">
              <th :class="th" class="pl-4">
                Pengguna aktif
              </th>
              <th v-for="r in data.params.rows" :key="r" :class="th">
                {{ fmtTicks(r) }} baris data
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="g in grid" :key="g.users" class="border-b border-neutral-100 dark:border-white/[0.05] last:border-b-0">
              <td class="px-3 py-2.5 pl-4 font-semibold text-neutral-900 dark:text-white">
                {{ fmtTicks(g.users) }}
                <div class="text-xs font-normal text-neutral-500">
                  {{ fmtCount(g.cells[0]?.rps || 0) }} permintaan/dtk
                </div>
              </td>
              <td v-for="s in g.cells" :key="s.rows" class="px-3 py-2.5" :title="cellTitle(s)">
                <span class="inline-flex items-center gap-1.5 text-xs font-semibold" :class="LOAD_STATUS[s.status].text">
                  <span class="size-2 rounded-full" :class="LOAD_STATUS[s.status].dot" />{{ LOAD_STATUS[s.status].label }}<template v-if="s.bottleneck"> · {{ BOTTLENECK[s.bottleneck] }}</template>
                </span>
                <div class="text-xs text-neutral-500">
                  {{ fmtCount(s.cpu_cores) }} inti · {{ fmtCount(s.db_conns) }} koneksi · {{ fmtCount(s.qps) }} q/dtk
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="space-y-2">
        <div class="flex items-baseline justify-between gap-2">
          <h4 class="text-sm font-semibold text-neutral-900 dark:text-white">
            Rute penyumbang beban terbesar
          </h4>
          <span class="text-xs text-neutral-500">
            {{ data.routes.length }} dari {{ data.total_routes }} rute · waktu per permintaan<template v-if="data.loose_routes"> · {{ data.loose_routes }} rute tidak dihitung (perlu diukur)</template>
          </span>
        </div>
        <div class="overflow-x-auto rounded-xl border border-neutral-200 dark:border-white/[0.08]" :class="loading ? 'opacity-60' : ''">
          <table class="w-full min-w-[820px] text-sm">
            <thead>
              <tr class="text-left text-xs text-neutral-500 border-b border-neutral-200 dark:border-white/[0.08]">
                <th :class="th" class="pl-4">
                  Rute
                </th>
                <th :class="th">
                  Operasi · arti n
                </th>
                <th :class="th" class="text-right">
                  Porsi
                </th>
                <th v-for="r in data.params.rows" :key="r" :class="th" class="text-right">
                  {{ fmtTicks(r) }} baris
                </th>
                <th :class="th" class="text-right">
                  Mulai lambat
                </th>
                <th :class="th" class="pr-4">
                  Ciri
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in data.routes" :key="`${r.method} ${r.path}`" class="border-b border-neutral-100 dark:border-white/[0.05] last:border-b-0">
                <td class="px-3 py-2 pl-4 max-w-[380px]">
                  <div class="flex items-baseline gap-2 min-w-0">
                    <span class="font-mono text-xs font-semibold w-12 shrink-0" :class="METHOD_TONE[r.method] || 'text-neutral-500'">{{ r.method }}</span>
                    <span class="font-mono text-xs text-neutral-900 dark:text-white truncate" :title="r.path">{{ r.path }}</span>
                  </div>
                  <div class="pl-14 font-mono text-xs text-neutral-500 truncate" :title="`Terhadap baris data: ${r.data_big_o} · kasus terburuk seragam: ${r.big_o} · Q(n) = ${r.query_formula}`">
                    data {{ r.data_big_o }} · Q(n) = {{ r.query_formula }}
                  </div>
                </td>
                <td class="px-3 py-2 text-xs text-neutral-600 dark:text-neutral-300 whitespace-nowrap" :title="costScaleInfo(r.scale).meaning">
                  {{ ROUTE_OPS[r.op] }} · {{ costScaleInfo(r.scale).label }}
                </td>
                <td class="px-3 py-2 text-right text-xs tabular-nums text-neutral-600 dark:text-neutral-300">
                  {{ (r.share * 100).toFixed(1) }}%
                </td>
                <td
                  v-for="pt in r.points"
                  :key="pt.rows"
                  class="px-3 py-2 text-right font-mono text-xs tabular-nums whitespace-nowrap"
                  :class="r.loose ? 'text-neutral-400 dark:text-neutral-500' : pt.status === 'ok' ? 'text-neutral-900 dark:text-white' : LOAD_STATUS[pt.status].text"
                  :title="r.loose ? 'Lebih dari dua tingkat koleksi kecil bersarang: angka ini batas bawah dan tidak dihitung ke kapasitas' : `CPU ${fmtMs(pt.cpu_ms)} · database ${fmtMs(pt.db_ms)} · ${fmtTicks(pt.queries)} query${pt.status === 'timeout' ? ' · diputus di batas waktu' : ''}`"
                >
                  {{ r.loose ? '≥ ' : pt.status === 'timeout' ? '> ' : '' }}{{ fmtMs(pt.latency_ms) }}
                </td>
                <td class="px-3 py-2 text-right text-xs tabular-nums whitespace-nowrap" :class="r.break_rows ? 'text-amber-700 dark:text-amber-400' : 'text-neutral-300 dark:text-neutral-600'">
                  {{ r.break_rows > 1 ? `±${fmtTicks(r.break_rows)} baris` : r.break_rows === 1 ? 'sejak awal' : '—' }}
                </td>
                <td class="px-3 py-2 pr-4 text-xs text-neutral-600 dark:text-neutral-300" :title="(r.signals || []).map(s => LOAD_SIGNALS[s]?.meaning || s).join('. ')">
                  {{ (r.signals || []).filter(s => s !== 'slow_at_scale').map(s => LOAD_SIGNALS[s]?.label || s).join(' · ') || '—' }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-3">
        <div class="rounded-xl border border-neutral-200 dark:border-white/[0.08] p-4 space-y-2">
          <h4 class="text-xs font-medium text-neutral-500">
            Kapan fungsi dijalankan
          </h4>
          <ul class="space-y-1">
            <li v-for="l in layers" :key="l.key" class="flex items-baseline gap-2 text-sm" :title="l.meaning">
              <span class="w-14 shrink-0 text-right font-semibold tabular-nums text-neutral-900 dark:text-white">{{ fmtTicks(l.n) }}</span>
              <span class="text-neutral-700 dark:text-neutral-200">{{ l.label }}</span>
              <span class="text-xs text-neutral-500 truncate">{{ l.meaning }}</span>
            </li>
          </ul>
        </div>
        <div class="rounded-xl border border-neutral-200 dark:border-white/[0.08] p-4 space-y-2">
          <h4 class="text-xs font-medium text-neutral-500">
            Berpangkat tinggi tetapi tidak menambah beban per pengguna
          </h4>
          <p v-if="!data.startup.length" class="text-sm text-neutral-500">
            Tidak ada fungsi O(n²) ke atas yang hanya berjalan saat start atau sebagai skrip.
          </p>
          <ul v-else class="space-y-1">
            <li v-for="f in data.startup" :key="`${f.file}:${f.line}`" class="flex items-baseline gap-2 text-sm min-w-0">
              <span class="font-mono text-xs font-semibold text-neutral-900 dark:text-white w-16 shrink-0">{{ f.big_o }}</span>
              <span class="font-mono text-xs text-neutral-700 dark:text-neutral-200 truncate" :title="`${f.file}:${f.line}`">{{ f.name }}</span>
              <span class="text-xs text-emerald-700 dark:text-emerald-400 whitespace-nowrap">{{ costTriggerInfo(f.trigger).label }}</span>
            </li>
          </ul>
        </div>
      </div>

      <details class="text-xs text-neutral-500">
        <summary class="cursor-pointer select-none hover:text-neutral-700 dark:hover:text-neutral-300">
          Asumsi di balik angka ini ({{ data.assumptions.length }})
        </summary>
        <ul class="mt-2 space-y-1 list-disc pl-5 max-w-4xl">
          <li v-for="a in data.assumptions" :key="a">
            {{ a }}
          </li>
        </ul>
      </details>
    </template>
  </section>
</template>
