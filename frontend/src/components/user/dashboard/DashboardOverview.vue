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
        <section v-if="!isSimple" class="home-balance" data-testid="home-balance">
          <div class="home-section-heading"><span>{{ t('homeOverview.balance') }}</span><Icon name="creditCard" size="md" /></div>
          <strong>{{ money(balance) }}</strong><p>{{ t('homeOverview.available') }}</p>
          <RouterLink v-if="rechargeEnabled" to="/purchase" class="home-balance-action">{{ t('homeOverview.recharge') }} <Icon name="arrowRight" size="sm" /></RouterLink>
        </section>
        <section class="home-panel home-account">
          <div class="home-section-heading"><h2>{{ t('homeOverview.keys') }}</h2><Icon name="key" size="md" /></div>
          <div class="home-key-row"><strong>{{ stats.total_api_keys }}</strong><span>{{ t('homeOverview.activeKeys', { n: stats.active_api_keys }) }}</span><RouterLink to="/keys">{{ t('homeOverview.manage') }} ↗</RouterLink></div>
          <div class="home-throughput-title"><h2>{{ t('homeOverview.throughput') }}</h2><span>{{ t('homeOverview.fiveMinutes') }}</span></div>
          <div class="home-throughput">
            <div><span>RPM</span><strong data-testid="home-rpm">{{ number(stats.rpm) }}</strong><small>{{ t('homeOverview.rpm') }}</small></div>
            <div><span>TPM</span><strong data-testid="home-tpm">{{ tokens(stats.tpm) }}</strong><small>{{ t('homeOverview.tpm') }}</small></div>
          </div>
          <dl class="home-request-summary"><div><dt>{{ t('homeOverview.todayRequests') }}</dt><dd>{{ number(stats.today_requests) }}</dd></div><div><dt>{{ t('homeOverview.totalRequests') }}</dt><dd>{{ number(stats.total_requests) }}</dd></div><div><dt>{{ t('homeOverview.average') }}</dt><dd>{{ (stats.average_duration_ms / 1000).toFixed(2) }}s</dd></div></dl>
        </section>
        <div class="home-quick-actions"><UserDashboardQuickActions /></div>
        <section v-if="endpoint" class="home-panel home-endpoint"><h2>{{ t('homeOverview.endpoint') }}</h2><code>{{ endpoint }}</code><button type="button" @click="copyToClipboard(endpoint)"><Icon name="copy" size="sm" />{{ copied ? t('homeOverview.copied') : t('homeOverview.copy') }}</button></section>
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
import { useRelayEndpoint } from '@/composables/useRelayEndpoint'
import { useClipboard } from '@/composables/useClipboard'
import '@/styles/dashboard-home.css'
const props = defineProps<{ stats: UserDashboardStats; balance: number; isSimple: boolean; rechargeEnabled: boolean }>()
const { t, locale } = useI18n()
const { endpoint } = useRelayEndpoint()
const { copied, copyToClipboard } = useClipboard()
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
