<script setup lang="ts">
import { ref } from 'vue'

import Icon from '@/components/icons/Icon.vue'

import type { StudioProgressItem, StudioResult } from '../types'

defineProps<{
  logo: string
  gallery: StudioResult[]
  progress: StudioProgressItem[]
  loading: boolean
  generating: boolean
  error: string
  notice: string
}>()

const emit = defineEmits<{
  clear: []
  download: [result: StudioResult]
  copy: [result: StudioResult]
  remove: [result: StudioResult]
}>()

const preview = ref<StudioResult | null>(null)

function formattedDate(value: number): string {
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(value)
}

function statusIcon(status: StudioProgressItem['status']): 'checkCircle' | 'xCircle' | 'sparkles' | 'clock' {
  if (status === 'completed') return 'checkCircle'
  if (status === 'failed' || status === 'stopped') return 'xCircle'
  if (status === 'running') return 'sparkles'
  return 'clock'
}
</script>

<template>
  <main class="studio-canvas">
    <header class="canvas-toolbar">
      <div>
        <strong>创作画廊</strong>
        <span>{{ gallery.length }} 个本地作品</span>
      </div>
      <nav aria-label="画廊工具">
        <button
          type="button"
          title="清空本地画廊"
          aria-label="清空本地画廊"
          :disabled="!gallery.length || generating"
          @click="emit('clear')"
        >
          <Icon name="trash" size="sm" />
        </button>
      </nav>
    </header>

    <div class="canvas-scroll">
      <div v-if="error" class="canvas-alert error" role="alert">
        <Icon name="exclamationCircle" size="sm" />
        <span>{{ error }}</span>
      </div>
      <div v-if="notice" class="canvas-alert notice" role="status">
        <Icon name="infoCircle" size="sm" />
        <span>{{ notice }}</span>
      </div>

      <section v-if="progress.length" class="progress-strip" aria-live="polite">
        <article v-for="item in progress" :key="item.id" :class="item.status">
          <Icon :name="statusIcon(item.status)" size="xs" :class="{ pulse: item.status === 'running' }" />
          <div>
            <strong>任务 {{ item.index + 1 }}</strong>
            <span>{{ item.message }}</span>
          </div>
        </article>
      </section>

      <div v-if="loading" class="canvas-loading" aria-label="正在读取本地画廊">
        <Icon name="refresh" size="lg" class="spinning" />
      </div>

      <section v-else-if="gallery.length" class="result-grid" aria-label="生成结果">
        <article v-for="result in gallery" :key="result.id" class="result-card">
          <button class="result-media" type="button" :aria-label="`预览 ${result.model}`" @click="preview = result">
            <img v-if="result.type === 'image'" :src="result.source" :alt="result.prompt">
            <video v-else :src="result.source" muted playsinline preload="metadata"></video>
            <span class="media-kind"><Icon :name="result.type === 'video' ? 'videoCamera' : 'photo'" size="xs" />{{ result.type === 'video' ? '视频' : '图片' }}</span>
          </button>
          <div class="result-meta">
            <div>
              <strong>{{ result.model }}</strong>
              <time>{{ formattedDate(result.createdAt) }}</time>
            </div>
            <p :title="result.prompt">{{ result.prompt }}</p>
          </div>
          <footer>
            <button type="button" title="复制提示词" aria-label="复制提示词" @click="emit('copy', result)">
              <Icon name="copy" size="xs" />
            </button>
            <button type="button" title="下载作品" aria-label="下载作品" @click="emit('download', result)">
              <Icon name="download" size="xs" />
            </button>
            <button class="danger" type="button" title="删除作品" aria-label="删除作品" @click="emit('remove', result)">
              <Icon name="trash" size="xs" />
            </button>
          </footer>
        </article>
      </section>

      <section v-else class="canvas-empty">
        <div class="logo-stage">
          <span class="orbit orbit-one"></span>
          <span class="orbit orbit-two"></span>
          <img :src="logo || '/logo.svg'" alt="Kitty">
        </div>
        <h2>AI 图片工作室</h2>
        <p>选择模式、Key 与模型，作品会显示在这里并保存在当前浏览器。</p>
        <div class="capability-list" aria-label="支持能力">
          <span><Icon name="photo" size="xs" />文生图</span>
          <span><Icon name="photo" size="xs" />图生图</span>
          <span><Icon name="videoCamera" size="xs" />文生视频</span>
          <span><Icon name="videoCamera" size="xs" />图生视频</span>
        </div>
      </section>
    </div>

    <Teleport to="body">
      <div v-if="preview" class="preview-backdrop" role="dialog" aria-modal="true" aria-label="作品预览" @click.self="preview = null">
        <div class="preview-dialog">
          <button class="preview-close" type="button" title="关闭预览" aria-label="关闭预览" @click="preview = null">
            <Icon name="x" size="sm" />
          </button>
          <img v-if="preview.type === 'image'" :src="preview.source" :alt="preview.prompt">
          <video v-else :src="preview.source" controls autoplay playsinline></video>
          <footer>
            <div>
              <strong>{{ preview.model }}</strong>
              <p>{{ preview.prompt }}</p>
            </div>
            <button type="button" @click="emit('download', preview)">
              <Icon name="download" size="sm" />下载
            </button>
          </footer>
        </div>
      </div>
    </Teleport>
  </main>
</template>

<style scoped>
.studio-canvas {
  display: grid;
  min-width: 0;
  min-height: 720px;
  grid-template-rows: auto minmax(0, 1fr);
  background: var(--studio-bg);
}

.canvas-toolbar {
  display: flex;
  min-height: 68px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 18px;
  border-bottom: 1px solid var(--studio-line);
  background: var(--studio-surface);
}

.canvas-toolbar > div {
  display: flex;
  min-width: 0;
  align-items: baseline;
  gap: 9px;
}

.canvas-toolbar strong {
  color: var(--studio-ink);
  font-size: 13px;
}

.canvas-toolbar span {
  color: var(--studio-muted);
  font-size: 9px;
}

.canvas-toolbar nav {
  display: flex;
  align-items: center;
  gap: 6px;
}

.canvas-toolbar button {
  display: inline-flex;
  height: 34px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--studio-line);
  border-radius: 6px;
  padding: 0 9px;
  color: var(--studio-muted);
  background: var(--studio-surface);
  font-size: 9px;
}

.canvas-toolbar button:hover:not(:disabled) {
  border-color: var(--studio-accent);
  color: var(--studio-accent);
}

.canvas-scroll {
  position: relative;
  min-height: 0;
  overflow-y: auto;
  padding: 18px;
}

.canvas-alert {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  margin-bottom: 10px;
  border: 1px solid;
  border-radius: 6px;
  padding: 9px 11px;
  font-size: 10px;
}

.canvas-alert.error {
  border-color: rgb(220 38 38 / 25%);
  color: #b91c1c;
  background: rgb(220 38 38 / 7%);
}

.canvas-alert.notice {
  border-color: color-mix(in srgb, var(--studio-accent) 28%, transparent);
  color: var(--studio-accent-strong);
  background: var(--studio-accent-soft);
}

.progress-strip {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(145px, 1fr));
  gap: 7px;
  margin-bottom: 12px;
}

.progress-strip article {
  display: grid;
  grid-template-columns: 22px minmax(0, 1fr);
  align-items: center;
  gap: 7px;
  min-height: 46px;
  border: 1px solid var(--studio-line);
  border-radius: 6px;
  padding: 7px 9px;
  color: var(--studio-muted);
  background: var(--studio-surface);
}

.progress-strip article.running {
  border-color: color-mix(in srgb, var(--studio-accent) 38%, var(--studio-line));
  color: var(--studio-accent);
}

.progress-strip article.failed,
.progress-strip article.stopped {
  color: #dc2626;
}

.progress-strip strong,
.progress-strip span {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.progress-strip strong { font-size: 9px; }
.progress-strip span { margin-top: 2px; font-size: 8px; }

.result-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
}

.result-card {
  position: relative;
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--studio-line);
  border-radius: 7px;
  background: var(--studio-surface);
  box-shadow: 0 8px 24px rgb(15 23 42 / 6%);
  transition: transform 180ms ease, border-color 180ms ease, box-shadow 180ms ease;
}

.result-card:hover {
  z-index: 1;
  border-color: color-mix(in srgb, var(--studio-accent) 42%, var(--studio-line));
  box-shadow: 0 14px 32px rgb(15 23 42 / 11%);
  transform: translateY(-2px);
}

.result-media {
  position: relative;
  display: block;
  width: 100%;
  aspect-ratio: 1;
  overflow: hidden;
  background: var(--studio-panel-soft);
}

.result-media img,
.result-media video {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 240ms ease;
}

.result-card:hover .result-media img,
.result-card:hover .result-media video {
  transform: scale(1.025);
}

.media-kind {
  position: absolute;
  top: 8px;
  left: 8px;
  display: inline-flex;
  min-height: 24px;
  align-items: center;
  gap: 4px;
  border-radius: 5px;
  padding: 0 7px;
  color: #fff;
  background: rgb(15 23 42 / 72%);
  font-size: 8px;
}

.result-meta {
  padding: 10px 10px 7px;
}

.result-meta > div {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}

.result-meta strong {
  overflow: hidden;
  color: var(--studio-ink);
  font-size: 9px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.result-meta time {
  flex: 0 0 auto;
  color: var(--studio-muted);
  font-size: 8px;
}

.result-meta p {
  display: -webkit-box;
  min-height: 30px;
  overflow: hidden;
  margin: 6px 0 0;
  color: var(--studio-muted);
  font-size: 9px;
  line-height: 1.55;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.result-card > footer {
  display: flex;
  justify-content: flex-end;
  gap: 4px;
  padding: 0 8px 8px;
}

.result-card > footer button {
  display: grid;
  width: 28px;
  height: 28px;
  place-items: center;
  border: 1px solid var(--studio-line);
  border-radius: 5px;
  color: var(--studio-muted);
}

.result-card > footer button:hover {
  border-color: var(--studio-accent);
  color: var(--studio-accent);
}

.result-card > footer button.danger:hover {
  border-color: #ef4444;
  color: #dc2626;
}

.canvas-loading,
.canvas-empty {
  display: grid;
  min-height: 570px;
  place-items: center;
  align-content: center;
}

.canvas-loading { color: var(--studio-accent); }

.canvas-empty h2 {
  margin: 18px 0 0;
  color: var(--studio-ink);
  font-size: 22px;
  font-weight: 760;
}

.canvas-empty > p {
  max-width: 420px;
  margin: 8px 0 0;
  color: var(--studio-muted);
  font-size: 10px;
  line-height: 1.6;
  text-align: center;
}

.logo-stage {
  position: relative;
  display: grid;
  width: 112px;
  height: 112px;
  place-items: center;
}

.logo-stage img {
  position: relative;
  z-index: 2;
  width: 62px;
  height: 62px;
  border-radius: 8px;
  object-fit: contain;
  filter: drop-shadow(0 8px 18px rgb(15 23 42 / 15%));
}

.orbit {
  position: absolute;
  border: 1px solid color-mix(in srgb, var(--studio-accent) 30%, transparent);
  border-radius: 50%;
}

.orbit-one {
  width: 88px;
  height: 88px;
  animation: orbit 9s linear infinite;
}

.orbit-one::after,
.orbit-two::after {
  position: absolute;
  top: -3px;
  left: 50%;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--studio-accent);
  box-shadow: 0 0 12px var(--studio-accent);
  content: '';
}

.orbit-two {
  width: 110px;
  height: 110px;
  border-style: dashed;
  animation: orbit 14s linear infinite reverse;
}

.capability-list {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 6px;
  margin-top: 18px;
}

.capability-list span {
  display: inline-flex;
  min-height: 28px;
  align-items: center;
  gap: 5px;
  border: 1px solid var(--studio-line);
  border-radius: 5px;
  padding: 0 8px;
  color: var(--studio-muted);
  background: var(--studio-surface);
  font-size: 8px;
}

.preview-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: grid;
  place-items: center;
  padding: 22px;
  background: rgb(3 7 18 / 84%);
}

.preview-dialog {
  position: relative;
  display: grid;
  width: min(980px, 96vw);
  max-height: 94vh;
  overflow: hidden;
  border: 1px solid rgb(255 255 255 / 16%);
  border-radius: 8px;
  background: #0f172a;
  box-shadow: 0 24px 80px rgb(0 0 0 / 42%);
}

.preview-dialog > img,
.preview-dialog > video {
  width: 100%;
  max-height: calc(94vh - 86px);
  object-fit: contain;
  background: #030712;
}

.preview-dialog > footer {
  display: flex;
  min-height: 72px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 14px;
  color: #e5e7eb;
}

.preview-dialog > footer div { min-width: 0; }
.preview-dialog > footer strong { font-size: 11px; }
.preview-dialog > footer p {
  overflow: hidden;
  margin: 4px 0 0;
  color: #94a3b8;
  font-size: 9px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-dialog > footer button {
  display: inline-flex;
  min-height: 34px;
  align-items: center;
  gap: 6px;
  border-radius: 6px;
  padding: 0 12px;
  color: #fff;
  background: #0f766e;
  font-size: 10px;
}

.preview-close {
  position: absolute;
  top: 10px;
  right: 10px;
  z-index: 2;
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border-radius: 6px;
  color: #fff;
  background: rgb(15 23 42 / 72%);
}

.spinning { animation: spin .8s linear infinite; }
.pulse { animation: pulse 1.4s ease-in-out infinite; }

@keyframes spin { to { transform: rotate(360deg); } }
@keyframes pulse { 50% { opacity: .42; } }
@keyframes orbit { to { transform: rotate(360deg); } }

@media (max-width: 1040px) {
  .studio-canvas { min-height: 680px; }
}

@media (max-width: 620px) {
  .canvas-toolbar {
    align-items: flex-start;
    flex-direction: column;
    padding: 11px 12px;
  }

  .canvas-toolbar nav {
    width: 100%;
    overflow-x: auto;
  }

  .canvas-scroll { padding: 10px; }
  .result-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
  .result-meta > div { align-items: flex-start; flex-direction: column; gap: 2px; }
  .canvas-empty { min-height: 500px; }
}

@media (prefers-reduced-motion: reduce) {
  .result-card,
  .result-media img,
  .result-media video { transition: none; }
  .result-card:hover,
  .result-card:hover .result-media img,
  .result-card:hover .result-media video { transform: none; }
  .spinning,
  .pulse,
  .orbit { animation: none; }
}
</style>
