<script setup lang="ts">
import { computed, ref, unref, watch, type MaybeRef } from 'vue'
import { useI18n } from 'vue-i18n'

import type { GeneratedImage } from '@/api/imageGeneration'
import Icon from '@/components/icons/Icon.vue'
import type { LocalMediaResult, RunnerInputPayload, ApplicationRun } from '../../types'
import type { VideoStudioResult } from '../../composables/useMediaStudio'
import {
  IMAGE_QUALITY_OPTIONS,
  IMAGE_RESOLUTION_OPTIONS,
  IMAGE_RATIO_OPTIONS,
  GROK_IMAGE_RATIO_OPTIONS,
  VIDEO_RATIO_OPTIONS,
  VIDEO_DURATION_OPTIONS,
  useMediaStudio,
  type ImageStudioMode,
} from '../../composables/useMediaStudio'
import MediaModeSwitch from './MediaModeSwitch.vue'
import MediaOutputPanel from './MediaOutputPanel.vue'
import { canResumeVideoRun, videoTaskFromRun } from '../../videoStatus'

export interface EmbeddedMediaKeyOption {
  id: string
  label: string
  platform: string
  groupName?: string
}

export interface EmbeddedMediaRuntime {
  keyOptions: MaybeRef<readonly EmbeddedMediaKeyOption[]>
  selectedKeyId: MaybeRef<string>
  mediaPlatform: MaybeRef<string>
  loadingKeys: MaybeRef<boolean>
  isRunning: MaybeRef<boolean>
  activeRun: MaybeRef<ApplicationRun | null>
  localResult: MaybeRef<LocalMediaResult | null>
  error: MaybeRef<string>
  successMessage: MaybeRef<string>
  setSelectedKeyId: (value: string) => void
  run: (payload: RunnerInputPayload) => void
  resume: () => void
  stop: () => void
  refreshKeys?: () => void
  clear?: () => void
}

const props = defineProps<{
  runtime: EmbeddedMediaRuntime
}>()

const { t } = useI18n()

const studio = useMediaStudio(t, {
  mediaPlatform: () => unref(props.runtime.mediaPlatform),
  isRunning: () => unref(props.runtime.isRunning),
})
const {
  form,
  references,
  generationError: studioGenerationError,
  referenceError,
  copyError,
  copiedValue,
  selectedModel,
  imageCountOptions,
  availableImageModels,
  availableVideoModels,
  availableVideoResolutions,
  isVideoMode,
  requiresReference,
  isGrokImage,
  usesSourceImageRatio,
  referenceLimit,
  sizeError,
  requestSize,
  canGenerate: studioCanGenerate,
  setMode,
  addReferenceFiles,
  removeReference,
  buildApplicationPayload,
  copyText,
} = studio

const runtimeKeyOptions = computed<readonly EmbeddedMediaKeyOption[]>(() => unref(props.runtime.keyOptions))
const selectedKeyId = computed({
  get: () => unref(props.runtime.selectedKeyId),
  set: (value: string) => props.runtime.setSelectedKeyId(value),
})
const isLoadingKeys = computed(() => unref(props.runtime.loadingKeys))
const isGenerating = computed(() => unref(props.runtime.isRunning))
const generationError = computed(() => studioGenerationError.value || unref(props.runtime.error))
const generationMessage = computed(() => unref(props.runtime.successMessage))
const effectiveKeyOptions = computed(() => {
  const options = runtimeKeyOptions.value
  return isVideoMode.value
    ? options.filter((option) => option.platform.toLowerCase() === 'grok')
    : options
})
const runtimeResult = computed(() => unref(props.runtime.localResult))
const runtimeRun = computed(() => unref(props.runtime.activeRun))
const runtimeVideoTask = computed(() => videoTaskFromRun(runtimeRun.value))
const results = computed<GeneratedImage[]>(() => runtimeResult.value?.type === 'images'
  ? runtimeResult.value.images || []
  : [])
const videoResult = computed<VideoStudioResult | null>(() => {
  const local = runtimeResult.value
  if (local?.type === 'video' && local.videoUrl) {
    return {
      requestId: local.requestId || runtimeRun.value?.next_action?.gateway_request_id || '',
      status: local.videoStatus || 'done',
      videoUrl: local.videoUrl,
      errorMessage: '',
      contentUrl: local.videoUrl,
    }
  }
  const task = runtimeVideoTask.value
  return task
    ? {
        requestId: task.requestId,
        status: task.status,
        videoUrl: task.contentUrl,
        errorMessage: task.errorMessage,
        contentUrl: task.contentUrl,
      }
    : null
})
const hasResumableVideoTask = computed(() => canResumeVideoRun(runtimeRun.value))
const canGenerate = computed(() => !isLoadingKeys.value
  && studioCanGenerate.value
  && !!selectedKeyId.value)

watch([effectiveKeyOptions, () => unref(props.runtime.selectedKeyId)], ([options, value]) => {
  if (options.some((option) => option.id === value)) return
  selectedKeyId.value = options[0]?.id || ''
}, { immediate: true })

function runMedia(): void {
  const payload = buildApplicationPayload()
  if (payload) props.runtime.run(payload)
}

function stopMedia(): void {
  props.runtime.stop()
}

function resumeMedia(): void {
  props.runtime.resume()
}

function clearMedia(): void {
  props.runtime.clear?.()
}

function refreshKeys(): void {
  props.runtime.refreshKeys?.()
}

const fileInput = ref<HTMLInputElement | null>(null)
const promptCount = computed(() => Array.from(form.prompt).length)
const outputMeta = computed(() => isVideoMode.value
  ? `${selectedModel.value.model} · ${requestSize.value}`
  : `${selectedModel.value.model} · ${requestSize.value} · ${t('native.studio.count', { count: form.count })}`)

const visibleRatioOptions = computed(() => {
  if (isVideoMode.value) return VIDEO_RATIO_OPTIONS
  return isGrokImage.value
    ? GROK_IMAGE_RATIO_OPTIONS
    : IMAGE_RATIO_OPTIONS
})

const quickPrompts = computed(() => [
  t('native.studio.quick.cinematic'),
  t('native.studio.quick.product'),
  t('native.studio.quick.clean'),
  t('native.studio.quick.details'),
])

const ratioTranslationKeys: Record<string, string> = {
  '1:1': 'square',
  '5:4': 'landscape54',
  '9:16': 'portrait916',
  '21:9': 'cinema',
  '16:9': 'landscape169',
  '3:2': 'photo32',
  '4:3': 'landscape43',
  '4:5': 'portrait45',
  '3:4': 'portrait34',
  '2:3': 'photo23',
  '2:1': 'landscape21',
  '1:2': 'portrait12',
  '19.5:9': 'landscape1959',
  '9:19.5': 'portrait9195',
  '20:9': 'landscape209',
  '9:20': 'portrait920',
  auto: 'auto',
  custom: 'custom',
}

function modelLabel(key: string): string {
  return t(`native.studio.modelOption.${key}Label`)
}

function modelDescription(key: string): string {
  return t(`native.studio.modelOption.${key}Description`)
}

function ratioLabel(value: string): string {
  return t(`native.studio.ratioOption.${ratioTranslationKeys[value] ?? 'custom'}`)
}

function qualityLabel(value: string): string {
  return t(`native.studio.qualityOption.${value}`)
}

function chooseMode(mode: ImageStudioMode): void {
  setMode(mode)
}

function handleReferenceChange(event: Event): void {
  const input = event.target as HTMLInputElement
  if (input.files) addReferenceFiles(input.files)
  input.value = ''
}

function handleReferenceDrop(event: DragEvent): void {
  if (event.dataTransfer?.files.length) addReferenceFiles(event.dataTransfer.files)
}

function appendPrompt(fragment: string): void {
  const separator = form.prompt.trim() ? '，' : ''
  form.prompt = `${form.prompt.trimEnd()}${separator}${fragment}`
}

</script>

<template>
  <section class="image-studio is-embedded" aria-labelledby="free-media-studio-title">
    <header class="studio-header">
      <div class="studio-title-block">
        <span class="studio-kicker">KITTY APPS · FREE MEDIA</span>
        <h2 id="free-media-studio-title" class="studio-title">{{ t('native.studio.title') }}</h2>
      </div>
      <div class="studio-facts" :aria-label="t('native.studio.statusAria')">
        <span><Icon name="server" size="sm" />/v1/images · /v1/videos</span>
        <span><Icon name="upload" size="sm" />{{ t('native.studio.references', { count: references.length, limit: referenceLimit }) }}</span>
        <RouterLink to="/keys"><Icon name="key" size="sm" />{{ t('native.studio.apiKeys') }}</RouterLink>
      </div>
    </header>

    <div class="studio-workspace">
      <section class="composer-panel" aria-labelledby="composer-title">
        <div class="panel-heading">
          <div>
            <span class="section-index">01</span>
            <h2 id="composer-title">{{ t('native.studio.composer') }}</h2>
          </div>
          <span class="model-readout">{{ modelLabel(selectedModel.labelKey) }}</span>
        </div>

        <fieldset class="composer-fields" :disabled="isGenerating">
          <MediaModeSwitch
            :model-value="form.mode"
            :disabled="isGenerating"
            @update:model-value="chooseMode"
          />

        <div
          v-if="requiresReference"
          class="reference-zone"
          @dragover.prevent
          @drop.prevent="handleReferenceDrop"
        >
          <input
            ref="fileInput"
            class="visually-hidden"
            type="file"
            accept="image/png,image/jpeg,image/webp"
            :multiple="referenceLimit > 1"
            @change="handleReferenceChange"
          >
          <button
            type="button"
            class="upload-trigger"
            :disabled="references.length >= referenceLimit"
            @click="fileInput?.click()"
          >
            <Icon name="upload" size="lg" />
            <span>{{ t('native.studio.upload') }}</span>
            <small>{{ t('native.studio.uploadHint') }}</small>
          </button>

          <div v-if="references.length" class="reference-list" :aria-label="t('native.studio.selectedReferences')">
            <figure v-for="reference in references" :key="reference.id" class="reference-item">
              <img v-if="reference.previewUrl" :src="reference.previewUrl" :alt="reference.file.name">
              <div v-else class="reference-placeholder"><Icon name="document" size="md" /></div>
              <figcaption :title="reference.file.name">{{ reference.file.name }}</figcaption>
              <button
                type="button"
                class="icon-button remove-reference"
                :aria-label="t('native.studio.removeReference', { name: reference.file.name })"
                :title="t('native.studio.removeReferenceTitle')"
                @click="removeReference(reference.id)"
              >
                <Icon name="x" size="sm" />
              </button>
            </figure>
          </div>
          <p v-if="referenceError" class="field-error" role="alert">{{ referenceError }}</p>
        </div>

        <div class="prompt-field">
          <div class="field-heading">
            <label for="free-media-prompt">{{ t('native.studio.prompt') }}</label>
            <span>{{ t('native.studio.promptCount', { count: promptCount }) }}</span>
          </div>
          <textarea
            id="free-media-prompt"
            v-model="form.prompt"
            rows="6"
            maxlength="8000"
            :placeholder="t('native.studio.promptPlaceholder')"
          />
          <div class="prompt-actions">
            <div class="quick-prompts" :aria-label="t('native.studio.quickAria')">
              <button
                v-for="item in quickPrompts"
                :key="item"
                type="button"
                @click="appendPrompt(item)"
              >
                + {{ item }}
              </button>
            </div>
            <button
              type="button"
              class="copy-prompt"
              :disabled="!form.prompt.trim()"
              @click="copyText(form.prompt)"
            >
              <Icon :name="copiedValue === form.prompt.trim() ? 'check' : 'copy'" size="sm" />
              {{ copiedValue === form.prompt.trim() ? t('native.studio.copied') : t('native.studio.copyPrompt') }}
            </button>
          </div>
        </div>

        <div class="parameter-band">
          <div class="parameter-intro">
            <span>{{ t('native.studio.outputSettings') }}</span>
            <p>{{ modelDescription(selectedModel.descriptionKey) }}</p>
          </div>
          <div class="parameter-grid">
            <label class="control-field control-field-wide">
              <span>{{ t('native.studio.key') }}</span>
              <div class="key-control">
                <select v-model="selectedKeyId" :disabled="isLoadingKeys || !effectiveKeyOptions.length">
                  <option v-if="isLoadingKeys" value="">{{ t('native.studio.loadingKeys') }}</option>
                  <option v-else-if="!effectiveKeyOptions.length" value="">{{ t('native.studio.noKeys') }}</option>
                  <option v-for="keyOption in effectiveKeyOptions" :key="keyOption.id" :value="keyOption.id">
                    {{ keyOption.label }}
                  </option>
                </select>
                <button
                  type="button"
                  class="icon-button"
                  :disabled="isLoadingKeys"
                  :aria-label="t('native.studio.refreshKeys')"
                  :title="t('native.studio.refreshKeys')"
                  @click="refreshKeys"
                >
                  <Icon name="refresh" size="sm" :class="{ spinning: isLoadingKeys }" />
                </button>
              </div>
              <small v-if="!isLoadingKeys && !effectiveKeyOptions.length">
                <RouterLink to="/keys">{{ t('native.studio.createKey') }}</RouterLink>
              </small>
            </label>

            <label class="control-field">
              <span>{{ t('native.studio.model') }}</span>
              <select v-if="isVideoMode" v-model="form.videoModel" :aria-label="t('native.studio.model')">
                <option v-for="model in availableVideoModels" :key="model.value" :value="model.value">
                  {{ modelLabel(model.labelKey) }} · {{ model.model }}
                </option>
              </select>
              <select v-else v-model="form.imageModel" :aria-label="t('native.studio.model')">
                <option v-for="model in availableImageModels" :key="model.value" :value="model.value">
                  {{ modelLabel(model.labelKey) }} · {{ model.model }}
                </option>
              </select>
            </label>

            <label class="control-field">
              <span>{{ t('native.studio.ratio') }}</span>
              <select
                v-if="usesSourceImageRatio"
                value="source"
                disabled
                :aria-label="t('native.studio.ratio')"
              >
                <option value="source">{{ t('native.studio.ratioOption.source') }}</option>
              </select>
              <select v-else v-model="form.ratio" :aria-label="t('native.studio.ratio')">
                <option v-for="ratio in visibleRatioOptions" :key="ratio.value" :value="ratio.value">
                  {{ ratioLabel(ratio.value) }}
                </option>
              </select>
            </label>

            <label v-if="!isVideoMode" class="control-field">
              <span>{{ t('native.studio.countLabel') }}</span>
              <select v-model.number="form.count">
                <option v-for="count in imageCountOptions" :key="count" :value="count">
                  {{ t('native.studio.count', { count }) }}
                </option>
              </select>
            </label>

            <label v-if="!isVideoMode && !isGrokImage" class="control-field">
              <span>{{ t('native.studio.quality') }}</span>
              <select v-model="form.quality">
                <option v-for="quality in IMAGE_QUALITY_OPTIONS" :key="quality.value" :value="quality.value">
                  {{ qualityLabel(quality.value) }}
                </option>
              </select>
            </label>

            <label v-if="!isVideoMode && isGrokImage" class="control-field">
              <span>{{ t('native.studio.imageResolution') }}</span>
              <select v-model="form.imageResolution">
                <option v-for="resolution in IMAGE_RESOLUTION_OPTIONS" :key="resolution" :value="resolution">
                  {{ resolution.toUpperCase() }}
                </option>
              </select>
            </label>

            <label v-if="isVideoMode" class="control-field">
              <span>{{ t('native.studio.videoResolution') }}</span>
              <select v-model="form.videoResolution" :aria-label="t('native.studio.videoResolution')">
                <option v-for="resolution in availableVideoResolutions" :key="resolution" :value="resolution">
                  {{ resolution }}
                </option>
              </select>
            </label>

            <label v-if="isVideoMode" class="control-field">
              <span>{{ t('native.studio.videoDuration') }}</span>
              <input
                v-model.number="form.videoDuration"
                type="number"
                min="1"
                max="15"
                step="1"
                list="free-media-video-durations"
              >
              <datalist id="free-media-video-durations">
                <option v-for="duration in VIDEO_DURATION_OPTIONS" :key="duration" :value="duration" />
              </datalist>
            </label>
          </div>

          <div v-if="!isVideoMode && !isGrokImage && form.ratio === 'custom'" class="custom-size">
            <label>
              <span>{{ t('native.studio.width') }}</span>
              <input v-model.number="form.customWidth" type="number" min="16" max="3840" step="16">
            </label>
            <span class="size-separator" aria-hidden="true">×</span>
            <label>
              <span>{{ t('native.studio.height') }}</span>
              <input v-model.number="form.customHeight" type="number" min="16" max="3840" step="16">
            </label>
            <p :class="{ invalid: sizeError }">{{ sizeError || t('native.studio.requestSize', { size: requestSize }) }}</p>
          </div>
        </div>

        </fieldset>

        <div class="composer-actions">
          <button
            type="button"
            class="primary-action"
            :disabled="!canGenerate"
            @click="runMedia"
          >
            <Icon :name="isVideoMode ? 'play' : 'sparkles'" size="md" />
            {{ isGenerating
              ? t(isVideoMode ? 'native.studio.generatingVideo' : 'native.studio.generating')
              : t(isVideoMode ? 'native.studio.generateVideo' : 'native.studio.generate') }}
          </button>
          <button
            v-if="isGenerating"
            type="button"
            class="secondary-action"
            @click="stopMedia"
          >
            <Icon name="x" size="sm" />
            {{ t('native.studio.cancel') }}
          </button>
          <button
            v-else
            type="button"
            class="secondary-action"
            :disabled="!results.length && (!videoResult || hasResumableVideoTask)"
            @click="clearMedia"
          >
            <Icon name="trash" size="sm" />
            {{ t('native.studio.clearResults') }}
          </button>
        </div>

        <p
          v-if="generationError || generationMessage || copyError"
          class="studio-message"
          :class="{ error: generationError || copyError, success: generationMessage && !generationError && !copyError }"
          :role="generationError || copyError ? 'alert' : 'status'"
        >
          <Icon :name="generationError || copyError ? 'exclamationCircle' : 'checkCircle'" size="sm" />
          {{ generationError || copyError || generationMessage }}
        </p>
      </section>

      <MediaOutputPanel
        :is-generating="isGenerating"
        :is-video-mode="isVideoMode"
        :requires-reference="requiresReference"
        :has-resumable-video-task="hasResumableVideoTask"
        :results="results"
        :video-result="videoResult"
        :prompt="form.prompt"
        :output-meta="outputMeta"
        :copied-value="copiedValue"
        @copy="copyText"
        @resume-video-task="resumeMedia"
      />
    </div>

  </section>
</template>

<style scoped>
.image-studio {
  --studio-bg: transparent;
  --studio-surface: var(--customer-surface);
  --studio-surface-soft: var(--customer-surface-soft);
  --studio-ink: var(--customer-ink);
  --studio-muted: var(--customer-muted);
  --studio-line: var(--customer-line);
  --studio-line-strong: var(--customer-line-strong);
  --studio-cyan: var(--customer-cyan);
  --studio-cyan-dark: var(--customer-cyan);
  --studio-cyan-soft: var(--customer-cyan-soft);
  --studio-copper: var(--customer-copper);
  --studio-copper-soft: var(--customer-copper-soft);
  --studio-error: var(--customer-danger);
  width: 100%;
  max-width: none;
  margin: 0;
  padding: 0;
  color: var(--studio-ink);
  background: var(--studio-bg);
  letter-spacing: 0;
}

.image-studio.is-embedded {
  padding-right: 0;
  padding-left: 0;
}

.image-studio.is-embedded .studio-title {
  font-size: 24px;
}

:global(.dark) .image-studio {
  --studio-bg: transparent;
}

.studio-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px;
  min-height: 104px;
  padding: 8px 2px 24px;
  border-bottom: 1px solid var(--studio-line-strong);
}

.studio-title-block {
  min-width: 0;
}

.studio-kicker,
.section-index {
  display: block;
  color: var(--studio-copper);
  font-size: 12px;
  font-weight: 750;
  line-height: 1.2;
  letter-spacing: 0;
}

.studio-title {
  margin: 8px 0 0;
  font-size: 32px;
  font-weight: 720;
  line-height: 1.12;
  letter-spacing: 0;
}

.studio-facts {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.studio-facts span,
.studio-facts a {
  display: inline-flex;
  min-height: 34px;
  align-items: center;
  gap: 7px;
  padding: 7px 10px;
  border: 1px solid var(--studio-line);
  border-radius: 4px;
  color: var(--studio-muted);
  background: var(--studio-surface);
  font-size: 12px;
  font-weight: 650;
  line-height: 1.2;
  text-decoration: none;
}

.studio-facts a:hover {
  border-color: var(--studio-cyan);
  color: var(--studio-cyan-dark);
}

.studio-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1.08fr) minmax(360px, 0.92fr);
  gap: 18px;
  padding: 24px 0;
}

.composer-panel {
  min-width: 0;
  border: 1px solid var(--studio-line);
  border-radius: 8px;
  background: var(--studio-surface);
}

.composer-panel {
  padding: 22px;
}

.composer-fields {
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}

.panel-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.panel-heading > div {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
}

.panel-heading h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 720;
  line-height: 1.25;
  letter-spacing: 0;
}

.model-readout {
  flex: 0 0 auto;
  color: var(--studio-cyan-dark);
  font-size: 12px;
  font-weight: 700;
}

.reference-zone {
  margin-top: 16px;
}

.upload-trigger {
  display: flex;
  width: 100%;
  min-height: 94px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 16px;
  border: 1px dashed var(--studio-line-strong);
  border-radius: 7px;
  color: var(--studio-cyan-dark);
  background: var(--studio-cyan-soft);
  cursor: pointer;
}

.upload-trigger span {
  font-size: 13px;
  font-weight: 700;
}

.upload-trigger small {
  color: var(--studio-muted);
  font-size: 11px;
}

.upload-trigger:hover:not(:disabled) {
  border-color: var(--studio-cyan);
}

.upload-trigger:disabled {
  cursor: not-allowed;
  opacity: 0.58;
}

.reference-list {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
  margin-top: 10px;
}

.reference-item {
  position: relative;
  min-width: 0;
  margin: 0;
}

.reference-item img,
.reference-placeholder {
  display: grid;
  width: 100%;
  aspect-ratio: 1;
  place-items: center;
  border: 1px solid var(--studio-line);
  border-radius: 5px;
  object-fit: cover;
  background: var(--studio-surface-soft);
}

.reference-item figcaption {
  margin-top: 5px;
  overflow: hidden;
  color: var(--studio-muted);
  font-size: 10px;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.icon-button {
  display: inline-grid;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  place-items: center;
  padding: 0;
  border: 1px solid var(--studio-line);
  border-radius: 5px;
  color: var(--studio-muted);
  background: var(--studio-surface);
  cursor: pointer;
}

.icon-button:hover:not(:disabled) {
  border-color: var(--studio-cyan);
  color: var(--studio-cyan-dark);
}

.icon-button:disabled {
  cursor: not-allowed;
  opacity: 0.48;
}

.remove-reference {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 28px;
  height: 28px;
  min-width: 28px;
  color: #f2f7f5;
  border-color: rgb(242 247 245 / 28%);
  background: rgb(19 28 32 / 78%);
}

.prompt-field {
  margin-top: 20px;
}

.field-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}

.field-heading label,
.control-field > span,
.custom-size label > span {
  color: var(--studio-ink);
  font-size: 12px;
  font-weight: 700;
}

.field-heading span {
  color: var(--studio-muted);
  font-size: 11px;
}

.prompt-field textarea,
.control-field select,
.control-field input,
.custom-size input {
  width: 100%;
  border: 1px solid var(--studio-line-strong);
  border-radius: 6px;
  color: var(--studio-ink);
  outline: none;
  background: var(--studio-surface);
  font: inherit;
}

.prompt-field textarea {
  min-height: 142px;
  resize: vertical;
  padding: 13px 14px;
  font-size: 14px;
  line-height: 1.65;
}

.prompt-field textarea:focus,
.control-field select:focus,
.control-field input:focus,
.custom-size input:focus {
  border-color: var(--studio-cyan);
  box-shadow: 0 0 0 3px rgb(14 116 144 / 12%);
}

.prompt-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 8px;
}

.quick-prompts {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.quick-prompts button,
.copy-prompt {
  min-height: 30px;
  padding: 5px 8px;
  border: 1px solid var(--studio-line);
  border-radius: 4px;
  color: var(--studio-muted);
  background: var(--studio-surface);
  font-size: 11px;
  font-weight: 650;
  cursor: pointer;
}

.quick-prompts button:hover,
.copy-prompt:hover:not(:disabled) {
  border-color: var(--studio-copper);
  color: var(--studio-copper);
}

.copy-prompt {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
}

.copy-prompt:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.parameter-band {
  margin-top: 20px;
  padding-top: 18px;
  border-top: 1px solid var(--studio-line);
}

.parameter-intro {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 12px;
}

.parameter-intro span {
  flex: 0 0 auto;
  font-size: 13px;
  font-weight: 720;
}

.parameter-intro p {
  margin: 0;
  color: var(--studio-muted);
  font-size: 11px;
  line-height: 1.45;
  text-align: right;
}

.parameter-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.control-field {
  display: grid;
  min-width: 0;
  gap: 6px;
}

.control-field-wide {
  grid-column: 1 / -1;
}

.control-field select,
.control-field input,
.custom-size input {
  height: 42px;
  min-width: 0;
  padding: 0 10px;
  font-size: 12px;
}

.control-field small {
  color: var(--studio-muted);
  font-size: 11px;
  line-height: 1.4;
}

.control-field small a {
  color: var(--studio-cyan-dark);
  font-weight: 650;
}

.key-control {
  display: flex;
  min-width: 0;
  gap: 7px;
}

.key-control select {
  min-width: 0;
  flex: 1 1 auto;
}

.field-error {
  margin: 7px 0 0;
  color: var(--studio-error) !important;
  font-size: 11px;
  line-height: 1.45;
}

.custom-size {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: end;
  gap: 10px;
  margin-top: 12px;
  padding: 12px;
  border-left: 3px solid var(--studio-copper);
  background: var(--studio-copper-soft);
}

.custom-size label {
  display: grid;
  gap: 6px;
}

.custom-size p {
  grid-column: 1 / -1;
  margin: 0;
  color: var(--studio-muted);
  font-size: 11px;
}

.custom-size p.invalid {
  color: var(--studio-error);
}

.size-separator {
  padding-bottom: 12px;
  color: var(--studio-copper);
  font-weight: 800;
}

.composer-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 20px;
}

.primary-action,
.secondary-action {
  display: inline-flex;
  min-height: 42px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 9px 14px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 720;
  cursor: pointer;
}

.primary-action {
  min-width: 150px;
  border: 1px solid var(--studio-cyan-dark);
  color: var(--customer-cyan-ink);
  background: var(--studio-cyan-dark);
}

.primary-action:hover:not(:disabled) {
  filter: brightness(1.07);
}

.primary-action:disabled {
  border-color: var(--studio-line-strong);
  color: var(--studio-muted);
  background: var(--studio-surface-soft);
  cursor: not-allowed;
}

.secondary-action {
  border: 1px solid var(--studio-line);
  color: var(--studio-muted);
  background: var(--studio-surface);
}

.secondary-action:hover:not(:disabled) {
  border-color: var(--studio-copper);
  color: var(--studio-copper);
}

.secondary-action:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.studio-message {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: 12px 0 0;
  padding: 10px 12px;
  border-left: 3px solid var(--studio-cyan);
  color: var(--studio-cyan-dark);
  background: var(--studio-cyan-soft);
  font-size: 12px;
  line-height: 1.45;
}

.studio-message.error {
  border-left-color: var(--studio-error);
  color: var(--studio-error);
  background: color-mix(in srgb, var(--studio-error) 8%, var(--studio-surface));
}

.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.spinning {
  animation: studio-spin 0.8s linear infinite;
}

@keyframes studio-spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 1060px) {
  .studio-workspace {
    grid-template-columns: minmax(0, 1fr);
  }

}

@media (max-width: 700px) {
  .image-studio {
    padding: 18px 14px;
  }

  .studio-header {
    min-height: 0;
    align-items: flex-start;
    flex-direction: column;
    padding-bottom: 18px;
  }

  .studio-title {
    font-size: 26px;
  }

  .studio-facts {
    justify-content: flex-start;
  }

  .studio-workspace {
    gap: 14px;
    padding: 18px 0;
  }

  .composer-panel {
    padding: 16px;
  }

  .reference-list {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .upload-trigger {
    flex-direction: column;
  }

  .prompt-actions,
  .parameter-intro {
    align-items: flex-start;
    flex-direction: column;
  }

  .parameter-intro p {
    text-align: left;
  }

  .parameter-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .control-field-wide {
    grid-column: auto;
  }

  .composer-actions {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }

  .primary-action,
  .secondary-action {
    width: 100%;
    min-width: 0;
  }

}

@media (max-width: 420px) {
  .studio-facts span,
  .studio-facts a {
    max-width: 100%;
  }

  .panel-heading > div {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }

  .custom-size {
    grid-template-columns: minmax(0, 1fr);
  }

  .size-separator {
    display: none;
  }

  .composer-actions {
    grid-template-columns: minmax(0, 1fr);
  }

}

@media (prefers-reduced-motion: reduce) {
  .spinning {
    animation-duration: 1.8s;
  }
}
</style>
