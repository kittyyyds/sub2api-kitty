import { computed, onMounted, onUnmounted, readonly, shallowRef } from 'vue'

import { aiIntelligenceAPI, type AIIntelligencePayload } from '@/api/aiIntelligence'

const CACHE_KEY = 'kitty-ai-intelligence:last-valid-v1'
const AUTO_REFRESH_MS = 5 * 60 * 1000

interface StoredSnapshot {
  savedAt: string
  payload: AIIntelligencePayload
}

function readStoredSnapshot(): AIIntelligencePayload | null {
  if (typeof window === 'undefined') return null
  try {
    const raw = window.localStorage.getItem(CACHE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as StoredSnapshot
    if (!parsed?.payload || !Array.isArray(parsed.payload.points) || !parsed.payload.service_status) {
      return null
    }
    return { ...parsed.payload, stale: true }
  } catch {
    return null
  }
}

function storeSnapshot(payload: AIIntelligencePayload): void {
  if (typeof window === 'undefined') return
  try {
    const snapshot: StoredSnapshot = {
      savedAt: new Date().toISOString(),
      payload: { ...payload, stale: false },
    }
    window.localStorage.setItem(CACHE_KEY, JSON.stringify(snapshot))
  } catch {
    // Storage can be unavailable in private browsing; the in-memory snapshot still works.
  }
}

function errorMessage(error: unknown): string {
  if (error && typeof error === 'object' && 'message' in error) {
    const message = (error as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }
  return ''
}

export function useAIIntelligence() {
  const payload = shallowRef<AIIntelligencePayload | null>(readStoredSnapshot())
  const loading = shallowRef(false)
  const error = shallowRef('')
  let requestController: AbortController | null = null
  let refreshTimer: number | null = null

  const stale = computed(() => payload.value?.stale ?? false)

  async function refresh(): Promise<void> {
    requestController?.abort()
    const controller = new AbortController()
    requestController = controller
    loading.value = true
    error.value = ''
    try {
      const next = await aiIntelligenceAPI.get({ signal: controller.signal })
      if (controller.signal.aborted) return
      payload.value = next
      if (!next.stale) storeSnapshot(next)
    } catch (cause) {
      if (controller.signal.aborted) return
      error.value = errorMessage(cause)
      if (payload.value) payload.value = { ...payload.value, stale: true }
    } finally {
      if (requestController === controller) loading.value = false
    }
  }

  onMounted(() => {
    void refresh()
    refreshTimer = window.setInterval(() => void refresh(), AUTO_REFRESH_MS)
  })

  onUnmounted(() => {
    requestController?.abort()
    if (refreshTimer !== null) window.clearInterval(refreshTimer)
  })

  return {
    payload: readonly(payload),
    loading: readonly(loading),
    error: readonly(error),
    stale,
    refresh,
  }
}
