<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { RechargeBonusRule, RechargeBonusTier } from '@/types/payment'
import Icon from '@/components/icons/Icon.vue'
import { currencySymbol, formatPaymentAmount } from './currency'
import { quoteRechargeBonus, type RechargeBonusMode } from '@/utils/rechargeBonus'
const props = defineProps<{ amounts: number[]; bonusRules: RechargeBonusRule[]; bonusTiers?: RechargeBonusTier[]; bonusMode?: RechargeBonusMode; rechargeMultiplier: number; min: number; max: number; currency: string; balance: number | null }>()
const emit = defineEmits<{ select: [amount: number] }>()
const { t } = useI18n()
const rules = computed(() => new Map(props.bonusRules.map(rule => [rule.recharge_amount, rule])))
const highestBonus = computed(() => Math.max(0, ...props.amounts.filter(allowed).map(amount => rules.value.get(amount)?.bonus_amount ?? 0)))
function allowed(amount: number) { return (props.min <= 0 || amount >= props.min) && (props.max <= 0 || amount <= props.max) }
function credit(amount: number) { return Math.round(amount * props.rechargeMultiplier * 100) / 100 }
function quote(amount: number) {
  let digits = 2
  try { digits = new Intl.NumberFormat(undefined, { style: 'currency', currency: props.currency }).resolvedOptions().maximumFractionDigits ?? 2 } catch { /* default precision */ }
  return quoteRechargeBonus(rules.value.has(amount) ? [] : (props.bonusTiers ?? []), amount, { multiplier: props.rechargeMultiplier, mode: props.bonusMode, currencyDigits: digits })
}
function total(amount: number) { return (Math.round((quote(amount).credited + (rules.value.get(amount)?.bonus_amount ?? 0)) * 100) / 100).toFixed(2) }
</script>

<template>
  <section class="recharge-packages" :aria-label="t('payment.packages.title')">
    <div class="package-account"><span>{{ t('payment.rechargeAccount') }}</span><strong>{{ t('payment.currentBalance') }}: {{ balance == null ? '—' : '$' + balance.toFixed(2) }}</strong></div>
    <div v-if="highestBonus > 0" class="package-campaign"><div><span class="campaign-kicker">{{ t('payment.packages.campaign') }}</span><h2>{{ t('payment.packages.campaignTitle') }}</h2><p>{{ t('payment.packages.campaignNote') }}</p></div><strong class="campaign-max">{{ t('payment.packages.maxBonus', { amount: highestBonus }) }}</strong></div>
    <div class="package-grid">
      <article v-for="(amount, index) in amounts" :key="amount" class="recharge-package" :class="{ 'is-featured': amount === 50, 'is-unavailable': !allowed(amount) }" :data-package-amount="amount">
        <div class="package-badge-slot"><span v-if="amount === 50" class="package-recommended">{{ t('payment.packages.recommended') }}</span><span v-else-if="amount === 1000" class="package-bonus-badge">{{ t('payment.includesBonus') }}</span></div>
        <h3>{{ t(`payment.packages.names.${index}`) }}</h3><p class="package-description">{{ t(`payment.packages.descriptions.${index}`) }}</p>
        <div class="package-price"><span><small>{{ currencySymbol(currency) }}</small><strong>{{ amount }}</strong></span><span v-if="rules.get(amount)" class="package-bonus-badge">{{ t('payment.packages.extra', { amount: rules.get(amount)!.bonus_amount }) }}</span></div>
        <p class="package-credit">{{ t('payment.packages.receive') }} <del v-if="rules.get(amount)">${{ credit(amount).toFixed(2) }}</del> <strong>${{ total(amount) }}</strong> {{ t('payment.packages.creditUnit') }}</p>
        <p v-if="quote(amount).mode === 'discount' && quote(amount).percent > 0" class="package-credit">{{ t('payment.rechargeBonus.payShort', { amount: formatPaymentAmount(quote(amount).payBase, currency) }) }}</p>
        <ul>
          <li><Icon name="check" size="sm" />{{ t('payment.baseCreditPreview', { amount: credit(amount).toFixed(2) }) }}</li>
          <li><Icon name="check" size="sm" /><span v-if="rules.get(amount)">{{ t(rules.get(amount)!.validity_days === 0 ? 'payment.permanentTierBonus' : 'payment.tierBonus', { amount: rules.get(amount)!.bonus_amount.toFixed(2), days: rules.get(amount)!.validity_days }) }}</span><span v-else-if="quote(amount).percent > 0">{{ quote(amount).mode === 'discount' ? t('payment.rechargeBonus.discountLabelWithPercent', { percent: quote(amount).percent }) : t('payment.permanentTierBonus', { amount: quote(amount).bonus.toFixed(2) }) }}</span><span v-else>{{ t('payment.noTierBonus') }}</span></li>
          <li><Icon name="check" size="sm" />{{ t('payment.packages.usage') }}</li>
        </ul>
        <button type="button" :disabled="!allowed(amount)" :aria-label="`${t('payment.packages.choose')} ${currencySymbol(currency)}${amount}`" @click="emit('select', amount)">{{ t(allowed(amount) ? 'payment.packages.charge' : 'payment.packages.unavailable') }}</button>
      </article>
    </div>
  </section>
</template>

<style scoped>
.recharge-packages { color: var(--kc-ink,#182b29); }
.package-account { display: flex; flex-direction: column; gap: 8px; padding: 18px 20px; border: 1px solid var(--kc-line,#e6e2da); border-radius: 16px; background: var(--kc-panel,#fff); font-size: 13px; }
.package-account span { color: var(--kc-muted,#65665f); }.package-account strong { color: var(--kc-accent-strong,#9c3f00); font-weight: 600; }
.package-campaign { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin: 20px 0; border: 1px solid color-mix(in srgb, var(--kc-accent) 28%, var(--kc-line)); border-radius: 16px; padding: 16px 20px; background: var(--kc-promo-soft,#fff0cf); color: var(--kc-accent-strong,#9c3f00); }
.package-campaign h2 { margin: 8px 0 5px; font-size: 18px; font-weight: 700; color: var(--kc-ink,#1c1c19); }.package-campaign p { font-size: 13px; line-height: 1.6; color: var(--kc-muted,#65665f); }
.campaign-kicker,.campaign-max { border: 1px solid color-mix(in srgb, var(--kc-accent) 28%, var(--kc-line)); background: var(--kc-panel,#fff); padding: 6px 12px; border-radius: 99px; font-size: 12px; font-weight: 600; }
.campaign-max { flex-shrink: 0; border-radius: 12px; padding: 10px 14px; }
.package-grid { display: grid; grid-template-columns: repeat(4,minmax(0,1fr)); gap: 16px; margin-top: 20px; }
.recharge-package { display: flex; flex-direction: column; min-width: 0; padding: 16px; border: 1px solid var(--kc-line,#e6e2da); border-radius: 16px; background: var(--kc-panel,#fff); box-shadow: 0 12px 26px rgb(23 27 26 / 5%); }
.recharge-package.is-featured { border-color: var(--kc-accent,#c45100); box-shadow: 0 10px 28px color-mix(in srgb, var(--kc-accent) 14%, transparent); }
.package-badge-slot { height: 42px; }.package-recommended { display: inline-block; background: var(--kc-promo-soft,#fff0cf); color: var(--kc-accent-strong,#9c3f00); border-radius: 99px; padding: 4px 10px; font-size: 12px; font-weight: 600; }
.recharge-package h3 { font-size: 19px; font-weight: 700; }.package-description { font-size: 13px; color: var(--kc-muted,#65665f); margin-top: 4px; }
.package-price { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin: 22px 0 6px; }.package-price > span:first-child { white-space: nowrap; }.package-price small { color: var(--kc-muted,#65665f); font-size: 21px; font-weight: 600; margin-right: 8px; }.package-price strong { font-size: clamp(30px,2.5vw,38px); font-weight: 700; letter-spacing: -.03em; line-height: 1.3; font-variant-numeric: tabular-nums; }
.package-bonus-badge { border: 1px solid color-mix(in srgb, var(--kc-accent) 28%, var(--kc-line)); border-radius: 99px; background: var(--kc-promo-soft,#fff0cf); color: var(--kc-accent-strong,#9c3f00); padding: 4px 8px; font-size: 11px; white-space: nowrap; }
.package-credit { font-size: 12px; color: var(--kc-muted,#65665f); min-height: 22px; }.package-credit del { opacity: .75; }.package-credit strong { color: var(--kc-accent-strong,#9c3f00); font-weight: 600; }
.recharge-package ul { display: grid; gap: 7px; margin: 20px 0 24px; color: var(--kc-muted,#68736e); font-size: 12px; line-height: 1.6; }.recharge-package li { display: flex; align-items: flex-start; gap: 7px; }.recharge-package li > svg { flex: 0 0 auto; margin-top: 2px; color: var(--kc-accent,#158575); }
.recharge-package > button { margin-top: auto; width: 100%; min-height: 44px; border-radius: 11px; background: var(--kc-action,#1c211f); color: var(--kc-on-action,#f4f5f2); font-size: 14px; font-weight: 600; transition: filter 150ms; }.recharge-package > button:hover { filter: brightness(1.08); }.recharge-package > button:disabled { opacity: .4; cursor: not-allowed; }.is-unavailable { opacity: .65; }
.dark .package-campaign { color: var(--kc-accent-strong); }.dark .package-campaign h2 { color: var(--kc-ink); }.dark .package-campaign p { color: var(--kc-muted); }.dark .campaign-kicker, .dark .campaign-max, .dark .package-bonus-badge { color: var(--kc-accent-strong); }
@media(max-width: 1200px) { .package-grid { grid-template-columns: repeat(2,minmax(0,1fr)); }.package-price strong { font-size: 38px; } }
@media(max-width: 520px) { .package-grid { gap: 12px; }.recharge-package { padding: 14px 12px; }.package-price strong { font-size: 30px; }.package-price small { font-size: 17px; margin-right: 5px; }.package-campaign { flex-wrap: wrap; }.campaign-max { display: none; }.package-price { min-height: 68px; align-content: start; }.recharge-package ul { margin-top: 14px; }.package-badge-slot { height: 35px; } }
</style>
