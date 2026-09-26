<template>
  <div class="amount-input">
    <div class="amount-input__group">
      <div class="amount-input__legend">
        <span class="amount-input__label">{{ t('payment.quickAmounts') }}</span>
        <span v-if="bonusRules.length" class="amount-input__legend-note">{{ t('payment.bonusShortNotice') }}</span>
      </div>
      <div v-if="bonusRules.length" class="amount-input__notice">
        <Icon name="gift" size="sm" />
        <div>
          <strong>{{ t('payment.bonusExplanationTitle') }}</strong>
          <p>{{ t('payment.bonusExplanation') }}</p>
        </div>
      </div>
      <div class="amount-input__grid">
        <button
          v-for="amt in filteredAmounts"
          :key="amt"
          type="button"
          :aria-pressed="modelValue === amt"
          :class="[
            'amount-input__preset',
            modelValue === amt
              ? 'amount-input__preset--active'
              : '',
          ]"
          @click="selectAmount(amt)"
        >
          <span v-if="bonusByAmount.get(amt)" class="amount-input__ribbon">{{ t('payment.includesBonus') }}</span>
          <span class="amount-input__price"><small>{{ amountSymbol }}</small><strong>{{ amt }}</strong></span>
          <span class="amount-input__credit">{{ t('payment.baseCreditPreview', { amount: (Math.round(amt * rechargeMultiplier * 100) / 100).toFixed(2) }) }}</span>
          <span v-if="bonusByAmount.get(amt)" class="amount-input__bonus">
            <Icon name="gift" size="xs" />
            <span>{{ t('payment.tierBonus', { amount: bonusByAmount.get(amt)!.bonus_amount.toFixed(2), days: bonusByAmount.get(amt)!.validity_days }) }}</span>
          </span>
          <span v-else class="amount-input__plain"><Icon name="creditCard" size="xs" />{{ t('payment.noTierBonus') }}</span>
          <span v-if="modelValue === amt" class="amount-input__check" aria-hidden="true"><Icon name="check" size="xs" /></span>
        </button>
      </div>
    </div>

    <div class="amount-input__group">
      <label for="checkout-custom-amount" class="amount-input__label">
        {{ t('payment.customAmount') }}
      </label>
      <div class="amount-input__field">
        <span class="amount-input__symbol" aria-hidden="true">
          {{ amountSymbol }}
        </span>
        <input
          id="checkout-custom-amount"
          type="text"
          inputmode="decimal"
          autocomplete="off"
          :value="customText"
          :placeholder="placeholderText"
          class="amount-input__control"
          @input="handleInput"
        />
        <span class="amount-input__currency">{{ normalizedCurrency }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { currencySymbol, normalizePaymentCurrency } from './currency'
import Icon from '@/components/icons/Icon.vue'
import type { RechargeBonusRule } from '@/types/payment'

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  currency?: string
  bonusRules?: RechargeBonusRule[]
  rechargeMultiplier?: number
}>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0,
  currency: undefined,
  bonusRules: () => [],
  rechargeMultiplier: 1,
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t } = useI18n()

const customText = ref('')
const normalizedCurrency = computed(() => normalizePaymentCurrency(props.currency))
const amountSymbol = computed(() => currencySymbol(normalizedCurrency.value))
const bonusByAmount = computed(() => new Map(props.bonusRules.map(rule => [rule.recharge_amount, rule])))

// 0 = no limit
const filteredAmounts = computed(() =>
  props.amounts.filter((a) => (props.min <= 0 || a >= props.min) && (props.max <= 0 || a <= props.max))
)

const placeholderText = computed(() => {
  if (props.min > 0 && props.max > 0) return `${props.min} - ${props.max}`
  if (props.min > 0) return `≥ ${props.min}`
  if (props.max > 0) return `≤ ${props.max}`
  return t('payment.enterAmount')
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

function selectAmount(amt: number) {
  customText.value = String(amt)
  emit('update:modelValue', amt)
}

function handleInput(e: Event) {
  const target = e.target as HTMLInputElement
  const val = target.value
  if (!AMOUNT_PATTERN.test(val)) {
    target.value = customText.value
    return
  }
  customText.value = val
  if (val === '') {
    emit('update:modelValue', null)
    return
  }
  const num = parseFloat(val)
  if (!isNaN(num) && num > 0) {
    emit('update:modelValue', num)
  } else {
    emit('update:modelValue', null)
  }
}

watch(() => props.modelValue, (v) => {
  if (v !== null && String(v) !== customText.value) {
    customText.value = String(v)
  }
}, { immediate: true })
</script>

<style scoped>
.amount-input {
  display: grid;
  gap: 16px;
  container-type: inline-size;
}

.amount-input__group {
  min-width: 0;
}

.amount-input__label {
  display: block;
  margin-bottom: 8px;
  color: var(--checkout-ink, #19201e);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.4;
}

.amount-input__legend {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 6px 12px;
  margin-bottom: 10px;
}

.amount-input__legend .amount-input__label {
  margin: 0;
}

.amount-input__legend-note {
  color: var(--checkout-muted, #66706c);
  font-size: 11px;
}

.amount-input__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.amount-input__preset {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  min-width: 0;
  min-height: 150px;
  padding: 26px 14px 28px;
  border: 1px solid var(--checkout-line, rgba(38, 52, 47, 0.13));
  border-radius: 8px;
  color: var(--checkout-ink, #19201e);
  background: var(--checkout-surface, #ffffff);
  font-size: 15px;
  font-weight: 500;
  line-height: 1.2;
  text-align: left;
  overflow-wrap: anywhere;
  transition: border-color 150ms ease, background-color 150ms ease, color 150ms ease, transform 150ms ease;
  box-shadow: 0 2px 5px rgba(38, 52, 47, 0.025);
}

.amount-input__ribbon {
  position: absolute;
  top: 7px;
  right: 9px;
  padding: 2px 6px;
  border-radius: 4px;
  color: var(--checkout-coral, #e96f62);
  background: var(--checkout-coral-soft, #fff0ec);
  font-size: 10px;
  font-weight: 600;
  line-height: 1.4;
}

.amount-input__price {
  display: flex;
  align-items: baseline;
  gap: 4px;
  max-width: 100%;
}

.amount-input__price small {
  color: var(--checkout-muted, #66706c);
  font-size: 14px;
  font-weight: 600;
}

.amount-input__price strong {
  min-width: 0;
  font-size: 36px;
  font-weight: 650;
  line-height: 1.15;
}

.amount-input__credit {
  color: var(--checkout-muted, #66706c);
  font-size: 11px;
  font-weight: 400;
  line-height: 1.5;
}

.amount-input__bonus,
.amount-input__plain {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-top: auto;
  max-width: 100%;
  padding: 4px 7px;
  border-radius: 5px;
  font-size: 11px;
  line-height: 1.5;
}

.amount-input__bonus {
  color: var(--checkout-mint, #23847d);
  background: var(--checkout-mint-soft, #e8f5f1);
  font-weight: 600;
}

.amount-input__plain {
  color: var(--checkout-muted, #66706c);
  background: var(--checkout-surface-soft, #f4f6f5);
  font-size: 10px;
}

.amount-input__bonus svg,
.amount-input__plain svg,
.amount-input__notice > svg {
  flex-shrink: 0;
}

.amount-input__check {
  position: absolute;
  right: 9px;
  bottom: 8px;
  display: grid;
  place-items: center;
  width: 19px;
  height: 19px;
  border-radius: 50%;
  color: #ffffff;
  background: var(--checkout-coral, #e96f62);
}

.amount-input__notice {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  margin-bottom: 16px;
  padding: 12px 14px;
  border: 1px solid color-mix(in srgb, var(--checkout-mint, #23847d) 20%, transparent);
  border-radius: 6px;
  color: var(--checkout-mint, #23847d);
  background: var(--checkout-mint-soft, #e8f5f1);
  font-size: 11px;
  font-weight: 400;
  line-height: 1.7;
}

.amount-input__notice p {
  margin: 4px 0 0;
}

.amount-input__notice strong {
  font-size: 12px;
  font-weight: 600;
}

.amount-input__preset:hover {
  border-color: var(--checkout-line-strong, rgba(38, 52, 47, 0.22));
  transform: translateY(-1px);
}

.amount-input__preset--active {
  border-color: var(--checkout-coral, #e96f62);
  color: var(--checkout-ink, #19201e);
  background: var(--checkout-coral-soft, #fff0ec);
  box-shadow: 0 0 0 1px var(--checkout-coral, #e96f62), 0 5px 13px rgba(233, 111, 98, 0.13);
}

.amount-input__preset:focus-visible,
.amount-input__control:focus-visible {
  outline: 2px solid var(--checkout-mint, #23847d);
  outline-offset: 2px;
}

.amount-input__field {
  position: relative;
  min-width: 0;
}

.amount-input__symbol,
.amount-input__currency {
  position: absolute;
  top: 50%;
  z-index: 1;
  color: var(--checkout-muted, #66706c);
  transform: translateY(-50%);
  pointer-events: none;
}

.amount-input__symbol {
  left: 14px;
  font-size: 14px;
  font-weight: 720;
}

.amount-input__currency {
  right: 14px;
  font-size: 10px;
  font-weight: 750;
}

.amount-input__control {
  width: 100%;
  min-width: 0;
  height: 44px;
  padding: 0 54px 0 36px;
  border: 1px solid var(--checkout-line-strong, rgba(38, 52, 47, 0.22));
  border-radius: 7px;
  color: var(--checkout-ink, #19201e);
  background: var(--checkout-surface, #fffffc);
  font-size: 15px;
  font-weight: 650;
  transition: border-color 150ms ease, box-shadow 150ms ease;
}

.amount-input__control::placeholder {
  color: var(--checkout-muted, #66706c);
  opacity: 0.7;
}

.amount-input__control:focus {
  border-color: var(--checkout-mint, #23847d);
  box-shadow: 0 0 0 3px rgba(35, 132, 125, 0.1);
}

@container (min-width: 450px) {
  .amount-input__grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@container (min-width: 550px) {
  .amount-input__grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@container (min-width: 550px) and (max-width: 680px) {
  .amount-input__price strong {
    font-size: 30px;
  }
}

@container (max-width: 360px) {
  .amount-input__grid {
    gap: 10px;
  }

  .amount-input__preset {
    min-height: 146px;
    padding-inline: 10px;
    padding-bottom: 28px;
  }

  .amount-input__price strong {
    font-size: 30px;
  }

  .amount-input__bonus,
  .amount-input__plain {
    padding: 3px 4px;
    font-size: 10px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .amount-input__preset,
  .amount-input__control {
    transition: none;
  }
}
</style>
