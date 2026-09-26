<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

withDefaults(defineProps<{
  label: string
  compact?: boolean
}>(), {
  compact: false,
})
</script>

<template>
  <div class="disabled-video-preview" :class="{ 'disabled-video-preview--compact': compact }">
    <button type="button" disabled :aria-label="label" :title="label">
      <Icon name="play" :size="compact ? 'md' : 'lg'" />
    </button>
    <p>{{ label }}</p>
  </div>
</template>

<style scoped>
.disabled-video-preview {
  display: grid;
  width: 100%;
  height: clamp(220px, 34vw, 420px);
  place-content: center;
  justify-items: center;
  gap: 10px;
  padding: 18px;
  border: 1px solid var(--studio-line, var(--customer-line));
  border-radius: 6px;
  color: var(--studio-muted, var(--customer-muted));
  background: var(--studio-surface-soft, var(--customer-surface-soft));
  text-align: center;
}

.disabled-video-preview--compact {
  height: clamp(130px, 20vw, 240px);
}

.disabled-video-preview button {
  display: grid;
  width: 52px;
  height: 52px;
  place-items: center;
  padding: 0;
  border: 1px solid var(--studio-line-strong, var(--customer-line-strong));
  border-radius: 50%;
  color: var(--studio-muted, var(--customer-muted));
  background: var(--studio-surface, var(--customer-surface));
  cursor: not-allowed;
  opacity: 0.7;
}

.disabled-video-preview--compact button {
  width: 44px;
  height: 44px;
}

.disabled-video-preview p {
  margin: 0;
  color: inherit;
  font-size: 12px;
  line-height: 1.5;
}
</style>
