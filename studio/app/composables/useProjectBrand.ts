/** Warna merek proyek (GET/PUT/DELETE /api/projects/:id/brand-colors): tema PDF Dokumen Proyek & acuan tampilan. */
export interface BrandColors {
  primary: string
  secondary?: string
  accent?: string
  source?: 'manual' | 'detected' | ''
  evidence?: string[]
}

export interface BrandColorsState {
  colors: BrandColors
  saved: boolean
  updated_at: string | null
}

export function useProjectBrand() {
  const api = useApi()
  const url = (pid: number) => `${api.apiBase}/api/projects/${pid}/brand-colors`
  return {
    // detect = true: deteksi ulang dari kode walau sudah tersimpan (pratinjau, tidak menimpa).
    get: (pid: number, detect = false) => $fetch<BrandColorsState>(url(pid), { query: detect ? { detect: 1 } : {} }),
    save: (pid: number, colors: BrandColors) => $fetch<BrandColorsState>(url(pid), { method: 'PUT', body: colors }),
    reset: (pid: number) => $fetch<BrandColorsState>(url(pid), { method: 'DELETE' })
  }
}
