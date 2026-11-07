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
.community-header-link {
  position: relative;
  isolation: isolate;
  display: inline-flex;
  flex: 0 0 auto;
  min-height: 2.25rem;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  overflow: hidden;
  border: 1px solid rgb(255 255 255 / 18%);
  border-radius: 0.75rem;
  padding: 0.45rem 0.8rem;
  color: #fff;
  background: linear-gradient(115deg, #a73b09, #bb4b08, #9d3455, #a73b09);
  background-size: 280% 100%;
  box-shadow: 0 3px 12px rgb(183 68 9 / 20%), inset 0 1px 0 rgb(255 255 255 / 13%);
  font-size: 0.75rem;
  font-weight: 750;
  line-height: 1.4;
  text-decoration: none;
  white-space: nowrap;
  animation: community-gradient-flow 7s ease-in-out infinite;
  transition: transform 0.18s ease, box-shadow 0.18s ease;
}
.community-header-link::after {
  position: absolute;
  z-index: -1;
  inset: 0;
  content: '';
  background: linear-gradient(110deg, transparent 20%, rgb(255 255 255 / 18%) 48%, transparent 75%);
  transform: translateX(-120%);
  animation: community-soft-shine 7s ease-in-out infinite;
  pointer-events: none;
}
.community-header-link:hover {
  color: #fff;
  box-shadow: 0 5px 16px rgb(183 68 9 / 28%), inset 0 1px 0 rgb(255 255 255 / 20%);
  transform: translateY(-1px);
}
.community-header-link:focus-visible {
  outline: 2px solid #c45100;
  outline-offset: 3px;
}
.dark .community-header-link {
  border-color: rgb(244 180 131 / 30%);
  box-shadow: 0 3px 14px rgb(185 72 11 / 20%), inset 0 1px 0 rgb(255 255 255 / 12%);
}
.dark .community-header-link:focus-visible { outline-color: #e9a67b; }
@keyframes community-gradient-flow {
  0%, 100% { background-position: 0% 50%; }
  50% { background-position: 100% 50%; }
}
@keyframes community-soft-shine {
  0%, 38% { transform: translateX(-120%); }
  72%, 100% { transform: translateX(120%); }
}
@media (max-width: 767px) {
  .community-header-link { min-height: 2.125rem; padding-inline: 0.6rem; font-size: 0.6875rem; }
}
@media (prefers-reduced-motion: reduce) {
  .community-header-link, .community-header-link::after { animation: none; transition: none; }
  .community-header-link:hover { transform: none; }
  .community-header-link::after { display: none; }
}
</style>
