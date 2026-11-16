<template>
  <fieldset class="payment-methods">
    <legend class="sr-only">
      {{ t('payment.paymentMethod') }}
    </legend>
    <div
      data-testid="payment-method-grid"
      class="payment-methods__grid grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4"
      :class="{ 'payment-methods__grid--detailed': detailed }"
    >
      <button
        v-for="method in sortedMethods"
        :key="method.type"
        type="button"
        :title="methodLabel(method)"
        :disabled="!method.available"
        :aria-pressed="selected === method.type"
        :class="[
          'payment-methods__option relative flex min-w-0 flex-col items-center justify-center',
          !method.available
            ? 'payment-methods__option--disabled cursor-not-allowed opacity-50'
            : selected === method.type
              ? ['payment-methods__option--selected', methodSelectedClass(method.type)]
              : 'payment-methods__option--idle',
        ]"
        @click="method.available && emit('select', method.type)"
      >
        <span class="payment-methods__content">
          <span class="payment-methods__icon">
            <img :src="methodIcon(method.type)" alt="" />
          </span>
          <span class="payment-methods__copy">
            <span data-testid="payment-method-label" class="payment-methods__label">
              {{ methodLabel(method) }}
            </span>
            <span v-if="detailed" class="payment-methods__description">{{ methodDescription(method) }}</span>
            <span
              v-if="method.fee_rate > 0"
              class="payment-methods__fee"
            >
              {{ t('payment.fee') }} {{ method.fee_rate }}%
            </span>
          </span>
        </span>
        <span v-if="selected === method.type && method.available" class="payment-methods__check" aria-hidden="true">
          <Icon name="check" size="xs" :stroke-width="2.6" />
        </span>
      </button>
    </div>
  </fieldset>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { METHOD_ORDER, isBuiltInAlipayMethod, isBuiltInWxpayMethod } from './providerConfig'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import stripeIcon from '@/assets/icons/stripe.svg'
import airwallexIcon from '@/assets/icons/airwallex.svg'
import paypalIcon from '@/assets/icons/paypal.svg'
import paymentIcon from '@/assets/icons/payment.svg'
import Icon from '@/components/icons/Icon.vue'

export interface PaymentMethodOption {
  type: string
  display_name?: string
  fee_rate: number
  available: boolean
}

const props = defineProps<{
  methods: PaymentMethodOption[]
  selected: string
  detailed?: boolean
}>()

const emit = defineEmits<{
  select: [type: string]
}>()

const { t } = useI18n()

const METHOD_ICONS: Record<string, string> = {
  alipay: alipayIcon,
  wxpay: wxpayIcon,
  stripe: stripeIcon,
  airwallex: airwallexIcon,
  dujiaopay: paymentIcon,
  paypal: paypalIcon,
  credit_card: paymentIcon,
}

const sortedMethods = computed(() => {
  const order: readonly string[] = METHOD_ORDER
  return [...props.methods].sort((a, b) => {
    const ai = order.indexOf(a.type)
    const bi = order.indexOf(b.type)
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
  })
})

function methodIcon(type: string): string {
  if (isBuiltInAlipayMethod(type)) return METHOD_ICONS.alipay
  if (isBuiltInWxpayMethod(type)) return METHOD_ICONS.wxpay
  if (type === 'airwallex') return METHOD_ICONS.airwallex
  if (type === 'dujiaopay') return METHOD_ICONS.dujiaopay
  if (type === 'paypal') return METHOD_ICONS.paypal
  return METHOD_ICONS[type] || paymentIcon
}

function methodLabel(method: PaymentMethodOption): string {
  return method.display_name || t(`payment.methods.${method.type}`, method.type)
}

function methodDescription(method: PaymentMethodOption): string {
  if (!method.available) return t('payment.methodUnavailableHint')
  if (isBuiltInAlipayMethod(method.type)) return t('payment.alipayCheckoutHint')
  if (isBuiltInWxpayMethod(method.type)) return t('payment.wechatCheckoutHint')
  return t('payment.otherCheckoutHint')
}

function methodSelectedClass(type: string): string {
  if (isBuiltInAlipayMethod(type)) return 'border-[#02A9F1] bg-blue-50 text-gray-900 shadow-sm dark:bg-blue-950 dark:text-gray-100'
  if (isBuiltInWxpayMethod(type)) return 'border-[#09BB07] bg-green-50 text-gray-900 shadow-sm dark:bg-green-950 dark:text-gray-100'
  if (type === 'stripe') return 'border-[#676BE5] bg-indigo-50 text-gray-900 shadow-sm dark:bg-indigo-950 dark:text-gray-100'
  if (type === 'airwallex') return 'border-[#FF6B3D] bg-orange-50 text-gray-900 shadow-sm dark:border-[#FF8E3C] dark:bg-orange-950 dark:text-gray-100'
  if (type === 'dujiaopay') return 'border-emerald-500 bg-emerald-50 text-gray-900 shadow-sm dark:border-emerald-400 dark:bg-emerald-950 dark:text-gray-100'
  if (type === 'paypal') return 'border-[#0070BA] bg-blue-50 text-gray-900 shadow-sm dark:border-[#169BD7] dark:bg-blue-950 dark:text-gray-100'
  return 'border-primary-500 bg-primary-50 text-gray-900 shadow-sm dark:bg-primary-950 dark:text-gray-100'
}
</script>

<style scoped>
.payment-methods {
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}

.payment-methods__grid {
  gap: 9px;
}

.payment-methods__grid--detailed {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.payment-methods__grid--detailed .payment-methods__option {
  min-height: 88px;
  height: auto;
  padding: 16px 30px 16px 14px;
}

.payment-methods__grid--detailed .payment-methods__icon {
  width: 40px;
  height: 40px;
  flex-basis: 40px;
}

.payment-methods__grid--detailed .payment-methods__label {
  font-size: 15px;
  font-weight: 600;
}

.payment-methods__grid--detailed .payment-methods__check {
  top: 50%;
  right: 10px;
  width: 20px;
  height: 20px;
  transform: translateY(-50%);
}

.payment-methods__description {
  margin-top: 6px;
  color: var(--checkout-muted, #66706c);
  font-size: 11px;
  font-weight: 400;
  line-height: 1.6;
  text-align: left;
  overflow-wrap: anywhere;
}

@media (max-width: 520px) {
  .payment-methods__grid--detailed {
    grid-template-columns: minmax(0, 1fr);
  }
}

.payment-methods__option {
  position: relative;
  height: 68px;
  padding: 9px 11px;
  overflow: hidden;
  border: 1px solid var(--checkout-line, rgba(38, 52, 47, 0.13));
  border-radius: 7px;
  color: var(--checkout-ink, #19201e);
  background: var(--checkout-surface-soft, #f2f5f0);
  transition: border-color 150ms ease, background-color 150ms ease, box-shadow 150ms ease, transform 150ms ease;
}

.payment-methods__option--idle:hover {
  border-color: var(--checkout-line-strong, rgba(38, 52, 47, 0.22));
  background: var(--checkout-surface, #fffffc);
  transform: translateY(-1px);
}

.payment-methods__option--selected {
  border-color: var(--checkout-mint, #23847d);
  background: var(--checkout-mint-soft, #e8f5f1);
  box-shadow: inset 0 0 0 1px var(--checkout-mint, #23847d);
}

.payment-methods__option:focus-visible {
  outline: 2px solid var(--checkout-mint, #23847d);
  outline-offset: 2px;
}

.payment-methods__option--disabled {
  border-color: var(--checkout-line, rgba(38, 52, 47, 0.13));
  color: var(--checkout-muted, #66706c);
  background: var(--checkout-surface-soft, #f2f5f0);
}

.payment-methods__content {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  justify-content: flex-start;
  gap: 9px;
}

.payment-methods__icon {
  display: grid;
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  place-items: center;
  border: 1px solid rgba(255, 255, 255, 0.7);
  border-radius: 7px;
  background: rgba(255, 255, 255, 0.82);
  box-shadow: 0 3px 9px rgba(39, 47, 44, 0.08);
}

.payment-methods__icon img {
  width: 23px;
  height: 23px;
  object-fit: contain;
}

.payment-methods__copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  align-items: flex-start;
  line-height: 1;
}

.payment-methods__label {
  display: -webkit-box;
  width: 100%;
  overflow: hidden;
  font-size: 12px;
  font-weight: 720;
  line-height: 1.25;
  text-align: left;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.payment-methods__fee {
  margin-top: 4px;
  color: var(--checkout-muted, #66706c);
  font-size: 9px;
  line-height: 1.2;
}

.payment-methods__check {
  position: absolute;
  top: 5px;
  right: 5px;
  display: grid;
  width: 16px;
  height: 16px;
  place-items: center;
  border-radius: 50%;
  color: #ffffff;
  background: var(--checkout-mint, #23847d);
}

.payment-methods__check :deep(svg) {
  color: #ffffff;
}

@media (max-width: 420px) {
  .payment-methods__option {
    height: 64px;
    padding-inline: 7px;
  }

  .payment-methods__content {
    gap: 6px;
  }

  .payment-methods__icon {
    width: 28px;
    height: 28px;
    flex-basis: 28px;
  }

  .payment-methods__icon img {
    width: 19px;
    height: 19px;
  }

  .payment-methods__label {
    font-size: 11px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .payment-methods__option {
    transition: none;
  }
}
</style>
