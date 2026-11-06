<template>
  <AppLayout>
    <div class="channel-status-page">
    <section class="card channel-status-summary">
    <header class="channel-status-heading"><div><h2>{{ ht('homeOverview.channel') }}</h2><p>{{ ht('homeOverview.channelHint') }}</p></div><span>{{ ht('homeOverview.channelCount', { n: items.length }) }}</span></header>
    <div v-if="items.length" class="channel-status-counts">
      <span v-for="count in statusCounts" :key="count.status" :class="`is-${count.tone}`"><strong>{{ count.count }}</strong> {{ count.label }}</span>
    </div>
    <MonitorHero
      :overall-status="overallStatus"
      :interval-seconds="DEFAULT_INTERVAL_SECONDS"
      :window="currentWindow"
      :loading="loading"
      :has-data="items.length > 0"
      :auto-refresh="autoRefresh"
      @update:window="handleWindowChange"
      @refresh="refresh()"
    />
    </section>

    <MonitorStatusBoard
      :items="items"
      :window="currentWindow"
      :loading="loading"
      :detail-cache="detailCache"
      :rate-by-group-name="rateByGroupName"
      @card-click="openDetail"
    />
    </div>

    <MonitorDetailDialog
      :show="showDetail"
      :monitor-id="detailTarget?.id ?? null"
      :title="detailTitle"
      @close="closeDetail"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  list as listChannelMonitorViews,
  status as fetchChannelMonitorDetail,
  type UserMonitorView,
  type UserMonitorDetail,
} from '@/api/channelMonitor'
import { getAvailable, getUserGroupRates } from '@/api/groups'
import AppLayout from '@/components/layout/AppLayout.vue'
import MonitorHero, {
  type MonitorWindow,
  type OverallStatus,
} from '@/components/user/monitor/MonitorHero.vue'
import MonitorStatusBoard from '@/components/user/monitor/MonitorStatusBoard.vue'
import MonitorDetailDialog from '@/components/user/MonitorDetailDialog.vue'
import { DEFAULT_INTERVAL_SECONDS, STATUS_OPERATIONAL } from '@/constants/channelMonitor'
import { useAutoRefresh } from '@/composables/useAutoRefresh'
import '@/styles/dashboard-home.css'


const { t } = useI18n()
const { t: ht } = useI18n()
const appStore = useAppStore()

// ── State ──
const items = ref<UserMonitorView[]>([])
const loading = ref(false)
const currentWindow = ref<MonitorWindow>('7d')
const detailCache = reactive<Record<number, UserMonitorDetail>>({})
const rateByGroupName = ref<Record<string, number>>({})
const showDetail = ref(false)
const detailTarget = ref<UserMonitorView | null>(null)

let abortController: AbortController | null = null

const autoRefresh = useAutoRefresh({
  storageKey: 'channel-status-auto-refresh',
  intervals: [30, 60, 120] as const,
  defaultInterval: DEFAULT_INTERVAL_SECONDS,
  defaultEnabled: true,
  onRefresh: () => refresh(true),
  shouldPause: () => document.hidden || loading.value,
})

function normalizeGroupName(name: string): string {
  return name.trim().toLocaleLowerCase()
}

// ── Computed ──
const overallStatus = computed<OverallStatus>(() => {
  if (items.value.length === 0) return 'operational'
  for (const it of items.value) {
    if (it.primary_status === 'failed' || it.primary_status === 'error') return 'degraded'
    if (it.primary_status !== STATUS_OPERATIONAL) return 'degraded'
  }
  return 'operational'
})

const detailTitle = computed(() => {
  return detailTarget.value?.name || t('channelStatus.detailTitle')
})

const statusCounts = computed(() => [
  { status: 'operational', tone: 'good', label: t('monitorCommon.cardStatus.operational') },
  { status: 'degraded', tone: 'warn', label: t('monitorCommon.cardStatus.degraded') },
  { status: 'failed', tone: 'bad', label: t('monitorCommon.cardStatus.failed') },
  { status: 'unknown', tone: 'muted', label: t('monitorCommon.cardStatus.error') },
].map(row => ({ ...row, count: items.value.filter(item =>
  row.status === 'failed' ? ['failed', 'error'].includes(item.primary_status)
    : row.status === 'unknown' ? !['operational', 'degraded', 'failed', 'error'].includes(item.primary_status)
      : item.primary_status === row.status).length })).filter(row => row.count > 0))

// ── Loaders ──
async function reload(silent = false) {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  if (!silent) loading.value = true
  try {
    const res = await listChannelMonitorViews({ signal: ctrl.signal })
    if (ctrl.signal.aborted || abortController !== ctrl) return
    items.value = res.items || []
  } catch (err: unknown) {
    const e = err as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    appStore.showError(extractApiErrorMessage(err, t('channelStatus.loadError')))
  } finally {
    if (abortController === ctrl) {
      if (!silent) loading.value = false
      autoRefresh.resetCountdown()
      abortController = null
    }
  }
}


async function refresh(silent = false) {
  void loadRateMultipliers()
  await reload(silent)
  // After base reload, refresh any cached detail records so non-7d availability
  // values stay in sync without forcing the user to switch tabs again.
  if (currentWindow.value !== '7d') {
    await Promise.all(items.value.map(it => loadDetail(it.id, true)))
  }
}

async function loadRateMultipliers() {
  try {
    const [groups, userRates] = await Promise.all([getAvailable(), getUserGroupRates()])
    rateByGroupName.value = Object.fromEntries(
      groups.map(group => [
        normalizeGroupName(group.name),
        userRates[group.id] ?? group.rate_multiplier,
      ]),
    )
  } catch (err) {
    console.warn('[channel-status] failed to load group rates', err)
  }
}

async function loadDetail(id: number, force = false) {
  if (!force && detailCache[id]) return
  try {
    detailCache[id] = await fetchChannelMonitorDetail(id)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('channelStatus.detailLoadError')))
  }
}

async function ensureDetailsForWindow() {
  if (currentWindow.value === '7d') return
  await Promise.all(items.value.map(it => loadDetail(it.id)))
}

// ── Handlers ──
async function handleWindowChange(value: MonitorWindow) {
  currentWindow.value = value
  await ensureDetailsForWindow()
}

function openDetail(row: UserMonitorView) {
  detailTarget.value = row
  showDetail.value = true
}

function closeDetail() {
  showDetail.value = false
  detailTarget.value = null
}

watch(items, () => {
  void ensureDetailsForWindow()
})

watch(
  () => appStore.cachedPublicSettings?.channel_monitor_enabled,
  (enabled) => {
    if (enabled === false) autoRefresh.stop()
    else if (autoRefresh.enabled.value) autoRefresh.start()
  },
)

onMounted(() => {
  void reload(false)
  void loadRateMultipliers()
  if (appStore.cachedPublicSettings?.channel_monitor_enabled !== false) {
    autoRefresh.setEnabled(autoRefresh.enabled.value)
  }
})

onBeforeUnmount(() => {
  if (abortController) abortController.abort()
})
</script>

<style scoped>
.channel-status-page { display: grid; min-width: 0; gap: 22px; }
.channel-status-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; }
.channel-status-summary { display: grid; gap: 16px; padding: 20px; }
.channel-status-heading h2 { color: var(--kc-ink); font-size: 20px; font-weight: 700; }
.channel-status-heading p,.channel-status-heading > span { margin-top: 6px; color: var(--kc-muted); font-size: 12px; }
.channel-status-counts { display: flex; flex-wrap: wrap; gap: 8px; font-size: 12px; }
.channel-status-counts > span { border-radius: 8px; padding: 5px 10px; background: var(--kc-panel-muted); color: var(--kc-muted); }
.channel-status-counts > .is-good { background: var(--kc-success-soft); color: var(--kc-success); }
.channel-status-counts > .is-warn { background: var(--kc-warning-soft); color: var(--kc-warning); }
.channel-status-counts > .is-bad { background: var(--kc-error-soft); color: var(--kc-danger); }
</style>
