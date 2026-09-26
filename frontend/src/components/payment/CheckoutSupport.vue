<template>
  <section class="checkout-support" aria-labelledby="checkout-support-title">
    <div class="checkout-support__heading">
      <Icon name="infoCircle" size="sm" />
      <h2 id="checkout-support-title">{{ t('payment.supportTitle') }}</h2>
    </div>
    <p>{{ t('payment.supportDescription') }}</p>
    <button type="button" :aria-label="t('payment.copySupportQQ')" @click="copyQQ">
      <span>QQ <strong>1722345668</strong></span>
      <span class="checkout-support__action" aria-live="polite">
        <Icon :name="copied ? 'check' : 'copy'" size="sm" />
        {{ copied ? t('payment.copied') : t('payment.copyQQ') }}
      </span>
    </button>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const { copied, copyToClipboard } = useClipboard()
const appStore = useAppStore()

async function copyQQ() {
  try {
    await copyToClipboard('1722345668', t('payment.copied'))
  } catch {
    appStore.showError(t('common.copyFailed'))
  }
}
</script>

<style scoped>
.checkout-support {
  min-width: 0;
  padding: 18px 20px;
  border-top: 2px solid #eac579;
  color: #82551b;
  background: #fff9e9;
}

.checkout-support__heading {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.checkout-support__heading svg {
  flex-shrink: 0;
  margin-top: 2px;
}

h2 {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  line-height: 1.5;
}

p {
  margin: 9px 0 14px;
  font-size: 12px;
  font-weight: 400;
  line-height: 1.8;
}

button {
  display: flex;
  width: 100%;
  min-height: 44px;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
  padding: 10px;
  border: 1px solid rgba(217, 149, 39, 0.5);
  border-radius: 6px;
  background: var(--checkout-surface, #ffffff);
  box-shadow: 0 2px 5px rgba(130, 85, 27, 0.04);
  font-size: 12px;
}

button:hover {
  border-color: #d99527;
}

button:focus-visible {
  outline: 2px solid #d99527;
  outline-offset: 2px;
}

strong {
  margin-left: 6px;
  font-size: 14px;
  font-variant-numeric: tabular-nums;
}

.checkout-support__action {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.dark .checkout-support {
  border-top-color: #8f703d;
  color: #ebc98f;
  background: rgba(160, 106, 36, 0.1);
}
</style>
