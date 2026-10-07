<template>
  <div class="home-overview" data-dashboard-overview>
    <div class="home-metrics">
      <article v-for="metric in metrics" :key="metric.key" class="home-metric" :data-metric="metric.key" :style="{ '--metric-tone': metric.color }">
        <div class="home-metric-label"><span>{{ metric.label }}</span><Icon :name="metric.icon" size="md" /></div>
        <strong>{{ metric.value }}</strong>
        <p>{{ metric.hint }}</p>
        <small v-if="metric.cache">{{ metric.cache }}</small>
      </article>
    </div>
    <div class="home-columns">
      <DashboardAnnouncementFeed />
      <aside class="home-aside">
        <section class="home-panel home-account">
          <h2>{{ t('homeOverview.accountStatus') }}</h2>
          <dl class="home-account-rows">
            <div v-if="!isSimple" data-testid="home-balance"><dt>{{ t('homeOverview.balance') }}</dt><dd class="home-account-balance">{{ money(balance) }}</dd></div>
            <div :title="t('homeOverview.keyRatio')"><dt>{{ t('homeOverview.keys') }}</dt><dd>{{ stats.active_api_keys }} / {{ stats.total_api_keys }}</dd></div>
            <div :title="t('homeOverview.fiveMinutes')"><dt>{{ t('homeOverview.recentRpm') }}</dt><dd data-testid="home-rpm">{{ number(stats.rpm) }}</dd></div>
            <div :title="t('homeOverview.fiveMinutes')"><dt>{{ t('homeOverview.recentTpm') }}</dt><dd data-testid="home-tpm">{{ tokens(stats.tpm) }}</dd></div>
          </dl>
        </section>
        <div class="home-quick-actions"><UserDashboardQuickActions /></div>

      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserDashboardStats } from '@/api/usage'
import Icon from '@/components/icons/Icon.vue'
import UserDashboardQuickActions from './UserDashboardQuickActions.vue'
import DashboardAnnouncementFeed from './DashboardAnnouncementFeed.vue'
import '@/styles/dashboard-home.css'
const props = defineProps<{ stats: UserDashboardStats; balance: number; isSimple: boolean; rechargeEnabled: boolean }>()
const { t, locale } = useI18n()
const money = (value: number) => '$' + value.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US', { minimumFractionDigits: 2, maximumFractionDigits: 4 })
const number = (value: number) => value.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US', { maximumFractionDigits: 2 })
const tokens = (value: number) => value >= 1e6 ? number(value / 1e6) + 'M' : value >= 1e3 ? number(value / 1e3) + 'K' : number(value)
const metrics = computed(() => [
  { key: 'today-cost', label: t('homeOverview.todayCost'), value: money(props.stats.today_actual_cost), hint: t('homeOverview.actual'), icon: 'dollar' as const, color: '#3e927b' },
  { key: 'total-cost', label: t('homeOverview.totalCost'), value: money(props.stats.total_actual_cost), hint: t('homeOverview.allTime'), icon: 'creditCard' as const, color: '#d28c51' },
  { key: 'today-tokens', label: t('homeOverview.todayTokens'), value: tokens(props.stats.today_tokens), hint: t('homeOverview.inputOutput', { input: tokens(props.stats.today_input_tokens), output: tokens(props.stats.today_output_tokens) }), cache: t('homeOverview.cache', { tokens: tokens(props.stats.today_cache_creation_tokens + props.stats.today_cache_read_tokens) }), icon: 'cube' as const, color: '#568ab7' },
  { key: 'total-tokens', label: t('homeOverview.totalTokens'), value: tokens(props.stats.total_tokens), hint: t('homeOverview.inputOutput', { input: tokens(props.stats.total_input_tokens), output: tokens(props.stats.total_output_tokens) }), cache: t('homeOverview.cache', { tokens: tokens(props.stats.total_cache_creation_tokens + props.stats.total_cache_read_tokens) }), icon: 'database' as const, color: '#9280b7' },
])
</script>
