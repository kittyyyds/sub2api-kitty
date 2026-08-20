<template>
  <div class="amount-input">
    <div class="amount-input__group">
      <label class="amount-input__label">
        {{ t('payment.quickAmounts') }}
      </label>
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
          <span>{{ amountSymbol }}</span>{{ amt }}
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

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  currency?: string
}>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0,
  currency: undefined,
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t } = useI18n()

const customText = ref('')
const normalizedCurrency = computed(() => normalizePaymentCurrency(props.currency))
const amountSymbol = computed(() => currencySymbol(normalizedCurrency.value))

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
  gap: 18px;
}

.amount-input__group {
  min-width: 0;
}

.amount-input__label {
  display: block;
  margin-bottom: 8px;
  color: var(--checkout-muted, #66706c);
  font-size: 11px;
  font-weight: 680;
  line-height: 1.4;
}

.amount-input__grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.amount-input__preset {
  min-width: 0;
  min-height: 48px;
  padding: 9px 10px;
  overflow: hidden;
  border: 1px solid var(--checkout-line, rgba(38, 52, 47, 0.13));
  border-radius: 7px;
  color: var(--checkout-ink, #19201e);
  background: var(--checkout-surface-soft, #f2f5f0);
  font-size: 15px;
  font-weight: 720;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: border-color 150ms ease, background-color 150ms ease, color 150ms ease, transform 150ms ease;
}

.amount-input__preset span {
  margin-right: 2px;
  color: var(--checkout-muted, #66706c);
  font-size: 11px;
  font-weight: 600;
}

.amount-input__preset:hover {
  border-color: var(--checkout-line-strong, rgba(38, 52, 47, 0.22));
  transform: translateY(-1px);
}

.amount-input__preset--active {
  border-color: var(--checkout-coral, #e96f62);
  color: #9c4139;
  background: var(--checkout-coral-soft, #fff0ec);
  box-shadow: inset 0 0 0 1px var(--checkout-coral, #e96f62);
}

.amount-input__preset--active span {
  color: inherit;
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
  height: 48px;
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

@media (max-width: 420px) {
  .amount-input__grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (prefers-reduced-motion: reduce) {
  .amount-input__preset,
  .amount-input__control {
    transition: none;
  }
}
</style>
