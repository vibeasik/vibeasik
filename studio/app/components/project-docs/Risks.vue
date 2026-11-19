<script setup lang="ts">
/** Bagian 13 Dokumen Proyek: risiko, roadmap, pertanyaan terbuka. */
import type { DocData } from '~/utils/projectDocLabels'
import { DOC_HORIZON, DOC_SEVERITY, DOC_TABLE, DOC_TD, DOC_TH } from '~/utils/projectDocLabels'

const props = defineProps<{ doc: DocData }>()
const prof = computed(() => props.doc.profile)
const hasAny = computed(() => Boolean(prof.value?.risks?.length || prof.value?.roadmap?.length || prof.value?.open_questions?.length))

const RISK_STATUS: Record<string, string> = {
  open: 'Terbuka', planned: 'Direncanakan', in_progress: 'Dikerjakan', handled: 'Ditangani', accepted: 'Diterima'
}
</script>

<template>
  <ProjectDocsSection
    id="risiko"
    no="13"
    title="Risiko, roadmap & pertanyaan terbuka"
    :empty="hasAny ? '' : 'Risiko dan roadmap belum tercatat di profil proyek.'"
  >
    <template v-if="hasAny">
      <table v-if="prof?.risks?.length" :class="DOC_TABLE">
        <thead>
          <tr>
            <th :class="DOC_TH">
              Risiko
            </th>
            <th :class="DOC_TH" class="w-24">
              Tingkat
            </th>
            <th :class="DOC_TH" class="w-28">
              Status
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in prof.risks" :key="r.title">
            <td :class="DOC_TD">
              <b class="text-neutral-900 dark:text-white">{{ r.title }}</b>
              <div class="text-[11px] text-neutral-500">
                {{ r.detail }}<template v-if="r.todo_id">
                  · todo #{{ r.todo_id }}
                </template>
              </div>
            </td>
            <td :class="DOC_TD">
              {{ DOC_SEVERITY[r.severity]?.label || r.severity }}
            </td>
            <td :class="DOC_TD">
              {{ RISK_STATUS[r.status || 'open'] }}
            </td>
          </tr>
        </tbody>
      </table>

      <table v-if="prof?.roadmap?.length" :class="DOC_TABLE">
        <thead>
          <tr>
            <th :class="DOC_TH" class="w-28">
              Waktu
            </th>
            <th :class="DOC_TH">
              Langkah roadmap
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in prof.roadmap" :key="r.title">
            <td :class="DOC_TD" class="font-medium">
              {{ DOC_HORIZON[r.horizon] || r.horizon }}
            </td>
            <td :class="DOC_TD">
              <b class="text-neutral-900 dark:text-white">{{ r.title }}</b>
              <div class="text-[11px] text-neutral-500">
                {{ r.rationale }}
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="prof?.open_questions?.length">
        <div class="text-xs font-medium text-neutral-500 mb-1">
          Pertanyaan terbuka
        </div>
        <ol class="list-decimal pl-5 space-y-0.5">
          <li v-for="q in prof.open_questions" :key="q">
            {{ q }}
          </li>
        </ol>
      </div>
    </template>
  </ProjectDocsSection>
</template>
