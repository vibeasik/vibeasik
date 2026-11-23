<script setup lang="ts">
import type { TrafficPoint, TrafficReport } from '~/types'

// Halaman: pageview (URL yang paling banyak dibuka). Bot tidak dihitung; id pengunjung berganti tiap hari,
// jadi "pengunjung unik" hanya perkiraan.
const props = defineProps<{ report: TrafficReport }>()
const emit = defineEmits<{ (e: 'setup'): void }>()

const views = computed(() => props.report.totals.page_views || 0)
const visitors = computed(() => props.report.totals.visitors || 0)
const perVisitor = computed(() => visitors.value > 0 ? views.value / visitors.value : 0)
const pages = computed(() => props.report.pages || [])
const multiHost = computed(() => new Set(pages.value.map(p => p.host).filter(Boolean)).size > 1)
const maxShare = computed(() => Math.max(0.0001, ...pages.value.map(p => p.share)))

const series = computed<TrafficPoint[]>(() => (props.report.page_series || []).map(p => ({
  t: p.t, requests: p.views, err_4xx: 0, err_5xx: 0, avg_ms: 0, p95: 0
})))

const deviceLabel: Record<string, string> = { desktop: 'Desktop', mobile: 'Mobile', bot: 'Bot/crawler', unknown: 'Tidak dikenal' }
// Nilai khusus dari backend; host perujuk lain tampil apa adanya (hanya teks, bukan tautan).
const refererLabel: Record<string, string> = { '(langsung)': 'Langsung', '(internal)': 'Navigasi internal' }
const lists = computed(() => [
  { title: 'Perujuk', icon: 'i-lucide-link', items: (props.report.page_referers || []).map(r => ({ ...r, value: refererLabel[r.value] || r.value })), empty: 'Belum ada data perujuk.' },
  { title: 'Perangkat', icon: 'i-lucide-monitor-smartphone', items: (props.report.page_devices || []).map(d => ({ ...d, value: deviceLabel[d.value] || d.value })), empty: 'Belum ada data perangkat.' }
])

const stepLabel = computed(() => props.report.step_sec >= 3600 ? `${props.report.step_sec / 3600} jam` : `${props.report.step_sec / 60} menit`)
// Hanya host yang tampak seperti nama domain yang dibuka; selain itu tidak ada tautan.
const pageUrl = (p: { host: string, path: string }) => p.host && /^[a-z0-9.-]+$/i.test(p.host) ? `https://${p.host}${p.path.startsWith('/') ? p.path : '/' + p.path}` : ''
</script>

<template>
  <div class="space-y-4">
    <!-- Kosong -->
    <div v-if="views === 0" class="p-12 text-center rounded-xl border border-neutral-200 dark:border-white/[0.08]">
      <div class="mx-auto size-12 rounded-full bg-sky-50 dark:bg-sky-500/10 text-sky-600 dark:text-sky-400 flex items-center justify-center">
        <UIcon name="i-lucide-file-text" class="size-6" />
      </div>
      <h3 class="mt-3 text-base font-bold text-neutral-900 dark:text-white">
        Belum ada pageview pada jendela ini
      </h3>
      <p class="mt-1 text-xs text-neutral-500 max-w-md mx-auto leading-relaxed">
        Pasang kit SDK <span class="font-mono">pageview</span>: satu handler di server untuk <span class="font-mono">/_vibeasik/pv</span>, lalu sisipkan
        <span class="font-mono break-all">&lt;script src="/vibeasik-pageview.js" defer&gt;&lt;/script&gt;</span> di halaman. Bot tidak dihitung.
      </p>
      <UButton class="mt-4" size="sm" color="neutral" variant="outline" icon="i-lucide-plug" @click="emit('setup')">
        Buka panel Pemasangan
      </UButton>
    </div>

    <template v-else>
      <!-- KPI: satu bingkai bersambung -->
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-px overflow-hidden rounded-xl border border-neutral-200 dark:border-white/[0.08] bg-neutral-200 dark:bg-white/[0.08]">
        <div class="bg-white dark:bg-[#04111f] px-4 py-3.5">
          <div class="text-xs font-medium text-neutral-500">
            Pageview
          </div>
          <div class="text-2xl font-semibold tracking-tight tabular-nums text-neutral-900 dark:text-white">
            {{ fmtNum(views) }}
          </div>
          <div class="text-xs text-neutral-500 truncate">
            bot tidak dihitung
          </div>
        </div>
        <div class="bg-white dark:bg-[#04111f] px-4 py-3.5">
          <div class="text-xs font-medium text-neutral-500">
            Pengunjung unik ≈
          </div>
          <div class="text-2xl font-semibold tracking-tight tabular-nums text-neutral-900 dark:text-white">
            {{ fmtNum(visitors) }}
          </div>
          <div class="text-xs text-neutral-500">
            id pengunjung berganti tiap hari
          </div>
        </div>
        <div class="bg-white dark:bg-[#04111f] px-4 py-3.5">
          <div class="text-xs font-medium text-neutral-500">
            Halaman per pengunjung
          </div>
          <div class="text-2xl font-semibold tracking-tight tabular-nums text-neutral-900 dark:text-white">
            {{ visitors > 0 ? fmtNum(Math.round(perVisitor * 10) / 10) : '–' }}
          </div>
          <div class="text-xs text-neutral-500 truncate">
            rata-rata pageview ÷ pengunjung
          </div>
        </div>
      </div>

      <!-- Grafik -->
      <div v-if="series.length" class="p-4 rounded-xl border border-neutral-200 dark:border-white/[0.08] space-y-2 min-w-0">
        <h3 class="text-xs font-bold text-neutral-800 dark:text-neutral-100">
          Pageview per {{ stepLabel }}
        </h3>
        <TrafficSeriesChart :points="series" mode="volume" :step-sec="report.step_sec" unit="pageview" />
      </div>

      <!-- Tabel halaman teratas -->
      <div class="rounded-xl border border-neutral-200 dark:border-white/[0.08] overflow-x-auto">
        <div class="px-3 py-2 text-xs font-bold text-neutral-800 dark:text-neutral-100 flex items-center gap-1.5 border-b border-neutral-100 dark:border-white/[0.08]">
          <UIcon name="i-lucide-file-text" class="size-4 text-neutral-400" /> Halaman teratas
          <span class="font-normal text-neutral-500">· {{ pages.length }} teratas</span>
        </div>
        <table class="w-full text-xs">
          <thead class="text-neutral-500">
            <tr>
              <th class="text-left font-semibold px-3 py-1.5 w-8">
                #
              </th>
              <th v-if="multiHost" class="text-left font-semibold px-2 py-1.5">
                Host
              </th>
              <th class="text-left font-semibold px-2 py-1.5">
                Halaman
              </th>
              <th class="text-right font-semibold px-2 py-1.5">
                Dibuka
              </th>
              <th class="text-right font-semibold px-3 py-1.5 w-28 sm:w-44">
                Porsi
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-neutral-100 dark:divide-white/[0.06]">
            <tr v-for="(p, i) in pages" :key="p.page">
              <td class="px-3 py-1.5 text-neutral-400 tabular-nums">
                {{ i + 1 }}
              </td>
              <td v-if="multiHost" class="px-2 py-1.5 font-mono text-neutral-600 dark:text-neutral-300 max-w-40 truncate" :title="p.host">
                {{ p.host || '–' }}
              </td>
              <td class="px-2 py-1.5 font-mono max-w-[16rem] sm:max-w-md truncate">
                <a
                  v-if="pageUrl(p)"
                  :href="pageUrl(p)"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-neutral-800 dark:text-neutral-100 hover:text-sky-600 dark:hover:text-sky-400 hover:underline"
                  :title="`Buka ${pageUrl(p)} di tab baru`"
                >{{ p.path || '/' }}</a>
                <span v-else class="text-neutral-800 dark:text-neutral-100" :title="p.path">{{ p.path || '/' }}</span>
              </td>
              <td class="px-2 py-1.5 text-right tabular-nums">
                {{ fmtNum(p.views) }}
              </td>
              <td class="px-3 py-1.5">
                <div class="flex items-center justify-end gap-2">
                  <div class="hidden sm:block h-1 w-20 rounded-full bg-neutral-100 dark:bg-neutral-800 overflow-hidden">
                    <div class="h-full rounded-full bg-primary-500 dark:bg-primary-400" :style="{ width: Math.min(100, p.share / maxShare * 100) + '%' }" />
                  </div>
                  <span class="tabular-nums text-neutral-500 w-12 text-right">{{ fmtPct(p.share) }}</span>
                </div>
              </td>
            </tr>
            <tr v-if="!pages.length">
              <td :colspan="multiHost ? 5 : 4" class="px-3 py-4 text-center text-neutral-500">
                Belum ada rincian halaman.
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Perujuk & perangkat -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div v-for="l in lists" :key="l.title" class="rounded-xl border border-neutral-200 dark:border-white/[0.08] p-3">
          <div class="text-xs font-bold text-neutral-800 dark:text-neutral-100 flex items-center gap-1.5 mb-2">
            <UIcon :name="l.icon" class="size-4 text-neutral-400" /> {{ l.title }}
          </div>
          <div v-if="l.items.length" class="space-y-1.5">
            <div v-for="it in l.items.slice(0, 8)" :key="it.value" class="text-xs">
              <div class="flex justify-between gap-2">
                <span class="truncate font-mono text-neutral-700 dark:text-neutral-200" :title="it.value">{{ it.value || '(kosong)' }}</span>
                <span class="tabular-nums text-neutral-500 shrink-0">{{ fmtNum(it.count) }} · {{ fmtPct(it.share) }}</span>
              </div>
              <div class="mt-0.5 h-1 rounded-full bg-neutral-100 dark:bg-neutral-800 overflow-hidden">
                <div class="h-full rounded-full bg-primary-500 dark:bg-primary-400" :style="{ width: Math.min(100, it.share * 100) + '%' }" />
              </div>
            </div>
          </div>
          <p v-else class="text-xs text-neutral-500">
            {{ l.empty }}
          </p>
        </div>
      </div>
    </template>
  </div>
</template>
