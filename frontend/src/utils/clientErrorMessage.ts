
function activeLocale(): string {
  if (typeof document !== 'undefined' && document.documentElement.lang) {
    return document.documentElement.lang.toLowerCase()
  }
  if (typeof navigator !== 'undefined' && navigator.language) {
    return navigator.language.toLowerCase()
  }
  try {
    return (localStorage.getItem('locale') || 'en').toLowerCase()
  } catch {
    return 'en'
  }
}

type ErrorMessageOptions = {
  code?: unknown
  status?: number
  fallback?: string
}

const ZH_FALLBACK = '操作失败，请稍后重试。'

const EXACT_ZH_MESSAGES: Record<string, string> = {
  'unknown error': '发生未知错误。',
  'unknown error occurred': '发生未知错误。',
  'network error': '网络错误，请稍后重试。',
  'network error. please check your connection.': '网络错误，请检查网络连接。',
  'failed to fetch': '网络请求失败，请检查网络连接。',
  'request failed': '请求失败，请稍后重试。',
  'session expired': '登录状态已过期，请重新登录。',
  'session expired. please log in again.': '登录状态已过期，请重新登录。',
  'authentication session changed while refreshing.': '登录会话已变化，请重新登录。',
  'unauthorized': '登录状态已失效，请重新登录。',
  'forbidden': '没有权限执行此操作。',
  'access denied': '没有权限执行此操作。',
  'permission denied': '没有权限执行此操作。',
  'not found': '请求的资源不存在。',
  'bad request': '请求参数有误。',
  'invalid request': '请求参数有误。',
  'validation failed': '请求参数校验失败。',
  'invalid credentials': '账号或密码错误。',
  'invalid email or password': '邮箱或密码错误。',
  'user not found': '用户不存在。',
  'user not active': '账号已被禁用。',
  'account disabled': '账号已被禁用。',
  'email already exists': '该邮箱已被注册。',
  'email already registered': '该邮箱已被注册。',
  'invalid verification code': '验证码错误。',
  'verification code expired': '验证码已过期。',
  'too many requests': '请求过于频繁，请稍后再试。',
  'rate limit exceeded': '请求频率已达到上限，请稍后再试。',
  'insufficient balance': '余额不足。',
  'quota exceeded': '配额已用完。',
  'api key is invalid': 'API Key 无效。',
  'invalid api key': 'API Key 无效。',
  'api key not found': 'API Key 不存在。',
  'internal server error': '服务器内部错误，请稍后重试。',
  'service unavailable': '服务暂时不可用，请稍后重试。',
  'bad gateway': '网关错误，请稍后重试。',
  'gateway timeout': '请求超时，请稍后重试。',
  'request timeout': '请求超时，请稍后重试。',
  'timeout': '请求超时，请稍后重试。',
  'ops monitoring is disabled': '运营监控功能未启用。',
  'country, region, or ip not allowed': '当前地区或 IP 不允许访问。',
  'proxy connection failed': '代理连接失败。',
}

const CODE_ZH_MESSAGES: Record<string, string> = {
  ERR_NETWORK: '网络错误，请检查网络连接。',
  OPS_DISABLED: '运营监控功能未启用。',
  TOKEN_REFRESH_UNAVAILABLE: '登录服务暂时不可用，请稍后重试。',
  TOKEN_REFRESH_FAILED: '登录状态已过期，请重新登录。',
  AUTH_SESSION_CHANGED: '登录会话已变化，请重新登录。',
  UNAUTHORIZED: '登录状态已失效，请重新登录。',
  INVALID_TOKEN: '登录状态已失效，请重新登录。',
  TOKEN_EXPIRED: '登录状态已过期，请重新登录。',
  TOKEN_REVOKED: '登录状态已被撤销，请重新登录。',
  FORBIDDEN: '没有权限执行此操作。',
  ACCESS_DENIED: '没有权限执行此操作。',
  NOT_FOUND: '请求的资源不存在。',
  BAD_REQUEST: '请求参数有误。',
  INVALID_REQUEST: '请求参数有误。',
  VALIDATION_FAILED: '请求参数校验失败。',
  INVALID_CREDENTIALS: '账号或密码错误。',
  USER_NOT_FOUND: '用户不存在。',
  USER_NOT_ACTIVE: '账号已被禁用。',
  EMAIL_EXISTS: '该邮箱已被注册。',
  EMAIL_ALREADY_REGISTERED: '该邮箱已被注册。',
  RATE_LIMITED: '请求过于频繁，请稍后再试。',
  TOO_MANY_REQUESTS: '请求过于频繁，请稍后再试。',
  INSUFFICIENT_BALANCE: '余额不足。',
  QUOTA_EXCEEDED: '配额已用完。',
  API_KEY_INVALID: 'API Key 无效。',
  API_KEY_NOT_FOUND: 'API Key 不存在。',
  INTERNAL_ERROR: '服务器内部错误，请稍后重试。',
  INTERNAL_SERVER_ERROR: '服务器内部错误，请稍后重试。',
  SERVICE_UNAVAILABLE: '服务暂时不可用，请稍后重试。',
}

const STATUS_ZH_MESSAGES: Record<number, string> = {
  400: '请求参数有误。',
  401: '登录状态已失效，请重新登录。',
  403: '没有权限执行此操作。',
  404: '请求的资源不存在。',
  408: '请求超时，请稍后重试。',
  409: '请求发生冲突，请刷新后重试。',
  422: '请求参数校验失败。',
  423: '当前操作被锁定，请稍后重试。',
  429: '请求过于频繁，请稍后再试。',
  500: '服务器内部错误，请稍后重试。',
  502: '网关错误，请稍后重试。',
  503: '服务暂时不可用，请稍后重试。',
  504: '请求超时，请稍后重试。',
}

const PATTERN_ZH_MESSAGES: Array<[RegExp, string]> = [
  [/^request failed with status code (\d{3})$/i, '请求失败，请稍后重试。'],
  [/^timeout of \d+ms exceeded$/i, '请求超时，请稍后重试。'],
  [/^invalid .*credential/i, '账号或密码错误。'],
  [/^user .*not found/i, '用户不存在。'],
  [/^invalid .*(verification|code)/i, '验证码错误。'],
  [/^.*(already exists|already registered)/i, '该内容已存在。'],
  [/^.*(not found)/i, '请求的资源不存在。'],
  [/^.*(permission|forbidden|not allowed)/i, '没有权限执行此操作。'],
  [/^.*(rate limit|too many requests)/i, '请求过于频繁，请稍后再试。'],
  [/^.*(balance|quota)/i, '余额或配额不足。'],
  [/^.*(api key).*(invalid|not found)/i, 'API Key 无效或不存在。'],
  [/^.*(required|missing)/i, '必填项未填写。'],
  [/^.*(invalid|malformed)/i, '输入内容无效。'],
  [/^.*(failed|failure|error)/i, ZH_FALLBACK],
]

function hasChinese(value: string): boolean {
  return /[\u3400-\u9fff]/.test(value)
}

function normalizeMessage(value: string): string {
  return value.trim().replace(/\s+/g, ' ').toLowerCase()
}

function messageFromCode(code?: unknown): string | undefined {
  if (code == null) return undefined
  return CODE_ZH_MESSAGES[String(code).trim().toUpperCase()]
}

function messageFromStatus(status?: number): string | undefined {
  if (!status) return undefined
  return STATUS_ZH_MESSAGES[status]
}

export function localizeClientErrorMessage(
  message?: string | null,
  options: ErrorMessageOptions = {},
): string {
  const raw = (message ?? '').trim()
  if (!activeLocale().startsWith('zh')) return raw

  if (raw && hasChinese(raw)) return raw

  const fromCode = messageFromCode(options.code)
  if (fromCode) return fromCode

  if (raw) {
    const normalized = normalizeMessage(raw)
    const exact = EXACT_ZH_MESSAGES[normalized]
    if (exact) return exact

    for (const [pattern, translated] of PATTERN_ZH_MESSAGES) {
      if (pattern.test(raw)) return translated
    }

    // Never let an unmapped English backend error leak into the Chinese UI.
    if (/[A-Za-z]/.test(raw) && !hasChinese(raw)) {
      return options.fallback || ZH_FALLBACK
    }
  }

  return messageFromStatus(options.status) || options.fallback || ZH_FALLBACK
}
