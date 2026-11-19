<script setup lang="ts">
/** Bagian 12 Dokumen Proyek: strategi & cakupan test, test spesifik, uji runtime, uji keamanan, audit statis. */
import type { DocData } from '~/utils/projectDocLabels'
import { DOC_SEVERITY, DOC_TABLE, DOC_TD, DOC_TH, docDate, docNum } from '~/utils/projectDocLabels'

const props = defineProps<{ doc: DocData }>()

const rt = computed(() => props.doc.runtime)
const sec = computed(() => props.doc.securityReport)
const aud = computed(() => props.doc.audit)

// Perintah test bawaan per bahasa/framework aplikasi (pengetahuan umum ekosistem).
const testCommand = (lang: string, fw: string, pm: string) => {
  const l = `${lang} ${fw}`.toLowerCase()
  if (l.includes('go')) return 'go test ./...'
  if (l.includes('flutter') || l.includes('dart')) return 'flutter test'
  if (l.includes('python')) return 'pytest'
  if (l.includes('php') || l.includes('laravel')) return 'php artisan test'
  if (l.includes('rust')) return 'cargo test'
  if (/(ts|js|typescript|javascript|nuxt|vue|react|next|node)/.test(l)) return `${pm || 'npm'} ${pm === 'bun' ? 'test' : 'run test'}`
  return ''
}

const testScript = computed(() => props.doc.components
  .map(c => ({ c, cmd: testCommand(c.language, c.framework, c.package_manager) }))
  .filter(x => x.cmd)
  .map(x => `# ${x.c.name}\ncd ${x.c.relative_path || '.'} && ${x.cmd}`)
  .join('\n\n'))

const stats = computed(() => [
  { l: 'Berkas test', v: docNum(props.doc.testFiles.length) },
  { l: 'Kasus test', v: docNum(props.doc.testCaseCount) },
  { l: 'Rasio baris test', v: `${props.doc.testRatio}%` },
  { l: 'Skor runtime / keamanan', v: `${rt.value?.score ?? '—'} / ${sec.value?.score ?? '—'}` }
])

const perComponent = computed(() => {
  const m = new Map<string, { files: number, cases: number, lines: number }>()
  for (const f of props.doc.testFiles) {
    const r = m.get(f.component) || { files: 0, cases: 0, lines: 0 }
    r.files++
    r.cases += f.cases.length
    r.lines += f.lines
    m.set(f.component, r)
  }
  return [...m.entries()]
})

const SEC_STATUS: Record<string, { label: string, cls: string }> = {
  pass: { label: 'Lulus', cls: 'text-emerald-600' },
  declared: { label: 'Dideklarasikan', cls: 'text-sky-600' },
  warn: { label: 'Lemah', cls: 'text-amber-600' },
  fail: { label: 'Gagal', cls: 'text-rose-600' },
  skipped: { label: 'Dilewati', cls: 'text-neutral-400' }
}

const statusCls = (code?: number) => !code ? 'text-neutral-400' : code < 300 ? 'text-emerald-600' : code < 500 ? 'text-amber-600' : 'text-rose-600'

const openFindings = computed(() => (aud.value?.findings || []).filter(f => !f.status || f.status === 'open'))
const topFindings = computed(() => {
  const rank: Record<string, number> = { CRITICAL: 0, WARNING: 1, INFO: 2, OPTIMIZATION: 3 }
  return [...openFindings.value].sort((a, b) => (rank[a.severity] ?? 9) - (rank[b.severity] ?? 9)).slice(0, 15)
})
</script>

<template>
  <ProjectDocsSection id="pengujian" no="12" title="Pengujian & mutu">
    <p>
      Mutu dibuktikan lewat empat lapis: test otomatis di repositori, uji runtime terhadap aplikasi yang berjalan,
      uji keamanan router & server, dan audit kode statis. Angka di bawah diambil dari hasil terakhir tiap lapis.
    </p>

    <ProjectDocsSection id="uji-strategi" no="12.1" title="Strategi & cakupan" sub>
      <div class="grid gap-px overflow-hidden rounded-xl border border-neutral-200 dark:border-white/[0.08] bg-neutral-200 dark:bg-white/[0.06] grid-cols-2 sm:grid-cols-4">
        <div v-for="s in stats" :key="s.l" class="bg-white dark:bg-[#04111f] px-4 py-3">
          <div class="text-xs font-medium text-neutral-500">
            {{ s.l }}
          </div>
          <div class="text-xl font-semibold tracking-tight tabular-nums text-neutral-900 dark:text-white">
            {{ s.v }}
          </div>
        </div>
      </div>
      <p class="text-xs text-neutral-500">
        Rasio baris test = baris berkas test ÷ seluruh baris kode; perkiraan kasar, bukan coverage hasil instrumen.
      </p>
      <table v-if="perComponent.length" :class="DOC_TABLE">
        <thead>
          <tr>
            <th :class="DOC_TH">
              Aplikasi
            </th>
            <th :class="DOC_TH" class="text-right">
              Berkas
            </th>
            <th :class="DOC_TH" class="text-right">
              Kasus
            </th>
            <th :class="DOC_TH" class="text-right">
              Baris
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="[name, r] in perComponent" :key="name">
            <td :class="DOC_TD" class="font-medium">
              {{ name }}
            </td>
            <td :class="DOC_TD" class="text-right">
              {{ r.files }}
            </td>
            <td :class="DOC_TD" class="text-right">
              {{ r.cases }}
            </td>
            <td :class="DOC_TD" class="text-right">
              {{ docNum(r.lines) }}
            </td>
          </tr>
        </tbody>
      </table>
      <ProjectDocsCode v-if="testScript" :code="testScript" title="Menjalankan test" lang="bash" />
    </ProjectDocsSection>

    <ProjectDocsSection
      id="uji-spesifik"
      no="12.2"
      title="Test spesifik"
      sub
      :empty="doc.testFiles.length ? '' : (doc.overview ? 'Peta kode tidak menemukan berkas test. Tambahkan test untuk alur utama.' : 'Peta kode belum dibangun, jadi daftar test belum bisa disusun.')"
    >
      <table v-if="doc.testFiles.length" :class="DOC_TABLE">
        <thead>
          <tr>
            <th :class="DOC_TH">
              Berkas test
            </th>
            <th :class="DOC_TH">
              Kasus yang diuji
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="f in doc.testFiles" :key="f.path" class="break-inside-avoid">
            <td :class="DOC_TD" class="font-mono break-all w-2/5">
              {{ f.path }}
              <div class="font-sans text-[11px] text-neutral-500">
                {{ f.component }} · {{ f.lines }} baris<template v-if="f.summary">
                  · {{ f.summary }}
                </template>
              </div>
            </td>
            <td :class="DOC_TD" class="font-mono text-[11px] break-all">
              {{ f.cases.length ? f.cases.join(', ') : '—' }}
            </td>
          </tr>
        </tbody>
      </table>
    </ProjectDocsSection>

    <ProjectDocsSection
      id="uji-runtime"
      no="12.3"
      title="Uji runtime"
      sub
      :empty="rt ? '' : 'Uji runtime belum pernah dijalankan. Jalankan dari halaman Uji Runtime untuk mengukur kecepatan API, login, dan halaman.'"
    >
      <template v-if="rt">
        <p>
          Sesi {{ docDate(rt.started_at) }} — status <b>{{ rt.status }}</b>, skor <b>{{ rt.score }}</b>.
          <template v-if="rt.summary">
            {{ rt.summary }}
          </template>
        </p>
        <table v-if="rt.components?.length" :class="DOC_TABLE">
          <thead>
            <tr>
              <th :class="DOC_TH">
                Aplikasi
              </th>
              <th :class="DOC_TH">
                Keadaan
              </th>
              <th :class="DOC_TH" class="text-right">
                Boot (ms)
              </th>
              <th :class="DOC_TH" class="text-right">
                Memori (MB)
              </th>
              <th :class="DOC_TH" class="text-right">
                Baris galat
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in rt.components" :key="c.component_id">
              <td :class="DOC_TD" class="font-medium">
                {{ c.name }}
              </td>
              <td :class="DOC_TD">
                {{ c.state }}
              </td>
              <td :class="DOC_TD" class="text-right">
                {{ c.boot_ms != null ? Math.round(c.boot_ms) : '—' }}
              </td>
              <td :class="DOC_TD" class="text-right">
                {{ c.memory_mb != null ? Math.round(c.memory_mb) : '—' }}
              </td>
              <td :class="DOC_TD" class="text-right">
                {{ c.error_lines }}
              </td>
            </tr>
          </tbody>
        </table>
        <table v-if="rt.probes?.length" :class="DOC_TABLE">
          <thead>
            <tr>
              <th :class="DOC_TH">
                Endpoint
              </th>
              <th :class="DOC_TH" class="text-right">
                Status
              </th>
              <th :class="DOC_TH" class="text-right">
                Dingin (ms)
              </th>
              <th :class="DOC_TH" class="text-right">
                Hangat (ms)
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(pr, i) in rt.probes.slice(0, 40)" :key="i">
              <td :class="DOC_TD" class="font-mono break-all">
                {{ pr.method }} {{ pr.path }}
              </td>
              <td :class="[DOC_TD, statusCls(pr.status_code)]" class="text-right font-mono">
                {{ pr.status_code || '—' }}
              </td>
              <td :class="DOC_TD" class="text-right">
                {{ Math.round(pr.cold_ms) }}
              </td>
              <td :class="DOC_TD" class="text-right">
                {{ Math.round(pr.warm_ms) }}
              </td>
            </tr>
          </tbody>
        </table>
        <table v-if="rt.pages?.length" :class="DOC_TABLE">
          <thead>
            <tr>
              <th :class="DOC_TH">
                Halaman
              </th>
              <th :class="DOC_TH" class="text-right">
                TTFB
              </th>
              <th :class="DOC_TH" class="text-right">
                FCP
              </th>
              <th :class="DOC_TH" class="text-right">
                Muat (ms)
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(pg, i) in rt.pages.slice(0, 30)" :key="i">
              <td :class="DOC_TD" class="font-mono break-all">
                {{ pg.path }} <span class="text-neutral-400">({{ pg.auth === 'auth' ? 'login' : 'tamu' }})</span>
              </td>
              <td :class="DOC_TD" class="text-right">
                {{ Math.round(pg.ttfb_ms) }}
              </td>
              <td :class="DOC_TD" class="text-right">
                {{ Math.round(pg.fcp_ms) }}
              </td>
              <td :class="DOC_TD" class="text-right">
                {{ Math.round(pg.load_ms) }}
              </td>
            </tr>
          </tbody>
        </table>
        <ul v-if="rt.logins?.length" class="text-xs space-y-0.5">
          <li v-for="(lg, i) in rt.logins" :key="i">
            <span :class="lg.ok ? 'text-emerald-600' : 'text-rose-600'">{{ lg.ok ? '✓' : '✗' }}</span>
            Login {{ lg.kind }} {{ lg.component }} — {{ lg.message }}
          </li>
        </ul>
      </template>
    </ProjectDocsSection>

    <ProjectDocsSection
      id="uji-keamanan"
      no="12.4"
      title="Uji keamanan"
      sub
      :empty="sec ? '' : 'Uji keamanan belum pernah dijalankan. Jalankan dari halaman Uji Keamanan.'"
    >
      <template v-if="sec">
        <p>
          Target <span class="font-mono">{{ sec.target }}</span>, {{ docDate(sec.started_at) }} — nilai <b>{{ sec.grade }}</b> (skor {{ sec.score }}).
          {{ sec.summary.pass || 0 }} lulus, {{ sec.summary.warn || 0 }} lemah, {{ sec.summary.fail || 0 }} gagal;
          {{ sec.routes_seen }} rute dan {{ sec.files_read }} berkas diperiksa.
        </p>
        <table :class="DOC_TABLE">
          <thead>
            <tr>
              <th :class="DOC_TH">
                Pemeriksaan
              </th>
              <th :class="DOC_TH" class="w-28">
                Hasil
              </th>
              <th :class="DOC_TH" class="w-24">
                Tingkat
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in sec.checks" :key="c.id">
              <td :class="DOC_TD">
                {{ c.title }}
                <div v-if="c.suggestion && c.status !== 'pass'" class="text-[11px] text-neutral-500">
                  {{ c.suggestion }}
                </div>
              </td>
              <td :class="[DOC_TD, SEC_STATUS[c.status]?.cls]" class="font-medium">
                {{ SEC_STATUS[c.status]?.label || c.status }}
              </td>
              <td :class="DOC_TD">
                {{ DOC_SEVERITY[c.severity]?.label || c.severity }}
              </td>
            </tr>
          </tbody>
        </table>
      </template>
    </ProjectDocsSection>

    <ProjectDocsSection
      id="uji-audit"
      no="12.5"
      title="Audit kode statis"
      sub
      :empty="aud ? '' : 'Proyek belum diaudit. Jalankan audit dari halaman Audit & Diagnostik.'"
    >
      <template v-if="aud">
        <p>
          Skor audit <b>{{ aud.score }}</b>, {{ openFindings.length }} temuan terbuka dari {{ aud.total_findings }}
          ({{ aud.severity_counts?.CRITICAL || 0 }} kritis, {{ aud.severity_counts?.WARNING || 0 }} peringatan).
        </p>
        <table v-if="topFindings.length" :class="DOC_TABLE">
          <thead>
            <tr>
              <th :class="DOC_TH">
                Temuan terbuka teratas
              </th>
              <th :class="DOC_TH" class="w-24">
                Tingkat
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(f, i) in topFindings" :key="f.id || i">
              <td :class="DOC_TD">
                <span v-if="f.rule_id" class="font-mono text-[11px] text-neutral-500 mr-1">{{ f.rule_id }}</span>{{ f.title }}
                <div v-if="f.target_file" class="font-mono text-[11px] text-neutral-500 break-all">
                  {{ f.target_file }}<template v-if="f.line">
                    :{{ f.line }}
                  </template>
                </div>
              </td>
              <td :class="DOC_TD">
                {{ DOC_SEVERITY[f.severity]?.label || f.severity }}
              </td>
            </tr>
          </tbody>
        </table>
      </template>
    </ProjectDocsSection>
  </ProjectDocsSection>
</template>
