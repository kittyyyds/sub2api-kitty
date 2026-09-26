<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  effort: string
}>()

const { t, te } = useI18n()

const normalizedEffort = computed(() => props.effort.trim().toLowerCase() || 'default')
const label = computed(() => {
  const key = `native.radar.effort.${normalizedEffort.value}`
  return te(key) ? t(key) : normalizedEffort.value.toUpperCase()
})
</script>

<template>
  <span class="effort-badge" :data-effort="normalizedEffort">{{ label }}</span>
</template>

<style scoped>
.effort-badge {
  display: inline-flex;
  min-width: 42px;
  height: 20px;
  align-items: center;
  justify-content: center;
  padding: 0 7px;
  border: 1px solid var(--ai-line);
  border-radius: 999px;
  color: var(--ai-muted);
  background: var(--ai-surface-soft);
  font-family: var(--ai-mono);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0;
  line-height: 1;
  text-transform: uppercase;
  white-space: nowrap;
}

.effort-badge[data-effort="xhigh"] {
  border-color: color-mix(in srgb, var(--ai-blue) 32%, var(--ai-line));
  color: var(--ai-blue);
  background: var(--ai-blue-soft);
}

.effort-badge[data-effort="high"] {
  border-color: color-mix(in srgb, var(--ai-cyan) 34%, var(--ai-line));
  color: var(--ai-cyan);
  background: var(--ai-cyan-soft);
}

.effort-badge[data-effort="medium"] {
  border-color: color-mix(in srgb, var(--ai-copper) 36%, var(--ai-line));
  color: var(--ai-copper);
  background: var(--ai-copper-soft);
}

.effort-badge[data-effort="max"] {
  border-color: color-mix(in srgb, var(--ai-violet) 34%, var(--ai-line));
  color: var(--ai-violet);
  background: var(--ai-violet-soft);
}
</style>
