<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { GeneratedImage } from '@/api/imageGeneration'
import Icon from '@/components/icons/Icon.vue'
import type { VideoStudioResult } from '../../composables/useMediaStudio'
import {
  isResumableVideoStatus,
  isSuccessfulVideoStatus,
  normalizeVideoStatus,
} from '../../videoStatus'
import DisabledVideoPreview from '../DisabledVideoPreview.vue'

const props = defineProps<{
  isGenerating: boolean
  isVideoMode: boolean
  requiresReference: boolean
  hasResumableVideoTask: boolean
  results: GeneratedImage[]
  videoResult: VideoStudioResult | null
  prompt: string
  outputMeta: string
  copiedValue: string
}>()

const emit = defineEmits<{
  copy: [value: string]
  resumeVideoTask: []
}>()

const { t } = useI18n()

const normalizedVideoStatus = computed(() => {
  const status = normalizeVideoStatus(props.videoResult?.status || (props.isGenerating ? 'pending' : ''))
  if (isSuccessfulVideoStatus(status)) return 'done'
  if (['pending', 'queued', 'submitted'].includes(status)) return 'pending'
  if (['processing', 'in_progress'].includes(status)) return 'processing'
  if (['failed', 'cancelled'].includes(status)) return 'failed'
  if (status === 'expired') return 'expired'
  return status ? 'processing' : 'pending'
})

const canResumeVideoTask = computed(() => props.hasResumableVideoTask
  && isResumableVideoStatus(props.videoResult?.status))

const outputState = computed(() => {
  if (props.isVideoMode && (props.isGenerating || props.videoResult)) {
    return t(`native.studio.videoStatus.${normalizedVideoStatus.value}`)
  }
  if (props.isGenerating) return t('native.studio.processing')
  if (props.results.length) return t('native.studio.count', { count: props.results.length })
  return t('native.studio.standby')
})

function imageDownloadName(index: number): string {
  return `kitty-image-${index + 1}.png`
}
</script>

<template>
  <aside class="output-panel" aria-labelledby="output-title">
    <div class="output-heading">
      <div>
        <span class="section-index">02</span>
        <h2 id="output-title">{{ t('native.studio.results') }}</h2>
      </div>
      <span class="output-state">{{ outputState }}</span>
    </div>

    <div v-if="isGenerating" class="output-loading" aria-live="polite">
      <span class="loader" aria-hidden="true" />
      <strong>{{ isVideoMode ? t('native.studio.loadingVideoTitle') : t('native.studio.loadingTitle') }}</strong>
      <p>{{ outputMeta }}</p>
      <code v-if="videoResult?.requestId" class="task-id">{{ videoResult.requestId }}</code>
    </div>

    <div v-else-if="isVideoMode && videoResult?.contentUrl" class="video-result">
      <DisabledVideoPreview :label="t('native.studio.playbackDisabled')" />
      <div class="video-caption">
        <p :title="prompt">{{ prompt }}</p>
        <code>{{ videoResult.requestId }}</code>
        <div class="result-actions">
          <a :href="videoResult.contentUrl" download="kitty-video.mp4">
            <Icon name="download" size="sm" />
            {{ t('native.studio.downloadVideo') }}
          </a>
          <button type="button" @click="emit('copy', videoResult.requestId)">
            <Icon :name="copiedValue === videoResult.requestId ? 'check' : 'copy'" size="sm" />
            {{ copiedValue === videoResult.requestId ? t('native.studio.copied') : t('native.studio.copyTaskId') }}
          </button>
        </div>
      </div>
    </div>

    <div v-else-if="isVideoMode && videoResult" class="output-empty video-task-state">
      <div class="empty-mark"><Icon name="play" size="xl" /></div>
      <strong>{{ outputState }}</strong>
      <code class="task-id">{{ videoResult.requestId }}</code>
      <div class="result-actions">
        <button v-if="canResumeVideoTask" type="button" @click="emit('resumeVideoTask')">
          <Icon name="refresh" size="sm" />
          {{ t('native.studio.resumeVideoTask') }}
        </button>
        <button type="button" @click="emit('copy', videoResult.requestId)">
          <Icon :name="copiedValue === videoResult.requestId ? 'check' : 'copy'" size="sm" />
          {{ copiedValue === videoResult.requestId ? t('native.studio.copied') : t('native.studio.copyTaskId') }}
        </button>
      </div>
    </div>

    <div v-else-if="!isVideoMode && results.length" class="result-grid">
      <figure v-for="(image, index) in results" :key="`${image.url}-${index}`" class="result-item">
        <div class="result-media">
          <img :src="image.url" :alt="t('native.studio.imageAlt', { index: index + 1 })">
          <span>{{ String(index + 1).padStart(2, '0') }}</span>
        </div>
        <figcaption>
          <p :title="image.revisedPrompt || prompt">{{ image.revisedPrompt || prompt }}</p>
          <div class="result-actions">
            <a
              :href="image.url"
              :download="imageDownloadName(index)"
              target="_blank"
              rel="noopener noreferrer"
            >
              <Icon name="download" size="sm" />
              {{ t('native.studio.download') }}
            </a>
            <button type="button" @click="emit('copy', image.url)">
              <Icon :name="copiedValue === image.url ? 'check' : 'copy'" size="sm" />
              {{ copiedValue === image.url ? t('native.studio.copied') : t('native.studio.copyUrl') }}
            </button>
          </div>
        </figcaption>
      </figure>
    </div>

    <div v-else class="output-empty">
      <div class="empty-mark"><Icon :name="requiresReference ? 'upload' : isVideoMode ? 'play' : 'sparkles'" size="xl" /></div>
      <strong>{{ t('native.studio.waitingTitle') }}</strong>
      <p>{{ t(requiresReference
        ? 'native.studio.waitingImage'
        : isVideoMode ? 'native.studio.waitingVideo' : 'native.studio.waitingText') }}</p>
    </div>

    <footer class="output-footer">
      <span>OUTPUT</span>
      <code>{{ outputMeta }}</code>
    </footer>
  </aside>
</template>

<style scoped>
.output-panel {
  display: flex;
  min-width: 0;
  min-height: 720px;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--studio-line);
  border-radius: 8px;
  background: var(--studio-surface);
}

.output-heading {
  display: flex;
  min-height: 72px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 20px;
  border-bottom: 1px solid var(--studio-line);
}

.output-heading > div {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
}

.section-index {
  display: block;
  color: var(--studio-copper);
  font-size: 12px;
  font-weight: 750;
  line-height: 1.2;
  letter-spacing: 0;
}

.output-heading h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 720;
  line-height: 1.25;
  letter-spacing: 0;
}

.output-state {
  flex: 0 0 auto;
  color: var(--studio-cyan-dark);
  font-size: 12px;
  font-weight: 700;
}

.output-loading,
.output-empty {
  display: flex;
  min-height: 480px;
  flex: 1 1 auto;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 32px;
  text-align: center;
}

.output-loading strong,
.output-empty strong {
  margin-top: 14px;
  font-size: 15px;
}

.output-loading p,
.output-empty p {
  max-width: 360px;
  margin: 7px 0 0;
  color: var(--studio-muted);
  font-size: 12px;
  line-height: 1.5;
}

.task-id {
  max-width: 100%;
  margin-top: 12px;
  overflow: hidden;
  color: var(--studio-muted);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.video-task-state .result-actions {
  width: min(220px, 100%);
}

.loader {
  width: 38px;
  height: 38px;
  border: 3px solid var(--studio-line);
  border-top-color: var(--studio-cyan);
  border-radius: 50%;
  animation: studio-spin 0.8s linear infinite;
}

.empty-mark {
  display: grid;
  width: 68px;
  height: 68px;
  place-items: center;
  border: 1px solid var(--studio-line);
  border-radius: 8px;
  color: var(--studio-copper);
  background: var(--studio-copper-soft);
}

.result-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  align-content: start;
  gap: 12px;
  min-height: 0;
  flex: 1 1 auto;
  padding: 14px;
  overflow: auto;
}

.result-item {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  border: 1px solid var(--studio-line);
  border-radius: 6px;
  background: var(--studio-surface);
}

.result-media {
  position: relative;
  display: grid;
  width: 100%;
  min-height: 240px;
  aspect-ratio: 1;
  place-items: center;
  background: var(--studio-surface-soft);
}

.result-media img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.result-media > span {
  position: absolute;
  top: 8px;
  left: 8px;
  padding: 3px 6px;
  border-radius: 3px;
  color: #f2f7f5;
  background: rgb(20 29 33 / 78%);
  font-size: 10px;
  font-weight: 750;
}

.result-item figcaption,
.video-caption {
  padding: 11px;
}

.result-item figcaption p,
.video-caption p {
  display: -webkit-box;
  min-height: 34px;
  margin: 0;
  overflow: hidden;
  color: var(--studio-muted);
  font-size: 11px;
  line-height: 1.5;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.video-result {
  display: flex;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
  justify-content: center;
  padding: 14px;
}

.video-caption code {
  display: block;
  margin-top: 7px;
  overflow: hidden;
  color: var(--studio-muted);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.result-actions {
  display: flex;
  gap: 7px;
  margin-top: 10px;
}

.result-actions a,
.result-actions button {
  display: inline-flex;
  min-height: 34px;
  flex: 1 1 0;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 6px 8px;
  border: 1px solid var(--studio-line);
  border-radius: 5px;
  color: var(--studio-muted);
  background: var(--studio-surface);
  font-size: 11px;
  font-weight: 680;
  text-decoration: none;
  cursor: pointer;
}

.result-actions a:hover,
.result-actions button:hover {
  border-color: var(--studio-cyan);
  color: var(--studio-cyan-dark);
}

.output-footer {
  display: flex;
  min-height: 48px;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 10px 16px;
  border-top: 1px solid var(--studio-line);
  color: var(--studio-muted);
  background: var(--studio-surface-soft);
  font-size: 10px;
}

.output-footer span {
  color: var(--studio-copper);
  font-weight: 800;
}

.output-footer code {
  min-width: 0;
  overflow: hidden;
  color: inherit;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@keyframes studio-spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 1060px) {
  .output-panel {
    min-height: 620px;
  }
}

@media (max-width: 700px) {
  .output-panel {
    min-height: 520px;
  }

  .result-actions {
    flex-direction: column;
  }
}

@media (prefers-reduced-motion: reduce) {
  .loader {
    animation: none;
  }
}
</style>
