<script setup lang="ts">
import type { TextEntry } from '~/types'

// Popup "Jadikan todo ubah teks": teks asli + lokasinya di atas, di bawahnya kolom untuk menulis perubahan yang diinginkan.
const open = defineModel<boolean>('open', { default: false })
const props = defineProps<{ entry: TextEntry | null, origin?: string }>()
const emit = defineEmits<{ (e: 'created', entryId: number, todoId: number): void }>()

const toast = useToast()
const { entryToTodo } = useTexts()

const instruction = ref('')
const priority = ref('medium')
const saving = ref(false)

const priorityItems = [
  { label: 'Rendah', value: 'low' },
  { label: 'Sedang', value: 'medium' },
  { label: 'Tinggi', value: 'high' }
]

watch(open, (v) => {
  if (!v) return
  instruction.value = ''
  priority.value = 'medium'
})

const submit = async () => {
  if (!props.entry || !instruction.value.trim()) return
  saving.value = true
  try {
    const res = await entryToTodo(props.entry.id, instruction.value.trim(), priority.value)
    if (res.already_exists) {
      toast.add({ title: `Sudah ada todo #${res.todo.id} untuk teks ini`, description: 'Todo itu belum selesai, jadi tidak dibuat ganda. Ubah isinya di halaman Todo.', icon: 'i-lucide-info', color: 'warning' })
    } else {
      toast.add({ title: `Todo #${res.todo.id} dibuat`, description: res.todo.title, icon: 'i-lucide-list-checks', color: 'success' })
    }
    emit('created', props.entry.id, res.todo.id)
    open.value = false
  } catch (err: any) {
    toast.add({ title: 'Gagal membuat todo', description: err?.data?.error || err?.message, icon: 'i-lucide-alert-circle', color: 'error' })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UModal v-model:open="open" title="Jadikan todo ubah teks" description="Tulis teks seperti apa yang kamu mau; pegawai vibeasik mengubahnya di kode." :ui="{ content: 'sm:max-w-2xl' }">
    <template #body>
      <div v-if="entry" class="space-y-4">
        <section class="space-y-2">
          <h4 class="text-xs font-semibold text-neutral-500 dark:text-slate-400">
            Teks asli
          </h4>
          <blockquote class="p-3 rounded-xl bg-neutral-50 dark:bg-white/[0.04] border border-neutral-200 dark:border-white/[0.08] text-sm leading-relaxed text-neutral-900 dark:text-white whitespace-pre-wrap break-words max-h-52 overflow-y-auto">
            {{ entry.text }}
          </blockquote>
          <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
            <dt class="text-neutral-500">
              Halaman
            </dt>
            <dd class="font-mono text-neutral-800 dark:text-slate-200 break-all">
              {{ entry.page }}<span v-if="origin" class="text-neutral-400"> · {{ origin }}</span>
            </dd>
            <dt class="text-neutral-500">
              Section
            </dt>
            <dd class="text-neutral-800 dark:text-slate-200">
              {{ entry.section }}
            </dd>
            <template v-if="entry.tag">
              <dt class="text-neutral-500">
                Elemen
              </dt>
              <dd class="font-mono text-neutral-800 dark:text-slate-200 break-all">
                {{ entry.tag }}<span v-if="entry.selector" class="text-neutral-400"> · {{ entry.selector }}</span>
              </dd>
            </template>
          </dl>
        </section>

        <UFormField label="Ingin diubah jadi apa?" help="Tulis teks barunya, atau arahannya (mis. “lebih santai, tanpa kata ‘solusi’”).">
          <UTextarea v-model="instruction" :rows="4" autofocus placeholder="mis. Ganti dengan kalimat yang lebih akrab, seperti orang bicara ke pelanggan." class="w-full" />
        </UFormField>
        <UFormField label="Prioritas">
          <USelect v-model="priority" :items="priorityItems" class="w-40" />
        </UFormField>
      </div>
    </template>
    <template #footer>
      <div class="flex justify-end gap-2 w-full">
        <UButton color="neutral" variant="ghost" @click="open = false">
          Batal
        </UButton>
        <UButton color="primary" icon="i-lucide-list-plus" :loading="saving" :disabled="!instruction.trim()" @click="submit">
          Jadikan todo
        </UButton>
      </div>
    </template>
  </UModal>
</template>
