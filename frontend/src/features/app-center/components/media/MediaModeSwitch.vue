<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import Icon from '@/components/icons/Icon.vue'
import type { ImageStudioMode } from '../../composables/useMediaStudio'

withDefaults(defineProps<{
  disabled?: boolean
}>(), {
  disabled: false,
})

const mode = defineModel<ImageStudioMode>({ required: true })
const { t } = useI18n()

const options = computed<ReadonlyArray<{
  value: ImageStudioMode
  label: string
  icon: 'sparkles' | 'photo' | 'videoCamera'
}>>(() => [
  { value: 'text', label: t('native.studio.textMode'), icon: 'sparkles' },
  { value: 'image', label: t('native.studio.imageMode'), icon: 'photo' },
  { value: 'text-video', label: t('native.studio.textVideoMode'), icon: 'videoCamera' },
  { value: 'image-video', label: t('native.studio.imageVideoMode'), icon: 'videoCamera' },
])
</script>

<template>
  <div class="mode-switch" role="group" :aria-label="t('native.studio.modeAria')">
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      :data-mode="option.value"
      :aria-pressed="mode === option.value"
      :disabled="disabled"
      :class="{ active: mode === option.value }"
      @click="mode = option.value"
    >
      <span class="mode-icon"><Icon :name="option.icon" size="md" /></span>
      <span>{{ option.label }}</span>
    </button>
  </div>
</template>

<style scoped>
.mode-switch {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 4px;
  margin-top: 20px;
  padding: 4px;
  border: 1px solid var(--studio-line);
  border-radius: 7px;
  background: var(--studio-surface-soft);
}

.mode-switch button {
  --mode-accent: #0f8f83;
  --mode-soft: #e7f7f4;
  display: inline-flex;
  min-width: 0;
  min-height: 46px;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 7px 8px;
  border: 2px solid transparent;
  border-radius: 5px;
  color: var(--studio-muted);
  background: transparent;
  font-size: 12px;
  font-weight: 680;
  line-height: 1.25;
  cursor: pointer;
  transition: border-color 150ms ease, color 150ms ease, background-color 150ms ease, box-shadow 150ms ease;
}

.mode-switch button[data-mode="image"] { --mode-accent: #db2777; --mode-soft: #fce7f3; }
.mode-switch button[data-mode="text-video"] { --mode-accent: #7c3aed; --mode-soft: #ede9fe; }
.mode-switch button[data-mode="image-video"] { --mode-accent: #2563eb; --mode-soft: #dbeafe; }

.mode-switch button:hover:not(:disabled) {
  color: var(--mode-accent);
  background: color-mix(in srgb, var(--mode-soft) 58%, transparent);
}

.mode-switch button:disabled {
  cursor: not-allowed;
  opacity: 0.62;
}

.mode-switch button.active {
  border-color: var(--mode-accent);
  color: var(--mode-accent);
  background: var(--mode-soft);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--mode-accent) 16%, transparent), 0 2px 7px rgb(15 23 42 / 10%);
}

.mode-switch button:focus-visible { outline: 2px solid var(--mode-accent); outline-offset: 1px; }
.mode-icon { display: inline-grid; flex: 0 0 auto; place-items: center; }

@media (max-width: 700px) {
  .mode-switch {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>

<style>
.dark .app-center .mode-switch button[data-mode="text"] { --mode-soft: rgb(20 184 166 / 16%); }
.dark .app-center .mode-switch button[data-mode="image"] { --mode-soft: rgb(236 72 153 / 16%); }
.dark .app-center .mode-switch button[data-mode="text-video"] { --mode-soft: rgb(139 92 246 / 17%); }
.dark .app-center .mode-switch button[data-mode="image-video"] { --mode-soft: rgb(59 130 246 / 17%); }
</style>
