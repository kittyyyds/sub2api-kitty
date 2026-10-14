<template>
  <section class="home-welcome-card" data-testid="home-welcome">
    <div class="home-welcome-copy">
      <span class="home-welcome-badge"><Icon name="home" size="xs" />{{ t('homeOverview.overview') }}</span>
      <h1>{{ t('homeOverview.greeting', { greeting, name: username || 'Kitty' }) }}</h1>
      <p data-testid="daily-quote">{{ quote }}</p>
    </div>
    <div class="home-welcome-actions">
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

const props = defineProps<{ accountId?: number; username?: string; rechargeEnabled: boolean; isSimple: boolean }>()
const { t } = useI18n()
const now = ref(new Date()), quoteIndex = ref(0), contactOpen = ref(false)
watch(() => [props.accountId, localDay(now.value)], () => {
  quoteIndex.value = dailyQuoteIndex(props.accountId ?? 'guest', now.value)
}, { immediate: true })
const greeting = computed(() => ({ lateNight: t('homeOverview.lateNight'), morning: t('homeOverview.morning'), afternoon: t('homeOverview.afternoon'), evening: t('homeOverview.evening') })[greetingPeriod(now.value)])
const quote = computed(() => t(`homeOverview.quotes.${quoteIndex.value}`))
const tick = () => { now.value = new Date() }
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  timer = setInterval(tick, 30_000)
  document.addEventListener('visibilitychange', tick)
})
onBeforeUnmount(() => { clearInterval(timer); document.removeEventListener('visibilitychange', tick) })
</script>
