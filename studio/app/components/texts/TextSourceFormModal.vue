<script setup lang="ts">
import type { TextSource } from '~/types'

// Hubungkan satu website ke Teks Website. Website-lah yang mengirim teks ke engine ini lewat skrip kecil.
const open = defineModel<boolean>('open', { default: false })
const emit = defineEmits<{ (e: 'created', src: TextSource): void }>()

const toast = useToast()
const { createSource } = useTexts()
const { projects, activeProjectId } = useActiveProject()

const name = ref('')
const projectId = ref<number | undefined>(undefined)
const componentId = ref<number | undefined>(undefined)
const saving = ref(false)

const projectItems = computed(() => projects.value.map(p => ({ label: p.name, value: p.id })))
const componentItems = computed(() => {
  const p = projects.value.find(x => x.id === projectId.value)
  const comps = (p?.components || []).filter(c => c.role?.startsWith('frontend') || c.role === 'other')
  return (comps.length ? comps : p?.components || []).map(c => ({ label: `${c.name} (${c.role})`, value: c.id }))
})

watch(open, (v) => {
  if (!v) return
  name.value = ''
  projectId.value = activeProjectId.value || undefined
  componentId.value = componentItems.value[0]?.value
})
watch(projectId, () => {
  componentId.value = componentItems.value[0]?.value
})

const submit = async () => {
  saving.value = true
  try {
    const src = await createSource({ name: name.value.trim() || undefined, project_id: projectId.value ?? null, component_id: componentId.value ?? null })
    toast.add({ title: 'Sumber teks dibuat', description: 'Pasang skripnya di website lalu buka halamannya.', icon: 'i-lucide-type', color: 'success' })
    emit('created', src)
    open.value = false
  } catch (err: any) {
    toast.add({ title: 'Gagal membuat sumber', description: err?.data?.error || err?.message, icon: 'i-lucide-alert-circle', color: 'error' })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UModal v-model:open="open" title="Hubungkan website" description="Website mengirim teks yang tampil ke vibeasik; vibeasik tidak mengambil apa pun dari website." :ui="{ content: 'sm:max-w-xl' }">
    <template #body>
      <div class="space-y-4">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <UFormField label="Proyek">
            <USelect v-model="projectId" :items="projectItems" placeholder="Pilih proyek" class="w-full" />
          </UFormField>
          <UFormField label="Komponen website" help="Dipakai untuk menulis skrip ke folder komponen & menautkan todo.">
            <USelect v-model="componentId" :items="componentItems" placeholder="Pilih komponen" class="w-full" :disabled="!projectId" />
          </UFormField>
        </div>
        <UFormField label="Nama sumber" help="Kosong = nama komponen. Bedakan bila ada beberapa lingkungan (dev, staging).">
          <UInput v-model="name" placeholder="mis. Website dev" class="w-full" />
        </UFormField>
      </div>
    </template>
    <template #footer>
      <div class="flex justify-end gap-2 w-full">
        <UButton color="neutral" variant="ghost" @click="open = false">
          Batal
        </UButton>
        <UButton color="primary" icon="i-lucide-link" :loading="saving" :disabled="!projectId" @click="submit">
          Hubungkan
        </UButton>
      </div>
    </template>
  </UModal>
</template>
