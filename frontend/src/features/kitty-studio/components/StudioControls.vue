<script setup lang="ts">
import { computed, ref } from 'vue'

import Icon from '@/components/icons/Icon.vue'
import type { ApiKey } from '@/types'

import { GROK_VIDEO_15_MODEL } from '../modelCapabilities'
import type { StudioForm, StudioMode, StudioModelOption } from '../types'

const props = defineProps<{
  form: StudioForm
  keys: ApiKey[]
  models: StudioModelOption[]
  referencePreviews: string[]
  selectedPlatform: string
  loadingKeys: boolean
  loadingModels: boolean
  generating: boolean
  canGenerate: boolean
  needsReference: boolean
  videoMode: boolean
}>()

const emit = defineEmits<{
  mode: [value: StudioMode]
  key: [value: string]
  update: [value: Partial<StudioForm>]
  files: [value: FileList]
  removeReference: [index: number]
  generate: []
  stop: []
  refresh: []
}>()

const fileInput = ref<HTMLInputElement | null>(null)

const modeOptions: ReadonlyArray<{ value: StudioMode; label: string; icon: 'photo' | 'videoCamera' }> = [
  { value: 'text-image', label: '文生图', icon: 'photo' },
  { value: 'image-image', label: '图生图', icon: 'photo' },
  { value: 'text-video', label: '文生视频', icon: 'videoCamera' },
  { value: 'image-video', label: '图生视频', icon: 'videoCamera' },
]

const ratioOptions = ['1:1', '16:9', '9:16', '3:2', '2:3'] as const
const resolutionOptions = computed(() => (
  (props.selectedPlatform.trim().toLowerCase() === 'grok' || props.form.model.startsWith('grok-imagine-image')) && !props.videoMode
    ? ['1k', '2k'] as const
    : ['1k', '2k', '4k'] as const
))
const videoResolutionOptions = computed(() => (
  props.selectedPlatform === 'video' || props.form.model === GROK_VIDEO_15_MODEL
    ? ['480p', '720p', '1080p'] as const
    : ['480p', '720p'] as const
))
const referenceLimit = computed(() => props.form.mode === 'image-video' ? 1 : 5)

function keyLabel(key: ApiKey): string {
  const value = key.key.length > 12 ? `${key.key.slice(0, 6)}...${key.key.slice(-4)}` : key.key
  return `${key.name || `Key ${key.id}`} · ${key.group?.name || '未分组'} · ${value}`
}

function selectValue(event: Event): string {
  return (event.target as HTMLSelectElement).value
}

function handleFiles(event: Event): void {
  const input = event.target as HTMLInputElement
  if (input.files?.length) emit('files', input.files)
  input.value = ''
}
</script>

<template>
  <aside class="studio-controls" aria-label="图片和视频生成设置">
    <header class="control-heading">
      <div>
        <span>Kitty Studio</span>
        <h1>Kitty 生图</h1>
      </div>
      <button
        type="button"
        title="刷新 Key 与模型"
        aria-label="刷新 Key 与模型"
        :disabled="loadingKeys || generating"
        @click="emit('refresh')"
      >
        <Icon name="refresh" size="sm" :class="{ spinning: loadingKeys || loadingModels }" />
      </button>
    </header>

    <div class="mode-switch" role="tablist" aria-label="生成模式">
      <button
        v-for="item in modeOptions.filter(option => selectedPlatform !== 'video' || option.value.endsWith('-video'))"
        :key="item.value"
        type="button"
        role="tab"
        :aria-selected="form.mode === item.value"
        :class="{ active: form.mode === item.value }"
        :disabled="generating"
        @click="emit('mode', item.value)"
      >
        <Icon :name="item.icon" size="xs" />
        {{ item.label }}
      </button>
    </div>

    <div class="control-scroll">
      <section class="control-section">
        <div class="section-label">
          <span>调用身份</span>
          <RouterLink to="/keys">管理 Key</RouterLink>
        </div>
        <label class="field">
          <span class="sr-only">API Key</span>
          <select
            :value="form.keyId"
            :disabled="generating || loadingKeys || !keys.length"
            @change="emit('key', selectValue($event))"
          >
            <option v-if="loadingKeys" value="">正在加载可用 Key...</option>
            <option v-else-if="!keys.length" value="">暂无开启媒体能力的 Key</option>
            <option v-for="key in keys" :key="key.id" :value="String(key.id)">{{ keyLabel(key) }}</option>
          </select>
        </label>
        <label class="field">
          <span>模型</span>
          <select
            :value="form.model"
            :disabled="generating || loadingModels || !models.length"
            @change="emit('update', { model: selectValue($event) })"
          >
            <option v-if="loadingModels" value="">正在读取模型...</option>
            <option v-else-if="!models.length" value="">当前分组不支持此模式</option>
            <option v-for="model in models" :key="model.id" :value="model.id">
              {{ model.label }}
            </option>
          </select>
          <small v-if="models.find((model) => model.id === form.model)">
            {{ models.find((model) => model.id === form.model)?.description }}
          </small>
        </label>
      </section>

      <section class="control-section">
        <label class="field prompt-field">
          <span>创作描述</span>
          <textarea
            :value="form.prompt"
            rows="7"
            maxlength="8000"
            :disabled="generating"
            placeholder="描述主体、环境、光线、镜头、材质与风格..."
            @input="emit('update', { prompt: ($event.target as HTMLTextAreaElement).value })"
          ></textarea>
          <small class="count">{{ form.prompt.length.toLocaleString() }} / 8,000</small>
        </label>
      </section>

      <section v-if="needsReference" class="control-section">
        <div class="section-label">
          <span>{{ form.mode === 'image-video' ? '起始图片' : '参考图片' }}</span>
          <small>{{ referencePreviews.length }} / {{ referenceLimit }}</small>
        </div>
        <button
          class="upload-zone"
          type="button"
          :disabled="generating || referencePreviews.length >= referenceLimit"
          @click="fileInput?.click()"
        >
          <Icon name="upload" size="md" />
          <span>选择图片</span>
          <small>PNG、JPG、WebP，单张不超过 20 MB</small>
        </button>
        <input ref="fileInput" class="sr-only" type="file" accept="image/*" multiple @change="handleFiles">
        <ul v-if="referencePreviews.length" class="reference-list">
          <li v-for="(preview, index) in referencePreviews.slice(0, referenceLimit)" :key="preview">
            <img :src="preview" :alt="`参考图 ${index + 1}`">
            <button type="button" title="移除参考图" aria-label="移除参考图" :disabled="generating" @click="emit('removeReference', index)">
              <Icon name="x" size="xs" />
            </button>
          </li>
        </ul>
      </section>

      <section class="control-section compact-options">
        <label class="field">
          <span>画面比例</span>
          <select :value="form.ratio" :disabled="generating" @change="emit('update', { ratio: selectValue($event) as StudioForm['ratio'] })">
            <option v-for="ratio in ratioOptions" :key="ratio" :value="ratio">{{ ratio }}</option>
          </select>
        </label>

        <template v-if="videoMode">
          <label class="field">
            <span>清晰度</span>
            <select :value="form.videoResolution" :disabled="generating" @change="emit('update', { videoResolution: selectValue($event) as StudioForm['videoResolution'] })">
              <option v-for="resolution in videoResolutionOptions" :key="resolution" :value="resolution">{{ resolution }}</option>
            </select>
          </label>
          <label class="field range-field">
            <span>时长 <b>{{ form.videoDuration }} 秒</b></span>
            <input
              :value="form.videoDuration"
              type="range"
              min="1"
              max="15"
              step="1"
              :disabled="generating"
              @input="emit('update', { videoDuration: Number(($event.target as HTMLInputElement).value) })"
            >
          </label>
        </template>

        <template v-else>
          <label class="field">
            <span>分辨率</span>
            <select :value="form.resolution" :disabled="generating" @change="emit('update', { resolution: selectValue($event) as StudioForm['resolution'] })">
              <option v-for="resolution in resolutionOptions" :key="resolution" :value="resolution">{{ resolution.toUpperCase() }}</option>
            </select>
          </label>
          <label class="field">
            <span>生成数量</span>
            <select :value="form.count" :disabled="generating" @change="emit('update', { count: Number(selectValue($event)) as StudioForm['count'] })">
              <option :value="1">1 张</option>
              <option :value="2">2 张</option>
              <option :value="4">4 张</option>
            </select>
          </label>
          <label class="field">
            <span>文件格式</span>
            <select :value="form.format" :disabled="generating" @change="emit('update', { format: selectValue($event) as StudioForm['format'] })">
              <option value="png">PNG</option>
              <option value="jpeg">JPEG</option>
              <option value="webp">WebP</option>
            </select>
          </label>
          <label class="field">
            <span>背景</span>
            <select :value="form.background" :disabled="generating" @change="emit('update', { background: selectValue($event) as StudioForm['background'] })">
              <option value="auto">自动</option>
              <option value="opaque">不透明</option>
              <option value="transparent">透明</option>
            </select>
          </label>
        </template>
      </section>
    </div>

    <footer class="generate-dock">
      <button v-if="generating" class="stop-button" type="button" @click="emit('stop')">
        <Icon name="x" size="sm" />
        停止等待
      </button>
      <button v-else class="generate-button" type="button" :disabled="!canGenerate" @click="emit('generate')">
        <Icon :name="videoMode ? 'videoCamera' : 'sparkles'" size="sm" />
        {{ videoMode ? '生成视频' : '开始生成' }}
      </button>
      <p>请求由当前站点直连模型，费用按所选 Key 分组规则结算</p>
    </footer>
  </aside>
</template>

<style scoped>
.studio-controls {
  display: grid;
  min-width: 0;
  min-height: 720px;
  grid-template-rows: auto auto minmax(0, 1fr) auto;
  border-right: 1px solid var(--studio-line);
  background: var(--studio-panel);
}

.control-heading {
  display: flex;
  min-height: 68px;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--studio-line);
}

.control-heading span {
  color: var(--studio-accent);
  font-size: 9px;
  font-weight: 750;
  text-transform: uppercase;
}

.control-heading h1 {
  margin: 2px 0 0;
  color: var(--studio-ink);
  font-size: 17px;
  font-weight: 760;
}

.control-heading button {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border: 1px solid var(--studio-line);
  border-radius: 6px;
  color: var(--studio-muted);
}

.control-heading button:hover:not(:disabled) {
  border-color: var(--studio-accent);
  color: var(--studio-accent);
}

.mode-switch {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 4px;
  padding: 9px 10px;
  border-bottom: 1px solid var(--studio-line);
  background: var(--studio-panel-soft);
}

.mode-switch button {
  display: flex;
  min-width: 0;
  min-height: 34px;
  align-items: center;
  justify-content: center;
  gap: 4px;
  border: 1px solid transparent;
  border-radius: 5px;
  color: var(--studio-muted);
  font-size: 9px;
  white-space: nowrap;
}

.mode-switch button:hover:not(:disabled) {
  color: var(--studio-ink);
  background: var(--studio-surface);
}

.mode-switch button.active {
  border-color: var(--studio-line-strong);
  color: var(--studio-accent);
  background: var(--studio-surface);
  box-shadow: 0 2px 8px rgb(15 23 42 / 6%);
  font-weight: 700;
}

.control-scroll {
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.control-section {
  padding: 14px 15px;
  border-bottom: 1px solid var(--studio-line);
}

.section-label,
.field > span {
  display: flex;
  min-height: 20px;
  align-items: center;
  justify-content: space-between;
  color: var(--studio-muted);
  font-size: 9px;
  font-weight: 700;
}

.section-label a {
  color: var(--studio-accent);
  font-size: 9px;
}

.section-label small,
.field small {
  color: var(--studio-muted);
  font-size: 8px;
  font-weight: 500;
}

.field {
  position: relative;
  display: block;
}

.field + .field {
  margin-top: 10px;
}

.field select,
.field textarea {
  width: 100%;
  border: 1px solid var(--studio-line-strong);
  border-radius: 6px;
  color: var(--studio-ink);
  background: var(--studio-surface);
  outline: 0;
}

.field select {
  height: 36px;
  padding: 0 28px 0 9px;
  font-size: 9px;
}

.field textarea {
  min-height: 126px;
  resize: vertical;
  padding: 10px 11px 22px;
  font-size: 11px;
  line-height: 1.55;
}

.field select:focus-visible,
.field textarea:focus-visible {
  border-color: var(--studio-accent);
  box-shadow: 0 0 0 3px var(--studio-ring);
}

.field > small:not(.count) {
  display: block;
  margin-top: 5px;
  line-height: 1.45;
}

.prompt-field .count {
  position: absolute;
  right: 8px;
  bottom: 6px;
}

.upload-zone {
  display: grid;
  width: 100%;
  min-height: 92px;
  place-items: center;
  gap: 3px;
  border: 1px dashed var(--studio-line-strong);
  border-radius: 6px;
  padding: 11px;
  color: var(--studio-muted);
}

.upload-zone:hover:not(:disabled) {
  border-color: var(--studio-accent);
  color: var(--studio-accent);
  background: var(--studio-accent-soft);
}

.upload-zone span {
  color: var(--studio-ink);
  font-size: 10px;
  font-weight: 700;
}

.upload-zone small {
  font-size: 8px;
}

.reference-list {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 6px;
  margin-top: 8px;
}

.reference-list li {
  position: relative;
  aspect-ratio: 1;
  overflow: hidden;
  border: 1px solid var(--studio-line);
  border-radius: 5px;
  background: var(--studio-panel-soft);
}

.reference-list img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.reference-list button {
  position: absolute;
  top: 3px;
  right: 3px;
  display: grid;
  width: 20px;
  height: 20px;
  place-items: center;
  border-radius: 4px;
  color: #fff;
  background: rgb(15 23 42 / 72%);
}

.compact-options {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.compact-options .field + .field {
  margin-top: 0;
}

.range-field {
  grid-column: 1 / -1;
}

.range-field b {
  color: var(--studio-ink);
  font-size: 9px;
}

.range-field input {
  width: 100%;
  accent-color: var(--studio-accent);
}

.generate-dock {
  padding: 12px 14px;
  border-top: 1px solid var(--studio-line);
  background: var(--studio-panel);
}

.generate-button,
.stop-button {
  display: flex;
  width: 100%;
  min-height: 42px;
  align-items: center;
  justify-content: center;
  gap: 7px;
  border-radius: 6px;
  color: #fff;
  font-size: 11px;
  font-weight: 760;
}

.generate-button {
  background: var(--studio-accent);
  box-shadow: 0 8px 20px var(--studio-accent-shadow);
}

.generate-button:hover:not(:disabled) {
  background: var(--studio-accent-strong);
}

.stop-button {
  background: #dc2626;
}

.generate-dock p {
  margin: 7px 0 0;
  color: var(--studio-muted);
  font-size: 8px;
  line-height: 1.45;
  text-align: center;
}

.spinning {
  animation: spin .8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 1040px) {
  .studio-controls {
    min-height: 0;
    border-right: 0;
    border-bottom: 1px solid var(--studio-line);
  }

  .control-scroll {
    max-height: none;
    overflow: visible;
  }
}

@media (max-width: 520px) {
  .mode-switch {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (prefers-reduced-motion: reduce) {
  .spinning { animation: none; }
}
</style>
