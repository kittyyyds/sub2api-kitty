<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

import type { AppPresentation, ApplicationDefinition } from '../types'

defineProps<{
  application: ApplicationDefinition
  presentation: AppPresentation
  selected: boolean
  disabled?: boolean
}>()

defineEmits<{
  select: []
}>()
</script>

<template>
  <button
    type="button"
    class="app-card"
    :class="{ selected }"
    :data-tone="presentation.category"
    :disabled="disabled"
    :aria-pressed="selected"
    @click="$emit('select')"
  >
    <span class="app-icon" :data-tone="presentation.category">
      <Icon :name="presentation.icon" size="lg" />
    </span>
    <span class="app-card-copy">
      <strong>{{ application.name || presentation.name }}</strong>
      <span class="app-tags">
        <small>{{ presentation.category === 'media' ? '自由创作' : presentation.category === 'video' ? '视频' : presentation.category === 'commerce' ? '电商' : '图像' }}</small>
        <small>{{ application.status || '可用' }}</small>
      </span>
      <span class="app-description">{{ application.description || presentation.description }}</span>
      <span class="app-version">版本 {{ application.version || presentation.version }}</span>
    </span>
    <span v-if="selected" class="selection-mark" aria-hidden="true">
      <Icon name="check" size="sm" :stroke-width="2.4" />
    </span>
    <Icon v-else name="chevronRight" size="sm" class="app-arrow" />
  </button>
</template>

<style scoped>
.app-card {
  --card-accent: #0f766e;
  --card-accent-soft: #e7f7f4;
  --card-icon-bg: #0f8f83;
  display: grid;
  width: 100%;
  min-height: 142px;
  grid-template-columns: 44px minmax(0, 1fr) 24px;
  gap: 12px;
  padding: 14px;
  border: 2px solid var(--customer-line);
  border-radius: 7px;
  color: var(--customer-ink);
  background: var(--customer-surface);
  text-align: left;
  transition: border-color 160ms ease, background-color 160ms ease, box-shadow 160ms ease;
}

.app-card[data-tone="image"] {
  --card-accent: #db2777;
  --card-accent-soft: #fce7f3;
  --card-icon-bg: #db2777;
}

.app-card[data-tone="commerce"] {
  --card-accent: #ea580c;
  --card-accent-soft: #ffedd5;
  --card-icon-bg: #ea580c;
}

.app-card[data-tone="video"] {
  --card-accent: #7c3aed;
  --card-accent-soft: #ede9fe;
  --card-icon-bg: #7c3aed;
}

.app-card:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--card-accent) 72%, var(--customer-line));
  box-shadow: 0 4px 12px rgb(15 23 42 / 8%);
}

.app-card.selected {
  border-color: var(--card-accent);
  background: var(--card-accent-soft);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--card-accent) 18%, transparent), 0 5px 14px rgb(15 23 42 / 10%);
}

.app-card:disabled { cursor: not-allowed; opacity: 0.65; }
.app-card:focus-visible { outline: 2px solid var(--card-accent); outline-offset: 2px; }

.app-icon {
  display: grid;
  width: 44px;
  height: 44px;
  place-items: center;
  border-radius: 6px;
  color: #ffffff;
  background: var(--card-icon-bg);
  box-shadow: inset 0 0 0 1px rgb(255 255 255 / 18%), 0 4px 10px color-mix(in srgb, var(--card-accent) 22%, transparent);
}

.app-card-copy { display: flex; min-width: 0; flex-direction: column; }
.app-card-copy strong { overflow-wrap: anywhere; font-size: 14px; line-height: 1.3; }
.app-tags { display: flex; flex-wrap: wrap; gap: 5px; margin-top: 7px; }
.app-tags small { padding: 2px 6px; border-radius: 3px; color: var(--customer-muted); background: var(--customer-surface-soft); font-size: 10px; }
.app-description { display: -webkit-box; margin-top: 9px; overflow: hidden; color: var(--customer-muted); font-size: 11px; line-height: 1.55; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.app-version { margin-top: auto; padding-top: 9px; color: var(--customer-muted); font-size: 10px; }
.app-arrow { align-self: center; color: var(--customer-muted); }
.selection-mark { display: grid; width: 24px; height: 24px; align-self: center; place-items: center; border-radius: 50%; color: #ffffff; background: var(--card-accent); box-shadow: 0 2px 7px color-mix(in srgb, var(--card-accent) 30%, transparent); }
</style>

<style>
.dark .app-center .app-card[data-tone="media"] { --card-accent-soft: rgb(20 184 166 / 16%); }
.dark .app-center .app-card[data-tone="image"] { --card-accent-soft: rgb(236 72 153 / 16%); }
.dark .app-center .app-card[data-tone="commerce"] { --card-accent-soft: rgb(249 115 22 / 16%); }
.dark .app-center .app-card[data-tone="video"] { --card-accent-soft: rgb(139 92 246 / 17%); }
</style>
