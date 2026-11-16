import { onUnmounted, ref, watch, type Ref } from 'vue'
import { getAccountRequestStats, type OpsAccountRequestStats } from '@/api/admin/accountRequestStats'

// Metrics are optional panel telemetry; never change an account or a gateway rule.
export function useAccountRequestMetrics(accountIds: Readonly<Ref<number[]>>, enabled: Ref<boolean>) {
  const stats = ref<Record<string, OpsAccountRequestStats>>({})
  const loading = ref(false)
  const unavailable = ref(false)
  let generation = 0
  let controller: AbortController | null = null
  let timer: ReturnType<typeof setInterval> | undefined

  async function refresh() {
    if (!enabled.value || loading.value || document.hidden) return
    const ids = [...new Set(accountIds.value)].filter(id => id > 0)
    const current = ++generation
    controller?.abort()
    controller = new AbortController()
    const signal = controller.signal
    loading.value = true
    unavailable.value = false
    try {
      const next: Record<string, OpsAccountRequestStats> = {}
      for (let offset = 0; offset < ids.length; offset += 100) {
        Object.assign(next, await getAccountRequestStats(ids.slice(offset, offset + 100), signal))
      }
      if (current === generation) stats.value = next
    } catch {
      if (current === generation) { stats.value = {}; unavailable.value = true }
    } finally {
      if (current === generation) loading.value = false
    }
  }

  watch([accountIds, enabled], () => {
    generation++
    controller?.abort()
    loading.value = false
    stats.value = {}
    if (timer) clearInterval(timer)
    timer = undefined
    if (enabled.value) {
      void refresh()
      timer = setInterval(() => { void refresh() }, 30_000)
    }
  }, { immediate: true })
  onUnmounted(() => { generation++; controller?.abort(); if (timer) clearInterval(timer) })
  return { stats, loading, unavailable, refresh }
}
