<template>
  <div>
    <div v-if="loading && items.length === 0" class="space-y-6">
      <section v-for="section in 3" :key="section">
        <div class="flex items-center gap-2">
          <div class="h-6 w-6 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700"></div>
          <div class="h-4 w-20 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
          <div class="h-3 w-10 animate-pulse rounded bg-gray-100 dark:bg-dark-700"></div>
        </div>
        <div class="mt-3 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          <div
            v-for="card in 4"
            :key="card"
            class="min-h-[168px] w-full animate-pulse rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex items-start gap-3">
              <div class="h-7 w-7 rounded-md bg-gray-100 dark:bg-dark-700"></div>
              <div class="flex-1 space-y-2">
                <div class="h-3.5 w-2/3 rounded bg-gray-200 dark:bg-dark-700"></div>
                <div class="h-3 w-1/2 rounded bg-gray-100 dark:bg-dark-700"></div>
              </div>
              <div class="h-3 w-12 rounded bg-gray-100 dark:bg-dark-700"></div>
            </div>
            <div class="mt-4 grid grid-cols-2 gap-4">
              <div class="h-6 rounded bg-gray-50 dark:bg-dark-900/40"></div>
              <div class="h-6 rounded bg-gray-50 dark:bg-dark-900/40"></div>
            </div>
            <div class="mt-4 h-5 w-full rounded bg-gray-100 dark:bg-dark-900/40"></div>
          </div>
        </div>
      </section>
    </div>

    <div v-else-if="compact" class="monitor-compact-board">
      <div class="monitor-compact-tools">
        <div class="monitor-provider-filters" :aria-label="ht('homeOverview.allProviders')"><button v-for="provider in compactProviders" :key="provider.key" type="button" :aria-pressed="selectedProvider === provider.key" @click="selectedProvider = provider.key">{{ provider.label }} <span>{{ provider.count }}</span></button></div>
        <input v-model="search" type="search" :aria-label="ht('homeOverview.channelSearch')" :placeholder="ht('homeOverview.channelSearch')" />
      </div>
      <div v-if="compactItems.length" class="monitor-compact-grid"><MonitorCard v-for="item in compactItems" :key="item.id" :item="item" :availability-value="resolveAvailability(item)" :rate-multiplier="resolveRateMultiplier(item)" @click="emit('cardClick', item)" /></div>
      <p v-else class="home-empty">{{ ht('homeOverview.channelEmpty') }}</p>
    </div>
    <div v-else class="space-y-6">
      <section
        v-for="section in providerSections"
        :key="section.key"
        data-monitor-provider-section
        :data-provider="section.key"
      >
        <div class="flex items-center gap-2">
          <div class="flex min-w-0 items-center gap-2">
            <span
              v-if="section.provider"
              class="grid h-6 w-6 shrink-0 place-items-center rounded-md"
              :class="section.iconClass"
            >
              <ProviderIcon :provider="section.provider" :size="15" />
            </span>
            <span v-else class="h-2 w-2 shrink-0 rounded-full bg-gray-400"></span>
            <h2 class="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">
              {{ section.label }}
            </h2>
          </div>
          <span class="shrink-0 whitespace-nowrap text-[11px] text-gray-400 dark:text-gray-500">
            {{ t('monitorCommon.monitorCount', { n: section.items.length }) }}
          </span>
        </div>

        <div
          v-if="section.items.length > 0"
          class="mt-3 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4"
        >
          <MonitorCard
            v-for="item in section.items"
            :key="item.id"
            :item="item"
            :availability-value="resolveAvailability(item)"
            :rate-multiplier="resolveRateMultiplier(item)"
            @click="emit('cardClick', item)"
          />
        </div>
        <div
          v-else
          class="mt-3 flex h-24 items-center justify-center rounded-lg border border-dashed border-gray-200 text-sm text-gray-400 dark:border-dark-700 dark:text-gray-500"
        >
          {{ t('common.noData') }}
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed, ref } from 'vue'
import type { UserMonitorView, UserMonitorDetail, Provider } from '@/api/channelMonitor'
import {
  PROVIDER_ANTHROPIC,
  PROVIDER_GROK,
  PROVIDER_OPENAI,
} from '@/constants/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import MonitorCard from './MonitorCard.vue'
import ProviderIcon from './ProviderIcon.vue'

interface MonitorSection {
  key: 'openai' | 'anthropic' | 'grok' | 'other'
  label: string
  provider?: Provider
  iconClass: string
  items: UserMonitorView[]
}

const props = defineProps<{
  compact?: boolean
  items: UserMonitorView[]
  window: '7d' | '15d' | '30d'
  loading: boolean
  detailCache: Record<number, UserMonitorDetail>
  rateByGroupName: Record<string, number>
}>()

const emit = defineEmits<{
  (e: 'cardClick', item: UserMonitorView): void
}>()

const { t } = useI18n()
const { t: ht } = useI18n()
const { providerLabel } = useChannelMonitorFormat()
const selectedProvider = ref('all'), search = ref('')
const compactProviders = computed(() => [{ key: 'all', label: ht('homeOverview.allProviders'), count: props.items.length }, ...Array.from(new Set(props.items.map(i => i.provider))).map(provider => ({ key: provider, label: providerLabel(provider), count: props.items.filter(i => i.provider === provider).length }))])
const compactItems = computed(() => props.items.filter(item => (selectedProvider.value === 'all' || selectedProvider.value === item.provider) && `${item.name} ${item.group_name} ${item.primary_model}`.toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase())))

const providerSections = computed<MonitorSection[]>(() => {
  const openAIItems = props.items.filter(item => item.provider === PROVIDER_OPENAI)
  const anthropicItems = props.items.filter(item => item.provider === PROVIDER_ANTHROPIC)
  const grokItems = props.items.filter(item => item.provider === PROVIDER_GROK)
  const otherItems = props.items.filter(
    item =>
      item.provider !== PROVIDER_OPENAI &&
      item.provider !== PROVIDER_ANTHROPIC &&
      item.provider !== PROVIDER_GROK,
  )

  return [
    {
      key: 'openai',
      label: providerLabel(PROVIDER_OPENAI),
      provider: PROVIDER_OPENAI,
      iconClass: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-300',
      items: openAIItems,
    },
    {
      key: 'anthropic',
      label: providerLabel(PROVIDER_ANTHROPIC),
      provider: PROVIDER_ANTHROPIC,
      iconClass: 'bg-orange-50 text-orange-600 dark:bg-orange-500/10 dark:text-orange-300',
      items: anthropicItems,
    },
    {
      key: 'grok',
      label: providerLabel(PROVIDER_GROK),
      provider: PROVIDER_GROK,
      iconClass: 'bg-zinc-100 text-zinc-700 dark:bg-zinc-500/10 dark:text-zinc-200',
      items: grokItems,
    },
    {
      key: 'other',
      label: t('channelStatus.providerSections.other'),
      iconClass: '',
      items: otherItems,
    },
  ]
})

function normalizeGroupName(name: string): string {
  return name.trim().toLocaleLowerCase()
}

function resolveRateMultiplier(item: UserMonitorView): number | null {
  if (!item.group_name) return null
  return props.rateByGroupName[normalizeGroupName(item.group_name)] ?? null
}

function resolveAvailability(item: UserMonitorView): number | null {
  if (props.window === '7d') {
    return item.availability_7d ?? null
  }
  const detail = props.detailCache[item.id]
  if (!detail) return null
  const primary = detail.models.find(m => m.model === item.primary_model)
  if (!primary) return null
  return props.window === '15d' ? primary.availability_15d ?? null : primary.availability_30d ?? null
}
</script>
