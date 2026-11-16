<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import Icon from '@/components/icons/Icon.vue'
import CustomerSupportPopover from '@/components/layout/CustomerSupportPopover.vue'
import { afterSalesGroupContact } from '@/config/customerContact'

const { t } = useI18n()

const contactActions = [
  {
    id: 'group',
    labelKey: 'nav.afterSalesGroup',
    hintKey: 'nav.afterSalesGroupQrHint',
    href: afterSalesGroupContact.href,
    qrSrc: afterSalesGroupContact.qrSrc,
    icon: 'users' as const,
  },
] as const
</script>

<template>
  <div class="customer-contact-actions" role="group" :aria-label="t('nav.customerContact')">
    <CustomerSupportPopover />

    <a
      v-for="action in contactActions"
      :key="action.id"
      :href="action.href"
      target="_blank"
      rel="noopener noreferrer"
      class="customer-contact-action"
      :class="`customer-contact-action--${action.id}`"
      :aria-label="t(action.labelKey)"
      :title="t(action.labelKey)"
    >
      <Icon :name="action.icon" size="sm" />
      <span class="customer-contact-action__label">{{ t(action.labelKey) }}</span>

      <span class="customer-contact-qr" role="tooltip">
        <img
          class="customer-contact-qr__image"
          :src="action.qrSrc"
          :alt="t(action.hintKey)"
          width="188"
          height="188"
        >
        <strong class="customer-contact-qr__title">{{ t(action.labelKey) }}</strong>
        <span class="customer-contact-qr__hint">{{ t(action.hintKey) }}</span>
      </span>
    </a>
  </div>
</template>

<style scoped>
.customer-contact-actions {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
}

.customer-contact-action {
  --contact-accent: #0a7f90;
  --contact-accent-strong: #075f6c;
  --contact-soft: rgba(10, 127, 144, 0.12);
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
  min-width: 2.25rem;
  min-height: 2.25rem;
  padding: 0 0.7rem;
  border: 1px solid var(--contact-accent);
  border-left-width: 3px;
  border-radius: 6px;
  color: var(--contact-accent-strong);
  background: var(--contact-soft);
  box-shadow: inset 0 0 0 1px rgba(108, 230, 241, 0.09), 0 0 14px rgba(10, 127, 144, 0.1);
  font-size: 0.8125rem;
  font-weight: 700;
  white-space: nowrap;
  transition: border-color 0.18s ease, color 0.18s ease, background-color 0.18s ease, box-shadow 0.18s ease;
}

.customer-contact-action--group {
  --contact-accent: #327fbd;
  --contact-accent-strong: #225f91;
  --contact-soft: rgba(50, 127, 189, 0.12);
}

.customer-contact-action:hover,
.customer-contact-action:focus-visible {
  border-color: #21b9c8;
  color: #087582;
  background: rgba(33, 185, 200, 0.16);
  box-shadow: inset 0 0 0 1px rgba(108, 230, 241, 0.16), 0 0 18px rgba(33, 185, 200, 0.18);
}

.customer-contact-action:focus-visible {
  outline: 2px solid rgba(33, 185, 200, 0.45);
  outline-offset: 2px;
}

.customer-contact-qr {
  position: absolute;
  z-index: 80;
  top: calc(100% + 0.75rem);
  left: 50%;
  display: grid;
  width: 212px;
  padding: 0.7rem;
  border: 1px solid rgba(33, 185, 200, 0.45);
  border-radius: 8px;
  color: #d8f6f8;
  background: #07151c;
  box-shadow: 0 18px 42px rgba(0, 0, 0, 0.38), inset 0 0 0 1px rgba(108, 230, 241, 0.08);
  opacity: 0;
  visibility: hidden;
  transform: translate(-50%, -4px);
  pointer-events: none;
  transition: opacity 0.16s ease, transform 0.16s ease, visibility 0.16s ease;
}

.customer-contact-action--group .customer-contact-qr {
  right: 0;
  left: auto;
  transform: translateY(-4px);
}

.customer-contact-action:focus-visible .customer-contact-qr {
  opacity: 1;
  visibility: visible;
  transform: translate(-50%, 0);
}

.customer-contact-action--group:focus-visible .customer-contact-qr {
  transform: translateY(0);
}

.customer-contact-qr__image {
  width: 100%;
  aspect-ratio: 1;
  border-radius: 4px;
  background: #07151c;
  object-fit: contain;
}

.customer-contact-qr__title {
  margin-top: 0.55rem;
  color: #f2fdff;
  font-size: 0.8125rem;
  line-height: 1.25;
}

.customer-contact-qr__hint {
  margin-top: 0.2rem;
  color: #89b8bd;
  font-size: 0.6875rem;
  font-weight: 500;
  line-height: 1.4;
  white-space: normal;
}

@media (hover: hover) and (pointer: fine) {
  .customer-contact-action:hover .customer-contact-qr {
    opacity: 1;
    visibility: visible;
    transform: translate(-50%, 0);
  }

  .customer-contact-action--group:hover .customer-contact-qr {
    transform: translateY(0);
  }
}

@media (max-width: 1180px) {
  .customer-contact-actions {
    flex-wrap: wrap;
    justify-content: center;
  }
}

@media (prefers-reduced-motion: reduce) {
  .customer-contact-action,
  .customer-contact-qr {
    transition: none;
  }
}
</style>
