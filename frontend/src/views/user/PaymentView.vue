<template>
  <AppLayout>
    <div class="checkout-page">
      <header class="checkout-heading">
        <div class="checkout-heading__mark" aria-hidden="true">
          <Icon name="creditCard" size="lg" :stroke-width="1.8" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="checkout-heading__eyebrow">{{ t('payment.secureCheckout') }}</p>
          <h1>{{ t('payment.title') }}</h1>
          <p class="checkout-heading__copy">{{ t('payment.checkoutSubtitle') }}</p>
        </div>
        <div class="checkout-protection">
          <Icon name="shield" size="sm" :stroke-width="2" />
          <span>{{ t('payment.protectedPayment') }}</span>
        </div>
      </header>

      <div v-if="loading" class="checkout-loading" role="status" :aria-label="t('common.loading')">
        <div class="checkout-spinner" />
      </div>

      <template v-else>
        <div
          v-if="tabs.length > 1 && paymentPhase === 'select' && !selectedPlan"
          class="checkout-tabs"
          role="tablist"
          :aria-label="t('payment.title')"
        >
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab.key"
            :class="{ active: activeTab === tab.key }"
            @click="activeTab = tab.key"
          >
            <Icon :name="tab.key === 'recharge' ? 'creditCard' : 'gift'" size="sm" />
            <span>{{ tab.label }}</span>
          </button>
        </div>

        <section v-if="paymentPhase === 'paying'" class="checkout-payment-stage">
          <PaymentStatusPanel
            :order-id="paymentState.orderId"
            :amount="paymentState.amount"
            :pay-amount="paymentState.payAmount"
            :qr-code="paymentState.qrCode"
            :expires-at="paymentState.expiresAt"
            :payment-type="paymentState.paymentType"
            :pay-url="paymentState.payUrl"
            :order-type="paymentState.orderType"
            :currency="paymentState.currency || selectedCurrency"
            :out-trade-no="paymentState.outTradeNo"
            :mobile-alipay-deep-link="paymentState.alipayMobilePrecreateDeepLink"
            @done="onPaymentDone"
            @success="onPaymentSuccess"
            @settled="onPaymentSettled"
          />
        </section>

        <template v-else>
          <template v-if="activeTab === 'recharge'">
            <div v-if="enabledMethods.length === 0" class="checkout-empty">
              <Icon name="creditCard" size="xl" />
              <p>{{ t('payment.notAvailable') }}</p>
            </div>

            <div v-else class="checkout-shell">
              <div class="checkout-flow">
                <section class="checkout-section" aria-labelledby="checkout-amount-title">
                  <div class="checkout-section__heading">
                    <span class="checkout-step">1</span>
                    <div>
                      <h2 id="checkout-amount-title">{{ t('payment.selectAmount') }}</h2>
                      <p>{{ t('payment.selectAmountHint') }}</p>
                    </div>
                  </div>
                  <AmountInput
                    v-model="amount"
                    :amounts="rechargeAmounts"
                    :min="globalMinAmount"
                    :max="globalMaxAmount"
                    :currency="selectedCurrency"
                  />
                  <p v-if="amountError" class="checkout-message checkout-message--warning" role="alert">
                    {{ amountError }}
                  </p>
                  <div v-if="activeBonusRule || balanceRechargeMultiplier !== 1" class="checkout-offer">
                    <Icon name="sparkles" size="sm" />
                    <div>
                      <p v-if="activeBonusRule">
                        {{ t('payment.rechargeBonus') }} +{{ activeBonusRule.bonus_amount.toFixed(2) }} / {{ activeBonusRule.validity_days }} {{ t('payment.days') }}
                      </p>
                      <p v-if="balanceRechargeMultiplier !== 1">
                        {{ t('payment.rechargeRatePreview', { usd: balanceRechargeMultiplier.toFixed(2) }) }}
                      </p>
                    </div>
                  </div>
                </section>

                <section class="checkout-section" aria-labelledby="checkout-method-title">
                  <div class="checkout-section__heading">
                    <span class="checkout-step checkout-step--mint">2</span>
                    <div>
                      <h2 id="checkout-method-title">{{ t('payment.selectMethod') }}</h2>
                      <p>{{ t('payment.selectMethodHint') }}</p>
                    </div>
                  </div>
                  <PaymentMethodSelector
                    :methods="methodOptions"
                    :selected="selectedMethod"
                    @select="selectedMethod = $event"
                  />
                </section>
              </div>

              <aside class="checkout-summary" aria-labelledby="checkout-summary-title">
                <div class="checkout-summary__account">
                  <span class="checkout-summary__avatar" aria-hidden="true">
                    <Icon name="user" size="sm" />
                  </span>
                  <div class="min-w-0 flex-1">
                    <p>{{ t('payment.rechargeAccount') }}</p>
                    <strong>{{ user?.username || '-' }}</strong>
                  </div>
                  <div class="checkout-summary__balance">
                    <span>{{ t('payment.currentBalance') }}</span>
                    <strong>${{ user?.balance?.toFixed(2) || '0.00' }}</strong>
                  </div>
                </div>

                <div class="checkout-summary__total">
                  <p id="checkout-summary-title">{{ t('payment.actualPay') }}</p>
                  <strong>{{ formatSelectedPaymentAmount(totalAmount) }}</strong>
                </div>

                <dl class="checkout-breakdown">
                  <div>
                    <dt>{{ t('payment.paymentAmount') }}</dt>
                    <dd>{{ formatSelectedPaymentAmount(validAmount) }}</dd>
                  </div>
                  <div v-if="feeRate > 0">
                    <dt>{{ t('payment.fee') }} ({{ feeRate }}%)</dt>
                    <dd>{{ formatSelectedPaymentAmount(feeAmount) }}</dd>
                  </div>
                  <div v-if="hasRechargeBoost" class="checkout-breakdown__accent">
                    <dt>{{ t('payment.creditedBalance') }}</dt>
                    <dd>${{ creditedAmount.toFixed(2) }}</dd>
                  </div>
                  <div v-if="checkout.recharge_bonus_summary.available_amount > 0">
                    <dt>{{ t('payment.bonusBalance') }}</dt>
                    <dd>{{ checkout.recharge_bonus_summary.available_amount.toFixed(2) }}</dd>
                  </div>
                </dl>

                <p v-if="activeBonusRule" class="checkout-summary__notice">
                  {{ t('payment.bonusPriorityNotice') }}
                </p>
                <p v-if="checkout.recharge_bonus_summary.next_expires_at" class="checkout-summary__notice">
                  {{ t('payment.nextBonusExpiry', { time: formatBonusExpiry(checkout.recharge_bonus_summary.next_expires_at) }) }}
                </p>

                <button
                  type="button"
                  :class="['btn checkout-submit', paymentButtonClass]"
                  :disabled="!canSubmit || submitting"
                  @click="handleSubmitRecharge"
                >
                  <span v-if="submitting" class="checkout-submit__content">
                    <span class="checkout-submit__spinner" />
                    {{ t('common.processing') }}
                  </span>
                  <span v-else class="checkout-submit__content">
                    {{ t('payment.createOrder') }} {{ formatSelectedPaymentAmount(totalAmount) }}
                    <Icon name="arrowRight" size="sm" :stroke-width="2" />
                  </span>
                </button>

                <div class="checkout-summary__security">
                  <Icon name="lock" size="xs" :stroke-width="2" />
                  <span>{{ t('payment.paymentSecurityHint') }}</span>
                </div>
              </aside>
            </div>
          </template>

          <template v-else-if="activeTab === 'subscription'">
            <div v-if="selectedPlan" class="checkout-shell">
              <div class="checkout-flow">
                <section class="checkout-section" aria-labelledby="subscription-plan-title">
                  <div class="checkout-section__heading">
                    <span class="checkout-step">1</span>
                    <div>
                      <h2 id="subscription-plan-title">{{ t('payment.confirmSubscription') }}</h2>
                      <p>{{ selectedPlan.name }}</p>
                    </div>
                  </div>

                  <div class="checkout-plan">
                    <div class="checkout-plan__topline">
                      <span :class="['rounded-md border px-2 py-0.5 text-xs font-medium', planBadgeClass]">
                        {{ platformLabel(selectedPlan.group_platform || '') }}
                      </span>
                      <span class="checkout-plan__validity">{{ planValiditySuffix }}</span>
                    </div>
                    <div class="checkout-plan__price">
                      <strong :class="planTextClass">{{ formatSelectedSubscriptionPaymentAmount(selectedPlan.price) }}</strong>
                      <span v-if="selectedPlan.original_price">
                        {{ formatSelectedSubscriptionPaymentAmount(selectedPlan.original_price) }}
                      </span>
                    </div>
                    <p v-if="selectedPlan.description" class="checkout-plan__description">{{ selectedPlan.description }}</p>
                    <dl class="checkout-plan__metrics">
                      <div>
                        <dt>{{ t('payment.planCard.rate') }}</dt>
                        <dd :class="planTextClass">×{{ selectedPlan.rate_multiplier ?? 1 }}</dd>
                      </div>
                      <div v-if="planHasPeakRate(selectedPlan)">
                        <dt>{{ t('payment.planCard.peakRate') }}</dt>
                        <dd>{{ planPeakRateLabel(selectedPlan) }}</dd>
                      </div>
                      <div v-if="selectedPlan.daily_limit_usd != null">
                        <dt>{{ t('payment.planCard.dailyLimit') }}</dt>
                        <dd>${{ selectedPlan.daily_limit_usd }}</dd>
                      </div>
                      <div v-if="selectedPlan.weekly_limit_usd != null">
                        <dt>{{ t('payment.planCard.weeklyLimit') }}</dt>
                        <dd>${{ selectedPlan.weekly_limit_usd }}</dd>
                      </div>
                      <div v-if="selectedPlan.monthly_limit_usd != null">
                        <dt>{{ t('payment.planCard.monthlyLimit') }}</dt>
                        <dd>${{ selectedPlan.monthly_limit_usd }}</dd>
                      </div>
                      <div v-if="selectedPlan.daily_limit_usd == null && selectedPlan.weekly_limit_usd == null && selectedPlan.monthly_limit_usd == null">
                        <dt>{{ t('payment.planCard.quota') }}</dt>
                        <dd>{{ t('payment.planCard.unlimited') }}</dd>
                      </div>
                    </dl>
                  </div>
                </section>

                <section class="checkout-section" aria-labelledby="subscription-method-title">
                  <div class="checkout-section__heading">
                    <span class="checkout-step checkout-step--mint">2</span>
                    <div>
                      <h2 id="subscription-method-title">{{ t('payment.selectMethod') }}</h2>
                      <p>{{ t('payment.selectMethodHint') }}</p>
                    </div>
                  </div>
                  <PaymentMethodSelector
                    :methods="subMethodOptions"
                    :selected="selectedMethod"
                    @select="selectedMethod = $event"
                  />
                </section>
              </div>

              <aside class="checkout-summary" aria-labelledby="subscription-summary-title">
                <div class="checkout-summary__account">
                  <span class="checkout-summary__avatar" aria-hidden="true">
                    <Icon name="gift" size="sm" />
                  </span>
                  <div class="min-w-0 flex-1">
                    <p>{{ t('payment.tabSubscribe') }}</p>
                    <strong class="truncate">{{ selectedPlan.name }}</strong>
                  </div>
                </div>

                <div class="checkout-summary__total">
                  <p id="subscription-summary-title">{{ t('payment.actualPay') }}</p>
                  <strong>{{ formatSelectedPaymentAmount(subTotalAmount) }}</strong>
                </div>
                <dl class="checkout-breakdown">
                  <div>
                    <dt>{{ t('payment.amountLabel') }}</dt>
                    <dd>{{ formatSelectedPaymentAmount(subPaymentAmount) }}</dd>
                  </div>
                  <div v-if="feeRate > 0">
                    <dt>{{ t('payment.fee') }} ({{ feeRate }}%)</dt>
                    <dd>{{ formatSelectedPaymentAmount(subFeeAmount) }}</dd>
                  </div>
                </dl>

                <button
                  type="button"
                  :class="['btn checkout-submit', paymentButtonClass]"
                  :disabled="!canSubmitSubscription || submitting"
                  @click="confirmSubscribe"
                >
                  <span v-if="submitting" class="checkout-submit__content">
                    <span class="checkout-submit__spinner" />
                    {{ t('common.processing') }}
                  </span>
                  <span v-else class="checkout-submit__content">
                    {{ t('payment.createOrder') }} {{ formatSelectedPaymentAmount(subTotalAmount) }}
                    <Icon name="arrowRight" size="sm" :stroke-width="2" />
                  </span>
                </button>
                <button type="button" class="btn btn-secondary checkout-cancel" @click="selectedPlan = null">
                  {{ t('common.cancel') }}
                </button>
                <div class="checkout-summary__security">
                  <Icon name="lock" size="xs" :stroke-width="2" />
                  <span>{{ t('payment.paymentSecurityHint') }}</span>
                </div>
              </aside>
            </div>

            <div v-else class="checkout-catalog">
              <div v-if="checkout.plans.length === 0" class="checkout-empty">
                <Icon name="gift" size="xl" />
                <p>{{ t('payment.noPlans') }}</p>
              </div>
              <div v-else :class="planGridClass">
                <SubscriptionPlanCard
                  v-for="plan in checkout.plans"
                  :key="plan.id"
                  :plan="plan"
                  :active-subscriptions="activeSubscriptions"
                  @select="selectPlan"
                />
              </div>

              <section v-if="activeSubscriptions.length > 0" class="checkout-subscriptions" aria-labelledby="active-subscriptions-title">
                <h2 id="active-subscriptions-title">{{ t('payment.activeSubscription') }}</h2>
                <div class="checkout-subscriptions__list">
                  <div v-for="sub in activeSubscriptions" :key="sub.id" class="checkout-subscription-row">
                    <div :class="['h-7 w-1 shrink-0 rounded-full', platformAccentBarClass(sub.group?.platform || '')]" />
                    <div class="min-w-0 flex-1">
                      <div class="flex min-w-0 items-center gap-2">
                        <strong class="truncate">{{ sub.group?.name || t('payment.groupFallback', { id: sub.group_id }) }}</strong>
                        <span :class="['shrink-0 rounded-full px-2 py-0.5 text-[10px] font-medium', platformBadgeLightClass(sub.group?.platform || '')]">
                          {{ platformLabel(sub.group?.platform || '') }}
                        </span>
                      </div>
                      <p>
                        <span>{{ t('payment.planCard.rate') }}: ×{{ sub.group?.rate_multiplier ?? 1 }}</span>
                        <span v-if="subscriptionHasPeakRate(sub)">{{ t('payment.planCard.peakRate') }}: {{ subscriptionPeakRateLabel(sub) }}</span>
                        <span v-if="sub.expires_at">{{ t('userSubscriptions.daysRemaining', { days: getDaysRemaining(sub.expires_at) }) }}</span>
                        <span v-else>{{ t('userSubscriptions.noExpiration') }}</span>
                      </p>
                    </div>
                    <span class="badge badge-success shrink-0 text-[10px]">{{ t('userSubscriptions.status.active') }}</span>
                  </div>
                </div>
              </section>
            </div>
          </template>

          <section
            v-if="(checkout.help_text || checkout.help_image_url) && !selectedPlan"
            class="checkout-help"
            aria-labelledby="checkout-help-title"
          >
            <div>
              <p id="checkout-help-title">{{ t('payment.needHelp') }}</p>
              <span v-if="checkout.help_text">{{ checkout.help_text }}</span>
            </div>
            <button
              v-if="checkout.help_image_url"
              type="button"
              class="checkout-help__preview"
              @click="openPreviewImage(checkout.help_image_url)"
            >
              <img :src="checkout.help_image_url" :alt="t('payment.helpImageAlt')" />
              <span>{{ t('payment.viewHelpImage') }}</span>
            </button>
          </section>
        </template>
      </template>
    </div>

    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="showRenewalModal"
          class="checkout-modal-backdrop"
          @click.self="closeRenewalModal"
        >
          <div
            ref="renewalModalRef"
            class="checkout-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="renewal-modal-title"
            tabindex="-1"
            @keydown.esc="closeRenewalModal"
          >
            <button
              type="button"
              class="checkout-modal__close"
              :aria-label="t('common.close')"
              @click="closeRenewalModal"
            >
              <Icon name="x" size="sm" :stroke-width="2" />
            </button>
            <h2 id="renewal-modal-title">{{ t('payment.selectPlan') }}</h2>
            <div class="checkout-modal__plans">
              <SubscriptionPlanCard
                v-for="plan in renewalPlans"
                :key="plan.id"
                :plan="plan"
                :active-subscriptions="activeSubscriptions"
                @select="selectPlanFromModal"
              />
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="modal">
        <div v-if="previewImage" class="checkout-modal-backdrop checkout-modal-backdrop--image" @click.self="closePreviewImage">
          <div
            ref="imageModalRef"
            class="checkout-image-modal"
            role="dialog"
            aria-modal="true"
            :aria-label="t('payment.helpImageAlt')"
            tabindex="-1"
            @keydown.esc="closePreviewImage"
          >
            <button
              type="button"
              class="checkout-modal__close checkout-image-modal__close"
              :aria-label="t('common.close')"
              @click="closePreviewImage"
            >
              <Icon name="x" size="sm" :stroke-width="2" />
            </button>
            <img :src="previewImage" :alt="t('payment.helpImageAlt')" />
          </div>
        </div>
      </Transition>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { usePaymentStore } from '@/stores/payment'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel, type PeakRateFields } from '@/utils/peak-rate'
import type { SubscriptionPlan, CheckoutInfoResponse, CreateOrderResult, OrderType } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import AmountInput from '@/components/payment/AmountInput.vue'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'
import { METHOD_ORDER, getPaymentPopupFeatures, isBuiltInAlipayMethod, isBuiltInWxpayMethod } from '@/components/payment/providerConfig'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  buildCreateOrderPayload,
  clearPaymentRecoverySnapshot,
  decidePaymentLaunch,
  getVisibleMethods,
  normalizeVisibleMethod,
  readPaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
  writePaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'
import { platformAccentBarClass, platformBadgeLightClass, platformBadgeClass, platformTextClass, platformLabel } from '@/utils/platformColors'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import PaymentStatusPanel from '@/components/payment/PaymentStatusPanel.vue'
import Icon from '@/components/icons/Icon.vue'
import { DEFAULT_PAYMENT_CURRENCY, formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import { planValiditySuffix as validitySuffixOf } from '@/components/payment/validity'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { buildPaymentErrorToastMessage, describePaymentScenarioError } from './paymentUx'
import { hasWechatResumeQuery, parseWechatResumeRoute, stripWechatResumeQuery } from './paymentWechatResume'

const i18n = useI18n()
const { t } = i18n
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const paymentStore = usePaymentStore()
const subscriptionStore = useSubscriptionStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)
const activeSubscriptions = computed(() => subscriptionStore.activeSubscriptions)

function getDaysRemaining(expiresAt: string): number {
  const diff = new Date(expiresAt).getTime() - Date.now()
  return Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)))
}

function subscriptionHasPeakRate(sub: { group?: PeakRateFields | null }): boolean {
  return hasPeakRate(sub.group)
}

function subscriptionPeakRateLabel(sub: { group?: PeakRateFields | null }): string {
  return formatPeakRateWindow(sub.group, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

const loading = ref(true)
const submitting = ref(false)
const errorMessage = ref('')
const errorHintMessage = ref('')
const activeTab = ref<'recharge' | 'subscription'>('recharge')
const amount = ref<number | null>(null)
const selectedMethod = ref('')
const selectedPlan = ref<SubscriptionPlan | null>(null)
const previewImage = ref('')
const renewalModalRef = ref<HTMLElement | null>(null)
const imageModalRef = ref<HTMLElement | null>(null)

let renewalReturnFocus: HTMLElement | null = null
let imageReturnFocus: HTMLElement | null = null

const paymentPhase = ref<'select' | 'paying'>('select')

interface CreateOrderOptions {
  openid?: string
  wechatResumeToken?: string
  paymentType?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
}

interface WeixinJSBridgeLike {
  invoke(
    action: string,
    payload: Record<string, unknown>,
    callback: (result: Record<string, unknown>) => void,
  ): void
}

function emptyPaymentState(): PaymentRecoverySnapshot {
  return {
    orderId: 0,
    amount: 0,
    qrCode: '',
    expiresAt: '',
    paymentType: '',
    payUrl: '',
    outTradeNo: '',
    clientSecret: '',
    intentId: '',
    currency: '',
    countryCode: '',
    paymentEnv: '',
    payAmount: 0,
    orderType: '',
    paymentMode: '',
    resumeToken: '',
    alipayMobilePrecreateDeepLink: false,
    createdAt: 0,
  }
}

function getWeixinJSBridge(): WeixinJSBridgeLike | undefined {
  return (window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge
}

function waitForWeixinJSBridge(timeoutMs = 4000): Promise<WeixinJSBridgeLike | null> {
  const existing = getWeixinJSBridge()
  if (existing) return Promise.resolve(existing)

  return new Promise((resolve) => {
    let settled = false
    const finish = (bridge: WeixinJSBridgeLike | null) => {
      if (settled) return
      settled = true
      document.removeEventListener('WeixinJSBridgeReady', handleReady)
      document.removeEventListener('onWeixinJSBridgeReady', handleReady)
      window.clearTimeout(timer)
      resolve(bridge)
    }
    const handleReady = () => finish(getWeixinJSBridge() ?? null)
    const timer = window.setTimeout(() => finish(getWeixinJSBridge() ?? null), timeoutMs)
    document.addEventListener('WeixinJSBridgeReady', handleReady, false)
    document.addEventListener('onWeixinJSBridgeReady', handleReady, false)
  })
}

async function invokeWechatJsapiPayment(payload: Record<string, unknown>): Promise<Record<string, unknown>> {
  const bridge = await waitForWeixinJSBridge()
  if (!bridge) {
    throw new Error('WECHAT_JSAPI_UNAVAILABLE')
  }
  return new Promise((resolve) => {
    bridge.invoke('getBrandWCPayRequest', payload, (result) => resolve(result || {}))
  })
}

const paymentState = ref<PaymentRecoverySnapshot>(emptyPaymentState())

function persistRecoverySnapshot(snapshot: PaymentRecoverySnapshot) {
  if (typeof window === 'undefined' || !snapshot.orderId) return
  writePaymentRecoverySnapshot(window.localStorage, snapshot, PAYMENT_RECOVERY_STORAGE_KEY)
}

function removeRecoverySnapshot() {
  if (typeof window === 'undefined') return
  clearPaymentRecoverySnapshot(window.localStorage, PAYMENT_RECOVERY_STORAGE_KEY)
}

function resetPayment() {
  paymentPhase.value = 'select'
  paymentState.value = emptyPaymentState()
  removeRecoverySnapshot()
}

async function redirectToPaymentResult(state: PaymentRecoverySnapshot): Promise<void> {
  const query: Record<string, string | undefined> = {}
  if (state.orderId > 0) {
    query.order_id = String(state.orderId)
  }
  if (state.outTradeNo) {
    query.out_trade_no = state.outTradeNo
  }
  if (state.resumeToken) {
    query.resume_token = state.resumeToken
  }
  await router.push({
    path: '/payment/result',
    query,
  })
}

function buildWechatOAuthAuthorizeUrl(
  authorizeUrl: string,
  context: { paymentType: string; orderType: OrderType; planId?: number; orderAmount: number },
): string {
  const normalizedUrl = authorizeUrl.trim()
  if (!normalizedUrl || typeof window === 'undefined') {
    return normalizedUrl
  }

  try {
    const targetUrl = new URL(normalizedUrl, window.location.origin)
    const redirectPath = targetUrl.searchParams.get('redirect') || '/purchase'
    const redirectUrl = new URL(redirectPath, window.location.origin)
    const paymentType = normalizeVisibleMethod(context.paymentType) || context.paymentType.trim() || 'wxpay'

    redirectUrl.searchParams.set('payment_type', paymentType)
    redirectUrl.searchParams.set('order_type', context.orderType)

    if (context.planId) {
      redirectUrl.searchParams.set('plan_id', String(context.planId))
    } else {
      redirectUrl.searchParams.delete('plan_id')
    }

    if (context.orderAmount > 0) {
      redirectUrl.searchParams.set('amount', String(context.orderAmount))
    } else {
      redirectUrl.searchParams.delete('amount')
    }

    targetUrl.searchParams.set('redirect', `${redirectUrl.pathname}${redirectUrl.search}`)
    return targetUrl.toString()
  } catch {
    return normalizedUrl
  }
}

function onPaymentDone() {
  const wasSubscription = paymentState.value.orderType === 'subscription'
  resetPayment()
  selectedPlan.value = null
  if (wasSubscription) {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

async function onPaymentSuccess() {
  const completedPayment = { ...paymentState.value }
  removeRecoverySnapshot()
  authStore.refreshUser()
  if (paymentState.value.orderType === 'subscription') {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
  await redirectToPaymentResult(completedPayment)
}

function onPaymentSettled() {
  removeRecoverySnapshot()
}

// All checkout data from single API call
const checkout = ref<CheckoutInfoResponse>({
  methods: {}, global_min: 0, global_max: 0,
  plans: [], balance_disabled: false, balance_recharge_multiplier: 1, subscription_usd_to_cny_rate: 0, recharge_fee_rate: 0, help_text: '', help_image_url: '', stripe_publishable_key: '',
  recharge_bonus_rules: [], recharge_bonus_summary: { available_amount: 0, active_grants: 0 },
})

const tabs = computed(() => {
  const result: { key: 'recharge' | 'subscription'; label: string }[] = []
  if (!checkout.value.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
  result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
  return result
})

const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
const enabledMethods = computed(() => Object.keys(visibleMethods.value))
const validAmount = computed(() => amount.value ?? 0)
const balanceRechargeMultiplier = computed(() => {
  const multiplier = checkout.value.balance_recharge_multiplier
  return Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1
})
const activeBonusRule = computed(() => checkout.value.recharge_bonus_rules.find(rule => Math.abs(rule.recharge_amount - validAmount.value) < 0.00000001) || null)
const rechargeAmounts = computed(() => Array.from(new Set([10, 20, ...checkout.value.recharge_bonus_rules.map(rule => rule.recharge_amount)])).sort((a, b) => a - b))
// 订阅 CNY 换算汇率（1 USD = X CNY）。0 = 未配置，订阅保持 price 直付（与后端 opt-in 条件严格镜像）。
const subscriptionUsdToCnyRate = computed(() => {
  const rate = checkout.value.subscription_usd_to_cny_rate
  return Number.isFinite(rate) && rate > 0 ? rate : 0
})
const creditedAmount = computed(() => Math.round((validAmount.value * balanceRechargeMultiplier.value + (activeBonusRule.value?.bonus_amount || 0)) * 100) / 100)
const hasRechargeBoost = computed(() => Math.abs(creditedAmount.value - validAmount.value) > 0.00000001)

function formatBonusExpiry(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

// Adaptive grid: center single card, 2-col for 2 plans, 3-col for 3+
const planGridClass = computed(() => {
  const n = checkout.value.plans.length
  if (n <= 2) return 'grid grid-cols-1 gap-5 sm:grid-cols-2'
  return 'grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3'
})

// Check if an amount fits a method's [min, max]. 0 = no limit.
function amountFitsMethod(amt: number, methodType: string): boolean {
  if (amt <= 0) return true
  const ml = visibleMethods.value[methodType]
  if (!ml) return false
  if (ml.single_min > 0 && amt < ml.single_min) return false
  if (ml.single_max > 0 && amt > ml.single_max) return false
  return true
}

// Visible methods decide the amount range shown to users.
const globalMinAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_min <= 0)) return 0
  return Math.min(...limits.map(limit => limit.single_min))
})
const globalMaxAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_max <= 0)) return 0
  return Math.max(...limits.map(limit => limit.single_max))
})

// Selected method's limits (for validation and error messages)
const selectedLimit = computed(() => visibleMethods.value[selectedMethod.value])
const selectedCurrency = computed(() => normalizePaymentCurrency(selectedLimit.value?.currency))
const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

function currencyFractionDigits(currency: string): number {
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
    }).resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

function roundPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** currencyFractionDigits(currency)
  return Math.round(value * factor) / factor
}

function ceilPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** currencyFractionDigits(currency)
  return Math.ceil(value * factor) / factor
}

function subscriptionPaymentAmountForCurrency(value: number, currency: string): number {
  const rate = subscriptionUsdToCnyRate.value
  if (rate <= 0 || currency !== DEFAULT_PAYMENT_CURRENCY) return roundPaymentAmount(value, currency)
  return roundPaymentAmount(value * rate, currency)
}

function formatSelectedPaymentAmount(value: number): string {
  return formatPaymentAmount(value, selectedCurrency.value, localeCode.value)
}

function formatSelectedSubscriptionPaymentAmount(value: number): string {
  return formatSelectedPaymentAmount(subscriptionPaymentAmountForCurrency(value, selectedCurrency.value))
}

const methodOptions = computed<PaymentMethodOption[]>(() =>
  enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(validAmount.value, type),
    }
  })
)

const feeRate = computed(() => checkout.value?.recharge_fee_rate ?? 0)
const feeAmount = computed(() =>
  feeRate.value > 0 && validAmount.value > 0
    ? Math.ceil(((validAmount.value * feeRate.value) / 100) * 100) / 100
    : 0
)
const totalAmount = computed(() =>
  feeRate.value > 0 && validAmount.value > 0
    ? Math.round((validAmount.value + feeAmount.value) * 100) / 100
    : validAmount.value
)

const amountError = computed(() => {
  if (validAmount.value <= 0) return ''
  // No method can handle this amount
  if (!enabledMethods.value.some((m) => amountFitsMethod(validAmount.value, m))) {
    return t('payment.amountNoMethod')
  }
  // Selected method can't handle this amount (but others can)
  const ml = selectedLimit.value
  if (ml) {
    if (ml.single_min > 0 && validAmount.value < ml.single_min) return t('payment.amountTooLow', { min: formatSelectedPaymentAmount(ml.single_min) })
    if (ml.single_max > 0 && validAmount.value > ml.single_max) return t('payment.amountTooHigh', { max: formatSelectedPaymentAmount(ml.single_max) })
  }
  return ''
})

const canSubmit = computed(() =>
  validAmount.value > 0
    && amountFitsMethod(validAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false
)

const subPaymentAmount = computed(() => {
  const price = selectedPlan.value?.price ?? 0
  return subscriptionPaymentAmountForCurrency(price, selectedCurrency.value)
})

const subFeeAmount = computed(() => {
  if (feeRate.value <= 0 || subPaymentAmount.value <= 0) return 0
  return ceilPaymentAmount((subPaymentAmount.value * feeRate.value) / 100, selectedCurrency.value)
})

const subTotalAmount = computed(() => {
  if (feeRate.value <= 0 || subPaymentAmount.value <= 0) return subPaymentAmount.value
  return roundPaymentAmount(subPaymentAmount.value + subFeeAmount.value, selectedCurrency.value)
})

function subscriptionTotalAmountForCurrency(value: number, currency: string): number {
  const paymentAmount = subscriptionPaymentAmountForCurrency(value, currency)
  if (feeRate.value <= 0 || paymentAmount <= 0) return paymentAmount
  const fee = ceilPaymentAmount((paymentAmount * feeRate.value) / 100, currency)
  return roundPaymentAmount(paymentAmount + fee, currency)
}

// Subscription-specific: method options based on gateway pay amount
const subMethodOptions = computed<PaymentMethodOption[]>(() => {
  const price = selectedPlan.value?.price ?? 0
  return enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    const currency = normalizePaymentCurrency(ml?.currency)
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(subscriptionTotalAmountForCurrency(price, currency), type),
    }
  })
})

const canSubmitSubscription = computed(() =>
  selectedPlan.value !== null
    && amountFitsMethod(subTotalAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false
)

// Auto-switch to first available method when current selection can't handle the amount
watch(() => [validAmount.value, selectedMethod.value] as const, ([amt, method]) => {
  if (amt <= 0 || amountFitsMethod(amt, method)) return
  const available = enabledMethods.value.find((m) => amountFitsMethod(amt, m))
  if (available) selectedMethod.value = available
})

// Payment button class: follows selected payment method color
const paymentButtonClass = computed(() => {
  const m = selectedMethod.value
  if (!m) return 'btn-primary'
  if (isBuiltInAlipayMethod(m)) return 'btn-alipay'
  if (isBuiltInWxpayMethod(m)) return 'btn-wxpay'
  if (m === 'stripe') return 'btn-stripe'
  if (m === 'airwallex') return 'btn-airwallex'
  if (m === 'paypal') return 'btn-paypal'
  return 'btn-primary'
})

// Subscription confirm: platform accent colors (clean card, no gradient)
const planBadgeClass = computed(() => platformBadgeClass(selectedPlan.value?.group_platform || ''))
const planTextClass = computed(() => platformTextClass(selectedPlan.value?.group_platform || ''))

// Renewal modal state
const showRenewalModal = ref(false)
const renewGroupId = ref<number | null>(null)
const renewalPlans = computed(() => {
  if (renewGroupId.value == null) return []
  return checkout.value.plans.filter(p => p.group_id === renewGroupId.value)
})

watch(showRenewalModal, async (isOpen) => {
  if (!isOpen) return
  renewalReturnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
  await nextTick()
  renewalModalRef.value?.focus()
})

watch(previewImage, async (imageUrl) => {
  if (!imageUrl) return
  await nextTick()
  imageModalRef.value?.focus()
})

const planValiditySuffix = computed(() => {
  if (!selectedPlan.value) return ''
  return validitySuffixOf(selectedPlan.value, t)
})

function planHasPeakRate(plan: SubscriptionPlan): boolean {
  return hasPeakRate(plan)
}

function planPeakRateLabel(plan: SubscriptionPlan): string {
  return formatPeakRateWindow(plan, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

function selectPlan(plan: SubscriptionPlan) {
  selectedPlan.value = plan
  errorMessage.value = ''
}

function selectPlanFromModal(plan: SubscriptionPlan) {
  showRenewalModal.value = false
  renewGroupId.value = null
  selectedPlan.value = plan
  errorMessage.value = ''
}

async function closeRenewalModal() {
  showRenewalModal.value = false
  renewGroupId.value = null
  await nextTick()
  renewalReturnFocus?.focus()
  renewalReturnFocus = null
}

function openPreviewImage(imageUrl: string) {
  const normalizedUrl = imageUrl.trim()
  if (!normalizedUrl) return
  imageReturnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
  previewImage.value = normalizedUrl
}

async function closePreviewImage() {
  previewImage.value = ''
  await nextTick()
  imageReturnFocus?.focus()
  imageReturnFocus = null
}

async function handleSubmitRecharge() {
  if (!canSubmit.value || submitting.value) return
  await createOrder(validAmount.value, 'balance')
}

async function confirmSubscribe() {
  if (!selectedPlan.value || submitting.value) return
  await createOrder(selectedPlan.value.price, 'subscription', selectedPlan.value.id)
}

async function createOrder(orderAmount: number, orderType: OrderType, planId?: number, options: CreateOrderOptions = {}) {
  submitting.value = true
  errorMessage.value = ''
  errorHintMessage.value = ''
  const requestType = normalizeVisibleMethod(options.paymentType || selectedMethod.value) || options.paymentType || selectedMethod.value
  try {
    const payload = buildCreateOrderPayload({
      amount: orderAmount,
      paymentType: requestType,
      orderType,
      planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && normalizeVisibleMethod(requestType) === 'alipay'),
      mobilePrecreateDeepLink: checkout.value.alipay_mobile_precreate_deep_link === true,
    })
    if (options.openid) {
      payload.openid = options.openid
    }
    if (options.wechatResumeToken) {
      payload.wechat_resume_token = options.wechatResumeToken
    }

    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const openWindow = (url: string) => {
      const win = window.open(url, 'paymentPopup', getPaymentPopupFeatures())
      if (!win || win.closed) {
        window.location.href = url
      }
    }
    const visibleMethod = normalizeVisibleMethod(requestType) || requestType
    // When user clicks the dedicated Stripe button, leave method blank so the
    // landing page renders Stripe's full Payment Element (card/link/alipay/wxpay).
    const stripeMethod = visibleMethod === 'stripe'
      ? ''
      : visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret && visibleMethod !== 'airwallex'
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const airwallexRouteUrl = result.client_secret && result.intent_id
      ? router.resolve({
        path: '/payment/airwallex',
        query: {
          order_id: String(result.order_id),
          out_trade_no: result.out_trade_no || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType,
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && visibleMethod === 'alipay'),
      mobilePrecreateDeepLink: checkout.value.alipay_mobile_precreate_deep_link === true,
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
      airwallexRouteUrl,
    })

    if (decision.kind === 'wechat_oauth' && decision.oauth?.authorize_url) {
      window.location.href = buildWechatOAuthAuthorizeUrl(decision.oauth.authorize_url, {
        paymentType: visibleMethod,
        orderType,
        planId,
        orderAmount,
      })
      return
    }

    if (decision.kind === 'unhandled') {
      applyScenarioError({ reason: 'UNHANDLED_PAYMENT_SCENARIO' }, visibleMethod)
      return
    }

    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)

    if (decision.kind === 'stripe_popup') {
      openWindow(decision.paymentState.payUrl)
      return
    }
    if (decision.kind === 'stripe_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'airwallex_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'wechat_jsapi' && decision.jsapi) {
      try {
        const jsapiResult = await invokeWechatJsapiPayment(decision.jsapi as Record<string, unknown>)
        const errMsg = String(jsapiResult.err_msg || '').toLowerCase()
        if (errMsg.includes('cancel')) {
          appStore.showInfo(t('payment.qr.cancelled'))
          resetPayment()
        } else if (errMsg && !errMsg.includes('ok')) {
          resetPayment()
          const fallbackApplied = await attemptMobileQrFallback(
            { reason: 'WECHAT_JSAPI_FAILED', message: errMsg },
            {
              orderAmount,
              orderType,
              planId,
              paymentType: visibleMethod,
              attempted: options.mobileQrFallbackAttempted === true,
            },
          )
          if (!fallbackApplied) {
            applyScenarioError({ reason: 'WECHAT_JSAPI_FAILED', message: errMsg }, visibleMethod)
          }
        } else {
          const resultState = { ...decision.paymentState }
          resetPayment()
          await redirectToPaymentResult(resultState)
        }
      } catch (err: unknown) {
        resetPayment()
        const fallbackApplied = await attemptMobileQrFallback(err, {
          orderAmount,
          orderType,
          planId,
          paymentType: visibleMethod,
          attempted: options.mobileQrFallbackAttempted === true,
        })
        if (!fallbackApplied) {
          throw err
        }
      }
      return
    }
    if (decision.kind === 'redirect_waiting' && decision.paymentState.payUrl) {
      if (isMobileDevice()) {
        window.location.href = decision.paymentState.payUrl
        return
      }
      openWindow(decision.paymentState.payUrl)
    }
  } catch (err: unknown) {
    const apiErr = err as Record<string, unknown>
    if (apiErr.reason === 'TOO_MANY_PENDING') {
      const metadata = apiErr.metadata as Record<string, unknown> | undefined
      errorMessage.value = t('payment.errors.tooManyPending', { max: metadata?.max || '' })
      errorHintMessage.value = ''
    } else if (apiErr.reason === 'CANCEL_RATE_LIMITED') {
      errorMessage.value = t('payment.errors.cancelRateLimited')
      errorHintMessage.value = ''
    } else if (await attemptMobileQrFallback(err, {
      orderAmount,
      orderType,
      planId,
      paymentType: requestType,
      attempted: options.mobileQrFallbackAttempted === true,
    })) {
      return
    } else {
      const handled = applyScenarioError(
        err,
        normalizeVisibleMethod(options.paymentType || selectedMethod.value) || selectedMethod.value,
      )
      if (!handled) {
        errorMessage.value = extractI18nErrorMessage(err, t, 'payment.errors', extractApiErrorMessage(err, t('payment.result.failed')))
        errorHintMessage.value = ''
      }
      if (handled) {
        return
      }
    }
    appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  } finally {
    submitting.value = false
  }
}

interface MobileQrFallbackContext {
  orderAmount: number
  orderType: OrderType
  planId?: number
  paymentType: string
  attempted: boolean
}

function shouldFallbackToDesktopQr(err: unknown, paymentMethod: string, attempted: boolean): boolean {
  if (attempted || !isMobileDevice()) {
    return false
  }

  const normalizedMethod = normalizeVisibleMethod(paymentMethod) || paymentMethod
  const reason = typeof err === 'object' && err && 'reason' in err && typeof err.reason === 'string'
    ? err.reason
    : ''
  const message = err instanceof Error
    ? err.message
    : (typeof err === 'object' && err && 'message' in err && typeof err.message === 'string'
      ? err.message
      : '')
  const normalizedMessage = message.toLowerCase()

  if (normalizedMethod === 'wxpay') {
    return reason === 'WECHAT_H5_NOT_AUTHORIZED'
      || reason === 'WECHAT_PAYMENT_MP_NOT_CONFIGURED'
      || reason === 'WECHAT_JSAPI_FAILED'
      || reason === 'PAYMENT_GATEWAY_ERROR'
      || reason === 'UNHANDLED_PAYMENT_SCENARIO'
      || normalizedMessage.includes('weixinjsbridge is unavailable')
      || normalizedMessage.includes('wechat_jsapi_unavailable')
  }

  if (normalizedMethod === 'alipay') {
    return reason === 'PAYMENT_GATEWAY_ERROR' || reason === 'UNHANDLED_PAYMENT_SCENARIO'
  }

  return false
}

async function attemptMobileQrFallback(err: unknown, context: MobileQrFallbackContext): Promise<boolean> {
  if (!shouldFallbackToDesktopQr(err, context.paymentType, context.attempted)) {
    return false
  }

  try {
    const visibleMethod = normalizeVisibleMethod(context.paymentType) || context.paymentType
    const payload = buildCreateOrderPayload({
      amount: context.orderAmount,
      paymentType: visibleMethod,
      orderType: context.orderType,
      planId: context.planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: false,
      isWechatBrowser: false,
    })
    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const stripeMethod = visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType: context.orderType,
      isMobile: false,
      isWechatBrowser: false,
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
    })

    if (decision.kind !== 'qr_waiting' || !decision.paymentState.qrCode) {
      return false
    }

    errorMessage.value = ''
    errorHintMessage.value = ''
    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)
    appStore.showWarning(t('payment.errors.mobilePaymentFallbackToQr'))
    return true
  } catch {
    return false
  }
}

function applyScenarioError(err: unknown, paymentMethod: string): boolean {
  const descriptor = describePaymentScenarioError(err, {
    paymentMethod,
    isMobile: isMobileDevice(),
    isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
  })
  if (!descriptor) {
    errorMessage.value = ''
    errorHintMessage.value = ''
    return false
  }
  errorMessage.value = t(descriptor.messageKey)
  errorHintMessage.value = descriptor.hintKey ? t(descriptor.hintKey) : ''
  appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  return true
}

async function resumeWechatPaymentFromQuery() {
  const resume = parseWechatResumeRoute(route.query, checkout.value.plans, validAmount.value)
  if (!resume) {
    return
  }

  selectedMethod.value = resume.paymentType
  if (resume.orderType === 'balance' && resume.orderAmount > 0) {
    amount.value = resume.orderAmount
  }
  if (resume.orderType === 'subscription' && resume.planId) {
    selectedPlan.value = checkout.value.plans.find(plan => plan.id === resume.planId) ?? null
  }

  await router.replace({ path: route.path, query: stripWechatResumeQuery(route.query) })

  if (resume.wechatResumeToken) {
    await createOrder(0, resume.orderType, resume.planId, {
      wechatResumeToken: resume.wechatResumeToken,
      paymentType: resume.paymentType,
      isResume: true,
    })
    return
  }

  if (resume.orderAmount > 0 && resume.openid) {
    await createOrder(resume.orderAmount, resume.orderType, resume.planId, {
      openid: resume.openid,
      paymentType: resume.paymentType,
      isResume: true,
    })
  }
}

onMounted(async () => {
  try {
    const res = await paymentAPI.getCheckoutInfo()
    checkout.value = res.data
    if (enabledMethods.value.length) {
      const order: readonly string[] = METHOD_ORDER
      const sorted = [...enabledMethods.value].sort((a, b) => {
        const ai = order.indexOf(a)
        const bi = order.indexOf(b)
        return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
      })
      selectedMethod.value = sorted[0]
    }
    if (typeof window !== 'undefined') {
      if (hasWechatResumeQuery(route.query)) {
        removeRecoverySnapshot()
      }
      const routeResumeToken = typeof route.query.resume_token === 'string'
        ? route.query.resume_token
        : typeof route.query.wechat_resume_token === 'string'
          ? route.query.wechat_resume_token
          : undefined
      const restored = readPaymentRecoverySnapshot(
        window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
        { resumeToken: routeResumeToken },
      )
      if (restored) {
        paymentState.value = restored
        paymentPhase.value = 'paying'
        const restoredMethod = normalizeVisibleMethod(restored.paymentType)
          || (visibleMethods.value[restored.paymentType] ? restored.paymentType : '')
        if (restoredMethod) {
          selectedMethod.value = restoredMethod
        }
      } else {
        removeRecoverySnapshot()
      }
    }
    await resumeWechatPaymentFromQuery()
    if (checkout.value.balance_disabled) {
      activeTab.value = 'subscription'
    }
    // Handle renewal navigation: ?tab=subscription&group=123
    if (route.query.tab === 'subscription') {
      activeTab.value = 'subscription'
      if (route.query.group) {
        const groupId = Number(route.query.group)
        const groupPlans = checkout.value.plans.filter(p => p.group_id === groupId)
        if (groupPlans.length === 1) {
          selectedPlan.value = groupPlans[0]
        } else if (groupPlans.length > 1) {
          renewGroupId.value = groupId
          showRenewalModal.value = true
        }
      }
    }
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { loading.value = false }
  // Fetch active subscriptions (uses cache, non-blocking)
  subscriptionStore.fetchActiveSubscriptions().catch(() => {})
})
</script>

<style scoped>
.checkout-page {
  --checkout-ink: #19201e;
  --checkout-muted: #66706c;
  --checkout-line: rgba(38, 52, 47, 0.13);
  --checkout-line-strong: rgba(38, 52, 47, 0.22);
  --checkout-surface: rgba(255, 255, 252, 0.92);
  --checkout-surface-soft: rgba(242, 245, 240, 0.9);
  --checkout-coral: #e96f62;
  --checkout-coral-soft: #fff0ec;
  --checkout-mint: #23847d;
  --checkout-mint-soft: #e8f5f1;
  --checkout-blue: #3977bd;
  --checkout-shadow: 0 18px 50px rgba(67, 72, 66, 0.08);
  width: min(100%, 1180px);
  min-width: 0;
  margin: 0 auto;
  color: var(--checkout-ink);
}

:global(.dark) .checkout-page {
  --checkout-ink: #f5f8f5;
  --checkout-muted: #b7c2bd;
  --checkout-line: rgba(231, 239, 235, 0.14);
  --checkout-line-strong: rgba(231, 239, 235, 0.25);
  --checkout-surface: rgba(31, 42, 48, 0.94);
  --checkout-surface-soft: rgba(38, 51, 56, 0.92);
  --checkout-coral: #ff8d7e;
  --checkout-coral-soft: rgba(149, 66, 58, 0.24);
  --checkout-mint: #65c8bd;
  --checkout-mint-soft: rgba(36, 128, 119, 0.22);
  --checkout-blue: #7bb3ef;
  --checkout-shadow: 0 18px 50px rgba(0, 8, 10, 0.23);
}

.checkout-heading {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 4px 0 26px;
  border-bottom: 1px solid var(--checkout-line);
}

.checkout-heading__mark {
  display: grid;
  width: 48px;
  height: 48px;
  flex: 0 0 48px;
  place-items: center;
  border: 1px solid rgba(233, 111, 98, 0.28);
  border-radius: 8px;
  color: var(--checkout-coral);
  background: var(--checkout-coral-soft);
  box-shadow: inset 0 0 0 4px rgba(255, 255, 255, 0.22);
}

.checkout-heading__eyebrow {
  margin: 0 0 3px;
  color: var(--checkout-coral);
  font-size: 11px;
  font-weight: 750;
  line-height: 1.3;
  text-transform: uppercase;
  letter-spacing: 0;
}

.checkout-heading h1 {
  margin: 0;
  color: var(--checkout-ink);
  font-size: 28px;
  font-weight: 760;
  line-height: 1.18;
  letter-spacing: 0;
}

.checkout-heading__copy {
  margin: 5px 0 0;
  color: var(--checkout-muted);
  font-size: 13px;
  line-height: 1.55;
}

.checkout-protection {
  display: inline-flex;
  min-height: 34px;
  flex: 0 0 auto;
  align-items: center;
  gap: 7px;
  padding: 7px 10px;
  border: 1px solid rgba(35, 132, 125, 0.2);
  border-radius: 8px;
  color: var(--checkout-mint);
  background: var(--checkout-mint-soft);
  font-size: 12px;
  font-weight: 650;
}

.checkout-loading {
  display: grid;
  min-height: 360px;
  place-items: center;
}

.checkout-spinner,
.checkout-submit__spinner {
  display: inline-block;
  border: 2px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: checkout-spin 0.75s linear infinite;
}

.checkout-spinner {
  width: 32px;
  height: 32px;
  color: var(--checkout-coral);
}

.checkout-tabs {
  display: grid;
  width: min(100%, 430px);
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px;
  margin: 22px 0;
  padding: 4px;
  border: 1px solid var(--checkout-line);
  border-radius: 8px;
  background: var(--checkout-surface-soft);
}

.checkout-tabs button {
  display: inline-flex;
  min-width: 0;
  min-height: 40px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 8px 14px;
  border: 1px solid transparent;
  border-radius: 6px;
  color: var(--checkout-muted);
  font-size: 14px;
  font-weight: 650;
  transition: border-color 160ms ease, background-color 160ms ease, color 160ms ease, box-shadow 160ms ease;
}

.checkout-tabs button:hover {
  color: var(--checkout-ink);
}

.checkout-tabs button.active {
  border-color: var(--checkout-line);
  color: var(--checkout-ink);
  background: var(--checkout-surface);
  box-shadow: 0 4px 14px rgba(52, 59, 55, 0.08);
}

.checkout-tabs button:focus-visible,
.checkout-help__preview:focus-visible,
.checkout-modal__close:focus-visible {
  outline: 2px solid var(--checkout-mint);
  outline-offset: 2px;
}

.checkout-shell {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(0, 1fr) minmax(300px, 350px);
  gap: 22px;
  align-items: start;
  margin-top: 22px;
}

.checkout-flow {
  display: grid;
  min-width: 0;
  gap: 16px;
}

.checkout-section,
.checkout-empty,
.checkout-subscriptions,
.checkout-help {
  border: 1px solid var(--checkout-line);
  border-radius: 8px;
  background: var(--checkout-surface);
  box-shadow: var(--checkout-shadow);
}

.checkout-section {
  min-width: 0;
  padding: 24px;
}

.checkout-section__heading {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 20px;
}

.checkout-section__heading h2 {
  margin: 0;
  color: var(--checkout-ink);
  font-size: 16px;
  font-weight: 720;
  line-height: 1.35;
  letter-spacing: 0;
}

.checkout-section__heading p {
  margin: 3px 0 0;
  color: var(--checkout-muted);
  font-size: 12px;
  line-height: 1.5;
}

.checkout-step {
  display: grid;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  place-items: center;
  border-radius: 50%;
  color: #9c4139;
  background: var(--checkout-coral-soft);
  font-size: 12px;
  font-weight: 800;
}

.checkout-step--mint {
  color: var(--checkout-mint);
  background: var(--checkout-mint-soft);
}

.checkout-message,
.checkout-offer,
.checkout-summary__notice {
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.55;
}

.checkout-message {
  margin: 12px 0 0;
  padding: 9px 11px;
}

.checkout-message--warning {
  border: 1px solid rgba(202, 128, 41, 0.25);
  color: #99601e;
  background: rgba(255, 244, 222, 0.9);
}

:global(.dark) .checkout-message--warning {
  color: #ffd294;
  background: rgba(133, 82, 28, 0.22);
}

.checkout-offer {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  margin-top: 14px;
  padding: 10px 12px;
  border: 1px solid rgba(35, 132, 125, 0.18);
  color: var(--checkout-mint);
  background: var(--checkout-mint-soft);
}

.checkout-offer p {
  margin: 0;
}

.checkout-summary {
  position: sticky;
  top: 92px;
  min-width: 0;
  overflow: hidden;
  padding: 22px;
  border: 1px solid rgba(255, 255, 255, 0.09);
  border-radius: 8px;
  color: #f4f7f4;
  background: #1d2826;
  box-shadow: 0 22px 55px rgba(33, 42, 39, 0.2);
}

.checkout-summary::before {
  position: absolute;
  top: 0;
  right: 0;
  left: 0;
  height: 3px;
  background: linear-gradient(90deg, var(--checkout-coral) 0 38%, #f1c56b 38% 57%, #67bcb2 57% 100%);
  content: '';
}

.checkout-summary__account {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
  padding-bottom: 18px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.12);
}

.checkout-summary__avatar {
  display: grid;
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  place-items: center;
  border-radius: 8px;
  color: #ffd0c9;
  background: rgba(233, 111, 98, 0.18);
}

.checkout-summary__account p,
.checkout-summary__balance span {
  margin: 0;
  color: rgba(244, 247, 244, 0.62);
  font-size: 10px;
  line-height: 1.4;
}

.checkout-summary__account strong {
  display: block;
  max-width: 100%;
  overflow: hidden;
  margin-top: 2px;
  font-size: 13px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.checkout-summary__balance {
  flex: 0 0 auto;
  text-align: right;
}

.checkout-summary__balance strong {
  color: #8ed8ce;
  font-size: 13px;
}

.checkout-summary__total {
  padding: 24px 0 20px;
  text-align: center;
}

.checkout-summary__total p {
  margin: 0 0 5px;
  color: rgba(244, 247, 244, 0.68);
  font-size: 12px;
}

.checkout-summary__total strong {
  display: block;
  max-width: 100%;
  overflow-wrap: anywhere;
  color: #ffffff;
  font-size: 31px;
  font-weight: 760;
  line-height: 1.18;
  letter-spacing: 0;
}

.checkout-breakdown {
  display: grid;
  gap: 10px;
  margin: 0;
  padding: 16px 0;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.checkout-breakdown div {
  display: flex;
  min-width: 0;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
}

.checkout-breakdown dt {
  color: rgba(244, 247, 244, 0.62);
  font-size: 12px;
}

.checkout-breakdown dd {
  margin: 0;
  overflow-wrap: anywhere;
  color: #f8faf8;
  font-size: 12px;
  font-weight: 650;
  text-align: right;
}

.checkout-breakdown__accent dd {
  color: #8ed8ce;
}

.checkout-summary__notice {
  margin: 12px 0 0;
  padding: 9px 10px;
  color: rgba(244, 247, 244, 0.73);
  background: rgba(255, 255, 255, 0.06);
}

.checkout-submit {
  width: 100%;
  min-height: 46px;
  margin-top: 18px;
  border-radius: 7px;
  font-size: 14px;
  font-weight: 700;
}

.checkout-submit__content {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.checkout-submit__spinner {
  width: 15px;
  height: 15px;
}

.checkout-cancel {
  width: 100%;
  margin-top: 9px;
}

.checkout-summary__security {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  margin-top: 12px;
  color: rgba(244, 247, 244, 0.5);
  font-size: 10px;
  line-height: 1.45;
  text-align: center;
}

.checkout-plan {
  min-width: 0;
}

.checkout-plan__topline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--checkout-line);
}

.checkout-plan__validity {
  color: var(--checkout-muted);
  font-size: 12px;
}

.checkout-plan__price {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 9px;
  padding-top: 18px;
}

.checkout-plan__price strong {
  font-size: 29px;
  line-height: 1.15;
}

.checkout-plan__price span {
  color: var(--checkout-muted);
  font-size: 13px;
  text-decoration: line-through;
}

.checkout-plan__description {
  margin: 10px 0 0;
  color: var(--checkout-muted);
  font-size: 13px;
  line-height: 1.6;
}

.checkout-plan__metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1px;
  overflow: hidden;
  margin: 18px 0 0;
  border: 1px solid var(--checkout-line);
  border-radius: 7px;
  background: var(--checkout-line);
}

.checkout-plan__metrics div {
  min-width: 0;
  padding: 11px;
  background: var(--checkout-surface-soft);
}

.checkout-plan__metrics dt {
  color: var(--checkout-muted);
  font-size: 10px;
}

.checkout-plan__metrics dd {
  margin: 4px 0 0;
  overflow-wrap: anywhere;
  color: var(--checkout-ink);
  font-size: 13px;
  font-weight: 680;
}

.checkout-catalog {
  display: grid;
  gap: 20px;
  margin-top: 22px;
}

.checkout-empty {
  display: grid;
  min-height: 220px;
  place-items: center;
  align-content: center;
  gap: 10px;
  margin-top: 22px;
  padding: 28px;
  color: var(--checkout-muted);
  text-align: center;
}

.checkout-empty p {
  margin: 0;
}

.checkout-subscriptions {
  padding: 22px;
}

.checkout-subscriptions h2 {
  margin: 0 0 14px;
  color: var(--checkout-ink);
  font-size: 15px;
  font-weight: 720;
}

.checkout-subscriptions__list {
  display: grid;
  gap: 8px;
}

.checkout-subscription-row {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 11px;
  padding: 12px;
  border: 1px solid var(--checkout-line);
  border-radius: 7px;
  background: var(--checkout-surface-soft);
}

.checkout-subscription-row strong {
  color: var(--checkout-ink);
  font-size: 13px;
}

.checkout-subscription-row p {
  display: flex;
  flex-wrap: wrap;
  gap: 5px 12px;
  margin: 4px 0 0;
  color: var(--checkout-muted);
  font-size: 11px;
}

.checkout-help {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-top: 20px;
  padding: 16px 18px;
  box-shadow: none;
}

.checkout-help p {
  margin: 0 0 3px;
  color: var(--checkout-ink);
  font-size: 13px;
  font-weight: 700;
}

.checkout-help span {
  color: var(--checkout-muted);
  font-size: 12px;
  line-height: 1.55;
}

.checkout-help__preview {
  display: flex;
  min-width: 0;
  flex: 0 0 auto;
  align-items: center;
  gap: 9px;
  padding: 5px;
  padding-right: 10px;
  border: 1px solid var(--checkout-line);
  border-radius: 7px;
  background: var(--checkout-surface-soft);
  transition: border-color 160ms ease, transform 160ms ease;
}

.checkout-help__preview:hover {
  border-color: var(--checkout-line-strong);
  transform: translateY(-1px);
}

.checkout-help__preview img {
  width: 42px;
  height: 42px;
  border-radius: 5px;
  object-fit: cover;
}

.checkout-payment-stage {
  width: min(100%, 620px);
  margin: 24px auto 0;
}

.checkout-modal-backdrop {
  position: fixed;
  z-index: 1000;
  inset: 0;
  display: grid;
  overflow-y: auto;
  place-items: center;
  padding: 20px;
  background: rgba(10, 17, 16, 0.66);
  backdrop-filter: blur(8px);
}

.checkout-modal,
.checkout-image-modal {
  position: relative;
  width: min(100%, 760px);
  max-height: calc(100vh - 40px);
  overflow-y: auto;
  border: 1px solid rgba(255, 255, 255, 0.24);
  border-radius: 8px;
  color: #19201e;
  background: #f8f8f4;
  box-shadow: 0 30px 90px rgba(0, 0, 0, 0.3);
}

.checkout-modal {
  padding: 24px;
}

.checkout-modal h2 {
  margin: 0 42px 18px 0;
  font-size: 20px;
  font-weight: 750;
  letter-spacing: 0;
}

.checkout-modal__plans {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.checkout-modal__close {
  position: absolute;
  z-index: 2;
  top: 14px;
  right: 14px;
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border: 1px solid rgba(38, 52, 47, 0.14);
  border-radius: 7px;
  color: #4d5753;
  background: rgba(255, 255, 255, 0.82);
}

.checkout-image-modal {
  display: grid;
  width: min(100%, 820px);
  place-items: center;
  padding: 16px;
}

.checkout-image-modal img {
  display: block;
  max-width: 100%;
  max-height: calc(100vh - 72px);
  border-radius: 6px;
  object-fit: contain;
}

.checkout-image-modal__close {
  color: #19201e;
}

.modal-enter-active,
.modal-leave-active {
  transition: opacity 180ms ease;
}

.modal-enter-active .checkout-modal,
.modal-enter-active .checkout-image-modal,
.modal-leave-active .checkout-modal,
.modal-leave-active .checkout-image-modal {
  transition: transform 180ms ease, opacity 180ms ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .checkout-modal,
.modal-enter-from .checkout-image-modal,
.modal-leave-to .checkout-modal,
.modal-leave-to .checkout-image-modal {
  opacity: 0;
  transform: translateY(10px) scale(0.985);
}

@keyframes checkout-spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 960px) {
  .checkout-shell {
    grid-template-columns: minmax(0, 1fr);
  }

  .checkout-summary {
    position: relative;
    top: auto;
  }
}

@media (max-width: 640px) {
  .checkout-heading {
    align-items: flex-start;
    flex-wrap: wrap;
    gap: 12px;
    padding-bottom: 20px;
  }

  .checkout-heading__mark {
    width: 42px;
    height: 42px;
    flex-basis: 42px;
  }

  .checkout-heading h1 {
    font-size: 23px;
  }

  .checkout-protection {
    width: 100%;
    justify-content: center;
  }

  .checkout-tabs {
    width: 100%;
    margin: 16px 0;
  }

  .checkout-tabs button {
    padding-inline: 8px;
    font-size: 13px;
  }

  .checkout-shell,
  .checkout-catalog,
  .checkout-empty {
    margin-top: 16px;
  }

  .checkout-section,
  .checkout-summary,
  .checkout-subscriptions {
    padding: 17px;
  }

  .checkout-section__heading {
    margin-bottom: 16px;
  }

  .checkout-summary__account {
    align-items: flex-start;
  }

  .checkout-summary__total strong {
    font-size: 27px;
  }

  .checkout-plan__metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .checkout-subscription-row {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .checkout-help {
    align-items: stretch;
    flex-direction: column;
  }

  .checkout-help__preview {
    width: 100%;
  }

  .checkout-modal-backdrop {
    align-items: end;
    padding: 10px;
  }

  .checkout-modal,
  .checkout-image-modal {
    max-height: calc(100vh - 20px);
  }

  .checkout-modal {
    padding: 18px;
  }

  .checkout-modal__plans {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (prefers-reduced-motion: reduce) {
  .checkout-tabs button,
  .checkout-help__preview,
  .modal-enter-active,
  .modal-leave-active,
  .modal-enter-active .checkout-modal,
  .modal-enter-active .checkout-image-modal,
  .modal-leave-active .checkout-modal,
  .modal-leave-active .checkout-image-modal {
    transition: none;
  }

  .checkout-spinner,
  .checkout-submit__spinner {
    animation-duration: 1.5s;
  }
}
</style>
