<script setup lang="ts">
/** Bagian 14–16 Dokumen Proyek (ditulis tangan): keputusan teknis (ADR), glosarium, riwayat versi. */
import type { DocData } from '~/utils/projectDocLabels'
import { DOC_ADR_STATUS, DOC_TABLE, DOC_TD, DOC_TH } from '~/utils/projectDocLabels'

const props = defineProps<{ doc: DocData }>()
const emit = defineEmits<{ edit: [tab: string] }>()
const n = computed(() => props.doc.notes)
</script>

<template>
  <div>
    <ProjectDocsSection id="keputusan" no="14" title="Keputusan teknis (ADR)">
      <template v-if="n.decisions.length">
        <div
          v-for="(d, i) in n.decisions"
          :key="i"
          class="rounded-xl border border-neutral-200 dark:border-white/[0.08] p-4 break-inside-avoid"
          :class="d.status === 'superseded' || d.status === 'deprecated' ? 'opacity-70' : ''"
        >
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-mono text-[11px] text-neutral-400">ADR-{{ String(i + 1).padStart(3, '0') }}</span>
            <span class="font-semibold text-neutral-900 dark:text-white" :class="d.status === 'superseded' ? 'line-through' : ''">{{ d.title || 'Tanpa judul' }}</span>
            <UBadge v-if="d.status" size="sm" variant="subtle" :color="DOC_ADR_STATUS[d.status]?.color || 'neutral'">
              {{ DOC_ADR_STATUS[d.status]?.label || d.status }}
            </UBadge>
            <span v-if="d.date" class="ml-auto text-xs text-neutral-500 tabular-nums">{{ d.date }}</span>
          </div>
          <dl class="mt-2 grid sm:grid-cols-[110px_1fr] gap-x-3 gap-y-1.5 text-xs">
            <template v-for="row in [['Konteks', d.context], ['Keputusan', d.decision], ['Akibat', d.consequences]]" :key="row[0]">
              <template v-if="row[1]">
                <dt class="font-medium text-neutral-500">
                  {{ row[0] }}
                </dt>
                <dd class="whitespace-pre-line">
                  {{ row[1] }}
                </dd>
              </template>
            </template>
          </dl>
        </div>
      </template>
      <p v-else class="text-xs italic text-neutral-400">
        Belum ada keputusan teknis yang dicatat.
      </p>
      <UButton size="xs" color="neutral" variant="outline" icon="i-lucide-pencil" class="print:hidden" @click="emit('edit', 'adr')">
        {{ n.decisions.length ? 'Edit keputusan' : 'Catat keputusan' }}
      </UButton>
    </ProjectDocsSection>

    <ProjectDocsSection id="glosarium" no="15" title="Glosarium">
      <table v-if="n.glossary.length" :class="DOC_TABLE">
        <thead>
          <tr>
            <th :class="DOC_TH" class="w-1/4">
              Istilah
            </th>
            <th :class="DOC_TH">
              Arti
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="g in [...n.glossary].sort((a, b) => a.term.localeCompare(b.term))" :key="g.term">
            <td :class="DOC_TD" class="font-medium text-neutral-900 dark:text-white">
              {{ g.term }}
            </td>
            <td :class="DOC_TD">
              {{ g.definition }}
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="text-xs italic text-neutral-400">
        Belum ada istilah.
      </p>
      <UButton size="xs" color="neutral" variant="outline" icon="i-lucide-pencil" class="print:hidden" @click="emit('edit', 'glosarium')">
        {{ n.glossary.length ? 'Edit glosarium' : 'Tambah istilah' }}
      </UButton>
    </ProjectDocsSection>

    <ProjectDocsSection id="riwayat" no="16" title="Riwayat versi dokumen">
      <table v-if="n.changelog.length" :class="DOC_TABLE">
        <thead>
          <tr>
            <th :class="DOC_TH" class="w-24">
              Versi
            </th>
            <th :class="DOC_TH" class="w-28">
              Tanggal
            </th>
            <th :class="DOC_TH">
              Perubahan
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(ch, i) in n.changelog" :key="i">
            <td :class="DOC_TD" class="font-mono font-medium">
              {{ ch.version || '—' }}
            </td>
            <td :class="DOC_TD" class="tabular-nums">
              {{ ch.date || '—' }}
            </td>
            <td :class="DOC_TD" class="whitespace-pre-line">
              {{ ch.changes }}
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="text-xs italic text-neutral-400">
        Belum ada riwayat versi.
      </p>
      <UButton size="xs" color="neutral" variant="outline" icon="i-lucide-pencil" class="print:hidden" @click="emit('edit', 'riwayat')">
        {{ n.changelog.length ? 'Edit riwayat' : 'Tambah versi' }}
      </UButton>
    </ProjectDocsSection>
  </div>
</template>
