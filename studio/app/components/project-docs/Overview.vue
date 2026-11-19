<script setup lang="ts">
/** Bagian 1–5 Dokumen Proyek: keterangan, latar belakang, tujuan, ruang lingkup & fitur, stack. */
import type { DocData } from '~/utils/projectDocLabels'
import { DOC_FEATURE_STATUS, DOC_MATURITY, DOC_STATUS, DOC_TABLE, DOC_TD, DOC_TH, docDate, docInline, docRole } from '~/utils/projectDocLabels'

const props = defineProps<{ doc: DocData }>()

const p = computed(() => props.doc.project)
const prof = computed(() => props.doc.profile)

const identity = computed(() => {
  const pr = p.value
  if (!pr) return []
  return [
    ['Nama proyek', pr.name],
    ['Jenis', pr.project_type || '—'],
    ['Status', DOC_STATUS[pr.status] || pr.status],
    ['Kematangan', prof.value?.maturity?.stage ? DOC_MATURITY[prof.value.maturity.stage] : '—'],
    ['Repositori', pr.git_remote || '—'],
    ['Branch', pr.git_branch || '—'],
    ['Folder', pr.root_path || '—'],
    ['Tag', pr.tags || '—'],
    ['Dibuat', docDate(pr.created_at)],
    ['Diperbarui', docDate(pr.updated_at)]
  ]
})

// Fitur dikelompokkan per domain agar ruang lingkup terbaca per area bisnis.
const featureGroups = computed(() => {
  const g = new Map<string, NonNullable<typeof prof.value>['features']>()
  for (const f of prof.value?.features || []) {
    const k = f.domain || 'Umum'
    if (!g.has(k)) g.set(k, [])
    g.get(k)!.push(f)
  }
  return [...g.entries()]
})
const featureCount = (s: string) => (prof.value?.features || []).filter(f => f.status === s).length
const plannedFeatures = computed(() => (prof.value?.features || []).filter(f => f.status === 'planned'))
</script>

<template>
  <div>
    <ProjectDocsSection id="keterangan" no="1" title="Keterangan proyek">
      <!-- eslint-disable-next-line vue/no-v-html -- docInline meng-escape teks lebih dulu -->
      <p v-if="prof?.short_description || p?.short_description" class="text-base text-neutral-900 dark:text-white font-medium" v-html="docInline(prof?.short_description || p?.short_description)" />
      <!-- eslint-disable-next-line vue/no-v-html -- docInline meng-escape teks lebih dulu -->
      <p v-if="prof?.full_description || p?.description" class="whitespace-pre-line" v-html="docInline(prof?.full_description || p?.description)" />
      <table :class="DOC_TABLE">
        <tbody>
          <tr v-for="[k, v] in identity" :key="k">
            <th :class="DOC_TH" class="w-40 font-medium">
              {{ k }}
            </th>
            <td :class="DOC_TD" class="font-mono break-all">
              {{ v }}
            </td>
          </tr>
        </tbody>
      </table>
    </ProjectDocsSection>

    <ProjectDocsSection
      id="latar-belakang"
      no="2"
      title="Latar belakang & masalah"
      :empty="prof?.purpose?.problem ? '' : 'Belum ada uraian masalah. Susun profil proyek di Ringkasan Proyek agar bagian ini terisi.'"
    >
      <template v-if="prof?.purpose?.problem">
        <!-- eslint-disable-next-line vue/no-v-html -- docInline meng-escape teks lebih dulu -->
        <p class="whitespace-pre-line" v-html="docInline(prof.purpose.problem)" />
        <div v-if="prof.purpose.solution" class="rounded-xl border border-neutral-200 dark:border-white/[0.08] px-4 py-3">
          <div class="text-xs font-medium text-neutral-500 mb-1">
            Solusi yang ditawarkan
          </div>
          <!-- eslint-disable-next-line vue/no-v-html -- docInline meng-escape teks lebih dulu -->
          <p class="whitespace-pre-line" v-html="docInline(prof.purpose.solution)" />
        </div>
        <p v-if="prof.maturity?.reason" class="text-xs text-neutral-500">
          Kematangan saat ini: <b>{{ DOC_MATURITY[prof.maturity.stage] || prof.maturity.stage }}</b> — {{ prof.maturity.reason }}
        </p>
      </template>
    </ProjectDocsSection>

    <ProjectDocsSection
      id="tujuan"
      no="3"
      title="Tujuan & pengguna sasaran"
      :empty="prof?.purpose?.value_proposition || prof?.purpose?.target_users?.length ? '' : 'Tujuan dan pengguna sasaran belum tercatat di profil proyek.'"
    >
      <template v-if="prof?.purpose?.value_proposition || prof?.purpose?.target_users?.length">
        <p v-if="prof.purpose.value_proposition">
          <b class="text-neutral-900 dark:text-white">Nilai utama:</b> {{ prof.purpose.value_proposition }}
        </p>
        <div v-if="prof.purpose.target_users?.length">
          <div class="text-xs font-medium text-neutral-500 mb-1.5">
            Pengguna sasaran
          </div>
          <ul class="list-disc pl-5 space-y-0.5">
            <li v-for="u in prof.purpose.target_users" :key="u">
              {{ u }}
            </li>
          </ul>
        </div>
      </template>
    </ProjectDocsSection>

    <ProjectDocsSection
      id="lingkup"
      no="4"
      title="Ruang lingkup & fitur"
      :empty="prof?.features?.length ? '' : 'Daftar fitur belum tersusun. Susun profil proyek agar fitur, halaman, dan endpoint tiap fitur tercatat.'"
    >
      <template v-if="prof?.features?.length">
        <p>
          Ruang lingkup memuat <b>{{ prof.features.length }}</b> fitur:
          {{ featureCount('done') }} selesai, {{ featureCount('partial') }} sebagian, {{ featureCount('planned') }} direncanakan.
          Fitur berstatus "direncanakan" belum termasuk rilis saat ini.
        </p>
        <div v-for="[domain, list] in featureGroups" :key="domain" class="break-inside-avoid">
          <div class="text-xs font-semibold text-neutral-900 dark:text-white mt-4 mb-1">
            {{ domain }}
          </div>
          <table :class="DOC_TABLE">
            <thead>
              <tr>
                <th :class="DOC_TH" class="w-1/4">
                  Fitur
                </th>
                <th :class="DOC_TH">
                  Keterangan
                </th>
                <th :class="DOC_TH" class="w-28">
                  Status
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="f in list" :key="f.name">
                <td :class="DOC_TD" class="font-medium text-neutral-900 dark:text-white">
                  {{ f.name }}
                </td>
                <td :class="DOC_TD">
                  {{ f.description }}
                  <div v-if="f.pages?.length || f.endpoints?.length" class="mt-1 font-mono text-[11px] text-neutral-500 break-all">
                    {{ [...(f.pages || []), ...(f.endpoints || [])].slice(0, 6).join(' · ') }}
                  </div>
                </td>
                <td :class="DOC_TD">
                  <UBadge size="sm" variant="subtle" :color="DOC_FEATURE_STATUS[f.status]?.color || 'neutral'">
                    {{ DOC_FEATURE_STATUS[f.status]?.label || f.status }}
                  </UBadge>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="plannedFeatures.length" class="text-xs text-neutral-500">
          Di luar lingkup rilis saat ini: {{ plannedFeatures.map(f => f.name).join(', ') }}.
        </p>
      </template>
    </ProjectDocsSection>

    <ProjectDocsSection id="stack" no="5" title="Stack teknologi" :empty="doc.stack.length ? '' : 'Belum ada aplikasi terdaftar. Tambahkan aplikasi atau jalankan pindai cepat.'">
      <template v-if="doc.stack.length">
        <table :class="DOC_TABLE">
          <thead>
            <tr>
              <th :class="DOC_TH" class="w-44">
                Lapisan
              </th>
              <th :class="DOC_TH">
                Teknologi
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in doc.stack" :key="row.layer">
              <td :class="DOC_TD" class="font-medium">
                {{ row.layer }}
              </td>
              <td :class="DOC_TD">
                <span class="flex flex-wrap gap-1.5">
                  <span v-for="it in row.items" :key="it" class="rounded-md border border-neutral-200 dark:border-white/[0.08] px-1.5 py-0.5 font-mono text-[11px]">{{ it }}</span>
                </span>
              </td>
            </tr>
          </tbody>
        </table>
        <table v-if="doc.components.length" :class="DOC_TABLE" class="mt-2">
          <thead>
            <tr>
              <th :class="DOC_TH">
                Aplikasi
              </th>
              <th :class="DOC_TH">
                Peran
              </th>
              <th :class="DOC_TH">
                Bahasa / framework
              </th>
              <th :class="DOC_TH" class="text-right">
                Port
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in doc.components" :key="c.id">
              <td :class="DOC_TD" class="font-medium">
                {{ c.name }}
                <div class="font-mono text-[11px] text-neutral-500">
                  {{ c.relative_path || '.' }}
                </div>
              </td>
              <td :class="DOC_TD">
                {{ docRole(c.role) }}
              </td>
              <td :class="DOC_TD" class="font-mono">
                {{ [c.language, c.framework].filter(Boolean).join(' · ') || '—' }}
              </td>
              <td :class="DOC_TD" class="text-right font-mono">
                {{ c.run_port || c.port || '—' }}
              </td>
            </tr>
          </tbody>
        </table>
      </template>
    </ProjectDocsSection>
  </div>
</template>
