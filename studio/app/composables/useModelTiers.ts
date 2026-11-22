import type { TierModels } from '~/utils/todoCardMeta'
import { useApiBase } from './api/base'

// Model setara per kelas model todo (utama ≈ Sonnet, kritis ≈ Opus, ekstraksi ≈ Haiku) untuk agent coding yang
// terpasang di komputer ini. Diambil sekali dan dipakai bersama semua kartu todo; build Community (tanpa
// orkestrator) → null, lencana memakai nama kelas saja.

let loading: Promise<void> | null = null

export const useModelTiers = () => {
  const tiers = useState<TierModels | null>('orchestrator_model_tiers', () => null)
  const apiBase = useApiBase()
  const { isLocked } = useEdition()

  const load = async () => {
    if (import.meta.server || isLocked('orchestrator')) return
    if (!loading) {
      loading = $fetch<{ tiers: TierModels }>(`${apiBase}/api/orchestrator/model-tiers`)
        .then((r) => { tiers.value = r.tiers || null })
        .catch(() => { tiers.value = null })
    }
    await loading
  }

  onMounted(load)
  return { tiers, load }
}
