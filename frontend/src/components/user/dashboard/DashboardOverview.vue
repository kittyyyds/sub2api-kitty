<template>
  <div class="home-overview" data-dashboard-overview>
    <div class="home-metrics">
      <article v-for="metric in metrics" :key="metric.key" class="home-metric" :data-metric="metric.key" :style="{ '--metric-tone': metric.color }">
        <div class="home-metric-body">
          <div class="home-metric-main">
            <div class="home-metric-label"><Icon :name="metric.icon" size="md" /><span>{{ metric.label }}</span></div>
            <strong>{{ metric.value }}</strong>
            <p>{{ metric.hint }}</p>
          </div>
          <dl class="home-metric-details">
            <div v-for="detail in metric.details" :key="detail.label"><dt>{{ detail.label }}</dt><dd>{{ detail.value }}</dd></div>
          </dl>
        </div>
      </article>
    </div>
    <div class="home-columns">
      <DashboardAnnouncementFeed />
      <aside class="home-aside">
        <section class="home-panel home-account">
          <h2>{{ t('homeOverview.accountStatus') }}</h2>
          <dl class="home-account-rows">
            <div v-if="!isSimple" data-testid="home-balance"><dt>{{ t('homeOverview.balance') }}</dt><dd class="home-account-balance">{{ money(balance) }}</dd></div>
            <div :title="t('homeOverview.keyRatio')"><dt>{{ t('homeOverview.keys') }}</dt><dd>{{ t('homeOverview.keyCount', { active: stats.active_api_keys, total: stats.total_api_keys }) }}</dd></div>
            <div :title="t('homeOverview.fiveMinutes')"><dt>{{ t('homeOverview.recentRpm') }}</dt><dd data-testid="home-rpm">{{ number(stats.rpm) }}</dd></div>
            <div :title="t('homeOverview.fiveMinutes')"><dt>{{ t('homeOverview.recentTpm') }}</dt><dd data-testid="home-tpm">{{ tokens(stats.tpm) }}</dd></div>
          </dl>
        </section>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserDashboardStats } from '@/api/usage'
import Icon from '@/components/icons/Icon.vue'
import DashboardAnnouncementFeed from './DashboardAnnouncementFeed.vue'
import '@/styles/dashboard-home.css'
const props = defineProps<{ stats: UserDashboardStats; balance: number; isSimple: boolean; rechargeEnabled: boolean }>()
const { t, locale } = useI18n()
// Two decimals like the header balance; sub-dollar amounts keep four so small charges stay visible.
const money = (value: number) => '$' + value.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US', { minimumFractionDigits: 2, maximumFractionDigits: Math.abs(value) >= 1 ? 2 : 4 })
const number = (value: number) => value.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US', { maximumFractionDigits: 2 })
const tokens = (value: number) => value >= 1e9 ? number(value / 1e9) + 'B' : value >= 1e6 ? number(value / 1e6) + 'M' : value >= 1e3 ? number(value / 1e3) + 'K' : number(value)
const share = (part: number, total: number) => total > 0 ? (part / total * 100).toFixed(1) + '%' : '—'
const tokenDetails = (input: number, output: number, cache: number, total: number) => [
  { label: t('homeOverview.input'), value: tokens(input) },
  { label: t('homeOverview.output'), value: tokens(output) },
  { label: t('homeOverview.cacheShare'), value: share(cache, total) },
]
const todayCache = computed(() => props.stats.today_cache_creation_tokens + props.stats.today_cache_read_tokens)
const totalCache = computed(() => props.stats.total_cache_creation_tokens + props.stats.total_cache_read_tokens)
const metrics = computed(() => [
  { key: 'today-cost', label: t('homeOverview.todayCost'), value: money(props.stats.today_actual_cost), hint: t('homeOverview.actual'), details: [{ label: t('homeOverview.todayRequests'), value: number(props.stats.today_requests) }], icon: 'dollar' as const, color: '#3e927b' },
  { key: 'total-cost', label: t('homeOverview.totalCost'), value: money(props.stats.total_actual_cost), hint: t('homeOverview.allTime'), details: [{ label: t('homeOverview.totalRequests'), value: number(props.stats.total_requests) }], icon: 'creditCard' as const, color: '#d28c51' },
  { key: 'today-tokens', label: t('homeOverview.todayTokens'), value: tokens(props.stats.today_tokens), hint: t('homeOverview.cache', { tokens: tokens(todayCache.value) }), details: tokenDetails(props.stats.today_input_tokens, props.stats.today_output_tokens, todayCache.value, props.stats.today_tokens), icon: 'cube' as const, color: '#568ab7' },
  { key: 'total-tokens', label: t('homeOverview.totalTokens'), value: tokens(props.stats.total_tokens), hint: t('homeOverview.cache', { tokens: tokens(totalCache.value) }), details: tokenDetails(props.stats.total_input_tokens, props.stats.total_output_tokens, totalCache.value, props.stats.total_tokens), icon: 'database' as const, color: '#9280b7' },
])
</script>
