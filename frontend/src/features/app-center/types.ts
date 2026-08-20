export const APPLICATION_SLUGS = [
  'free-media-studio',
  'inspiration-image-refiner',
  'ecommerce-visual-planner',
  'grok-video-director',
] as const

export type ApplicationSlug = (typeof APPLICATION_SLUGS)[number]

export type ApplicationExecutor =
  | 'chat_completions'
  | 'image_generation'
  | 'image_edit'
  | 'video_generation'
  | 'video_edit'
  | 'video_extension'

export type ApplicationCredentialRole = 'planner' | 'media'

export type ApplicationRunStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | (string & {})

export interface ApplicationFormOption {
  label: string
  value: unknown
}

export interface ApplicationFormField {
  key: string
  label: string
  type: string
  required?: boolean
  description?: string
  placeholder?: string
  default?: unknown
  options?: ApplicationFormOption[]
  min?: number
  max?: number
  step?: number
  accept?: string
  multiple?: boolean
  max_items?: number
  visible_when?: Record<string, unknown>
}

export interface ApplicationFormDefinition {
  fields: ApplicationFormField[]
}

export interface ApplicationDefinition {
  slug: string
  revision: number
  version: string
  name: string
  description: string
  category: string
  status: string
  tags: string[]
  supported_modes: string[]
  form: ApplicationFormDefinition
}

export interface ApplicationNextAction {
  step_key: string
  executor: ApplicationExecutor
  credential_role: ApplicationCredentialRole
  api_key_id: string | number
  session_id: string
  path: string
  method: string
  content_type: string
  request: Record<string, unknown>
  body: Record<string, unknown>
  repair_instruction?: string
  gateway_request_id?: string
}

export interface ApplicationRunStep {
  id?: string | number
  step_key: string
  sequence?: number
  executor?: ApplicationExecutor
  credential_role?: ApplicationCredentialRole
  status: ApplicationRunStatus
  output?: unknown
  error?: string
  error_message?: string
  gateway_request_id?: string
  error_code?: string
  attempt?: number
  started_at?: string
  completed_at?: string
  created_at?: string
  updated_at?: string
}

export interface ApplicationRun {
  id: string | number
  app_slug: string
  app_revision?: number
  status: ApplicationRunStatus
  input?: Record<string, unknown>
  output?: unknown
  error?: string
  error_message?: string
  planner_api_key_id?: string | number
  media_api_key_id?: string | number
  session_id?: string
  current_step_key?: string
  error_code?: string
  retry_count?: number
  next_action?: ApplicationNextAction | null
  steps?: ApplicationRunStep[]
  created_at?: string
  updated_at?: string
  completed_at?: string
  cancelled_at?: string
}

export interface ApplicationRunList {
  items: ApplicationRun[]
  total?: number
}

export interface StartApplicationRunRequest {
  planner_api_key_id?: string | number
  media_api_key_id: string | number
  input: Record<string, unknown>
}

export interface CompleteApplicationStepRequest {
  output: unknown
  gateway_request_id?: string
}

export interface FailApplicationStepRequest {
  error: string
  output?: unknown
  gateway_request_id?: string
}

export interface CheckpointApplicationStepRequest {
  output: Record<string, unknown>
  gateway_request_id: string
}

export interface RunnerInputPayload {
  values: Record<string, unknown>
  files: File[]
}

export interface LocalMediaResult {
  type: 'images' | 'video' | 'planner'
  images?: Array<{ url: string; revisedPrompt: string }>
  videoUrl?: string
  requestId?: string
  videoStatus?: string
  plannerOutput?: unknown
}

export interface AppPresentation {
  slug: ApplicationSlug
  name: string
  description: string
  category: 'media' | 'image' | 'commerce' | 'video'
  version: string
  icon: 'swatches' | 'lightbulb' | 'shoppingBag' | 'videoCamera'
  requiresReference: boolean
  acceptsMultipleReferences: boolean
}
