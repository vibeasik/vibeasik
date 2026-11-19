<script setup lang="ts">
/** Satu bagian bernomor di Dokumen Proyek; id menjadi jangkar daftar isi. */
defineProps<{
  id: string
  no: string
  title: string
  sub?: boolean
  empty?: string
}>()
</script>

<template>
  <section :id="id" class="scroll-mt-24" :class="sub ? 'pt-2' : 'pt-8 first:pt-0'">
    <component
      :is="sub ? 'h3' : 'h2'"
      class="flex items-baseline gap-2.5 font-semibold tracking-tight text-neutral-950 dark:text-white print:text-black"
      :class="sub ? 'text-sm mb-2' : 'text-lg mb-3 pb-2 border-b border-neutral-200 dark:border-white/[0.07]'"
    >
      <span class="font-mono text-xs text-neutral-400 dark:text-slate-500 tabular-nums">{{ no }}</span>
      <span>{{ title }}</span>
    </component>
    <div class="space-y-3 text-sm leading-relaxed text-neutral-700 dark:text-slate-300 print:text-neutral-800">
      <slot>
        <p v-if="empty" class="text-xs italic text-neutral-400 dark:text-slate-500">
          {{ empty }}
        </p>
      </slot>
    </div>
  </section>
</template>
