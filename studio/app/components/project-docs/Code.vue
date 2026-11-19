<script setup lang="ts">
/** Blok kode dokumen: judul kecil, nomor baris awal opsional, tombol salin (disembunyikan saat dicetak). */
const props = defineProps<{
  code: string
  title?: string
  lang?: string
  startLine?: number
}>()

const { copyToClipboard } = useCopyToClipboard()
</script>

<template>
  <figure class="rounded-xl border border-neutral-200 dark:border-white/[0.08] overflow-hidden break-inside-avoid">
    <figcaption
      v-if="title || lang"
      class="flex items-center justify-between gap-2 px-3 py-1.5 border-b border-neutral-200 dark:border-white/[0.08] bg-neutral-50 dark:bg-white/[0.03] text-xs"
    >
      <span class="font-mono text-neutral-600 dark:text-slate-400 truncate">{{ title }}<span v-if="startLine" class="text-neutral-400">:{{ startLine }}</span></span>
      <span class="flex items-center gap-2">
        <span v-if="lang" class="text-neutral-400 uppercase tracking-wide text-[10px]">{{ lang }}</span>
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          class="print:hidden"
          :icon="'i-lucide-copy'"
          title="Salin kode"
          @click="copyToClipboard(props.code, 'Kode disalin')"
        />
      </span>
    </figcaption>
    <pre class="m-0 p-3 overflow-x-auto text-xs leading-relaxed font-mono bg-white dark:bg-[#020a14] text-neutral-800 dark:text-slate-200 print:whitespace-pre-wrap print:break-all"><code>{{ code }}</code></pre>
  </figure>
</template>
