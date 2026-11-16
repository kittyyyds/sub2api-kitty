<template>
  <AppLayout>
    <div class="checkout-page">
      <div
        v-if="!loading && tabs.length > 1 && paymentPhase === 'select' && !selectedPlan"
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
      <div class="checkout-body" :class="{ 'is-package-selection': !cashierOpen && activeTab === 'recharge' && paymentPhase === 'select' }">
      <header v-show="cashierOpen || activeTab !== 'recharge' || paymentPhase !== 'select'" class="checkout-heading">
        <div class="checkout-heading__mark" aria-hidden="true">
          <Icon name="creditCard" size="lg" :stroke-width="1.8" />
        </div>
        <div class="min-w-0 flex-1">
          <h1>{{ t('payment.cashierTitle') }}</h1>
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
          <div v-if="tabs.length === 0" class="checkout-empty">
            <p>{{ t('payment.billingUnavailable') }}</p>
          </div>
          <template v-else-if="activeTab === 'recharge'">
            <div v-if="enabledMethods.length === 0" class="checkout-empty">
              <Icon name="creditCard" size="xl" />
              <p>{{ t('payment.notAvailable') }}</p>
            </div>

            <template v-else>
              <div v-if="renderedBonusNotice" class="prose dark:prose-invert mb-4 max-w-none" v-html="renderedBonusNotice" />
              <RechargePackages v-show="!cashierOpen" :amounts="rechargeAmounts" :bonus-rules="checkout.recharge_bonus_rules" :bonus-tiers="rechargeBonusTiers" :bonus-mode="rechargeBonusMode"
                :recharge-multiplier="balanceRechargeMultiplier" :min="globalMinAmount" :max="globalMaxAmount" :currency="selectedCurrency"
                :balance="balancePreviewReady ? user?.balance ?? 0 : null" @select="selectRechargePackage" />
              <button v-if="cashierOpen" type="button" class="checkout-back-packages" @click="backToPackages"><Icon name="arrowLeft" size="sm" />{{ t('payment.packages.back') }}</button>
            <div v-show="cashierOpen" class="checkout-shell">
              <div class="checkout-flow">
                <section class="checkout-selected-package" aria-labelledby="checkout-amount-title">
                  <span>{{ t('payment.packages.selected') }}</span>
                  <h2 id="checkout-amount-title" tabindex="-1">{{ formatSelectedPaymentAmount(validAmount) }}</h2>
                  <p>{{ t('payment.creditedBalance') }} <strong>${{ creditedAmount.toFixed(2) }}</strong></p>
                  <p v-if="balanceRechargeMultiplier !== 1">{{ t('payment.rechargeRatePreview', { currency: selectedCurrency, usd: balanceRechargeMultiplier.toFixed(2) }) }}</p>
                  <p v-if="activeBonusRule" class="checkout-selected-bonus">{{ t(activeBonusRule.validity_days === 0 ? 'payment.permanentTierBonus' : 'payment.tierBonus', { amount: activeBonusRule.bonus_amount.toFixed(2), days: activeBonusRule.validity_days }) }}</p>
                </section>

                <section class="checkout-section" aria-labelledby="checkout-method-title">
                  <div class="checkout-section__heading">
                    <span class="checkout-step checkout-step--mint">1</span>
                    <div>
                      <h2 id="checkout-method-title">{{ t('payment.selectMethod') }}</h2>
                      <p>{{ t('payment.selectMethodHint') }}</p>
                    </div>
                  </div>
                  <PaymentMethodSelector
                    detailed
                    :methods="methodOptions"
                    :selected="selectedMethod"
                    @select="selectedMethod = $event"
                  />
                  <p v-if="amountError" class="checkout-message checkout-message--warning" role="alert">{{ amountError }}</p>
                </section>
                <RechargeCouponPicker :amount="validAmount" :method="selectedMethod" :currency="selectedCurrency" :disabled="submitting" :initial-id="initialCouponId" @change="onCouponChange" />
              </div>

              <aside class="checkout-sidebar">
                <section class="checkout-summary" aria-labelledby="checkout-summary-title">
                <div class="checkout-summary__total">
                  <p id="checkout-summary-title">{{ t('payment.actualPay') }}</p>
                  <strong>{{ formatSelectedPaymentAmount(totalAmount) }}</strong>
                </div>

                <dl class="checkout-breakdown">
                  <div>
                    <dt>{{ t('payment.paymentAmount') }}</dt>
                    <dd>{{ formatSelectedPaymentAmount(validAmount) }}</dd>
                  </div>
                  <div v-if="discountAmount > 0" class="checkout-breakdown__accent">
                    <dt>{{ t('payment.rechargeBonus.discountLabelWithPercent', { percent: bonusQuote.percent }) }}</dt>
                    <dd>-{{ formatSelectedPaymentAmount(discountAmount) }}</dd>
                  </div>
                  <div v-if="couponQuote && couponQuote.discount_amount > 0" class="checkout-breakdown__accent">
                    <dt>{{ t('coupon.discount') }}</dt><dd>-{{ formatSelectedPaymentAmount(couponQuote.discount_amount) }}</dd>
                  </div>
                  <div v-if="feeRate > 0">
                    <dt>{{ t('payment.fee') }} ({{ feeRate }}%)</dt>
                    <dd>{{ formatSelectedPaymentAmount(feeAmount) }}</dd>
                  </div>
                  <div>
                    <dt>{{ t('payment.baseCredit') }}</dt>
                    <dd data-testid="base-credit">${{ baseCreditAmount.toFixed(2) }}</dd>
                  </div>
                  <div v-if="showBonusRow" class="checkout-breakdown__accent">
                    <dt>{{ t('payment.siteRechargeBonus') }}</dt><dd>+${{ bonusQuote.bonus.toFixed(2) }}</dd>
                  </div>
                  <div v-if="couponQuote && couponQuote.coupon_bonus > 0" class="checkout-breakdown__accent">
                    <dt>{{ t('coupon.couponBonus') }}</dt><dd>+${{ couponQuote.coupon_bonus.toFixed(2) }}</dd>
                  </div>
                  <div v-if="activeBonusRule" class="checkout-breakdown__accent">
                    <dt>{{ t('payment.siteRechargeBonus') }} / {{ activeBonusRule.validity_days === 0 ? t('payment.neverExpires') : activeBonusRule.validity_days + ' ' + t('payment.days') }}</dt>
                    <dd data-testid="new-bonus">+${{ activeBonusRule.bonus_amount.toFixed(2) }}</dd>
                  </div>
                  <div class="checkout-breakdown__accent">
                    <dt>{{ t('payment.creditedBalance') }}</dt>
                    <dd data-testid="total-credit">${{ creditedAmount.toFixed(2) }}</dd>
                  </div>
                </dl>

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
                </section>

                <section class="checkout-account" aria-labelledby="checkout-account-title">
                  <div class="checkout-account__heading">
                    <span class="checkout-summary__avatar" aria-hidden="true"><Icon name="user" size="sm" /></span>
                    <div class="min-w-0">
                      <h2 id="checkout-account-title">{{ t('payment.rechargeAccount') }}</h2>
                      <p>{{ user?.username || '-' }}</p>
                    </div>
                  </div>
                  <dl class="checkout-balances">
                    <div>
                      <dt>{{ t('payment.currentBalance') }}</dt>
                      <dd data-testid="current-balance">{{ balancePreviewReady ? '$' + (user?.balance ?? 0).toFixed(2) : '-' }}</dd>
                    </div>
                    <div>
                      <dt>{{ t('payment.projectedBalance') }}</dt>
                      <dd data-testid="projected-balance">{{ balancePreviewReady && canSubmit ? '$' + projectedBalance.toFixed(2) : '-' }}</dd>
                    </div>
                  </dl>
                  <p v-if="checkout.recharge_bonus_summary.available_amount > 0" class="checkout-account__note">
                    {{ t('payment.includedBonusBalance', { amount: checkout.recharge_bonus_summary.available_amount.toFixed(2) }) }}
                  </p>
                  <p v-if="checkout.recharge_bonus_summary.next_expires_at" class="checkout-account__note">
                    {{ t('payment.nextBonusExpiry', { time: formatBonusExpiry(checkout.recharge_bonus_summary.next_expires_at) }) }}
                  </p>
                  <p class="checkout-account__note checkout-account__note--muted">{{ t(balancePreviewReady ? 'payment.balanceEstimateNotice' : 'payment.balanceUnavailable') }}</p>
                </section>
                <CheckoutSupport />
              </aside>
            </div>
            </template>
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
                    detailed
                    :selected="selectedMethod"
                    @select="selectedMethod = $event"
                  />
                </section>
              </div>

              <aside class="checkout-sidebar">
              <div
                v-if="selectedActiveSubscription"
                class="card bg-white p-6 dark:bg-dark-800"
                data-test="renewal-mode-selector"
              >
                <div class="mb-3">
                  <p class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('payment.renewalMode.title') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('payment.renewalMode.currentExpiry', { date: formatDateTimeToMinute(selectedActiveSubscription.expires_at) }) }}
                  </p>
                </div>
                <div class="grid grid-cols-2 gap-3" role="radiogroup" :aria-label="t('payment.renewalMode.title')">
                  <button
                    type="button"
                    role="radio"
                    :aria-checked="renewalMode === 'restart'"
                    :class="[
                      'min-h-[60px] min-w-0 rounded-lg border px-3 py-2 text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2',
                      renewalMode === 'restart'
                        ? 'border-primary-500 bg-primary-50 text-primary-700 shadow-sm dark:bg-primary-900/30 dark:text-primary-300'
                        : 'border-gray-300 bg-white text-gray-700 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200'
                    ]"
                    @click="renewalMode = 'restart'"
                  >
                    {{ t('payment.renewalMode.restart') }}
                  </button>
                  <button
                    type="button"
                    role="radio"
                    :aria-checked="renewalMode === 'extend'"
                    :class="[
                      'min-h-[60px] min-w-0 rounded-lg border px-3 py-2 text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2',
                      renewalMode === 'extend'
                        ? 'border-primary-500 bg-primary-50 text-primary-700 shadow-sm dark:bg-primary-900/30 dark:text-primary-300'
                        : 'border-gray-300 bg-white text-gray-700 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200'
                    ]"
                    @click="renewalMode = 'extend'"
                  >
                    {{ t('payment.renewalMode.extend') }}
                  </button>
                </div>
                <p class="mt-3 text-xs leading-relaxed" :class="renewalMode === 'restart' ? 'text-amber-600 dark:text-amber-400' : 'text-gray-500 dark:text-gray-400'">
                  {{ t(`payment.renewalMode.${renewalMode}Hint`) }}
                </p>
              </div>

                <section class="checkout-summary" aria-labelledby="subscription-summary-title">
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
                </section>
                <CheckoutSupport />
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
              <div v-if="checkout.help_text" class="checkout-help__text markdown-body overflow-x-auto break-words" v-html="renderedHelpText"></div>
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
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import '@/styles/announcement-markdown.css'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { usePaymentStore } from '@/stores/payment'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { useAppStore } from '@/stores'
import { FeatureFlags, resolveFeatureFlag } from '@/utils/featureFlags'
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel, type PeakRateFields } from '@/utils/peak-rate'
import type { SubscriptionPlan, CheckoutInfoResponse, CreateOrderResult, OrderType, SubscriptionRenewalMode } from '@/types/payment'
import { normalizeRechargeBonusMode, normalizeRechargeBonusTiers, quoteRechargeBonus } from '@/utils/rechargeBonus'
import AppLayout from '@/components/layout/AppLayout.vue'
import RechargePackages from '@/components/payment/RechargePackages.vue'
import RechargeCouponPicker from '@/components/payment/RechargeCouponPicker.vue'
import type { CouponQuote } from '@/api/coupons'
import CheckoutSupport from '@/components/payment/CheckoutSupport.vue'
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
import { formatDateTimeToMinute } from '@/utils/format'
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
const balancePreviewReady = ref(false)

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
const cashierOpen = ref(false)
async function selectRechargePackage(value: number) {
  if (!rechargeAmounts.includes(value)) return
  amount.value = value
  cashierOpen.value = true
  await router.push({ path: route.path, query: { ...route.query, checkout: '1', amount: String(value) } })
  await nextTick()
  const heading = document.getElementById('checkout-amount-title')
  document.querySelector('.checkout-page')?.scrollIntoView({ block: 'start', behavior: 'auto' })
  heading?.focus({ preventScroll: true })
}
async function backToPackages() {
  cashierOpen.value = false
  const query = { ...route.query }
  delete query.checkout
  delete query.amount
  await router.push({ path: route.path, query })
  await nextTick()
  document.querySelector<HTMLButtonElement>(`[data-package-amount="${amount.value}"] button`)?.focus()
}
function syncCheckoutRoute() {
  if (hasWechatResumeQuery(route.query)) return
  const value = Number(route.query.amount)
  cashierOpen.value = route.query.checkout === '1' && rechargeAmounts.includes(value)
  if (cashierOpen.value) amount.value = value
}
watch(() => [route.query.checkout, route.query.amount], syncCheckoutRoute)
const selectedMethod = ref('')
const initialCouponId = Number(route.query.coupon || route.query.coupon_id) || undefined
const selectedCouponId = ref(initialCouponId || 0)
const couponQuote = ref<CouponQuote | null>(null)
const couponQuotePending = ref(!!initialCouponId)
function onCouponChange(id: number, quote: CouponQuote | null, pending: boolean) {
  selectedCouponId.value = id
  couponQuote.value = quote
  couponQuotePending.value = pending
}
const selectedPlan = ref<SubscriptionPlan | null>(null)
const renewalMode = ref<SubscriptionRenewalMode>('extend')
const previewImage = ref('')
const renewalModalRef = ref<HTMLElement | null>(null)
const imageModalRef = ref<HTMLElement | null>(null)

let renewalReturnFocus: HTMLElement | null = null
let imageReturnFocus: HTMLElement | null = null

const selectedActiveSubscription = computed(() => {
  if (!selectedPlan.value) return null
  return activeSubscriptions.value.find(subscription =>
    subscription.group_id === selectedPlan.value?.group_id && subscription.status === 'active'
  ) ?? null
})

const paymentPhase = ref<'select' | 'paying'>('select')

interface CreateOrderOptions {
  couponId?: number
  openid?: string
  wechatResumeToken?: string
  paymentType?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
  renewalMode?: SubscriptionRenewalMode
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
  context: { paymentType: string; orderType: OrderType; planId?: number; orderAmount: number; renewalMode?: SubscriptionRenewalMode; couponId?: number },
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

    if (context.orderType === 'subscription' && context.renewalMode) {
      redirectUrl.searchParams.set('renewal_mode', context.renewalMode)
    } else {
      redirectUrl.searchParams.delete('renewal_mode')
    }

    if (context.orderType === 'balance' && context.couponId) {
      redirectUrl.searchParams.set('coupon_id', String(context.couponId))
    } else {
      redirectUrl.searchParams.delete('coupon_id')
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
  recharge_bonus_tiers: [], recharge_bonus_mode: 'bonus', recharge_bonus_notice: '',
})

const renderedHelpText = computed(() => DOMPurify.sanitize(
  marked.parse(checkout.value.help_text || '', { async: false, gfm: true, breaks: false }),
))

// 充值赠送活动文案：后台 Markdown 配置，空字符串时金额卡顶部不渲染
const renderedBonusNotice = computed(() => {
  const raw = (checkout.value.recharge_bonus_notice || '').trim()
  if (!raw) return ''
  return DOMPurify.sanitize(marked.parse(raw, { async: false, gfm: true, breaks: true }))
})

// 订阅功能开关（public settings 的 subscription_enabled，opt-out）。关闭后购买页只保留充值：
// 不再渲染「订阅」tab，只剩单个 tab 时顶部切换器也随之隐藏。
const subscriptionEnabled = computed(() => resolveFeatureFlag(appStore.cachedPublicSettings, FeatureFlags.subscription))

const tabs = computed(() => {
  const result: { key: 'recharge' | 'subscription'; label: string }[] = []
  if (!checkout.value.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
  if (subscriptionEnabled.value) result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
  return result
})

// tab 列表随 checkout（balance_disabled）与订阅开关变化。当前 tab 不在列表里时收敛到第一个可用 tab，
// 两个方向都覆盖：关闭订阅 → 回到充值；仅订阅站点重新打开订阅 → 进入订阅。列表为空时模板展示不可用提示。
watch(tabs, (available) => {
  if (available.some((tab) => tab.key === activeTab.value)) return
  const leavingSubscription = activeTab.value === 'subscription'
  activeTab.value = available[0]?.key ?? 'recharge'
  if (leavingSubscription) selectedPlan.value = null
})

const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
const enabledMethods = computed(() => Object.keys(visibleMethods.value))
const validAmount = computed(() => amount.value ?? 0)
const balanceRechargeMultiplier = computed(() => {
  const multiplier = checkout.value.balance_recharge_multiplier
  return Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1
})
const matchingBonusRule = computed(() => checkout.value.recharge_bonus_rules.find(rule => Math.abs(rule.recharge_amount - validAmount.value) < 0.00000001) || null)
const activeBonusRule = computed(() => selectedCouponId.value && !couponQuote.value?.stackable ? null : matchingBonusRule.value)
const rechargeAmounts = [5, 10, 30, 50, 100, 500, 1000, 5000]

// 订阅 CNY 换算汇率（1 USD = X CNY）。0 = 未配置，订阅保持 price 直付（与后端 opt-in 条件严格镜像）。
const subscriptionUsdToCnyRate = computed(() => {
  const rate = checkout.value.subscription_usd_to_cny_rate
  return Number.isFinite(rate) && rate > 0 ? rate : 0
})
const baseCreditAmount = computed(() => Math.round(validAmount.value * balanceRechargeMultiplier.value * 100) / 100)
// Existing bonus grants are already included in users.balance.
const projectedBalance = computed(() => (user.value?.balance ?? 0) + creditedAmount.value)

function formatBonusExpiry(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
const rechargeBonusTiers = computed(() => normalizeRechargeBonusTiers(checkout.value.recharge_bonus_tiers))
const rechargeBonusMode = computed(() => normalizeRechargeBonusMode(checkout.value.recharge_bonus_mode))

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

// 充值优惠：阈值按输入金额命中；赠金模式按到账基数（输入 × 倍率）加赠送，折扣模式按百分比减实付。
// 与后端 quoteRechargeBonus 一致；渠道限额、手续费、实付都按折后基数（payBaseAmount）计算，提交仍发送输入金额。
const bonusQuote = computed(() => quoteRechargeBonus(matchingBonusRule.value || (selectedCouponId.value && !couponQuote.value?.stackable) ? [] : rechargeBonusTiers.value, validAmount.value, {
  multiplier: balanceRechargeMultiplier.value,
  mode: rechargeBonusMode.value,
  currencyDigits: currencyFractionDigits(selectedCurrency.value),
}))
const payBaseAmount = computed(() => couponQuote.value?.pay_base ?? bonusQuote.value.payBase)
const discountAmount = computed(() => roundPaymentAmount(validAmount.value - payBaseAmount.value - (couponQuote.value?.discount_amount || 0), selectedCurrency.value))
const creditedAmount = computed(() => couponQuote.value ? Math.round((couponQuote.value.credited + couponQuote.value.package_bonus) * 100) / 100 : Math.round((bonusQuote.value.credited + (activeBonusRule.value?.bonus_amount || 0)) * 100) / 100)
const showBonusRow = computed(() => bonusQuote.value.mode !== 'discount' && bonusQuote.value.bonus > 0)

const methodOptions = computed<PaymentMethodOption[]>(() =>
  enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(payBaseAmount.value, type),
    }
  })
)

const feeRate = computed(() => checkout.value?.recharge_fee_rate ?? 0)
const feeAmount = computed(() => couponQuote.value ? roundPaymentAmount(couponQuote.value.pay_amount - couponQuote.value.pay_base, selectedCurrency.value) :
  feeRate.value > 0 && payBaseAmount.value > 0
    ? Math.ceil(((payBaseAmount.value * feeRate.value) / 100) * 100) / 100
    : 0
)
const totalAmount = computed(() => couponQuote.value?.pay_amount ?? (
  feeRate.value > 0 && payBaseAmount.value > 0
    ? Math.round((payBaseAmount.value + feeAmount.value) * 100) / 100
    : payBaseAmount.value)
)

const amountError = computed(() => {
  if (validAmount.value <= 0) return ''
  // No method can handle this amount
  if (!enabledMethods.value.some((m) => amountFitsMethod(payBaseAmount.value, m))) {
    return t('payment.amountNoMethod')
  }
  // Selected method can't handle this amount (but others can)
  const ml = selectedLimit.value
  if (ml) {
    if (ml.single_min > 0 && payBaseAmount.value < ml.single_min) return t('payment.amountTooLow', { min: formatSelectedPaymentAmount(ml.single_min) })
    if (ml.single_max > 0 && payBaseAmount.value > ml.single_max) return t('payment.amountTooHigh', { max: formatSelectedPaymentAmount(ml.single_max) })
  }
  return ''
})

const canSubmit = computed(() =>
  (!selectedCouponId.value || (!couponQuotePending.value && !!couponQuote.value))
    && validAmount.value > 0
    && amountFitsMethod(payBaseAmount.value, selectedMethod.value)
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
watch(() => [payBaseAmount.value, selectedMethod.value] as const, ([amt, method]) => {
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
  renewalMode.value = 'extend'
  errorMessage.value = ''
}

function selectPlanFromModal(plan: SubscriptionPlan) {
  showRenewalModal.value = false
  renewGroupId.value = null
  selectedPlan.value = plan
  renewalMode.value = 'extend'
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
  await createOrder(selectedPlan.value.price, 'subscription', selectedPlan.value.id, { renewalMode: renewalMode.value })
}

async function createOrder(orderAmount: number, orderType: OrderType, planId?: number, options: CreateOrderOptions = {}) {
  submitting.value = true
  errorMessage.value = ''
  errorHintMessage.value = ''
  const requestType = normalizeVisibleMethod(options.paymentType || selectedMethod.value) || options.paymentType || selectedMethod.value
  const effectiveCouponId = orderType === 'balance' ? (options.couponId ?? selectedCouponId.value) : undefined
  const effectiveRenewalMode = orderType === 'subscription'
    ? (options.renewalMode ?? renewalMode.value)
    : undefined
  try {
    const payload = buildCreateOrderPayload({
      amount: orderAmount,
      paymentType: requestType,
      orderType,
      planId,
      renewalMode: effectiveRenewalMode,
      couponId: effectiveCouponId,
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
        renewalMode: effectiveRenewalMode,
        couponId: effectiveCouponId,
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
              renewalMode: effectiveRenewalMode,
              couponId: effectiveCouponId,
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
          renewalMode: effectiveRenewalMode,
          couponId: effectiveCouponId,
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
      renewalMode: effectiveRenewalMode,
      couponId: effectiveCouponId,
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
  couponId?: number
  orderAmount: number
  orderType: OrderType
  planId?: number
  renewalMode?: SubscriptionRenewalMode
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
      renewalMode: context.renewalMode,
      couponId: context.couponId,
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

  selectedCouponId.value = resume.couponId || 0
  selectedMethod.value = resume.paymentType
  if (resume.orderType === 'balance' && resume.orderAmount > 0) {
    amount.value = resume.orderAmount
  }
  if (resume.orderType === 'subscription' && resume.planId) {
    selectedPlan.value = checkout.value.plans.find(plan => plan.id === resume.planId) ?? null
    renewalMode.value = resume.renewalMode ?? 'extend'
  }

  await router.replace({ path: route.path, query: stripWechatResumeQuery(route.query) })

  if (resume.wechatResumeToken) {
    await createOrder(0, resume.orderType, resume.planId, {
      wechatResumeToken: resume.wechatResumeToken,
      paymentType: resume.paymentType,
      isResume: true,
      renewalMode: resume.renewalMode,
      couponId: resume.couponId,
    })
    return
  }

  if (resume.orderAmount > 0 && resume.openid) {
    await createOrder(resume.orderAmount, resume.orderType, resume.planId, {
      openid: resume.openid,
      paymentType: resume.paymentType,
      isResume: true,
      renewalMode: resume.renewalMode,
      couponId: resume.couponId,
    })
  }
}

onMounted(async () => {
  try {
    const res = await paymentAPI.getCheckoutInfo()
    checkout.value = res.data
    // Checkout expires old grants; refresh balance afterwards to avoid counting expired credit.
    try {
      await authStore.refreshUser()
      balancePreviewReady.value = true
    } catch {
      balancePreviewReady.value = false
    }
    if (enabledMethods.value.length) {
      const order: readonly string[] = METHOD_ORDER
      const sorted = [...enabledMethods.value].sort((a, b) => {
        const ai = order.indexOf(a)
        const bi = order.indexOf(b)
        return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
      })
      selectedMethod.value = sorted[0]
      amount.value = rechargeAmounts.find(value => sorted.some(method =>
        visibleMethods.value[method].available && amountFitsMethod(value, method)
      )) ?? null
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
    syncCheckoutRoute()
    await resumeWechatPaymentFromQuery()
    // balance_disabled → the tabs watcher above moves activeTab to the subscription tab (when enabled).
    // Handle renewal navigation: ?tab=subscription&group=123 (ignored when subscriptions are disabled)
    if (route.query.tab === 'subscription' && subscriptionEnabled.value) {
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
  // Fetch active subscriptions (uses cache, non-blocking); skipped when the subscription feature is off
  if (subscriptionEnabled.value) {
    subscriptionStore.fetchActiveSubscriptions().catch(() => {})
  }
})
</script>

<style scoped>
.checkout-body.is-package-selection { padding: 0; border: 0; background: transparent; }
.checkout-back-packages { display: inline-flex; align-items: center; gap: 8px; padding: 8px 0; margin-bottom: 16px; color: var(--kc-accent-strong,#9c3f00); font-size: 14px; }
.checkout-selected-package { padding: 24px; border: 1px solid var(--kc-line,#e2e7e4); border-radius: 16px; background: var(--kc-panel-muted,#f7f8f5); }
.checkout-selected-package > span { font-size: 13px; color: var(--kc-muted,#68736e); }
.checkout-selected-package h2 { margin: 10px 0; font-size: 38px; line-height: 1.3; font-weight: 700; color: var(--kc-ink,#182b29); }
.checkout-selected-package p { font-size: 14px; line-height: 1.8; color: var(--kc-muted,#68736e); }
.checkout-selected-package strong { color: var(--kc-accent-strong,#9c3f00); }
.checkout-selected-package .checkout-selected-bonus { margin-top: 8px; color: var(--kc-warm, #a36118); }

.checkout-page {
  scroll-margin-top: calc(var(--kc-header-height, 72px) + 24px);
  --checkout-ink: var(--kc-ink, #19201e);
  --checkout-muted: var(--kc-muted, #66706c);
  --checkout-line: var(--kc-line, rgba(38, 52, 47, 0.13));
  --checkout-line-strong: var(--kc-line-strong, rgba(38, 52, 47, 0.22));
  --checkout-surface: var(--kc-panel, #ffffff);
  --checkout-surface-soft: var(--kc-panel-muted, #f4f6f5);
  --checkout-coral: var(--kc-action, #e96f62);
  --checkout-coral-soft: var(--kc-accent-soft, #fff0ec);
  --checkout-mint: var(--kc-accent-strong, #23847d);
  --checkout-mint-soft: var(--kc-accent-soft, #e8f5f1);
  --checkout-blue: var(--kc-action, #3977bd);
  --checkout-settlement: var(--kc-panel-muted, #f6faf9);
  --checkout-shadow: var(--kc-shadow, 0 18px 50px rgba(67, 72, 66, 0.08));
  width: min(100%, 1240px);
  min-width: 0;
  margin: 0 auto;
  color: var(--checkout-ink);
  font-weight: 400;
}

.checkout-body {
  padding: 28px;
  border-top: 3px solid var(--checkout-coral);
  background: var(--checkout-surface);
}

.dark .checkout-page {
  --checkout-ink: var(--kc-ink, #f5f8f5);
  --checkout-muted: var(--kc-muted, #b7c2bd);
  --checkout-line: var(--kc-line, rgba(231, 239, 235, 0.14));
  --checkout-line-strong: var(--kc-line-strong, rgba(231, 239, 235, 0.25));
  --checkout-surface: var(--kc-panel, #202a2d);
  --checkout-surface-soft: var(--kc-panel-muted, #283337);
  --checkout-settlement: var(--kc-panel-muted, #243331);
  --checkout-coral: var(--kc-action, #ff8d7e);
  --checkout-coral-soft: var(--kc-accent-soft, rgba(149, 66, 58, 0.24));
  --checkout-mint: var(--kc-accent-strong, #65c8bd);
  --checkout-mint-soft: var(--kc-accent-soft, rgba(36, 128, 119, 0.22));
  --checkout-blue: var(--kc-action, #7bb3ef);
  --checkout-shadow: var(--kc-shadow, 0 18px 50px rgba(0, 8, 10, 0.23));
}

.checkout-heading {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 0 24px;
  border-bottom: 1px solid var(--checkout-line);
}

.checkout-heading__mark {
  display: grid;
  width: 48px;
  height: 48px;
  flex: 0 0 48px;
  place-items: center;
  border: 1px solid var(--checkout-coral);
  border-radius: 8px;
  color: var(--kc-on-action, #ffffff);
  background: var(--checkout-coral);
  box-shadow: var(--kc-shadow, 0 4px 10px rgba(233, 111, 98, 0.22));
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
  font-size: 25px;
  font-weight: 700;
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
  border: 1px solid var(--kc-line-strong, rgba(35, 132, 125, 0.2));
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
  width: 100%;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px;
  margin: 0 0 20px;
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
  color: var(--checkout-mint);
  background: var(--checkout-surface);
  box-shadow: var(--kc-shadow, 0 4px 14px rgba(52, 59, 55, 0.08));
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
  grid-template-columns: minmax(0, 1fr) minmax(290px, 330px);
  gap: 28px;
  align-items: start;
  margin-top: 26px;
}

.checkout-flow {
  display: grid;
  min-width: 0;
  gap: 22px;
}

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
  padding: 0 0 22px;
  border-bottom: 1px solid var(--checkout-line);
}

.checkout-section__heading {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 20px;
}

.checkout-section__heading > div {
  min-width: 0;
}

.checkout-limit {
  flex-shrink: 0;
  margin-left: auto;
  padding: 4px 8px;
  border-radius: 4px;
  color: var(--checkout-mint);
  background: var(--checkout-mint-soft);
  font-size: 11px;
  font-weight: 500;
  white-space: nowrap;
}

.checkout-section__heading h2 {
  margin: 0;
  color: var(--checkout-ink);
  font-size: 18px;
  font-weight: 700;
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
  width: 32px;
  height: 32px;
  flex: 0 0 32px;
  place-items: center;
  border-radius: 50%;
  color: var(--kc-on-action, #ffffff);
  background: var(--checkout-coral);
  font-size: 14px;
  font-weight: 800;
}

.checkout-step--mint {
  color: var(--kc-on-action, #ffffff);
  background: var(--checkout-mint);
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
  border: 1px solid var(--kc-line-strong, rgba(202, 128, 41, 0.25));
  color: var(--kc-warm, #99601e);
  background: rgba(255, 244, 222, 0.9);
}

.dark .checkout-message--warning {
  color: var(--kc-warm, #ffd294);
  background: rgba(133, 82, 28, 0.22);
}

.checkout-offer {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  margin-top: 14px;
  padding: 10px 12px;
  border: 1px solid var(--kc-line-strong, rgba(35, 132, 125, 0.18));
  color: var(--checkout-mint);
  background: var(--checkout-mint-soft);
}

.checkout-offer p {
  margin: 0;
}

.checkout-summary {
  min-width: 0;
  padding: 22px;
  border: 1px solid var(--checkout-line);
  border-radius: 8px;
  background: var(--checkout-surface);
  box-shadow: var(--kc-shadow, 0 5px 16px rgba(38, 52, 47, 0.07));
  color: var(--checkout-ink);
}

.checkout-sidebar {
  display: grid;
  min-width: 0;
  gap: 20px;
  padding: 0 0 20px;
  background: var(--checkout-settlement);
}

.checkout-account {
  min-width: 0;
  padding: 0 20px;
}

.checkout-account__heading {
  display: flex;
  align-items: center;
  gap: 10px;
}

.checkout-account__heading h2 {
  margin: 0;
  color: var(--checkout-muted);
  font-size: 12px;
  font-weight: 500;
}

.checkout-account__heading p {
  margin: 3px 0 0;
  font-size: 14px;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.checkout-balances {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0;
  margin: 18px 0 14px;
  border-top: 1px solid var(--checkout-line);
  border-bottom: 1px solid var(--checkout-line);
}

.checkout-balances > div {
  padding: 12px 0;
  min-width: 0;
}

.checkout-balances > div:last-child {
  padding-left: 14px;
  border-left: 1px solid var(--checkout-line);
}

.checkout-balances dt {
  color: var(--checkout-muted);
  font-size: 12px;
  line-height: 1.5;
}

.checkout-balances dd {
  margin: 6px 0 0;
  font-size: 21px;
  font-weight: 700;
  overflow-wrap: anywhere;
  font-variant-numeric: tabular-nums;
}

.checkout-balances > div:last-child dd {
  color: var(--checkout-mint);
}

.checkout-account__note {
  margin: 8px 0 0;
  color: var(--checkout-muted);
  font-size: 11px;
  line-height: 1.7;
}

.checkout-account__note--muted {
  font-size: 10px;
}

.checkout-summary__account {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
  padding-bottom: 18px;
  border-bottom: 1px solid var(--checkout-line);
}

.checkout-summary__avatar {
  display: grid;
  width: 38px;
  height: 38px;
  flex: 0 0 38px;
  place-items: center;
  border-radius: 8px;
  color: var(--checkout-mint);
  background: var(--checkout-mint-soft);
}

.checkout-summary__account p,
.checkout-summary__balance span {
  margin: 0;
  color: var(--checkout-muted);
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
  color: var(--kc-accent-strong, #8ed8ce);
  font-size: 13px;
}

.checkout-summary__total {
  padding: 0 0 22px;
  text-align: left;
}

.checkout-summary__total p {
  margin: 0 0 10px;
  color: var(--checkout-muted);
  font-size: 12px;
}

.checkout-summary__total strong {
  display: block;
  max-width: 100%;
  overflow-wrap: anywhere;
  color: var(--checkout-coral);
  font-size: 40px;
  font-weight: 700;
  line-height: 1.18;
  letter-spacing: 0;
}

.checkout-breakdown {
  display: grid;
  gap: 10px;
  margin: 0;
  padding: 16px 0;
  border-top: 1px dashed var(--checkout-line-strong);
}

.checkout-breakdown div {
  display: flex;
  min-width: 0;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
}

.checkout-breakdown dt {
  color: var(--checkout-muted);
  font-size: 12px;
}

.checkout-breakdown dd {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--checkout-ink);
  font-size: 12px;
  font-weight: 650;
  text-align: right;
}

.checkout-breakdown > div:last-child {
  padding-top: 10px;
  border-top: 1px solid var(--checkout-line);
}

.checkout-breakdown > div:last-child dd {
  font-size: 15px;
}

.checkout-breakdown__accent dd {
  color: var(--checkout-mint);
}

.checkout-summary__notice {
  margin: 12px 0 0;
  padding: 9px 10px;
  color: var(--checkout-muted);
  background: var(--checkout-surface-soft);
}

.checkout-submit {
  width: 100%;
  min-height: 48px;
  margin-top: 12px;
  border-radius: 7px;
  font-size: 14px;
  font-weight: 700;
}

.checkout-page .checkout-submit {
  color: var(--kc-on-action, #ffffff);
  background: var(--kc-action, var(--checkout-blue));
  border-color: var(--kc-action, var(--checkout-blue));
  box-shadow: var(--kc-shadow, none);
}

.checkout-page .checkout-submit:hover:not(:disabled) {
  background: var(--kc-action-hover, var(--checkout-blue));
  border-color: var(--kc-action-hover, var(--checkout-blue));
}

.checkout-submit__content {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  justify-content: center;
  gap: 8px;
  flex-wrap: wrap;
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
  color: var(--checkout-muted);
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

.checkout-help > div {
  min-width: 0;
}

.checkout-help__text {
  color: var(--checkout-muted);
  font-size: 12px;
  line-height: 1.55;
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
  border: 1px solid var(--kc-line-strong, rgba(255, 255, 255, 0.24));
  border-radius: 8px;
  color: var(--kc-ink, #19201e);
  background: var(--kc-panel, #f8f8f4);
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
  border: 1px solid var(--kc-line-strong, rgba(38, 52, 47, 0.14));
  border-radius: 7px;
  color: var(--kc-muted, #4d5753);
  background: var(--kc-panel-muted, rgba(255, 255, 255, 0.82));
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
  color: var(--kc-ink, #19201e);
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

@media (max-width: 1180px) {
  .checkout-shell {
    grid-template-columns: minmax(0, 1fr);
  }

  .checkout-sidebar {
    padding-left: 0;
    border-left: 0;
  }
}

@media (max-width: 640px) {
  .checkout-body {
    padding: 20px 12px;
  }

  .checkout-section__heading {
    display: grid;
    grid-template-columns: 32px minmax(0, 1fr);
  }

  .checkout-section__heading > div {
    flex: 1;
  }

  .checkout-limit {
    grid-column: 2;
    justify-self: start;
    margin: 0;
  }

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
    margin: 0 0 16px;
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
    font-size: 36px;
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
