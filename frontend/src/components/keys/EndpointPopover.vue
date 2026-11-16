<script setup lang="ts">
import { computed, onBeforeUnmount, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { classifyEndpointLatency, useEndpointLatency } from '@/composables/useEndpointLatency'
import type { EndpointLatencyResult } from '@/composables/useEndpointLatency'
import type { CustomEndpoint } from '@/types'

interface DisplayEndpoint extends CustomEndpoint {
  isDefault: boolean
  isRecommended: boolean
}

const props = withDefaults(defineProps<{
  apiBaseUrl: string
  customEndpoints: CustomEndpoint[]
  defaultEndpointDescription?: string
  autoTest?: boolean
}>(), {
  defaultEndpointDescription: '',
  autoTest: true,
})

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const copiedEndpoint = shallowRef<string | null>(null)

let copiedResetTimer: number | undefined

function normalizeDefaultEndpoint(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) return ''

  try {
    const url = new URL(trimmed)
    const pathname = url.pathname.replace(/\/+$/, '').replace(/\/v1$/i, '')
    const normalizedPath = pathname && pathname !== '/' ? pathname : ''
    return `${url.origin}${normalizedPath}${url.search}${url.hash}`
  } catch {
    return trimmed.replace(/\/+$/, '').replace(/\/v1$/i, '')
  }
}

const resolvedApiBaseUrl = computed(() =>
  normalizeDefaultEndpoint(props.apiBaseUrl)
    || normalizeDefaultEndpoint(globalThis.location?.origin || ''),
)

const allEndpoints = computed<DisplayEndpoint[]>(() => {
  const items: DisplayEndpoint[] = []
  if (resolvedApiBaseUrl.value) {
    items.push({
      name: t('keys.endpoints.title'),
      endpoint: resolvedApiBaseUrl.value,
      description: props.defaultEndpointDescription.trim(),
      isDefault: true,
      isRecommended: !props.customEndpoints.some(endpoint => endpoint.recommended),
    })
  }

  for (const endpoint of props.customEndpoints) {
    const normalizedEndpoint = endpoint.endpoint.trim()
    if (!normalizedEndpoint) continue
    items.push({
      ...endpoint,
      name: endpoint.name.trim() || normalizedEndpoint,
      endpoint: normalizedEndpoint,
      isDefault: false,
      isRecommended: endpoint.recommended === true,
    })
  }

  return items
})

const endpointUrls = computed(() => allEndpoints.value.map(item => item.endpoint))
const {
  states,
  testingAll,
  testEndpoint,
  testAll,
} = useEndpointLatency(endpointUrls)

const configuredEndpointsReady = computed(() =>
  Boolean(
    props.apiBaseUrl.trim()
    || props.customEndpoints.some(endpoint => endpoint.endpoint.trim()),
  ),
)
const autoTestStarted = shallowRef(false)

watch(
  [() => props.autoTest, configuredEndpointsReady],
  ([autoTest, ready]) => {
    if (!autoTest || !ready || autoTestStarted.value) return
    autoTestStarted.value = true
    void testAll()
  },
  { immediate: true, flush: 'post' },
)

async function copy(url: string) {
  const success = await copyToClipboard(url, t('keys.endpoints.copied'))
  if (!success) return

  copiedEndpoint.value = url
  if (copiedResetTimer !== undefined) {
    window.clearTimeout(copiedResetTimer)
  }
  copiedResetTimer = window.setTimeout(() => {
    if (copiedEndpoint.value === url) {
      copiedEndpoint.value = null
    }
  }, 1800)
}

function copyHint(endpoint: string): string {
  return copiedEndpoint.value === endpoint
    ? t('keys.endpoints.copiedHint')
    : t('keys.endpoints.copy')
}

function speedTestHint(item: DisplayEndpoint): string {
  return t('keys.endpoints.speedTestEndpoint', { name: item.name })
}

function resultLabel(result: EndpointLatencyResult | undefined): string {
  if (!result || result.status === 'idle') return t('keys.endpoints.notTested')
  if (result.status === 'testing') return t('keys.endpoints.testing')
  if (result.status === 'timeout') return t('keys.endpoints.timeout')
  if (result.status === 'error') {
    return result.statusCode
      ? t('keys.endpoints.httpError', { status: result.statusCode })
      : t('keys.endpoints.failed')
  }
  return `${result.latencyMs} ms`
}

function resultClasses(result: EndpointLatencyResult | undefined): string {
  switch (result?.status) {
    case 'success': {
      switch (classifyEndpointLatency(result.latencyMs)) {
        case 'fast':
          return 'bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-300 dark:ring-emerald-800'
        case 'moderate':
          return 'bg-amber-50 text-amber-700 ring-amber-200 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-800'
        default:
          return 'bg-red-50 text-red-700 ring-red-200 dark:bg-red-950/40 dark:text-red-300 dark:ring-red-800'
      }
    }
    case 'testing':
      return 'bg-amber-50 text-amber-700 ring-amber-200 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-800'
    case 'timeout':
    case 'error':
      return 'bg-red-50 text-red-700 ring-red-200 dark:bg-red-950/40 dark:text-red-300 dark:ring-red-800'
    default:
      return 'bg-gray-100 text-gray-500 ring-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:ring-dark-600'
  }
}

function resultLevel(result: EndpointLatencyResult | undefined): string {
  if (result?.status === 'success') return classifyEndpointLatency(result.latencyMs)
  return result?.status ?? 'idle'
}

function resultIcon(result: EndpointLatencyResult | undefined) {
  switch (result?.status) {
    case 'success':
      return 'checkCircle' as const
    case 'testing':
      return 'refresh' as const
    case 'timeout':
      return 'clock' as const
    case 'error':
      return 'xCircle' as const
    default:
      return 'clock' as const
  }
}

onBeforeUnmount(() => {
  if (copiedResetTimer !== undefined) {
    window.clearTimeout(copiedResetTimer)
  }
})
</script>

<template>
  <section
    v-if="allEndpoints.length > 0"
    class="endpoint-panel"
    :aria-label="t('keys.endpoints.title')"
  >
    <div class="endpoint-list">
      <div v-for="item in allEndpoints" :key="item.endpoint" class="endpoint-row">
        <div class="endpoint-title">
          <span class="endpoint-name" :title="item.description || item.name">
            {{ item.isDefault && item.description ? item.description : item.name }}
          </span>
          <span v-if="item.isDefault" class="endpoint-tag endpoint-tag-default">
            {{ t('keys.endpoints.default') }}
          </span>
          <span v-if="item.isRecommended" class="endpoint-tag endpoint-tag-recommended">
            {{ t('keys.endpoints.recommended') }}
          </span>
        </div>

        <button
          type="button"
          class="endpoint-url"
          :title="item.description || item.endpoint"
          @click="copy(item.endpoint)"
        >
          {{ item.endpoint }}
        </button>

        <div class="endpoint-actions">
          <span
            class="endpoint-latency inline-flex items-center justify-center gap-1 rounded text-[10px] font-medium tabular-nums ring-1 ring-inset"
            :class="resultClasses(states[item.endpoint])"
            :data-latency-level="resultLevel(states[item.endpoint])"
            role="status"
            aria-live="polite"
          >
            <Icon
              :name="resultIcon(states[item.endpoint])"
              size="xs"
              :class="states[item.endpoint]?.status === 'testing' ? 'animate-spin' : ''"
              :stroke-width="2"
            />
            {{ resultLabel(states[item.endpoint]) }}
          </span>
          <button
            type="button"
            class="endpoint-action"
            :class="{ 'endpoint-action-copied': copiedEndpoint === item.endpoint }"
            :title="copyHint(item.endpoint)"
            :aria-label="copyHint(item.endpoint)"
            @click="copy(item.endpoint)"
          >
            <Icon :name="copiedEndpoint === item.endpoint ? 'check' : 'copy'" size="xs" :stroke-width="2" />
          </button>
          <button
            type="button"
            class="endpoint-action"
            :disabled="states[item.endpoint]?.status === 'testing'"
            :title="speedTestHint(item)"
            :aria-label="speedTestHint(item)"
            @click="testEndpoint(item.endpoint)"
          >
            <Icon name="bolt" size="xs" :stroke-width="2" />
          </button>
        </div>
      </div>
      <button
        type="button"
        class="endpoint-action endpoint-test-all"
        :disabled="testingAll"
        :title="testingAll ? t('keys.endpoints.testingAll') : t('keys.endpoints.speedTestAll')"
        :aria-label="testingAll ? t('keys.endpoints.testingAll') : t('keys.endpoints.speedTestAll')"
        @click="testAll"
      >
        <Icon name="refresh" size="xs" :class="testingAll ? 'animate-spin' : ''" :stroke-width="2" />
      </button>
    </div>
  </section>
</template>

<style scoped>
.endpoint-panel { min-width: 0; }
.endpoint-list { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.endpoint-row {
  display: flex;
  align-items: center;
  flex: 0 1 auto;
  max-width: 100%;
  min-height: 34px;
  gap: 8px;
  padding: 4px 7px 4px 10px;
  border: 1px solid var(--kc-line, #e5e7eb);
  border-radius: 8px;
  color: var(--kc-muted, #606662);
  background: var(--kc-panel, #fff);
}
.endpoint-title, .endpoint-actions { display: flex; align-items: center; gap: 5px; flex-shrink: 0; }
.endpoint-name { font-size: 12px; font-weight: 500; white-space: nowrap; }
.endpoint-tag { border-radius: 4px; padding: 1px 4px; font-size: 10px; line-height: 1.5; white-space: nowrap; }
.endpoint-tag-default { color: var(--kc-warm, #98601d); background: color-mix(in srgb, var(--kc-warm, #98601d) 8%, transparent); }
.endpoint-tag-recommended { color: var(--kc-accent, #c45100); background: var(--kc-accent-soft, #fef8f4); }
.endpoint-url {
  min-width: 0;
  padding-left: 8px;
  border-left: 1px solid var(--kc-line, #e5e7eb);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 11px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  text-align: left;
  color: var(--kc-muted, #606662);
}
.endpoint-url:hover, .endpoint-action:hover { color: var(--kc-accent, #c45100); }
.endpoint-url:focus-visible, .endpoint-action:focus-visible { outline: 2px solid var(--kc-accent, #c45100); outline-offset: 2px; }
.endpoint-latency { min-width: 62px; height: 23px; padding: 0 5px; white-space: nowrap; }
.endpoint-latency[data-latency-level="idle"] { color: var(--kc-muted, #606662); background: var(--kc-panel-muted, #f7f7f3); --tw-ring-color: var(--kc-line, #e5e7eb); }
.endpoint-action { display: inline-flex; height: 24px; width: 24px; flex-shrink: 0; align-items: center; justify-content: center; border-radius: 5px; color: var(--kc-muted, #606662); transition: color .15s, background .15s; }
.endpoint-action:hover { background: var(--kc-hover, #f3f6f2); }
.endpoint-action:disabled { cursor: wait; opacity: .55; }
.endpoint-action-copied { color: var(--kc-accent, #c45100); }
.endpoint-test-all { height: 32px; width: 32px; }
@media (max-width: 639px) {
  .endpoint-row { flex: 1 1 100%; flex-wrap: wrap; gap: 6px; padding: 7px 9px; }
  .endpoint-title { order: 0; }
  .endpoint-actions { order: 1; margin-left: auto; }
  .endpoint-url { order: 2; flex-basis: 100%; padding: 0; border: 0; }
}
</style>
