import type { TextEntryPage, TextEntryQuery, TextFacets, TextSDKInfo, TextSource, TextSourceInput, TextTodoResult } from '~/types'

/** Teks Website: sumber (website pengirim), daftar teks per halaman/section, pemasangan skrip, dan todo ubah teks. */
export const useTexts = () => {
  const { apiBase } = useApi()
  const base = `${apiBase}/api/texts`
  const src = (id: number) => `${base}/sources/${id}`

  return {
    listSources: (projectId?: number | null) => $fetch<TextSource[]>(`${base}/sources`, { query: { project_id: projectId || undefined } }),
    createSource: (body: TextSourceInput) => $fetch<TextSource>(`${base}/sources`, { method: 'POST', body }),
    updateSource: (id: number, body: TextSourceInput) => $fetch<TextSource>(src(id), { method: 'PUT', body }),
    deleteSource: (id: number) => $fetch<{ deleted: boolean }>(src(id), { method: 'DELETE' }),
    rotateKey: (id: number) => $fetch<{ rotated: boolean }>(`${src(id)}/rotate-key`, { method: 'POST' }),
    getSDK: (id: number) => $fetch<TextSDKInfo>(`${src(id)}/sdk`),
    installSDK: (id: number, overwrite = false) =>
      $fetch<{ written: boolean, path: string, dir: string }>(`${src(id)}/sdk/install`, { method: 'POST', body: { overwrite } }),
    getFacets: (id: number, page?: string) => $fetch<TextFacets>(`${src(id)}/facets`, { query: { page: page || undefined } }),
    listEntries: (id: number, q: TextEntryQuery = {}) =>
      $fetch<TextEntryPage>(`${src(id)}/entries`, { query: { ...q, page: q.page || undefined, section: q.section || undefined, q: q.q || undefined, todo: q.todo || undefined } }),
    entryToTodo: (entryId: number, instruction: string, priority?: string) =>
      $fetch<TextTodoResult>(`${base}/entries/${entryId}/todo`, { method: 'POST', body: { instruction, priority } })
  }
}

export const todoStatusLabel: Record<string, { label: string, color: 'neutral' | 'primary' | 'success' | 'warning' }> = {
  pending: { label: 'menunggu', color: 'warning' },
  in_progress: { label: 'dikerjakan', color: 'primary' },
  suggested: { label: 'usulan', color: 'neutral' },
  completed: { label: 'selesai', color: 'success' },
  rejected: { label: 'ditolak', color: 'neutral' }
}
