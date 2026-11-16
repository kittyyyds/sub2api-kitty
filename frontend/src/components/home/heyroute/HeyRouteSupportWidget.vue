<script setup lang="ts">
import { shallowRef } from 'vue'

import Icon from '@/components/icons/Icon.vue'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['support']
  docUrl: string
}

defineProps<Props>()

const open = shallowRef(false)
</script>

<template>
  <div class="hr-support-widget">
    <Transition name="hr-support-panel">
      <aside v-if="open" class="hr-support-panel">
        <strong>{{ copy.title }}</strong>
        <p>{{ copy.description }}</p>
        <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">
          <Icon name="book" size="sm" />{{ copy.docs }}
        </a>
        <router-link to="/login"><Icon name="login" size="sm" />{{ copy.login }}</router-link>
      </aside>
    </Transition>
    <button
      type="button"
      class="hr-support-button"
      :aria-label="open ? copy.close : copy.open"
      :title="open ? copy.close : copy.open"
      :aria-expanded="open"
      @click="open = !open"
    >
      <Icon :name="open ? 'x' : 'chat'" size="md" />
    </button>
  </div>
</template>

<style scoped>
.hr-support-widget {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 90;
  display: grid;
  justify-items: end;
  gap: 10px;
}

.hr-support-button {
  display: grid;
  width: 52px;
  height: 52px;
  place-items: center;
  border: 1px solid var(--hr-glass-border);
  border-radius: 8px;
  color: #0c5c64;
  background: var(--hr-panel-translucent);
  box-shadow: 0 16px 34px var(--hr-shadow), inset 0 0 0 1px rgba(246, 248, 244, 0.3);
  backdrop-filter: blur(16px) saturate(140%);
  cursor: pointer;
}

:global(.heyroute-preview[data-theme='dark'] .hr-support-button) {
  color: #86e0d6;
}

.hr-support-panel {
  width: min(300px, calc(100vw - 40px));
  border: 1px solid var(--hr-glass-border);
  border-radius: 8px;
  padding: 18px;
  color: var(--hr-ink);
  background: var(--hr-panel);
  box-shadow: 0 24px 54px var(--hr-shadow);
}

.hr-support-panel strong {
  display: block;
  font-size: 16px;
}

.hr-support-panel p {
  margin: 8px 0 16px;
  color: var(--hr-muted);
  font-size: 12px;
  line-height: 1.5;
}

.hr-support-panel a {
  display: inline-flex;
  min-height: 40px;
  align-items: center;
  gap: 7px;
  margin-right: 8px;
  border: 1px solid var(--hr-line);
  border-radius: 6px;
  padding: 0 11px;
  color: var(--hr-ink);
  font-size: 11px;
  font-weight: 650;
  text-decoration: none;
}

.hr-support-button:focus-visible,
.hr-support-panel a:focus-visible {
  outline: 2px solid var(--hr-focus);
  outline-offset: 3px;
}

.hr-support-panel-enter-active,
.hr-support-panel-leave-active {
  transition: opacity 180ms ease, transform 180ms cubic-bezier(.16, 1, .3, 1);
}

.hr-support-panel-enter-from,
.hr-support-panel-leave-to {
  opacity: 0;
  transform: translateY(8px);
}

@media (max-width: 560px) {
  .hr-support-widget {
    right: 12px;
    bottom: 12px;
  }
}
</style>
