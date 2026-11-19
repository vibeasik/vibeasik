<script setup lang="ts">
/** Bagian 6–11 Dokumen Proyek: arsitektur, data, API, konfigurasi, cara menjalankan, struktur kode. */
import type { DocData } from '~/utils/projectDocLabels'
import { DOC_TABLE, DOC_TD, DOC_TH, docInline, docNum, docRole, isGeneratedPath } from '~/utils/projectDocLabels'

const props = defineProps<{ doc: DocData }>()

const prof = computed(() => props.doc.profile)
const env = computed(() => props.doc.knowledge?.environment_scope)
const files = computed(() => props.doc.knowledge?.files_scope)
const cm = computed(() => props.doc.overview)

// Rute API dikelompokkan per domain; dokumen memuat paling banyak ROUTE_LIMIT baris agar PDF tetap wajar.
const ROUTE_LIMIT = 200
const routeGroups = computed(() => {
  const g = new Map<string, typeof props.doc.backendRoutes>()
  for (const r of props.doc.backendRoutes.slice(0, ROUTE_LIMIT)) {
    const k = r.domain || r.function_group || r.component_name || 'Umum'
    if (!g.has(k)) g.set(k, [])
    g.get(k)!.push(r)
  }
  return [...g.entries()].sort((a, b) => a[0].localeCompare(b[0]))
})

const methodColor = (m: string) => ({ GET: 'text-sky-600', POST: 'text-emerald-600', PUT: 'text-amber-600', PATCH: 'text-amber-600', DELETE: 'text-rose-600' } as Record<string, string>)[m.toUpperCase()] || 'text-neutral-500'

const runScript = (c: DocData['components'][number]) => [
  `# ${c.name} (${docRole(c.role)})`,
  `cd ${c.relative_path || '.'}`,
  c.dev_command ? `${c.dev_command}        # mode pengembangan` : '',
  c.build_command ? `${c.build_command}        # build produksi` : '',
  c.health_url ? `curl -fsS ${c.health_url}   # cek kesehatan` : ''
].filter(Boolean).join('\n')

// Pohon direktori dipotong agar tidak memenuhi halaman. Bila Base Knowledge kosong, disusun dari folder peta kode.
const tree = computed(() => {
  let t = files.value?.directory_tree || ''
  if (t.split('\n').length < 3 && cm.value?.components?.length) {
    t = cm.value.components.map(c => [
      `${c.path || c.name}/  (${c.files} berkas, ${c.lines} baris)`,
      ...(c.dirs || []).filter(d => !isGeneratedPath(`${d.path}/`)).slice(0, 25).map(d => `  ${d.path}/  (${d.files})`)
    ].join('\n')).join('\n')
  }
  const lines = t.split('\n')
  return lines.length > 60 ? [...lines.slice(0, 60), `… (${lines.length - 60} baris lagi)`].join('\n') : t
})

const core = computed(() => (cm.value?.core || []).filter(f => !isGeneratedPath(f.path)).slice(0, 12))
const hotspots = computed(() => (cm.value?.hotspots || []).filter(f => !isGeneratedPath(f.path)).slice(0, 6))

const languages = computed(() => Object.entries(cm.value?.languages || {}).sort((a, b) => b[1] - a[1]))
</script>

<template>
  <div>
    <ProjectDocsSection id="arsitektur" no="6" title="Arsitektur & aplikasi">
      <p>
        Proyek terdiri dari <b>{{ doc.components.length }}</b> aplikasi.
        <template v-if="prof?.integrations?.length">
          Hubungan antaraplikasi dan layanan dicatat pada tabel integrasi di bawah.
        </template>
      </p>
      <div class="grid sm:grid-cols-2 gap-3">
        <div
          v-for="c in doc.components"
          :key="c.id"
          class="rounded-xl border border-neutral-200 dark:border-white/[0.08] p-4 break-inside-avoid"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="font-semibold text-neutral-900 dark:text-white truncate">{{ c.name }}</span>
            <span class="text-xs text-neutral-500">{{ docRole(c.role) }}</span>
          </div>
          <div class="font-mono text-[11px] text-neutral-500 mt-0.5">
            {{ [c.language, c.framework].filter(Boolean).join(' · ') }}<template v-if="c.run_port || c.port">
              · :{{ c.run_port || c.port }}
            </template>
          </div>
          <template v-for="a in (prof?.apps || []).filter(x => x.component_id === c.id)" :key="a.component_id">
            <!-- eslint-disable-next-line vue/no-v-html -- docInline meng-escape teks lebih dulu -->
            <p class="text-xs mt-2" v-html="docInline(a.summary)" />
            <ul v-if="a.responsibilities?.length" class="list-disc pl-4 mt-1 text-xs space-y-0.5">
              <li v-for="r in a.responsibilities" :key="r">
                {{ r }}
              </li>
            </ul>
            <p v-if="a.talks_to?.length" class="text-xs text-neutral-500 mt-1">
              Berkomunikasi dengan: {{ a.talks_to.join(', ') }}
            </p>
          </template>
        </div>
      </div>
      <table v-if="prof?.integrations?.length" :class="DOC_TABLE">
        <thead>
          <tr>
            <th :class="DOC_TH">
              Dari
            </th>
            <th :class="DOC_TH">
              Ke
            </th>
            <th :class="DOC_TH">
              Melalui
            </th>
            <th :class="DOC_TH">
              Tujuan
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(i, idx) in prof.integrations" :key="idx">
            <td :class="DOC_TD" class="font-medium">
              {{ i.from }}
            </td>
            <td :class="DOC_TD" class="font-medium">
              {{ i.to }}
            </td>
            <td :class="DOC_TD" class="font-mono">
              {{ i.via }}
            </td>
            <td :class="DOC_TD">
              {{ i.purpose }}
            </td>
          </tr>
        </tbody>
      </table>
    </ProjectDocsSection>

    <ProjectDocsSection
      id="data"
      no="7"
      title="Model data"
      :empty="prof?.data_entities?.length || env?.database_type ? '' : 'Entitas data belum tercatat. Susun profil proyek atau kompilasi Base Knowledge.'"
    >
      <p v-if="env?.database_type">
        Basis data: <b>{{ env.database_type }}</b><template v-if="env.orm_driver">
          melalui <span class="font-mono">{{ env.orm_driver }}</span>
        </template><template v-if="env.database_name">
          , nama <span class="font-mono">{{ env.database_name }}</span>
        </template>.
      </p>
      <table v-if="prof?.data_entities?.length" :class="DOC_TABLE">
        <thead>
          <tr>
            <th :class="DOC_TH" class="w-1/4">
              Entitas / tabel
            </th>
            <th :class="DOC_TH">
              Keterangan
            </th>
            <th :class="DOC_TH" class="w-1/4">
              Dipakai oleh
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="e in prof.data_entities" :key="e.name">
            <td :class="DOC_TD" class="font-mono font-medium">
              {{ e.name }}
            </td>
            <td :class="DOC_TD">
              {{ e.description }}
            </td>
            <td :class="DOC_TD">
              {{ (e.used_by || []).join(', ') || '—' }}
            </td>
          </tr>
        </tbody>
      </table>
    </ProjectDocsSection>

    <ProjectDocsSection
      id="api"
      no="8"
      title="Antarmuka: API & halaman"
      :empty="doc.routes.length ? '' : 'Katalog rute belum ada. Buka Rute, File & Env lalu sinkronkan rute tiap aplikasi.'"
    >
      <template v-if="doc.routes.length">
        <p>
          Tercatat <b>{{ docNum(doc.backendRoutes.length) }}</b> endpoint API dan <b>{{ docNum(doc.pageRoutes.length) }}</b> halaman.
          <template v-if="doc.backendRoutes.length > ROUTE_LIMIT">
            Dokumen ini memuat {{ ROUTE_LIMIT }} endpoint pertama; katalog lengkap ada di halaman Rute, File & Env.
          </template>
        </p>
        <div v-for="[group, list] in routeGroups" :key="group">
          <div class="text-xs font-semibold text-neutral-900 dark:text-white mt-4 mb-1">
            {{ group }} <span class="font-normal text-neutral-400 tabular-nums">({{ list.length }})</span>
          </div>
          <table :class="DOC_TABLE">
            <tbody>
              <tr v-for="r in list" :key="`${r.component_id}-${r.method}-${r.path}`">
                <td :class="[DOC_TD, methodColor(r.method)]" class="w-16 font-mono font-semibold">
                  {{ r.method.toUpperCase() }}
                </td>
                <td :class="DOC_TD" class="font-mono break-all">
                  {{ r.path }}
                </td>
                <td :class="DOC_TD">
                  {{ r.description || '—' }}
                  <div v-if="r.middleware" class="font-mono text-[11px] text-neutral-500">
                    {{ r.middleware }}
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <ProjectDocsCode v-if="doc.curlExample" :code="doc.curlExample" title="Contoh pemanggilan API" lang="bash" />
        <div v-if="doc.pageRoutes.length" class="break-inside-avoid">
          <div class="text-xs font-semibold text-neutral-900 dark:text-white mt-4 mb-1">
            Halaman antarmuka
          </div>
          <p class="font-mono text-[11px] leading-relaxed break-all">
            {{ doc.pageRoutes.slice(0, 120).map(r => r.path).join(' · ') }}
          </p>
        </div>
      </template>
    </ProjectDocsSection>

    <ProjectDocsSection
      id="konfigurasi"
      no="9"
      title="Konfigurasi & environment"
      :empty="env?.detected_env_keys?.length ? '' : 'Variabel environment belum terdeteksi. Kompilasi Base Knowledge untuk mengisinya.'"
    >
      <template v-if="env?.detected_env_keys?.length">
        <p>Nilai rahasia tidak pernah ditampilkan; kolom contoh memuat nilai aman untuk berkas <span class="font-mono">.env.example</span>.</p>
        <table :class="DOC_TABLE">
          <thead>
            <tr>
              <th :class="DOC_TH">
                Variabel
              </th>
              <th :class="DOC_TH">
                Kegunaan
              </th>
              <th :class="DOC_TH" class="w-20">
                Wajib
              </th>
              <th :class="DOC_TH">
                Contoh
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="k in env.detected_env_keys" :key="k.key">
              <td :class="DOC_TD" class="font-mono font-medium">
                {{ k.key }}
                <UIcon v-if="k.is_secret" name="i-lucide-lock" class="size-3 text-amber-500 align-middle" title="Rahasia" />
              </td>
              <td :class="DOC_TD">
                {{ k.purpose || '—' }}
              </td>
              <td :class="DOC_TD">
                {{ k.required ? 'Ya' : 'Tidak' }}
              </td>
              <td :class="DOC_TD" class="font-mono break-all">
                {{ k.is_secret ? '••••••' : (k.safe_example || '—') }}
              </td>
            </tr>
          </tbody>
        </table>
        <p v-if="env.auth_methods?.length || env.login_endpoints?.length" class="text-xs">
          <b>Autentikasi:</b> {{ (env.auth_methods || []).join(', ') || '—' }}
          <template v-if="env.login_endpoints?.length">
            · endpoint masuk <span class="font-mono">{{ env.login_endpoints.join(', ') }}</span>
          </template>
        </p>
      </template>
    </ProjectDocsSection>

    <ProjectDocsSection id="menjalankan" no="10" title="Menjalankan, build & deploy" :empty="doc.components.length ? '' : 'Belum ada aplikasi terdaftar.'">
      <template v-if="doc.components.length">
        <p>Perintah tiap aplikasi, dijalankan dari folder proyek <span class="font-mono break-all">{{ doc.project?.root_path }}</span>. Port mengikuti pengelola port engine; jangan memilih port sendiri.</p>
        <ProjectDocsCode
          v-for="c in doc.components.filter(x => x.dev_command || x.build_command)"
          :key="c.id"
          :code="runScript(c)"
          :title="c.name"
          lang="bash"
        />
      </template>
    </ProjectDocsSection>

    <ProjectDocsSection id="kode" no="11" title="Struktur kode & contoh" :empty="cm || tree ? '' : 'Peta kode belum dibangun. Buka Peta Kode untuk membangunnya.'">
      <template v-if="cm">
        <p>
          <b>{{ docNum(cm.file_count) }}</b> berkas, <b>{{ docNum(cm.total_lines) }}</b> baris, {{ docNum(cm.symbol_count) }} simbol.
          Bahasa: {{ languages.map(([l, n]) => `${l} (${n})`).join(', ') }}.
        </p>
        <table v-if="core.length" :class="DOC_TABLE">
          <thead>
            <tr>
              <th :class="DOC_TH">
                Modul inti (paling banyak dipakai)
              </th>
              <th :class="DOC_TH" class="w-24 text-right">
                Dipakai
              </th>
              <th :class="DOC_TH" class="w-20 text-right">
                Baris
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="f in core" :key="f.path">
              <td :class="DOC_TD" class="font-mono break-all">
                {{ f.path }}
                <div v-if="f.note" class="font-sans text-[11px] text-neutral-500">
                  {{ f.note }}
                </div>
              </td>
              <td :class="DOC_TD" class="text-right">
                {{ f.used_by }}
              </td>
              <td :class="DOC_TD" class="text-right">
                {{ docNum(f.lines) }}
              </td>
            </tr>
          </tbody>
        </table>
        <p v-if="hotspots.length" class="text-xs">
          <b>Berkas besar (perlu dipecah bila terus tumbuh):</b>
          <span class="font-mono">{{ hotspots.map(h => `${h.path} (${h.lines})`).join(' · ') }}</span>
        </p>
      </template>
      <ProjectDocsCode v-if="tree" :code="tree" title="Struktur direktori" lang="text" />
      <ProjectDocsCode
        v-for="s in doc.entrySnippets"
        :key="s.path"
        :code="s.snippet || ''"
        :title="s.path"
        :start-line="s.snippet_start"
        :lang="s.lang"
      />
    </ProjectDocsSection>
  </div>
</template>
