import { computed, onScopeDispose, reactive, readonly, shallowRef, toValue, watch } from 'vue'
import type { MaybeRefOrGetter } from 'vue'

export type EndpointLatencyStatus = 'idle' | 'testing' | 'success' | 'error' | 'timeout'

export interface EndpointLatencyResult {
  status: EndpointLatencyStatus
  latencyMs: number | null
  statusCode: number | null
}

interface UseEndpointLatencyOptions {
  timeoutMs?: number
  fetcher?: typeof fetch
  now?: () => number
}

const DEFAULT_TIMEOUT_MS = 8_000

function idleResult(): EndpointLatencyResult {
  return {
    status: 'idle',
    latencyMs: null,
    statusCode: null,
  }
}

export function endpointHealthUrl(endpoint: string): string {
  const url = new URL(endpoint)
  url.pathname = '/health'
  url.search = ''
  url.hash = ''
  url.searchParams.set('_latency_probe', Date.now().toString())
  return url.toString()
}

export function useEndpointLatency(
  endpoints: MaybeRefOrGetter<readonly string[]>,
  options: UseEndpointLatencyOptions = {},
) {
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS
  const fetcher = options.fetcher ?? ((input, init) => globalThis.fetch(input, init))
  const now = options.now ?? (() => globalThis.performance?.now?.() ?? Date.now())
  const states = reactive<Record<string, EndpointLatencyResult>>({})
  const testingAll = shallowRef(false)
  const activeControllers = new Map<string, AbortController>()

  const normalizedEndpoints = computed(() =>
    Array.from(new Set(toValue(endpoints).map(endpoint => endpoint.trim()).filter(Boolean))),
  )

  watch(
    normalizedEndpoints,
    currentEndpoints => {
      const currentSet = new Set(currentEndpoints)
      for (const endpoint of currentEndpoints) {
        states[endpoint] ??= idleResult()
      }
      for (const endpoint of Object.keys(states)) {
        if (!currentSet.has(endpoint)) {
          activeControllers.get(endpoint)?.abort()
          activeControllers.delete(endpoint)
          delete states[endpoint]
        }
      }
    },
    { immediate: true },
  )

  const recommendedEndpoint = computed<string | null>(() => {
    let fastestEndpoint: string | null = null
    let fastestLatency = Number.POSITIVE_INFINITY

    for (const endpoint of normalizedEndpoints.value) {
      const result = states[endpoint]
      if (
        result?.status === 'success'
        && result.latencyMs !== null
        && result.latencyMs < fastestLatency
      ) {
        fastestEndpoint = endpoint
        fastestLatency = result.latencyMs
      }
    }

    return fastestEndpoint
  })

  async function testEndpoint(endpoint: string): Promise<void> {
    const normalizedEndpoint = endpoint.trim()
    if (!normalizedEndpoint) return

    activeControllers.get(normalizedEndpoint)?.abort()

    const controller = new AbortController()
    activeControllers.set(normalizedEndpoint, controller)
    states[normalizedEndpoint] = {
      status: 'testing',
      latencyMs: null,
      statusCode: null,
    }

    let timedOut = false
    const timeout = globalThis.setTimeout(() => {
      timedOut = true
      controller.abort()
    }, timeoutMs)
    const startedAt = now()

    try {
      const response = await fetcher(endpointHealthUrl(normalizedEndpoint), {
        method: 'GET',
        cache: 'no-store',
        credentials: 'omit',
        signal: controller.signal,
      })

      if (activeControllers.get(normalizedEndpoint) !== controller) return

      const latencyMs = Math.max(1, Math.round(now() - startedAt))
      states[normalizedEndpoint] = response.ok
        ? { status: 'success', latencyMs, statusCode: response.status }
        : { status: 'error', latencyMs, statusCode: response.status }
    } catch {
      if (activeControllers.get(normalizedEndpoint) !== controller) return

      states[normalizedEndpoint] = {
        status: timedOut ? 'timeout' : 'error',
        latencyMs: null,
        statusCode: null,
      }
    } finally {
      globalThis.clearTimeout(timeout)
      if (activeControllers.get(normalizedEndpoint) === controller) {
        activeControllers.delete(normalizedEndpoint)
      }
    }
  }

  async function testAll(): Promise<void> {
    if (testingAll.value || normalizedEndpoints.value.length === 0) return

    testingAll.value = true
    try {
      await Promise.all(normalizedEndpoints.value.map(endpoint => testEndpoint(endpoint)))
    } finally {
      testingAll.value = false
    }
  }

  onScopeDispose(() => {
    for (const controller of activeControllers.values()) {
      controller.abort()
    }
    activeControllers.clear()
  })

  return {
    states: readonly(states),
    testingAll: readonly(testingAll),
    recommendedEndpoint,
    testEndpoint,
    testAll,
  }
}
