import type {
  ApplicationExecutor,
  ApplicationRun,
  ApplicationRunStep,
} from './types'

export const VIDEO_EXECUTORS = [
  'video_generation',
  'video_edit',
  'video_extension',
] as const satisfies readonly ApplicationExecutor[]

export const VIDEO_RESUMABLE_STATUSES = [
  'pending',
  'queued',
  'submitted',
  'processing',
  'in_progress',
] as const

export const VIDEO_TERMINAL_STATUSES = [
  'failed',
  'expired',
  'cancelled',
  'done',
  'completed',
  'succeeded',
] as const

export type VideoResumableStatus = (typeof VIDEO_RESUMABLE_STATUSES)[number]
export type VideoTerminalStatus = (typeof VIDEO_TERMINAL_STATUSES)[number]
export type VideoTaskStatus = VideoResumableStatus | VideoTerminalStatus | (string & {})

export interface PersistedVideoTask {
  requestId: string
  status: VideoTaskStatus
  contentUrl: string
  errorMessage: string
  stepKey: string
}

interface VideoOutput {
  requestId: string
  status: VideoTaskStatus | ''
  contentUrl: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function videoOutput(value: unknown): VideoOutput {
  if (!isRecord(value)) return { requestId: '', status: '', contentUrl: '' }
  return {
    requestId: text(value.request_id) || text(value.requestId) || text(value.id),
    status: normalizeVideoStatus(value.status),
    contentUrl: text(value.video_url) || text(value.content_url) || text(value.url),
  }
}

export function normalizeVideoStatus(value: unknown): VideoTaskStatus | '' {
  return text(value).toLowerCase() as VideoTaskStatus | ''
}

export function isVideoExecutor(value: unknown): value is (typeof VIDEO_EXECUTORS)[number] {
  return typeof value === 'string' && VIDEO_EXECUTORS.includes(value as (typeof VIDEO_EXECUTORS)[number])
}

export function isResumableVideoStatus(value: unknown): value is VideoResumableStatus {
  const status = normalizeVideoStatus(value)
  return VIDEO_RESUMABLE_STATUSES.includes(status as VideoResumableStatus)
}

export function isTerminalVideoStatus(value: unknown): value is VideoTerminalStatus {
  const status = normalizeVideoStatus(value)
  return VIDEO_TERMINAL_STATUSES.includes(status as VideoTerminalStatus)
}

export function isSuccessfulVideoStatus(value: unknown): value is 'done' | 'completed' | 'succeeded' {
  return ['done', 'completed', 'succeeded'].includes(normalizeVideoStatus(value))
}

export function isFailedVideoStatus(value: unknown): value is 'failed' | 'expired' | 'cancelled' {
  return ['failed', 'expired', 'cancelled'].includes(normalizeVideoStatus(value))
}

function fallbackStatus(run: ApplicationRun, step: ApplicationRunStep | undefined, requestId: string): VideoTaskStatus | '' {
  const stepStatus = normalizeVideoStatus(step?.status)
  if (isTerminalVideoStatus(stepStatus)) return stepStatus
  const runStatus = normalizeVideoStatus(run.status)
  if (isTerminalVideoStatus(runStatus)) return runStatus
  return requestId ? 'processing' : ''
}

export function videoTaskFromRun(run: ApplicationRun | null | undefined): PersistedVideoTask | null {
  if (!run) return null

  const action = run.next_action && isVideoExecutor(run.next_action.executor)
    ? run.next_action
    : null
  const videoSteps = (run.steps || []).filter((step) => isVideoExecutor(step.executor))
  const step = (action
    ? videoSteps.find((item) => item.step_key === action.step_key)
    : undefined)
    || [...videoSteps].reverse().find((item) => {
      const output = videoOutput(item.output)
      return !!item.gateway_request_id || !!output.requestId
    })
    || [...videoSteps].reverse()[0]

  const stepOutput = videoOutput(step?.output)
  const runOutput = videoOutput(run.output)
  const requestId = text(action?.gateway_request_id)
    || text(step?.gateway_request_id)
    || stepOutput.requestId
    || runOutput.requestId
  if (!requestId) return null

  const persistedStatus = stepOutput.status || runOutput.status
  const runStatus = normalizeVideoStatus(run.status)
  const status = runStatus === 'cancelled'
    ? 'cancelled'
    : runStatus === 'completed' && !isSuccessfulVideoStatus(persistedStatus)
      ? 'completed'
      : persistedStatus || fallbackStatus(run, step, requestId) || 'processing'

  return {
    requestId,
    status,
    contentUrl: stepOutput.contentUrl || runOutput.contentUrl,
    errorMessage: text(step?.error_message) || text(step?.error) || text(run.error_message) || text(run.error),
    stepKey: step?.step_key || action?.step_key || '',
  }
}

export function canResumeVideoRun(run: ApplicationRun | null | undefined): boolean {
  const task = videoTaskFromRun(run)
  if (!task || !isResumableVideoStatus(task.status)) return false
  return ['pending', 'running', 'failed'].includes(normalizeVideoStatus(run?.status))
}
