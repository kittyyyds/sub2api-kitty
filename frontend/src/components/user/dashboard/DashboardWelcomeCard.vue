<template>
  <section class="home-welcome-card" data-testid="home-welcome">
    <div class="home-welcome-copy">
      <h1>{{ welcomeMessage }}</h1>
      <p data-testid="daily-quote">{{ quote }}</p>
    </div>
    <div class="home-welcome-actions">
      <nav class="home-welcome-shortcuts" data-testid="home-shortcuts" :aria-label="t('dashboard.quickActions')">
        <RouterLink to="/keys" :title="t('dashboard.generateNewKey')"><Icon name="key" size="sm" />{{ t('dashboard.createApiKey') }}</RouterLink>
        <RouterLink to="/usage" :title="t('dashboard.checkDetailedLogs')"><Icon name="chart" size="sm" />{{ t('dashboard.viewUsage') }}</RouterLink>
        <RouterLink v-if="canUseBatchImage" to="/batch-image" :title="t('dashboard.batchImageAgentDesc')"><Icon name="sparkles" size="sm" />{{ t('dashboard.batchImageAgent') }}</RouterLink>
        <RouterLink to="/redeem" :title="t('dashboard.addBalanceWithCode')"><Icon name="gift" size="sm" />{{ t('dashboard.redeemCode') }}</RouterLink>
      </nav>
      <RouterLink v-if="rechargeEnabled && !isSimple" to="/purchase" class="home-welcome-recharge"><Icon name="creditCard" size="sm" />{{ t('homeOverview.topUp') }}</RouterLink>
      <button type="button" class="home-welcome-contact" aria-haspopup="dialog" @click="contactOpen = true"><Icon name="chatBubble" size="sm" />{{ t('homeOverview.contactSupport') }}</button>
    </div>
    <CustomerContactDialog :show="contactOpen" initial-tab="service" @close="contactOpen = false" />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import CustomerContactDialog from '@/components/layout/CustomerContactDialog.vue'
import { dailyQuoteIndex, greetingPeriod, localDay } from '@/utils/dashboardGreeting'
import { userDisplayName } from '@/utils/userDisplayName'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'

const props = defineProps<{ accountId?: number; username?: string; email?: string; rechargeEnabled: boolean; isSimple: boolean }>()
const { t } = useI18n()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()
const now = ref(new Date()), quoteIndex = ref(0), contactOpen = ref(false)
watch(() => [props.accountId, localDay(now.value)], () => {
  quoteIndex.value = dailyQuoteIndex(props.accountId ?? 'guest', now.value)
}, { immediate: true })
const greeting = computed(() => ({ lateNight: t('homeOverview.lateNight'), morning: t('homeOverview.morning'), afternoon: t('homeOverview.afternoon'), evening: t('homeOverview.evening') })[greetingPeriod(now.value)])
const welcomeMessage = computed(() => {
  const name = userDisplayName(props)
  return name ? t('homeOverview.greeting', { greeting: greeting.value, name }) : greeting.value
})
const quote = computed(() => t(`homeOverview.quotes.${quoteIndex.value}`))
const tick = () => { now.value = new Date() }
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  timer = setInterval(tick, 30_000)
  document.addEventListener('visibilitychange', tick)
  void refreshBatchImageAccess()
})
onBeforeUnmount(() => { clearInterval(timer); document.removeEventListener('visibilitychange', tick) })
</script>
