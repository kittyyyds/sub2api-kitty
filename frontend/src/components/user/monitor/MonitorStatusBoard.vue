<template>
  <div class="status-board">
    <div class="status-tools">
      <div class="status-filters" :aria-label="t('homeOverview.allProviders')">
        <button v-for="provider in providers" :key="provider.key" type="button" :aria-pressed="selected === provider.key" @click="selected = provider.key">{{ provider.label }} <span>{{ provider.count }}</span></button>
      </div>
      <input v-model="search" class="input" type="search" :aria-label="t('homeOverview.channelSearch')" :placeholder="t('homeOverview.channelSearch')" />
    </div>
    <div v-if="loading && !items.length" class="p-12 text-center text-gray-500" role="status">{{ t('common.loading') }}</div>
    <div v-else-if="sections.length" class="status-providers" data-testid="monitor-provider-grid">
      <section v-for="section in sections" :key="section.key" class="status-provider card" data-monitor-provider-section :data-provider="section.key">
        <h2><button type="button" class="status-provider-heading" :aria-expanded="!collapsed[section.key]" :aria-controls="`provider-content-${section.key}`" @click="collapsed[section.key] = !collapsed[section.key]">
          <span class="status-provider-icon"><ProviderIcon :provider="section.key" :size="18" /></span>
          <strong>{{ section.label }}</strong><span>{{ t('monitorCommon.monitorCount', { n: section.items.length }) }}</span>
          <Icon name="chevronDown" size="sm" class="ml-auto transition-transform" :class="{ '-rotate-90': collapsed[section.key] }" />
        </button></h2>
        <div v-show="!collapsed[section.key]" :id="`provider-content-${section.key}`">
          <MonitorStatusRow v-for="item in section.items" :key="item.id" :item="item" :availability-value="availability(item)" :rate-multiplier="rate(item)" @click="emit('cardClick', item)" />
        </div>
      </section>
    </div>
    <p v-else class="p-12 text-center text-gray-500">{{ t('homeOverview.channelEmpty') }}</p>
  </div>
</template>
<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView, UserMonitorDetail } from '@/api/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import Icon from '@/components/icons/Icon.vue'
import ProviderIcon from './ProviderIcon.vue'
import MonitorStatusRow from './MonitorStatusRow.vue'
const props = defineProps<{ items: UserMonitorView[]; window: '7d' | '15d' | '30d'; loading: boolean; detailCache: Record<number, UserMonitorDetail>; rateByGroupName: Record<string, number> }>()
const emit = defineEmits<{ cardClick: [item: UserMonitorView] }>()
const { t } = useI18n()
const { providerLabel } = useChannelMonitorFormat()
const selected = ref('all'), search = ref('')
const collapsed = reactive<Record<string, boolean>>({})
const keys = computed(() => Array.from(new Set(props.items.map(i => i.provider))))
const providers = computed(() => [{ key: 'all', label: t('homeOverview.allProviders'), count: props.items.length }, ...keys.value.map(key => ({ key, label: providerLabel(key), count: props.items.filter(i => i.provider === key).length }))])
const sections = computed(() => keys.value.filter(key => selected.value === 'all' || selected.value === key).map(key => ({ key, label: providerLabel(key), items: props.items.filter(i => i.provider === key && `${i.name} ${i.group_name} ${i.primary_model}`.toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase())) })).filter(section => section.items.length))
function rate(item: UserMonitorView) { return item.group_name ? props.rateByGroupName[item.group_name.trim().toLocaleLowerCase()] ?? null : null }
function availability(item: UserMonitorView) {
  if (props.window === '7d') return item.availability_7d ?? null
  const primary = props.detailCache[item.id]?.models.find(m => m.model === item.primary_model)
  return (props.window === '15d' ? primary?.availability_15d : primary?.availability_30d) ?? null
}
</script>
<style scoped>
.status-board { min-width: 0; }
.status-tools { display: flex; align-items: center; flex-wrap: wrap; justify-content: space-between; gap: 14px; margin-bottom: 20px; }
.status-tools input { max-width: 240px; }
.status-filters { display: flex; flex-wrap: wrap; gap: 6px; }
.status-filters button { border: 1px solid var(--kc-line); border-radius: 9px; padding: 8px 12px; color: var(--kc-muted); font-size: 12px; }
.status-filters button[aria-pressed=true] { background: var(--kc-accent-soft); color: var(--kc-accent-strong); border-color: var(--kc-accent); }
.status-filters span { margin-left: 4px; opacity: .7; }
.status-providers { display: grid; grid-template-columns: minmax(0,1fr); align-items: start; gap: 20px; }
.status-provider { min-width: 0; overflow: hidden; }
.status-provider-heading { display: flex; align-items: center; width: 100%; gap: 10px; padding: 18px 20px; color: var(--kc-muted); font-size: 11px; text-align: left; }
.status-provider-heading strong { color: var(--kc-ink); font-size: 14px; }
.status-provider-icon { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 10px; color: var(--kc-accent); background: var(--kc-accent-soft); }
@media(min-width: 1100px) { .status-providers { grid-template-columns: repeat(2,minmax(0,1fr)); } }
</style>
