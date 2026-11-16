<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
defineProps<{ balance: number; rechargeEnabled: boolean }>()
const { t } = useI18n()
</script>

<template>
  <section v-if="Number.isFinite(balance) && balance <= 0" class="low-balance-notice" role="status">
    <span class="low-balance-icon"><Icon name="exclamationTriangle" size="lg" /></span>
    <div><h2>{{ t('dashboard.insufficientBalance') }}</h2><p>{{ t('dashboard.insufficientBalanceHint', { amount: balance.toFixed(2) }) }}</p></div>
    <router-link v-if="rechargeEnabled" to="/purchase" class="low-balance-recharge">{{ t('payment.packages.charge') }}<Icon name="arrowRight" size="sm" /></router-link>
  </section>
</template>

<style scoped>
.low-balance-notice { display: flex; align-items: center; gap: 16px; border: 1px solid #f8d79d; border-radius: 16px; background: #fff7e8; padding: 16px 20px; color: #ad590b; }
.low-balance-icon { display: grid; place-items: center; width: 56px; height: 56px; flex-shrink: 0; border: 1px solid #f8d79d; border-radius: 50%; background: #fff; }
h2 { color: #302b24; font-size: 16px; font-weight: 700; } p { margin-top: 4px; font-size: 13px; line-height: 1.6; }
.low-balance-recharge { margin-left: auto; display: flex; align-items: center; justify-content: center; gap: 8px; min-height: 42px; flex-shrink: 0; padding: 10px 18px; border-radius: 10px; background: var(--kc-action,#1c211f); color: var(--kc-on-action,#f4f5f2); font-size: 13px; font-weight: 600; }
.dark .low-balance-notice { background: #30291d; border-color: #70552d; color: #ecc28e; } .dark .low-balance-notice h2 { color: #f1dfc7; } .dark .low-balance-icon { background: #392f21; border-color: #70552d; }
@media(max-width: 540px) { .low-balance-notice { flex-wrap: wrap; padding: 16px; gap: 12px; }.low-balance-icon { width: 40px; height: 40px; }.low-balance-notice > div { flex: 1; }.low-balance-recharge { width: 100%; } }
</style>
