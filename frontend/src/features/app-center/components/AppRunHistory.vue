<script setup lang="ts">
import { computed } from 'vue'

import Icon from '@/components/icons/Icon.vue'

import { canResumeImageRun, imageTaskFromRun } from '../imageStatus'
import type { ApplicationRun } from '../types'
import { canResumeVideoRun, isSuccessfulVideoStatus, videoTaskFromRun } from '../videoStatus'
import DisabledVideoPreview from './DisabledVideoPreview.vue'

interface PersistedHistoryImage {
  url: string
  revisedPrompt: string
}

const props = defineProps<{
  runs: readonly ApplicationRun[]
  selectedRun: ApplicationRun | null
  loading: boolean
  videoPreviewUrl?: string
  videoPreviewLoading?: boolean
  videoPreviewError?: string
}>()

defineEmits<{ refresh: []; select: [run: ApplicationRun]; resume: [run: ApplicationRun] }>()

function formatDate(value?: string): string {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(date)
}

function shortRunID(value: string | number): string {
  const id = String(value)
  return id.length > 10 ? id.slice(0, 8) : id
}

function canResume(run: ApplicationRun): boolean {
  return canResumeVideoRun(run) || canResumeImageRun(run)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function absoluteHTTPURL(value: unknown): string {
  if (typeof value !== 'string') return ''
  const raw = value.trim()
  if (!raw) return ''
  try {
    const parsed = new URL(raw)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? raw : ''
  } catch {
    return ''
  }
}

function persistedImages(output: unknown): PersistedHistoryImage[] {
  if (!isRecord(output) || !Array.isArray(output.images)) return []
  const images: PersistedHistoryImage[] = []
  for (const value of output.images) {
    if (!isRecord(value)) continue
    const url = absoluteHTTPURL(value.url)
    if (!url) continue
    images.push({
      url,
      revisedPrompt: typeof value.revised_prompt === 'string'
        ? value.revised_prompt.trim()
        : typeof value.revisedPrompt === 'string'
          ? value.revisedPrompt.trim()
          : '',
    })
    if (images.length === 8) break
  }
  return images
}

function isMediaStep(step: NonNullable<ApplicationRun['steps']>[number]): boolean {
  return step.credential_role === 'media'
    || step.executor === 'image_generation'
    || step.executor === 'image_edit'
    || step.executor === 'video_generation'
    || step.executor === 'video_edit'
    || step.executor === 'video_extension'
}

const selectedVideoTask = computed(() => videoTaskFromRun(props.selectedRun))
const selectedImageTask = computed(() => imageTaskFromRun(props.selectedRun))
const resumeLabel = computed(() => canResumeImageRun(props.selectedRun)
  ? '继续查询图片'
  : '继续查询视频')
const selectedVideoCompleted = computed(() => isSuccessfulVideoStatus(selectedVideoTask.value?.status))
const selectedImages = computed(() => {
  const run = props.selectedRun
  if (!run) return []
  const fromRun = persistedImages(run.output)
  if (fromRun.length) return fromRun
  for (const step of [...(run.steps || [])].reverse()) {
    if (!isMediaStep(step)) continue
    const fromStep = persistedImages(step.output)
    if (fromStep.length) return fromStep
  }
  return []
})
</script>

<template>
  <section class="history-panel">
    <header><div><Icon name="clock" size="sm" /><strong>运行历史</strong><span>{{ runs.length }}</span></div><button type="button" :disabled="loading" title="刷新运行历史" @click="$emit('refresh')"><Icon name="refresh" size="sm" :class="{ spinning: loading }" /></button></header>
    <div v-if="loading && !runs.length" class="history-empty">正在加载运行记录</div>
    <div v-else-if="!runs.length" class="history-empty">暂无运行记录</div>
    <div v-else class="history-list">
      <button v-for="run in runs" :key="run.id" type="button" :class="{ selected: selectedRun?.id === run.id }" @click="$emit('select', run)">
        <span class="history-id" :title="String(run.id)">#{{ shortRunID(run.id) }}</span><span class="history-app" :title="run.app_slug">{{ run.app_slug }}</span><span class="history-status" :data-status="run.status">{{ run.status }}</span><time>{{ formatDate(run.created_at) }}</time><Icon name="chevronRight" size="xs" />
      </button>
    </div>
    <div v-if="selectedRun" class="history-detail">
      <div><strong :title="String(selectedRun.id)">运行 #{{ shortRunID(selectedRun.id) }}</strong><span>{{ selectedRun.steps?.length || 0 }} 个步骤</span></div>
      <div v-if="selectedVideoTask" class="history-video">
        <DisabledVideoPreview v-if="selectedVideoCompleted && videoPreviewUrl" label="在线播放已关闭，请下载后查看" compact />
        <div v-else-if="selectedVideoCompleted && videoPreviewLoading" class="history-video-message">正在加载视频…</div>
        <div v-else-if="selectedVideoCompleted && videoPreviewError" class="history-video-message error">{{ videoPreviewError }}</div>
        <div class="history-video-meta">
          <code>{{ selectedVideoTask.requestId }}</code>
          <div class="history-video-actions">
            <span>{{ selectedVideoTask.status }}</span>
            <a v-if="videoPreviewUrl" :href="videoPreviewUrl" download="app-history-video.mp4" title="下载视频"><Icon name="download" size="xs" />下载</a>
          </div>
        </div>
      </div>
      <div v-if="selectedImageTask && !selectedImages.length" class="history-image-task">
        <code>{{ selectedImageTask.taskId }}</code><span>{{ selectedImageTask.status }}</span>
      </div>
      <div v-if="selectedImages.length" class="history-images" aria-label="历史图片结果">
        <figure v-for="(image, index) in selectedImages" :key="`${image.url}-${index}`" class="history-image">
          <div class="history-image-media">
            <img :src="image.url" :alt="`历史图片 ${index + 1}`" loading="lazy" decoding="async">
          </div>
          <figcaption>
            <span :title="image.revisedPrompt || `图片 ${index + 1}`">{{ image.revisedPrompt || `图片 ${index + 1}` }}</span>
            <div class="history-image-actions">
              <a :href="image.url" target="_blank" rel="noopener noreferrer" title="打开图片">
                <Icon name="externalLink" size="xs" />打开
              </a>
              <a :href="image.url" :download="`app-history-${index + 1}.png`" title="下载图片">
                <Icon name="download" size="xs" />下载
              </a>
            </div>
          </figcaption>
        </figure>
      </div>
      <pre>{{ JSON.stringify({ status: selectedRun.status, output: selectedRun.output, error: selectedRun.error_message || selectedRun.error }, null, 2) }}</pre>
      <button v-if="canResume(selectedRun)" type="button" class="resume-button" @click="$emit('resume', selectedRun)"><Icon name="refresh" size="sm" />{{ resumeLabel }}</button>
    </div>
  </section>
</template>

<style scoped>
.history-panel { overflow: hidden; border: 1px solid var(--customer-line); border-radius: 7px; background: var(--customer-surface); }
header { display: flex; min-height: 52px; align-items: center; justify-content: space-between; padding: 10px 14px; border-bottom: 1px solid var(--customer-line); }
header > div { display: flex; align-items: center; gap: 7px; color: var(--customer-copper); }
header strong { color: var(--customer-ink); font-size: 12px; }
header span { padding: 2px 6px; border-radius: 3px; color: var(--customer-muted); background: var(--customer-surface-soft); font-size: 9px; }
header button { display: grid; width: 30px; height: 30px; place-items: center; border: 1px solid var(--customer-line); border-radius: 5px; color: var(--customer-muted); }
.history-empty { padding: 34px; color: var(--customer-muted); font-size: 11px; text-align: center; }
.history-list { display: grid; }
.history-list button { display: grid; min-height: 46px; grid-template-columns: 80px minmax(0, 1fr) 74px 96px 14px; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid var(--customer-line); color: var(--customer-muted); text-align: left; }
.history-list button:hover,
.history-list button.selected { background: var(--customer-cyan-soft); }
.history-id { overflow: hidden; color: var(--customer-ink); font-size: 10px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }
.history-app { overflow: hidden; font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.history-status { justify-self: start; padding: 3px 6px; border-radius: 3px; background: var(--customer-surface-soft); font-size: 9px; }
time { font-size: 9px; }
.history-detail { padding: 12px; background: var(--customer-surface-soft); }
.history-detail > div { display: flex; justify-content: space-between; gap: 10px; color: var(--customer-muted); font-size: 10px; }
.history-detail strong { min-width: 0; overflow-wrap: anywhere; color: var(--customer-ink); }
.history-detail > .history-video { display: grid; gap: 7px; margin-top: 10px; }
.history-video-meta { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: 8px; }
.history-video-message { display: flex; min-height: 90px; align-items: center; justify-content: center; padding: 14px; border: 1px solid var(--customer-line); border-radius: 5px; color: var(--customer-muted); background: var(--customer-surface); text-align: center; }
.history-video-message.error { color: var(--color-error, #dc2626); }
.history-video code { overflow: hidden; color: var(--customer-muted); font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.history-video span { flex: 0 0 auto; color: var(--customer-muted); font-size: 9px; }
.history-video-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 7px; }
.history-video-actions a { display: inline-flex; min-height: 28px; align-items: center; justify-content: center; gap: 4px; padding: 4px 7px; border: 1px solid var(--customer-line); border-radius: 4px; color: var(--customer-cyan); background: var(--customer-surface); font-size: 9px; font-weight: 700; text-decoration: none; }
.history-video-actions a:hover { border-color: var(--customer-cyan); }
.history-image-task { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: 8px; margin-top: 10px; }
.history-image-task code { overflow: hidden; color: var(--customer-muted); font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.history-image-task span { flex: 0 0 auto; color: var(--customer-muted); font-size: 9px; }
.history-detail > .history-images { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 8px; margin-top: 10px; }
.history-image { min-width: 0; margin: 0; overflow: hidden; border: 1px solid var(--customer-line); border-radius: 5px; background: var(--customer-surface); }
.history-image-media { display: grid; width: 100%; aspect-ratio: 1; place-items: center; overflow: hidden; background: var(--customer-surface-soft); }
.history-image-media img { display: block; width: 100%; height: 100%; object-fit: contain; }
.history-image figcaption { display: grid; min-width: 0; gap: 7px; padding: 8px; }
.history-image figcaption > span { overflow: hidden; color: var(--customer-muted); font-size: 9px; line-height: 1.4; text-overflow: ellipsis; white-space: nowrap; }
.history-image-actions { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 5px; }
.history-image-actions a { display: inline-flex; min-height: 28px; align-items: center; justify-content: center; gap: 4px; border: 1px solid var(--customer-line); border-radius: 4px; color: var(--customer-cyan); background: var(--customer-surface); font-size: 9px; font-weight: 700; text-decoration: none; }
.history-image-actions a:hover { border-color: var(--customer-cyan); }
.history-detail pre { max-height: 220px; margin: 9px 0 0; overflow: auto; color: var(--customer-muted); font-size: 9px; line-height: 1.55; white-space: pre-wrap; }
.resume-button { display: inline-flex; min-height: 34px; align-items: center; gap: 6px; margin-top: 10px; padding: 7px 10px; border: 1px solid var(--customer-line); border-radius: 5px; color: var(--customer-cyan); background: var(--customer-surface); font-size: 10px; font-weight: 700; }
.resume-button:hover { border-color: var(--customer-cyan); }
.spinning { animation: spin .8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (max-width: 620px) { .history-list button { grid-template-columns: 62px minmax(0, 1fr) 68px 14px; } time { display: none; } }
@media (prefers-reduced-motion: reduce) { .spinning { animation: none; } }
</style>
