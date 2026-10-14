<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaVideoCapabilities, PlazaVideoCountRange } from '@/api/modelPlaza'

const props = defineProps<{ description?: string; capabilities?: PlazaVideoCapabilities }>()
const { t } = useI18n()
function range(value?: PlazaVideoCountRange) {
  if (!value || !Number.isFinite(value.min) || !Number.isFinite(value.max)) return ''
  return value.min === value.max ? String(value.min) : `${value.min}–${value.max}`
}
const facts = computed(() => {
  const caps = props.capabilities
  if (!caps) return []
  const duration = caps.second_options?.length ? caps.second_options.join(' / ') : range(caps.seconds)
  return [
    { key: 'duration', value: duration ? t('modelPlaza.video.seconds', { value: duration }) : '' },
    { key: 'resolutions', value: caps.resolutions?.join(' / ') },
    { key: 'ratios', value: caps.aspect_ratios?.join(' / ') },
    { key: 'images', value: range(caps.reference_images) },
    { key: 'videos', value: range(caps.reference_videos) },
    { key: 'audios', value: range(caps.reference_audios) },
    { key: 'referenceDuration', value: caps.max_reference_seconds ? t('modelPlaza.video.seconds', { value: caps.max_reference_seconds }) : '' },
    { key: 'combinedDuration', value: caps.max_combined_seconds ? t('modelPlaza.video.seconds', { value: caps.max_combined_seconds }) : '' },
  ].filter(fact => fact.value)
})
</script>

<template>
  <div v-if="description || facts.length" class="video-model-details">
    <p v-if="description" class="video-model-description">{{ description }}</p>
    <details v-if="facts.length">
      <summary>{{ t('modelPlaza.video.specifications') }}</summary>
      <dl>
        <div v-for="fact in facts" :key="fact.key">
          <dt>{{ t(`modelPlaza.video.${fact.key}`) }}</dt><dd>{{ fact.value }}</dd>
        </div>
      </dl>
    </details>
  </div>
</template>

<style scoped>
.video-model-details { margin-top: 8px; color: var(--catalog-muted); font-size: 12px; line-height: 1.65; font-weight: 400; white-space: normal; overflow-wrap: anywhere; }
.video-model-description { margin: 0 0 6px; }
summary { cursor: pointer; color: var(--catalog-accent); }
dl { margin-top: 6px; }
dl > div { display: flex; flex-wrap: wrap; gap: 4px 10px; }
dt { font-weight: 500; }
dd { margin: 0; }
</style>
