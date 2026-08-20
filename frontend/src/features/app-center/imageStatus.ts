import type {
  ApplicationExecutor,
  ApplicationRun,
  ApplicationRunStep,
} from './types'

export const IMAGE_EXECUTORS = [
  'image_generation',
  'image_edit',
] as const satisfies readonly ApplicationExecutor[]

export type ImageTaskStatus = 'processing' | 'completed' | 'failed' | 'cancelled' | (string & {})

export interface PersistedImageTask {
  taskId: string
  status: ImageTaskStatus
  errorMessage: string
  stepKey: string
}

interface ImageTaskOutput {
  taskId: string
  status: ImageTaskStatus | ''
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function normalizeStatus(value: unknown): ImageTaskStatus | '' {
  return text(value).toLowerCase() as ImageTaskStatus | ''
}

function taskOutput(value: unknown): ImageTaskOutput {
  if (!isRecord(value)) return { taskId: '', status: '' }
  return {
    taskId: text(value.task_id) || text(value.request_id),
    status: normalizeStatus(value.status),
  }
}

export function isImageExecutor(value: unknown): value is (typeof IMAGE_EXECUTORS)[number] {
  return typeof value === 'string' && IMAGE_EXECUTORS.includes(value as (typeof IMAGE_EXECUTORS)[number])
}

export function isResumableImageStatus(value: unknown): boolean {
  return normalizeStatus(value) === 'processing'
}

function fallbackStatus(
  run: ApplicationRun,
  step: ApplicationRunStep | undefined,
  taskId: string,
): ImageTaskStatus | '' {
  const runStatus = normalizeStatus(run.status)
  if (runStatus === 'cancelled') return 'cancelled'
  if (runStatus === 'completed') return 'completed'
  if (runStatus === 'failed' && normalizeStatus(step?.status) !== 'running') return 'failed'
  return taskId ? 'processing' : ''
}

export function imageTaskFromRun(run: ApplicationRun | null | undefined): PersistedImageTask | null {
  if (!run) return null

  const action = run.next_action && isImageExecutor(run.next_action.executor)
    ? run.next_action
    : null
  const imageSteps = (run.steps || []).filter((step) => isImageExecutor(step.executor))
  const step = (action
    ? imageSteps.find((item) => item.step_key === action.step_key)
    : undefined)
    || [...imageSteps].reverse().find((item) => {
      const output = taskOutput(item.output)
      return !!item.gateway_request_id || !!output.taskId
    })
    || [...imageSteps].reverse()[0]

  const stepOutput = taskOutput(step?.output)
  const taskId = text(action?.gateway_request_id)
    || text(step?.gateway_request_id)
    || stepOutput.taskId
  if (!taskId) return null

  const runStatus = normalizeStatus(run.status)
  const status = runStatus === 'cancelled'
    ? 'cancelled'
    : runStatus === 'completed'
      ? 'completed'
      : stepOutput.status || fallbackStatus(run, step, taskId) || 'processing'

  return {
    taskId,
    status,
    errorMessage: text(step?.error_message) || text(step?.error) || text(run.error_message) || text(run.error),
    stepKey: step?.step_key || action?.step_key || '',
  }
}

export function canResumeImageRun(run: ApplicationRun | null | undefined): boolean {
  const task = imageTaskFromRun(run)
  if (!task || !isResumableImageStatus(task.status)) return false
  return ['pending', 'running', 'failed'].includes(normalizeStatus(run?.status))
}
