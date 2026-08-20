import { computed, onBeforeUnmount, shallowRef } from 'vue'

import { keysAPI } from '@/api/keys'
import { videoGenerationAPI } from '@/api/videoGeneration'
import type { ApiKey } from '@/types'
import { maskApiKey } from '@/utils/maskApiKey'

import { appCenterAPI } from '../api'
import { APP_PRESENTATIONS, isSupportedApplicationSlug } from '../appRegistry'
import {
  executeMediaAction,
  executePlannerAction,
  isImageContinuationError,
  isVideoContinuationError,
} from '../gateway'
import {
  canResumeImageRun,
  imageTaskFromRun,
  isImageExecutor,
  isResumableImageStatus,
} from '../imageStatus'
import type {
  ApplicationDefinition,
  ApplicationNextAction,
  ApplicationRun,
  ApplicationRunStep,
  ApplicationSlug,
  LocalMediaResult,
  RunnerInputPayload,
} from '../types'
import {
  canResumeVideoRun,
  isSuccessfulVideoStatus,
  isResumableVideoStatus,
  isVideoExecutor,
  videoTaskFromRun,
} from '../videoStatus'

const MAX_ACTIONS_PER_RUN = 10

export interface AppKeyOption {
  id: string
  label: string
  platform: string
}

function safeError(error: unknown, fallback: string): string {
  if (typeof error === 'object' && error !== null && 'message' in error) {
    const message = String((error as { message?: unknown }).message || '').trim()
    if (message) return message.length > 600 ? `${message.slice(0, 600)}...` : message
  }
  return fallback
}

function isCancelled(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
    || (typeof error === 'object' && error !== null
      && (('name' in error && (error as { name?: unknown }).name === 'AbortError')
        || ('code' in error && (error as { code?: unknown }).code === 'ERR_CANCELED')))
}

function keyOption(key: ApiKey): AppKeyOption {
  const name = key.name.trim() || `Key ${key.id}`
  return {
    id: String(key.id),
    label: `${name} · ${maskApiKey(key.key)} · ${key.group?.name || '未分组'}`,
    platform: key.group?.platform || '',
  }
}

export function useAppCenter() {
  const applications = shallowRef<ApplicationDefinition[]>([])
  const history = shallowRef<ApplicationRun[]>([])
  const availableKeys = shallowRef<ApiKey[]>([])
  const selectedSlug = shallowRef<ApplicationSlug>('free-media-studio')
  const usePlanner = shallowRef(false)
  const selectedPlannerKeyId = shallowRef('')
  const selectedMediaKeyId = shallowRef('')
  const activeRun = shallowRef<ApplicationRun | null>(null)
  const selectedHistoryRun = shallowRef<ApplicationRun | null>(null)
  const historyVideoPreviewUrl = shallowRef('')
  const historyVideoPreviewLoading = shallowRef(false)
  const historyVideoPreviewError = shallowRef('')
  const localResult = shallowRef<LocalMediaResult | null>(null)
  const progressSteps = shallowRef<ApplicationRunStep[]>([])
  const isLoading = shallowRef(false)
  const isLoadingHistory = shallowRef(false)
  const isLoadingKeys = shallowRef(false)
  const isRunning = shallowRef(false)
  const error = shallowRef('')
  const successMessage = shallowRef('')

  let loadController: AbortController | null = null
  let runController: AbortController | null = null
  let historyRunController: AbortController | null = null
  let historyVideoController: AbortController | null = null
  let localVideoObjectURL = ''
  let historyVideoObjectURL = ''
  let submittedVideoRequestID = ''
  let submittedImageTaskID = ''

  const visibleApplications = computed(() => applications.value.filter((app) => isSupportedApplicationSlug(app.slug)))
  const selectedApplication = computed(() => visibleApplications.value.find((app) => app.slug === selectedSlug.value) || null)
  const selectedPresentation = computed(() => APP_PRESENTATIONS[selectedSlug.value])

  const plannerKeyOptions = computed(() => availableKeys.value.map(keyOption))
  const mediaKeyOptions = computed(() => availableKeys.value
    .filter((key) => {
      const platform = key.group?.platform?.toLowerCase() || ''
      if (key.group?.allow_image_generation !== true) return false
      return selectedSlug.value === 'grok-video-director'
        ? platform === 'grok'
        : platform === 'grok' || platform === 'openai'
    })
    .map(keyOption))

  const selectedPlannerKey = computed(() => availableKeys.value.find((key) => String(key.id) === selectedPlannerKeyId.value))
  const selectedMediaKey = computed(() => availableKeys.value.find((key) => String(key.id) === selectedMediaKeyId.value))
  const selectedMediaPlatform = computed(() => selectedMediaKey.value?.group?.platform?.toLowerCase() || '')

  function syncSelectedKeys(): void {
    if (usePlanner.value && !plannerKeyOptions.value.some((key) => key.id === selectedPlannerKeyId.value)) {
      selectedPlannerKeyId.value = plannerKeyOptions.value[0]?.id || ''
    }
    if (!mediaKeyOptions.value.some((key) => key.id === selectedMediaKeyId.value)) {
      selectedMediaKeyId.value = mediaKeyOptions.value[0]?.id || ''
    }
  }

  function selectApplication(slug: string): void {
    if (!isSupportedApplicationSlug(slug) || isRunning.value) return
    selectedSlug.value = slug
    usePlanner.value = slug !== 'free-media-studio'
    activeRun.value = null
    localResult.value = null
    progressSteps.value = []
    error.value = ''
    successMessage.value = ''
    submittedVideoRequestID = ''
    submittedImageTaskID = ''
    syncSelectedKeys()
  }

  async function loadApplications(): Promise<void> {
    loadController?.abort()
    const controller = new AbortController()
    loadController = controller
    isLoading.value = true
    error.value = ''
    try {
      applications.value = await appCenterAPI.listApplications(controller.signal)
      const firstSupported = applications.value.find((app) => isSupportedApplicationSlug(app.slug))
      if (!applications.value.some((app) => app.slug === selectedSlug.value) && firstSupported) {
        selectedSlug.value = firstSupported.slug as ApplicationSlug
      }
    } catch (loadError) {
      if (!isCancelled(loadError)) {
        applications.value = []
        error.value = safeError(loadError, '应用目录加载失败。')
      }
    } finally {
      if (loadController === controller) {
        loadController = null
        isLoading.value = false
      }
    }
  }

  async function loadKeys(): Promise<void> {
    isLoadingKeys.value = true
    try {
      const response = await keysAPI.list(1, 100, { status: 'active' })
      availableKeys.value = response.items.filter((key) => key.status === 'active')
      syncSelectedKeys()
    } catch (loadError) {
      availableKeys.value = []
      error.value = safeError(loadError, 'API 密钥加载失败。')
    } finally {
      isLoadingKeys.value = false
    }
  }

  async function loadHistory(): Promise<void> {
    isLoadingHistory.value = true
    try {
      const response = await appCenterAPI.listRuns()
      history.value = response.items
    } catch (loadError) {
      error.value = safeError(loadError, '运行历史加载失败。')
    } finally {
      isLoadingHistory.value = false
    }
  }

  async function loadInitial(): Promise<void> {
    await Promise.all([loadApplications(), loadKeys(), loadHistory()])
  }

  function updateLocalStep(action: ApplicationNextAction, status: ApplicationRunStep['status'], message = ''): void {
    const current = progressSteps.value.find((step) => step.step_key === action.step_key)
    const next: ApplicationRunStep = {
      ...(current || {}),
      step_key: action.step_key,
      executor: action.executor,
      status,
      ...(message ? { error_message: message } : {}),
    }
    progressSteps.value = current
      ? progressSteps.value.map((step) => step.step_key === action.step_key ? next : step)
      : [...progressSteps.value, next]
  }

  function replaceLocalResult(result: LocalMediaResult | undefined): void {
    if (!result) return
    if (localVideoObjectURL && localVideoObjectURL !== result.videoUrl) {
      URL.revokeObjectURL(localVideoObjectURL)
      localVideoObjectURL = ''
    }
    if (result.videoUrl?.startsWith('blob:')) localVideoObjectURL = result.videoUrl
    localResult.value = result
  }

  function clearHistoryVideoPreview(): void {
    historyVideoController?.abort()
    historyVideoController = null
    if (historyVideoObjectURL) {
      URL.revokeObjectURL(historyVideoObjectURL)
      historyVideoObjectURL = ''
    }
    historyVideoPreviewUrl.value = ''
    historyVideoPreviewLoading.value = false
    historyVideoPreviewError.value = ''
  }

  async function loadHistoryVideoPreview(run: ApplicationRun | null): Promise<void> {
    clearHistoryVideoPreview()
    const task = videoTaskFromRun(run)
    if (!run || !task || !isSuccessfulVideoStatus(task.status)) return

    const mediaKeyID = Number(run.media_api_key_id)
    if (!Number.isSafeInteger(mediaKeyID) || mediaKeyID <= 0) {
      historyVideoPreviewError.value = '这条历史视频缺少有效的媒体密钥记录。'
      return
    }

    const controller = new AbortController()
    historyVideoController = controller
    historyVideoPreviewLoading.value = true
    try {
      let mediaKey = availableKeys.value.find((key) => Number(key.id) === mediaKeyID)
      if (!mediaKey) {
        try {
          mediaKey = await keysAPI.getById(mediaKeyID, { signal: controller.signal })
        } catch (lookupError) {
          if (isCancelled(lookupError)) throw lookupError
        }
      }
      if (!mediaKey || !['active', 'quota_exhausted', 'expired'].includes(mediaKey.status)) {
        historyVideoPreviewError.value = '原媒体密钥已停用或删除，无法加载这条历史视频。'
        return
      }
      const objectURL = await videoGenerationAPI.createContentURL(
        mediaKey.key,
        task.requestId,
        controller.signal,
        { sessionId: run.session_id },
      )
      if (historyVideoController !== controller) {
        URL.revokeObjectURL(objectURL)
        return
      }
      historyVideoObjectURL = objectURL
      historyVideoPreviewUrl.value = objectURL
    } catch (previewError) {
      if (!isCancelled(previewError) && historyVideoController === controller) {
        historyVideoPreviewError.value = safeError(previewError, '历史视频加载失败。')
      }
    } finally {
      if (historyVideoController === controller) {
        historyVideoController = null
        historyVideoPreviewLoading.value = false
      }
    }
  }

  async function executeNextAction(
    run: ApplicationRun,
    action: ApplicationNextAction,
    files: File[],
    signal: AbortSignal,
  ): Promise<ApplicationRun> {
    const key = action.credential_role === 'planner' ? selectedPlannerKey.value : selectedMediaKey.value
    if (!key) throw new Error(action.credential_role === 'planner' ? '请选择策划密钥。' : '请选择媒体密钥。')
    if (isVideoExecutor(action.executor) && action.gateway_request_id?.trim()) {
      submittedVideoRequestID = action.gateway_request_id.trim()
    }
    if (isImageExecutor(action.executor) && action.gateway_request_id?.trim()) {
      submittedImageTaskID = action.gateway_request_id.trim()
    }

    updateLocalStep(action, 'running')
    try {
      const result = action.executor === 'chat_completions'
        ? await executePlannerAction(key.key, action, signal)
        : await executeMediaAction(
          key.key,
          action,
          files,
          signal,
          typeof run.input?.mode === 'string' ? run.input.mode : '',
          // A paid upstream task must keep its checkpoint even if local polling is stopped.
          async (requestId) => {
            submittedVideoRequestID = requestId
            activeRun.value = await appCenterAPI.checkpointStep(run.id, action.step_key, {
              output: { request_id: requestId, status: 'processing' },
              gateway_request_id: requestId,
            })
          },
          async (taskId) => {
            submittedImageTaskID = taskId
            activeRun.value = await appCenterAPI.checkpointStep(run.id, action.step_key, {
              output: { task_id: taskId, status: 'processing' },
              gateway_request_id: taskId,
            })
          },
        )
      replaceLocalResult(result.localResult)
      const updated = await appCenterAPI.completeStep(run.id, action.step_key, {
        output: result.output,
        ...(result.gatewayRequestId ? { gateway_request_id: result.gatewayRequestId } : {}),
      }, signal)
      updateLocalStep(action, 'completed')
      if (result.gatewayRequestId && isVideoExecutor(action.executor)) submittedVideoRequestID = ''
      if (result.gatewayRequestId && isImageExecutor(action.executor)) submittedImageTaskID = ''
      return updated
    } catch (stepError) {
      if (isCancelled(stepError)) throw stepError
      const message = safeError(stepError, '运行步骤失败。')
      const resumableVideo = isVideoContinuationError(stepError)
        && isResumableVideoStatus(stepError.status)
      const resumableImage = isImageContinuationError(stepError)
        && isResumableImageStatus(stepError.status)
      if ((resumableVideo || resumableImage) && stepError.checkpointed) {
        updateLocalStep(action, 'running', message)
        throw stepError
      }
      updateLocalStep(action, 'failed', message)
      try {
        activeRun.value = await appCenterAPI.failStep(run.id, action.step_key, {
          error: message,
          ...(isVideoContinuationError(stepError) ? {
            output: { request_id: stepError.requestId, status: stepError.status },
            gateway_request_id: stepError.requestId,
          } : isImageContinuationError(stepError) ? {
            output: { task_id: stepError.taskId, status: stepError.status },
            gateway_request_id: stepError.taskId,
          } : {}),
        }, signal)
      } catch {
        // Keep the original gateway error as the actionable message.
      }
      throw new Error(message)
    }
  }

  async function driveRun(
    initialRun: ApplicationRun,
    files: File[],
    controller: AbortController,
  ): Promise<ApplicationRun> {
    let run = initialRun
    let actionCount = 0
    while (run.next_action) {
      actionCount += 1
      if (actionCount > MAX_ACTIONS_PER_RUN) throw new Error('运行步骤超过安全上限，已停止自动执行。')
      run = await executeNextAction(run, run.next_action, files, controller.signal)
      activeRun.value = run
    }
    if (run.status === 'failed') throw new Error(run.error_message || run.error || '应用运行失败。')
    if (run.status === 'cancelled') throw new Error('运行已取消。')
    return run
  }

  async function runApplication(payload: RunnerInputPayload): Promise<void> {
    if (isRunning.value) return
    if (usePlanner.value && !selectedPlannerKey.value) {
      error.value = '请选择策划密钥。'
      return
    }
    if (!selectedMediaKey.value) {
      error.value = '请选择媒体密钥。'
      return
    }

    const controller = new AbortController()
    runController = controller
    isRunning.value = true
    error.value = ''
    successMessage.value = ''
    activeRun.value = null
    localResult.value = null
    progressSteps.value = []
    submittedVideoRequestID = ''
    submittedImageTaskID = ''

    try {
      let run = await appCenterAPI.startRun(selectedSlug.value, {
        ...(usePlanner.value && selectedPlannerKey.value
          ? { planner_api_key_id: selectedPlannerKey.value.id }
          : {}),
        media_api_key_id: selectedMediaKey.value.id,
        input: payload.values,
      }, controller.signal)
      activeRun.value = run

      run = await driveRun(run, payload.files, controller)
      successMessage.value = '运行完成，结果已更新。'
    } catch (runError) {
      if (!isCancelled(runError)) error.value = safeError(runError, '应用运行失败。')
    } finally {
      if (runController === controller) {
        runController = null
        isRunning.value = false
      }
      await loadHistory()
    }
  }

  async function resumeRun(candidate: ApplicationRun): Promise<void> {
    if (isRunning.value) return
    const controller = new AbortController()
    runController = controller
    isRunning.value = true
    error.value = ''
    successMessage.value = ''
    localResult.value = null

    try {
      let run = await appCenterAPI.getRun(candidate.id, controller.signal)
      if (!canResumeVideoRun(run) && !canResumeImageRun(run)) {
        throw new Error('这条记录没有可继续查询的媒体任务。')
      }
      if (!isSupportedApplicationSlug(run.app_slug)) throw new Error('这条记录的应用版本当前不可用。')

      selectedSlug.value = run.app_slug
      usePlanner.value = run.planner_api_key_id !== undefined && run.planner_api_key_id !== null
      selectedPlannerKeyId.value = String(run.planner_api_key_id || '')
      selectedMediaKeyId.value = String(run.media_api_key_id || '')
      if (!selectedMediaKey.value) throw new Error('原媒体密钥已不可用，请先恢复该密钥。')

      if (run.status === 'failed') run = await appCenterAPI.retryRun(run.id, controller.signal)

      const videoTask = videoTaskFromRun(run)
      const imageTask = imageTaskFromRun(run)
      if (videoTask && isResumableVideoStatus(videoTask.status)
        && run.next_action && isVideoExecutor(run.next_action.executor)) {
        run = {
          ...run,
          next_action: { ...run.next_action, gateway_request_id: videoTask.requestId },
        }
        submittedVideoRequestID = videoTask.requestId
      } else if (imageTask && isResumableImageStatus(imageTask.status)
        && run.next_action && isImageExecutor(run.next_action.executor)) {
        run = {
          ...run,
          next_action: { ...run.next_action, gateway_request_id: imageTask.taskId },
        }
        submittedImageTaskID = imageTask.taskId
      } else {
        throw new Error('媒体续查步骤未能恢复。')
      }

      activeRun.value = run
      selectedHistoryRun.value = run
      progressSteps.value = run.steps || []
      run = await driveRun(run, [], controller)
      selectedHistoryRun.value = run
      successMessage.value = '媒体任务已完成，结果已更新。'
    } catch (resumeError) {
      if (!isCancelled(resumeError)) error.value = safeError(resumeError, '继续查询媒体任务失败。')
    } finally {
      if (runController === controller) {
        runController = null
        isRunning.value = false
      }
      await loadHistory()
    }
  }

  async function cancelRun(): Promise<void> {
    await stopRun()
  }

  async function stopRun(): Promise<void> {
    const run = activeRun.value
    runController?.abort()
    runController = null
    isRunning.value = false
    if (!run) {
      successMessage.value = '已停止等待。'
      await loadHistory()
      return
    }

    const persistedImageTask = imageTaskFromRun(run)
    if (submittedVideoRequestID || videoTaskFromRun(run)?.requestId
      || submittedImageTaskID || isResumableImageStatus(persistedImageTask?.status)) {
      successMessage.value = `${submittedImageTaskID || isResumableImageStatus(persistedImageTask?.status) ? '图片' : '视频'}任务已提交，可稍后继续查询。`
      await loadHistory()
      return
    }

    try {
      activeRun.value = await appCenterAPI.cancelRun(run.id)
      successMessage.value = '已停止并取消本次运行。'
    } catch (cancelError) {
      error.value = safeError(cancelError, '停止运行失败。')
    } finally {
      await loadHistory()
    }
  }

  function clearCurrentResult(): void {
    if (localVideoObjectURL) {
      URL.revokeObjectURL(localVideoObjectURL)
      localVideoObjectURL = ''
    }
    localResult.value = null
  }

  async function resumeActiveRun(): Promise<void> {
    const candidate = activeRun.value || selectedHistoryRun.value
    if (candidate) await resumeRun(candidate)
  }

  async function openHistoryRun(run: ApplicationRun): Promise<void> {
    historyRunController?.abort()
    selectedHistoryRun.value = run
    clearHistoryVideoPreview()
    const controller = new AbortController()
    historyRunController = controller
    try {
      const detailedRun = await appCenterAPI.getRun(run.id, controller.signal)
      if (historyRunController !== controller || controller.signal.aborted) return
      selectedHistoryRun.value = detailedRun
      await loadHistoryVideoPreview(detailedRun)
    } catch (loadError) {
      if (!isCancelled(loadError) && historyRunController === controller) {
        error.value = safeError(loadError, '运行详情加载失败。')
      }
    } finally {
      if (historyRunController === controller) historyRunController = null
    }
  }

  onBeforeUnmount(() => {
    loadController?.abort()
    runController?.abort()
    historyRunController?.abort()
    clearHistoryVideoPreview()
    if (localVideoObjectURL) URL.revokeObjectURL(localVideoObjectURL)
  })

  return {
    applications: visibleApplications,
    history,
    selectedSlug,
    usePlanner,
    selectedApplication,
    selectedPresentation,
    selectedPlannerKeyId,
    selectedMediaKeyId,
    plannerKeyOptions,
    mediaKeyOptions,
    selectedMediaPlatform,
    activeRun,
    selectedHistoryRun,
    historyVideoPreviewUrl,
    historyVideoPreviewLoading,
    historyVideoPreviewError,
    localResult,
    progressSteps,
    isLoading,
    isLoadingHistory,
    isLoadingKeys,
    isRunning,
    error,
    successMessage,
    loadInitial,
    loadApplications,
    loadKeys,
    loadHistory,
    selectApplication,
    runApplication,
    cancelRun,
    stopRun,
    clearCurrentResult,
    resumeActiveRun,
    resumeRun,
    openHistoryRun,
  }
}
