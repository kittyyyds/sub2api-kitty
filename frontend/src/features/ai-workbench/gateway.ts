import { buildGatewayUrl } from '@/api/url'

export interface GatewayModel {
  id: string
  ownedBy: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function asText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

export function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
    || (isRecord(error) && (error.name === 'AbortError' || error.code === 'ERR_CANCELED'))
}

export function gatewayAuthorization(apiKey: string): HeadersInit {
  const key = apiKey.trim()
  if (!key) throw new Error('请选择一个可用的 API Key。')
  return { Authorization: `Bearer ${key}` }
}

export async function readGatewayError(response: Response, fallback: string): Promise<Error> {
  const text = await response.text()
  let payload: unknown = null
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    payload = null
  }
  const nested = isRecord(payload) && isRecord(payload.error) ? payload.error : null
  const message = isRecord(payload)
    ? asText(payload.message) || asText(payload.detail) || asText(nested?.message)
    : ''
  const resolved = message || text.trim() || `${fallback}（HTTP ${response.status}）`
  return new Error(resolved.length > 800 ? `${resolved.slice(0, 800)}...` : resolved)
}

function normalizeOpenAIModels(payload: unknown): GatewayModel[] {
  if (!isRecord(payload) || !Array.isArray(payload.data)) return []
  return payload.data.flatMap((raw) => {
    if (!isRecord(raw)) return []
    const id = asText(raw.id)
    return id ? [{ id, ownedBy: asText(raw.owned_by) }] : []
  })
}

function normalizeGeminiModels(payload: unknown): GatewayModel[] {
  if (!isRecord(payload) || !Array.isArray(payload.models)) return []
  return payload.models.flatMap((raw) => {
    if (!isRecord(raw)) return []
    const id = asText(raw.name).replace(/^models\//, '')
    return id ? [{ id, ownedBy: 'google' }] : []
  })
}

async function fetchModelList(path: string, apiKey: string, signal?: AbortSignal): Promise<GatewayModel[]> {
  const response = await fetch(buildGatewayUrl(path), {
    method: 'GET',
    headers: gatewayAuthorization(apiKey),
    signal,
  })
  if (!response.ok) throw await readGatewayError(response, '模型列表加载失败')
  const payload = await response.json() as unknown
  return path.startsWith('/v1beta/') ? normalizeGeminiModels(payload) : normalizeOpenAIModels(payload)
}

export async function listGatewayModels(
  apiKey: string,
  platform = '',
  signal?: AbortSignal,
): Promise<GatewayModel[]> {
  const normalizedPlatform = platform.trim().toLowerCase()
  let models: GatewayModel[] = []
  let firstError: unknown = null

  try {
    models = await fetchModelList('/v1/models', apiKey, signal)
  } catch (error) {
    firstError = error
  }

  if (!models.length && ['gemini', 'antigravity'].includes(normalizedPlatform)) {
    try {
      models = await fetchModelList('/v1beta/models', apiKey, signal)
    } catch (error) {
      if (!firstError) firstError = error
    }
  }

  if (!models.length && firstError) throw firstError
  const unique = new Map<string, GatewayModel>()
  for (const model of models) {
    if (!unique.has(model.id)) unique.set(model.id, model)
  }
  return [...unique.values()].sort((a, b) => a.id.localeCompare(b.id))
}

export function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error || new Error(`无法读取文件：${file.name}`))
    reader.onload = () => resolve(String(reader.result || ''))
    reader.readAsDataURL(file)
  })
}

export function fileToText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error || new Error(`无法读取文件：${file.name}`))
    reader.onload = () => resolve(String(reader.result || ''))
    reader.readAsText(file)
  })
}
