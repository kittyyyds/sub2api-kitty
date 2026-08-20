import { computed, onBeforeUnmount, reactive, shallowRef, watch } from 'vue'

import { keysAPI } from '@/api/keys'
import { isAbortError, listGatewayModels } from '@/features/ai-workbench/gateway'
import type { ApiKey } from '@/types'

import { generateStudioAsset, videoResolutionAllowed } from './studioGateway'
import {
  isVideoMode,
  requiresReference,
  studioModelById,
  studioModelOptions,
} from './modelCapabilities'
import {
  clearStudioResults,
  deleteStudioResult,
  listStudioResults,
  loadStudioDraft,
  saveStudioDraft,
  saveStudioResult,
} from './storage'
import type {
  StudioForm,
  StudioMode,
  StudioProgressItem,
  StudioResult,
} from './types'

const DEFAULT_FORM: StudioForm = {
  mode: 'text-image',
  prompt: '',
  keyId: '',
  model: '',
  count: 1,
  ratio: '1:1',
  resolution: '2k',
  videoResolution: '720p',
  videoDuration: 8,
  format: 'png',
  background: 'auto',
}

const MAX_REFERENCE_BYTES = 20 * 1024 * 1024
const MAX_PARALLEL_GENERATIONS = 2

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message.trim() ? error.message : fallback
}

function newId(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`
}

function createPreview(file: File): string {
  return URL.createObjectURL(file)
}

function extensionFor(result: StudioResult): string {
  if (result.type === 'video') return 'mp4'
  if (result.mimeType === 'image/jpeg') return 'jpg'
  if (result.mimeType === 'image/webp') return 'webp'
  if (result.mimeType === 'image/svg+xml') return 'svg'
  return 'png'
}

export function useKittyStudio() {
  const form = reactive<StudioForm>(loadStudioDraft(DEFAULT_FORM))
  const availableKeys = shallowRef<ApiKey[]>([])
  const gatewayModelIds = shallowRef<string[]>([])
  const references = shallowRef<File[]>([])
  const referencePreviews = shallowRef<string[]>([])
  const gallery = shallowRef<StudioResult[]>([])
  const progress = shallowRef<StudioProgressItem[]>([])
  const loadingKeys = shallowRef(false)
  const loadingModels = shallowRef(false)
  const loadingGallery = shallowRef(false)
  const generating = shallowRef(false)
  const error = shallowRef('')
  const notice = shallowRef('')
  let generationController: AbortController | null = null
  let modelController: AbortController | null = null

  const selectedKey = computed(() => availableKeys.value.find((key) => String(key.id) === form.keyId) || null)
  const selectedPlatform = computed(() => selectedKey.value?.group?.platform || '')
  const modelOptions = computed(() => studioModelOptions(selectedPlatform.value, gatewayModelIds.value, form.mode))
  const selectedModel = computed(() => studioModelById(modelOptions.value, form.model))
  const needsReference = computed(() => requiresReference(form.mode))
  const videoMode = computed(() => isVideoMode(form.mode))
  const maxReferenceCount = computed(() => form.mode === 'image-video' ? 1 : 5)
  const canGenerate = computed(() => !generating.value
    && !!form.prompt.trim()
    && !!selectedKey.value
    && !!selectedModel.value
    && (!needsReference.value || references.value.length > 0)
    && (!videoMode.value || videoResolutionAllowed(form.model, form.videoResolution)))

  function syncModel(): void {
    if (!modelOptions.value.some((model) => model.id === form.model)) {
      form.model = modelOptions.value[0]?.id || ''
    }
    if (videoMode.value && !videoResolutionAllowed(form.model, form.videoResolution)) {
      form.videoResolution = '720p'
    }
    if (form.model.startsWith('grok-imagine-image') && form.resolution === '4k') {
      form.resolution = '2k'
    }
  }

  async function loadModels(): Promise<void> {
    modelController?.abort()
    const key = selectedKey.value
    if (!key) {
      gatewayModelIds.value = []
      syncModel()
      return
    }
    const controller = new AbortController()
    modelController = controller
    loadingModels.value = true
    error.value = ''
    try {
      const models = await listGatewayModels(key.key, key.group?.platform || '', controller.signal)
      gatewayModelIds.value = models.map((model) => model.id)
      syncModel()
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        gatewayModelIds.value = []
        syncModel()
        error.value = errorMessage(loadError, '模型列表加载失败。')
      }
    } finally {
      if (modelController === controller) {
        modelController = null
        loadingModels.value = false
      }
    }
  }

  async function loadKeys(): Promise<void> {
    loadingKeys.value = true
    error.value = ''
    try {
      const response = await keysAPI.list(1, 100, { status: 'active' })
      availableKeys.value = response.items.filter((key) => (
        key.status === 'active'
        && key.group?.allow_image_generation === true
      ))
      if (!availableKeys.value.some((key) => String(key.id) === form.keyId)) {
        form.keyId = availableKeys.value[0] ? String(availableKeys.value[0].id) : ''
      }
      await loadModels()
    } catch (loadError) {
      availableKeys.value = []
      error.value = errorMessage(loadError, '可用生图 Key 加载失败。')
    } finally {
      loadingKeys.value = false
    }
  }

  async function loadGallery(): Promise<void> {
    loadingGallery.value = true
    try {
      const stored = await listStudioResults()
      gallery.value = stored.map((result) => result.blob
        ? { ...result, source: URL.createObjectURL(result.blob) }
        : result)
    } catch (loadError) {
      error.value = errorMessage(loadError, '本地画廊加载失败。')
    } finally {
      loadingGallery.value = false
    }
  }

  async function setKey(value: string): Promise<void> {
    if (generating.value || form.keyId === value) return
    form.keyId = value
    form.model = ''
    await loadModels()
  }

  function setMode(mode: StudioMode): void {
    if (generating.value || form.mode === mode) return
    const previousMode = form.mode
    form.mode = mode
    form.model = ''
    if (isVideoMode(mode)) {
      form.count = 1
      form.ratio = '16:9'
    } else if (isVideoMode(previousMode)) {
      form.ratio = '1:1'
    }
    if (mode === 'image-video' && references.value.length > 1) {
      referencePreviews.value.slice(1).forEach((url) => URL.revokeObjectURL(url))
      references.value = references.value.slice(0, 1)
      referencePreviews.value = referencePreviews.value.slice(0, 1)
    }
    syncModel()
    error.value = ''
  }

  function updateForm(patch: Partial<StudioForm>): void {
    if (generating.value) return
    Object.assign(form, patch)
    if (videoMode.value && !videoResolutionAllowed(form.model, form.videoResolution)) {
      form.videoResolution = '720p'
    }
    if (form.model.startsWith('grok-imagine-image') && form.resolution === '4k') {
      form.resolution = '2k'
    }
    error.value = ''
  }

  function addReferences(files: FileList | File[]): void {
    if (generating.value) return
    error.value = ''
    const incoming = Array.from(files)
    const remaining = maxReferenceCount.value - references.value.length
    if (remaining <= 0) {
      error.value = `当前模式最多上传 ${maxReferenceCount.value} 张参考图。`
      return
    }
    const accepted = incoming.filter((file) => {
      if (!file.type.startsWith('image/')) {
        error.value = `${file.name} 不是受支持的图片。`
        return false
      }
      if (file.size > MAX_REFERENCE_BYTES) {
        error.value = `${file.name} 超过 20 MB。`
        return false
      }
      return true
    }).slice(0, remaining)
    references.value = [...references.value, ...accepted]
    referencePreviews.value = [...referencePreviews.value, ...accepted.map(createPreview)]
    if (incoming.length > accepted.length) {
      error.value ||= `当前模式最多上传 ${maxReferenceCount.value} 张参考图。`
    }
  }

  function removeReference(index: number): void {
    URL.revokeObjectURL(referencePreviews.value[index] || '')
    references.value = references.value.filter((_, itemIndex) => itemIndex !== index)
    referencePreviews.value = referencePreviews.value.filter((_, itemIndex) => itemIndex !== index)
  }

  function updateProgress(id: string, patch: Partial<StudioProgressItem>): void {
    progress.value = progress.value.map((item) => item.id === id ? { ...item, ...patch } : item)
  }

  async function generate(): Promise<void> {
    if (!canGenerate.value || !selectedKey.value || !selectedModel.value) return
    const key = selectedKey.value
    const model = selectedModel.value
    const formSnapshot = { ...form }
    const referenceSnapshot = [...references.value]
    const total = isVideoMode(formSnapshot.mode) ? 1 : formSnapshot.count
    const jobs = Array.from({ length: total }, (_, index): StudioProgressItem => ({
      id: newId('generation'),
      index,
      status: 'queued',
      message: '等待生成',
    }))
    progress.value = jobs
    generating.value = true
    error.value = ''
    notice.value = ''
    generationController?.abort()
    const controller = new AbortController()
    generationController = controller
    let cursor = 0
    const failures: string[] = []

    const worker = async () => {
      while (cursor < jobs.length && !controller.signal.aborted) {
        const job = jobs[cursor]
        cursor += 1
        updateProgress(job.id, { status: 'running', message: isVideoMode(formSnapshot.mode) ? '视频生成中' : '图片生成中' })
        try {
          const assets = await generateStudioAsset({
            apiKey: key.key,
            model,
            form: { ...formSnapshot, count: 1 },
            references: referenceSnapshot,
            signal: controller.signal,
          })
          const created: StudioResult[] = []
          for (const asset of assets) {
            const stored: StudioResult = {
              id: newId('studio-result'),
              type: asset.type,
              source: asset.source,
              mimeType: asset.mimeType,
              prompt: formSnapshot.prompt.trim(),
              model: model.id,
              mode: formSnapshot.mode,
              keyId: formSnapshot.keyId,
              createdAt: Date.now(),
              requestId: asset.requestId,
              blob: asset.blob,
            }
            await saveStudioResult(stored)
            created.push(asset.blob ? { ...stored, source: URL.createObjectURL(asset.blob) } : stored)
          }
          gallery.value = [...created, ...gallery.value]
          updateProgress(job.id, { status: 'completed', message: '生成完成' })
        } catch (jobError) {
          if (isAbortError(jobError) || controller.signal.aborted) {
            updateProgress(job.id, { status: 'stopped', message: '已停止等待' })
          } else {
            const message = errorMessage(jobError, '生成失败。')
            failures.push(message)
            updateProgress(job.id, { status: 'failed', message })
          }
        }
      }
    }

    try {
      await Promise.all(Array.from({ length: Math.min(MAX_PARALLEL_GENERATIONS, jobs.length) }, worker))
      if (controller.signal.aborted) {
        notice.value = '已停止浏览器等待；已提交到上游的任务仍可能继续执行并产生费用。'
      } else if (failures.length) {
        error.value = [...new Set(failures)].join('；')
      } else {
        notice.value = `${jobs.length} 个作品已生成并保存到本地画廊。`
      }
    } finally {
      if (generationController === controller) generationController = null
      generating.value = false
    }
  }

  function stop(): void {
    generationController?.abort()
  }

  async function removeResult(result: StudioResult): Promise<void> {
    if (result.source.startsWith('blob:')) URL.revokeObjectURL(result.source)
    gallery.value = gallery.value.filter((item) => item.id !== result.id)
    await deleteStudioResult(result.id)
  }

  async function clearGallery(): Promise<void> {
    gallery.value.forEach((result) => {
      if (result.source.startsWith('blob:')) URL.revokeObjectURL(result.source)
    })
    gallery.value = []
    await clearStudioResults()
  }

  function downloadResult(result: StudioResult): void {
    const anchor = document.createElement('a')
    anchor.href = result.source
    anchor.download = `kitty-${result.model}-${new Date(result.createdAt).toISOString().replace(/[:.]/g, '-')}.${extensionFor(result)}`
    anchor.target = '_blank'
    anchor.rel = 'noopener'
    anchor.click()
  }

  async function copyPrompt(result: StudioResult): Promise<void> {
    await navigator.clipboard.writeText(result.prompt)
    notice.value = '提示词已复制。'
  }

  watch(form, () => saveStudioDraft({ ...form }), { deep: true })
  watch(modelOptions, syncModel)

  onBeforeUnmount(() => {
    generationController?.abort()
    modelController?.abort()
    referencePreviews.value.forEach((url) => URL.revokeObjectURL(url))
    gallery.value.forEach((result) => {
      if (result.source.startsWith('blob:')) URL.revokeObjectURL(result.source)
    })
  })

  return {
    form,
    availableKeys,
    gatewayModelIds,
    references,
    referencePreviews,
    gallery,
    progress,
    loadingKeys,
    loadingModels,
    loadingGallery,
    generating,
    error,
    notice,
    selectedKey,
    selectedPlatform,
    modelOptions,
    selectedModel,
    needsReference,
    videoMode,
    canGenerate,
    loadKeys,
    loadGallery,
    loadModels,
    setKey,
    setMode,
    updateForm,
    addReferences,
    removeReference,
    generate,
    stop,
    removeResult,
    clearGallery,
    downloadResult,
    copyPrompt,
  }
}
