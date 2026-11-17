<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
</script>

<template>
  <RouterLink to="/community" class="community-header-link" :aria-label="t('qqbot.communityTitle')">
    <Icon name="gift" size="sm" aria-hidden="true" />
    <span>{{ t('qqbot.communityTitle') }}</span>
  </RouterLink>
</template>

<style scoped>
/* Quiet promo: Kitty's soft orange, so the recharge action stays the loudest thing on the page. */
.community-header-link {
  display: inline-flex;
  flex: 0 0 auto;
  min-height: 2.25rem;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  border: 1px solid color-mix(in srgb, var(--kc-accent, #c45100) 32%, transparent);
  border-radius: 0.75rem;
  padding: 0.45rem 0.8rem;
  color: var(--kc-accent-strong, #9c3f00);
  background: var(--kc-accent-soft, #fef8f4);
  font-size: 0.8125rem;
  font-weight: 650;
  line-height: 1.4;
  text-decoration: none;
  white-space: nowrap;
  transition: background-color 0.18s ease, border-color 0.18s ease;
}
.community-header-link:hover {
  border-color: var(--kc-accent, #c45100);
  background: var(--kc-promo-soft, #fff0cf);
}
.community-header-link:focus-visible {
  outline: 2px solid var(--kc-accent, #c45100);
  outline-offset: 3px;
}
@media (max-width: 767px) {
  .community-header-link { min-height: 2.125rem; padding-inline: 0.6rem; font-size: 0.75rem; }
}
@media (prefers-reduced-motion: reduce) {
  .community-header-link { transition: none; }
}
</style>
