<template>
  <AppLayout>
    <div class="dashboard-view-content">
      <header class="dashboard-home-header">
        <div>
          <div class="dashboard-home-tabs" role="tablist" :aria-label="t('homeOverview.overview')">
            <button id="home-tab-overview" type="button" role="tab" :aria-selected="tab === 'overview'" aria-controls="home-panel-overview" @click="tab = 'overview'"><Icon name="home" size="sm" />{{ t('homeOverview.overview') }}</button>
            <button id="home-tab-analysis" type="button" role="tab" :aria-selected="tab === 'analysis'" aria-controls="home-panel-analysis" @click="tab = 'analysis'"><Icon name="chart" size="sm" />{{ t('homeOverview.analysis') }}</button>
          </div>
        </div>
        <div class="dashboard-home-refresh"><small v-if="updatedAt">{{ t('homeOverview.updated', { time: updatedAt }) }}</small><button type="button" :disabled="refreshing" @click="refreshAll"><Icon name="refresh" size="sm" :class="{ 'animate-spin': refreshing }" />{{ t('homeOverview.refresh') }}</button></div>
      </header>
      <DashboardWelcomeCard v-if="tab === 'overview'" :account-id="user?.id" :username="user?.username" :recharge-enabled="!!stats && rechargeEnabled" :is-simple="authStore.isSimpleMode" />
      <div v-if="statsError" class="home-error" role="alert">{{ stats ? t('homeOverview.stale') : t('homeOverview.statsError') }}<button type="button" @click="loadStats">{{ t('homeOverview.retry') }}</button></div>
      <div v-if="loading && !stats" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <template v-if="stats">
        <LowBalanceNotice v-if="user && !authStore.isSimpleMode" :balance="user.balance" :recharge-enabled="rechargeEnabled" />
        <div v-show="tab === 'overview'" id="home-panel-overview" role="tabpanel" aria-labelledby="home-tab-overview">
          <DashboardOverview :stats="stats" :balance="user?.balance ?? 0" :is-simple="authStore.isSimpleMode" :recharge-enabled="rechargeEnabled" />
        </div>
        <div v-if="tab === 'analysis'" id="home-panel-analysis" role="tabpanel" aria-labelledby="home-tab-analysis" class="dashboard-home-analysis">
          <UserDashboardStats :stats="stats" :balance="user?.balance ?? 0" :is-simple="authStore.isSimpleMode" :platform-quotas="platformQuotas" />
          <p v-if="chartsError" class="home-error" role="alert">{{ t('homeOverview.chartsError') }}<button type="button" @click="loadCharts">{{ t('homeOverview.retry') }}</button></p>
          <UserDashboardCharts v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity" :loading="loadingCharts" :trend="trendData" :models="modelStats" @dateRangeChange="refreshAnalysis" @granularityChange="loadCharts" @refresh="refreshAll" />
          <p v-if="usageError" class="home-error" role="alert">{{ t('homeOverview.usageError') }}<button type="button" @click="loadRecent">{{ t('homeOverview.retry') }}</button></p>
          <div class="dashboard-recent-grid"><UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" /><UserDashboardQuickActions /></div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import { getMyPlatformQuotas } from '@/api/user'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem } from '@/types'
import { formatDateLocalInput } from '@/utils/format'
import { FeatureFlags, resolveFeatureFlag } from '@/utils/featureFlags'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import DashboardOverview from '@/components/user/dashboard/DashboardOverview.vue'
import DashboardWelcomeCard from '@/components/user/dashboard/DashboardWelcomeCard.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'
import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'
import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import LowBalanceNotice from '@/components/user/dashboard/LowBalanceNotice.vue'
import '@/styles/dashboard-home.css'

const { t } = useI18n()
const appStore = useAppStore(), authStore = useAuthStore(), announcements = useAnnouncementStore()
const user = computed(() => authStore.user)
const rechargeEnabled = computed(() => resolveFeatureFlag(appStore.cachedPublicSettings, FeatureFlags.payment) && !appStore.cachedPublicSettings?.payment_balance_disabled)
const tab = ref<'overview' | 'analysis'>('overview')
const stats = ref<UserStatsType | null>(null), loading = ref(false), refreshing = ref(false), statsError = ref(false), updatedAt = ref('')
const loadingUsage = ref(false), loadingCharts = ref(false), chartsError = ref(false), usageError = ref(false)
const trendData = ref<TrendDataPoint[]>([]), modelStats = ref<ModelStat[]>([]), recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)
const startDate = ref(formatDateLocalInput(new Date(Date.now() - 6 * 86400000))), endDate = ref(formatDateLocalInput(new Date())), granularity = ref('day')
let chartGeneration = 0, usageGeneration = 0, disposed = false
async function loadStats() {
  if (loading.value) return
  loading.value = true
  try {
    await authStore.refreshUser()
    const result = await usageAPI.getDashboardStats()
    if (disposed) return
    stats.value = result; statsError.value = false; updatedAt.value = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  } catch (error) { if (!disposed) statsError.value = true; console.error('Failed to load dashboard stats:', error) }
  finally { if (!disposed) loading.value = false }
}
async function loadCharts() {
  const generation = ++chartGeneration
  loadingCharts.value = true; chartsError.value = false
  try {
    const [trend, models] = await Promise.all([usageAPI.getDashboardTrend({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value as 'day' | 'hour' }), usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })])
    if (disposed || generation !== chartGeneration) return
    trendData.value = trend.trend || []; modelStats.value = models.models || []
  } catch { if (!disposed && generation === chartGeneration) chartsError.value = true }
  finally { if (!disposed && generation === chartGeneration) loadingCharts.value = false }
}
async function loadRecent() {
  const generation = ++usageGeneration
  loadingUsage.value = true; usageError.value = false
  try {
    const result = await usageAPI.getByDateRange(startDate.value, endDate.value)
    if (!disposed && generation === usageGeneration) recentUsage.value = result.items.slice(0, 5)
  } catch { if (!disposed && generation === usageGeneration) usageError.value = true }
  finally { if (!disposed && generation === usageGeneration) loadingUsage.value = false }
}
async function loadPlatformQuotas() {
  try { const data = await getMyPlatformQuotas(); if (!disposed) platformQuotas.value = data.platform_quotas ?? [] }
  catch { /* Keep a prior successful quota snapshot; do not invent unlimited quotas. */ }
}
async function refreshAnalysis() { await Promise.all([loadCharts(), loadRecent(), loadPlatformQuotas()]) }
async function refreshAll() {
  if (refreshing.value) return
  refreshing.value = true
  try { await Promise.all([loadStats(), announcements.fetchAnnouncements(true), ...(tab.value === 'analysis' ? [refreshAnalysis()] : [])]) }
  finally { if (!disposed) refreshing.value = false }
}
watch(tab, value => { if (value === 'analysis') void refreshAnalysis() })
onMounted(() => { void loadStats() })
onBeforeUnmount(() => { disposed = true; chartGeneration++; usageGeneration++ })
</script>
