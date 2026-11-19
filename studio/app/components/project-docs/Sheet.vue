<script setup lang="ts">
/**
 * Panel kanan Dokumen Proyek: daftar isi + lembar teknis ringkas (stack, aplikasi, angka kode & mutu) + status
 * sumber data. Saat dicetak, daftar isi & status sumber disembunyikan; lembar teknis menjadi lampiran.
 */
import type { DocData } from '~/utils/projectDocLabels'
import { DOC_MATURITY, docDate, docNum, docRole } from '~/utils/projectDocLabels'
import { DOC_SECTIONS } from '~/composables/useProjectDocument'

const props = defineProps<{ doc: DocData, activeId: string }>()

const scoreCls = (n?: number | null) => n == null ? 'text-neutral-300' : n >= 80 ? 'text-emerald-600' : n >= 60 ? 'text-amber-600' : 'text-rose-600'

const quality = computed(() => [
  { l: 'Kesehatan', v: props.doc.project?.health_score },
  { l: 'Audit', v: props.doc.audit?.score },
  { l: 'Runtime', v: props.doc.runtime?.score },
  { l: 'Keamanan', v: props.doc.securityReport?.score }
])

const facts = computed(() => [
  { l: 'Berkas', v: docNum(props.doc.overview?.file_count ?? props.doc.knowledge?.files_scope?.total_files) },
  { l: 'Baris kode', v: docNum(props.doc.overview?.total_lines) },
  { l: 'Endpoint API', v: docNum(props.doc.backendRoutes.length) },
  { l: 'Halaman', v: docNum(props.doc.pageRoutes.length) },
  { l: 'Berkas test', v: docNum(props.doc.testFiles.length) },
  { l: 'Kasus test', v: docNum(props.doc.testCaseCount) }
])

const SOURCE_STYLE: Record<string, { dot: string, label: string }> = {
  ok: { dot: 'bg-emerald-500', label: 'Ada' },
  empty: { dot: 'bg-neutral-300', label: 'Belum ada' },
  locked: { dot: 'bg-violet-500', label: 'Pro' },
  error: { dot: 'bg-rose-500', label: 'Galat' }
}
</script>

<template>
  <aside class="space-y-4 text-xs print:break-before-page">
    <!-- Daftar isi (layar saja) -->
    <nav class="rounded-xl border border-neutral-200 dark:border-white/[0.08] bg-white dark:bg-[#04111f] p-3 print:hidden" aria-label="Daftar isi">
      <div class="px-1 pb-2 text-[11px] font-semibold uppercase tracking-wide text-neutral-400">
        Daftar isi
      </div>
      <ul class="space-y-px">
        <li v-for="s in DOC_SECTIONS" :key="s.id">
          <a
            :href="`#${s.id}`"
            class="relative flex gap-2 rounded-md px-2 py-1 hover:bg-neutral-100 dark:hover:bg-white/[0.05]"
            :class="activeId === s.id || s.children?.some(c => c.id === activeId) ? 'text-neutral-950 dark:text-white font-medium' : 'text-neutral-600 dark:text-slate-400'"
          >
            <span v-if="activeId === s.id || s.children?.some(c => c.id === activeId)" class="absolute -left-1 top-1.5 bottom-1.5 w-0.5 rounded-full bg-sky-500" />
            <span class="font-mono text-neutral-400 w-5 shrink-0 tabular-nums">{{ s.no }}</span>{{ s.title }}
          </a>
          <ul v-if="s.children" class="ml-7 space-y-px">
            <li v-for="c in s.children" :key="c.id">
              <a :href="`#${c.id}`" class="block rounded-md px-2 py-0.5 hover:bg-neutral-100 dark:hover:bg-white/[0.05]" :class="activeId === c.id ? 'text-neutral-950 dark:text-white font-medium' : 'text-neutral-500 dark:text-slate-500'">
                {{ c.no }} {{ c.title }}
              </a>
            </li>
          </ul>
        </li>
      </ul>
    </nav>

    <!-- Lembar teknis -->
    <section class="rounded-xl border border-neutral-200 dark:border-white/[0.08] bg-white dark:bg-[#04111f] overflow-hidden">
      <header class="px-4 py-3 border-b border-neutral-200 dark:border-white/[0.08]">
        <div class="text-[11px] font-semibold uppercase tracking-wide text-neutral-400">
          <span class="hidden print:inline">Lampiran A · </span>Lembar teknis
        </div>
        <div class="text-sm font-semibold text-neutral-950 dark:text-white truncate">
          {{ doc.project?.name }}
        </div>
        <div class="text-neutral-500">
          {{ doc.profile?.maturity?.stage ? DOC_MATURITY[doc.profile.maturity.stage] : (doc.project?.project_type || '') }}
          <template v-if="doc.project?.git_branch">
            · <span class="font-mono">{{ doc.project.git_branch }}</span>
          </template>
        </div>
      </header>

      <div class="grid grid-cols-4 gap-px bg-neutral-200 dark:bg-white/[0.06]">
        <div v-for="q in quality" :key="q.l" class="bg-white dark:bg-[#04111f] px-2 py-2.5 text-center">
          <div class="text-lg font-semibold tabular-nums leading-none" :class="scoreCls(q.v)">
            {{ q.v ?? '—' }}
          </div>
          <div class="mt-1 text-[10px] text-neutral-500">
            {{ q.l }}
          </div>
        </div>
      </div>

      <dl class="px-4 py-3 grid grid-cols-2 gap-x-3 gap-y-1.5 border-t border-neutral-200 dark:border-white/[0.08]">
        <template v-for="f in facts" :key="f.l">
          <dt class="text-neutral-500">
            {{ f.l }}
          </dt>
          <dd class="text-right font-mono tabular-nums text-neutral-900 dark:text-white">
            {{ f.v }}
          </dd>
        </template>
      </dl>

      <div v-if="doc.stack.length" class="px-4 py-3 border-t border-neutral-200 dark:border-white/[0.08] space-y-2">
        <div v-for="row in doc.stack" :key="row.layer">
          <div class="text-[11px] text-neutral-500 mb-0.5">
            {{ row.layer }}
          </div>
          <div class="flex flex-wrap gap-1">
            <span v-for="it in row.items" :key="it" class="rounded-md bg-neutral-100 dark:bg-white/[0.06] px-1.5 py-0.5 font-mono text-[11px]">{{ it }}</span>
          </div>
        </div>
      </div>

      <ul v-if="doc.components.length" class="px-4 py-3 border-t border-neutral-200 dark:border-white/[0.08] space-y-2">
        <li v-for="c in doc.components" :key="c.id">
          <div class="flex justify-between gap-2">
            <span class="font-medium text-neutral-900 dark:text-white truncate">{{ c.name }}</span>
            <span class="font-mono text-neutral-500">{{ c.run_port || c.port ? `:${c.run_port || c.port}` : '' }}</span>
          </div>
          <div class="text-neutral-500">
            {{ docRole(c.role) }} · <span class="font-mono">{{ c.framework || c.language }}</span>
          </div>
          <div v-if="c.dev_command" class="font-mono text-[11px] text-neutral-600 dark:text-slate-400 truncate" :title="c.dev_command">
            $ {{ c.dev_command }}
          </div>
        </li>
      </ul>

      <div class="px-4 py-2.5 border-t border-neutral-200 dark:border-white/[0.08] text-[11px] text-neutral-400">
        Disusun {{ docDate(doc.loadedAt) }}
      </div>
    </section>

    <!-- Sumber data (layar saja) -->
    <section class="rounded-xl border border-neutral-200 dark:border-white/[0.08] bg-white dark:bg-[#04111f] p-3 print:hidden">
      <div class="px-1 pb-2 text-[11px] font-semibold uppercase tracking-wide text-neutral-400">
        Sumber data
      </div>
      <ul class="space-y-1">
        <li v-for="s in doc.sources" :key="s.key" class="flex items-start gap-2 px-1">
          <span class="mt-1 size-1.5 rounded-full shrink-0" :class="SOURCE_STYLE[s.state]?.dot" />
          <span class="min-w-0 flex-1">
            <span class="text-neutral-800 dark:text-slate-200">{{ s.label }}</span>
            <span v-if="s.state !== 'ok'" class="block text-[11px] text-neutral-500">
              {{ s.hint }}
              <NuxtLink v-if="s.fixTo" :to="s.fixTo" class="text-sky-600 hover:underline">Buka</NuxtLink>
            </span>
          </span>
          <span class="text-[10px] text-neutral-400">{{ SOURCE_STYLE[s.state]?.label }}</span>
        </li>
      </ul>
    </section>
  </aside>
</template>
