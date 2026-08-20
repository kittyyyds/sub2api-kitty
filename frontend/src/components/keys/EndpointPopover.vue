<script setup lang="ts">
import { computed, onBeforeUnmount, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useEndpointLatency } from '@/composables/useEndpointLatency'
import type { EndpointLatencyResult } from '@/composables/useEndpointLatency'
import type { CustomEndpoint } from '@/types'

interface DisplayEndpoint extends CustomEndpoint {
  isDefault: boolean
}

const props = defineProps<{
  apiBaseUrl: string
  customEndpoints: CustomEndpoint[]
}>()

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
      description: '',
      isDefault: true,
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
    })
  }

  return items
})

const endpointUrls = computed(() => allEndpoints.value.map(item => item.endpoint))
const {
  states,
  testingAll,
  recommendedEndpoint,
  testEndpoint,
  testAll,
} = useEndpointLatency(endpointUrls)

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
    case 'success':
      return 'bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-300 dark:ring-emerald-800'
    case 'testing':
      return 'bg-amber-50 text-amber-700 ring-amber-200 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-800'
    case 'timeout':
    case 'error':
      return 'bg-red-50 text-red-700 ring-red-200 dark:bg-red-950/40 dark:text-red-300 dark:ring-red-800'
    default:
      return 'bg-gray-100 text-gray-500 ring-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:ring-dark-600'
  }
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
    class="endpoint-panel card w-full overflow-hidden"
    :aria-label="t('keys.endpoints.title')"
  >
    <div class="flex min-h-11 items-center justify-between gap-3 border-b border-gray-200 bg-gray-50/80 px-3 py-2 dark:border-dark-700 dark:bg-dark-800/80">
      <div class="flex min-w-0 items-center gap-2">
        <Icon name="server" size="sm" class="shrink-0 text-primary-500 dark:text-primary-400" />
        <h3 class="truncate text-xs font-semibold text-gray-700 dark:text-gray-200">
          {{ t('keys.endpoints.title') }}
        </h3>
        <span class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] tabular-nums text-gray-500 dark:bg-dark-700 dark:text-gray-400">
          {{ allEndpoints.length }}
        </span>
      </div>

      <button
        type="button"
        class="btn btn-secondary btn-sm h-7 shrink-0 gap-1.5 px-2 text-xs"
        :disabled="testingAll"
        @click="testAll"
      >
        <Icon
          name="refresh"
          size="xs"
          :class="testingAll ? 'animate-spin' : ''"
          :stroke-width="2"
        />
        {{ testingAll ? t('keys.endpoints.testingAll') : t('keys.endpoints.speedTestAll') }}
      </button>
    </div>

    <div class="endpoint-list divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
      <div
        v-for="item in allEndpoints"
        :key="item.endpoint"
        class="endpoint-row flex min-w-0 flex-col gap-2 px-3 py-2 transition-colors sm:flex-row sm:items-center"
      >
        <div class="flex w-full min-w-0 flex-1 items-center gap-1.5">
          <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-primary-500 dark:bg-primary-400"></span>
          <span class="shrink-0 text-xs font-medium text-gray-700 dark:text-gray-200">
            {{ item.name }}
          </span>
          <span
            v-if="item.isDefault"
            class="shrink-0 rounded bg-primary-50 px-1 py-px text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
          >
            {{ t('keys.endpoints.default') }}
          </span>
          <span
            v-if="recommendedEndpoint === item.endpoint"
            class="shrink-0 rounded bg-emerald-50 px-1 py-px text-[10px] font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
          >
            {{ t('keys.endpoints.recommended') }}
          </span>
          <span class="text-gray-300 dark:text-dark-500">|</span>
          <button
            type="button"
            class="min-w-0 truncate font-mono text-[11px] text-gray-500 hover:text-primary-600 focus:outline-none focus-visible:underline dark:text-gray-400 dark:hover:text-primary-400"
            :title="item.description || item.endpoint"
            @click="copy(item.endpoint)"
          >
            {{ item.endpoint }}
          </button>
        </div>

        <div class="flex shrink-0 items-center justify-end gap-1 sm:justify-start">
          <span
            class="inline-flex h-6 min-w-[4.5rem] items-center justify-center gap-1 rounded px-1.5 text-[10px] font-medium tabular-nums ring-1 ring-inset"
            :class="resultClasses(states[item.endpoint])"
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
            class="inline-flex h-6 w-6 items-center justify-center rounded text-gray-400 transition-colors hover:bg-gray-200 hover:text-primary-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-gray-500 dark:hover:bg-dark-700 dark:hover:text-primary-400"
            :class="copiedEndpoint === item.endpoint ? 'text-emerald-500 dark:text-emerald-400' : ''"
            :title="copyHint(item.endpoint)"
            :aria-label="copyHint(item.endpoint)"
            @click="copy(item.endpoint)"
          >
            <Icon
              :name="copiedEndpoint === item.endpoint ? 'check' : 'copy'"
              size="xs"
              :stroke-width="2"
            />
          </button>

          <button
            type="button"
            class="inline-flex h-6 w-6 items-center justify-center rounded text-gray-400 transition-colors hover:bg-amber-50 hover:text-amber-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-500 disabled:cursor-wait disabled:opacity-60 dark:text-gray-500 dark:hover:bg-amber-950/40 dark:hover:text-amber-300"
            :disabled="states[item.endpoint]?.status === 'testing'"
            :title="speedTestHint(item)"
            :aria-label="speedTestHint(item)"
            @click="testEndpoint(item.endpoint)"
          >
            <Icon name="bolt" size="xs" :stroke-width="2" />
          </button>
        </div>
      </div>
    </div>
  </section>
</template>
