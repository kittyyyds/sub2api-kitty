<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()

const shortcuts = computed(() => [
  {
    id: 'gpt-intelligence',
    to: '/custom/reset-radar',
    label: t('nav.resetRadar'),
    emoji: '🧠',
    tone: 'intelligence',
  },
  ...(isFeatureFlagEnabled(FeatureFlags.modelPlaza)
    ? [{
        id: 'model-pricing',
        to: '/model-plaza?embedded=1',
        label: t('nav.modelPlaza'),
        emoji: '💰',
        tone: 'pricing',
      }]
    : []),
])
</script>

<template>
  <nav class="customer-header-shortcuts" :aria-label="t('nav.customerShortcuts')">
    <router-link
      v-for="shortcut in shortcuts"
      :key="shortcut.id"
      :to="shortcut.to"
      class="customer-header-shortcut"
      :class="`customer-header-shortcut--${shortcut.tone}`"
      :data-header-shortcut="shortcut.id"
      :aria-label="shortcut.label"
      :title="shortcut.label"
    >
      <span class="customer-header-shortcut__emoji" aria-hidden="true">{{ shortcut.emoji }}</span>
      <span class="customer-header-shortcut__label">{{ shortcut.label }}</span>
    </router-link>
  </nav>
</template>

<style scoped>
.customer-header-shortcuts {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
}

.customer-header-shortcut {
  --shortcut-accent: #087f8c;
  --shortcut-accent-strong: #075e68;
  --shortcut-soft: rgba(8, 127, 140, 0.12);
  display: inline-flex;
  min-width: 2.25rem;
  min-height: 2.25rem;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
  padding: 0 0.7rem;
  border: 1px solid var(--shortcut-accent);
  border-left-width: 3px;
  border-radius: 6px;
  color: var(--shortcut-accent-strong);
  background: var(--shortcut-soft);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.22), 0 0 14px var(--shortcut-soft);
  font-size: 0.8125rem;
  font-weight: 800;
  white-space: nowrap;
  transition: transform 0.18s ease, border-color 0.18s ease, color 0.18s ease, background-color 0.18s ease, box-shadow 0.18s ease;
}

.customer-header-shortcut--intelligence {
  --shortcut-accent: #d44973;
  --shortcut-accent-strong: #a82a51;
  --shortcut-soft: rgba(212, 73, 115, 0.13);
}

.customer-header-shortcut--pricing {
  --shortcut-accent: #b87908;
  --shortcut-accent-strong: #805407;
  --shortcut-soft: rgba(184, 121, 8, 0.14);
}

.customer-header-shortcut:hover,
.customer-header-shortcut:focus-visible {
  transform: translateY(-1px);
  border-color: var(--shortcut-accent);
  color: var(--shortcut-accent-strong);
  background: color-mix(in srgb, var(--shortcut-soft) 82%, white 18%);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.32), 0 5px 16px var(--shortcut-soft);
}

.customer-header-shortcut:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--shortcut-accent) 55%, transparent);
  outline-offset: 2px;
}

.customer-header-shortcut__emoji {
  flex: 0 0 auto;
  font-size: 1rem;
  line-height: 1;
}

.customer-header-shortcut__label {
  min-width: 0;
}

:global(.dark) .customer-header-shortcut {
  color: color-mix(in srgb, var(--shortcut-accent) 65%, white 35%);
  background: color-mix(in srgb, var(--shortcut-soft) 78%, #07151c 22%);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.06), 0 0 15px var(--shortcut-soft);
}

@media (max-width: 767px) {
  .customer-header-shortcuts {
    width: 100%;
    flex-wrap: wrap;
  }
}

@media (prefers-reduced-motion: reduce) {
  .customer-header-shortcut {
    transition: none;
  }
}
</style>
