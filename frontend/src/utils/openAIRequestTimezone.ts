export interface OpenAIRequestTimezoneSettings {
  enabled: boolean
  timezone: string
  syncSearch: boolean
  mode?: 'manual' | 'proxy'
}

export function readOpenAIRequestTimezone(extra?: Record<string, unknown> | null): OpenAIRequestTimezoneSettings {
  return {
    ...(extra?.openai_request_timezone_mode === 'proxy' ? { mode: 'proxy' as const } : {}),
    enabled: extra?.openai_request_timezone_enabled === true,
    timezone: typeof extra?.openai_request_timezone === 'string' && extra.openai_request_timezone
      ? extra.openai_request_timezone : 'Asia/Singapore',
    syncSearch: extra?.openai_request_timezone_sync_search === true
  }
}

export function withOpenAIRequestTimezone(extra: Record<string, unknown> | null | undefined, settings: OpenAIRequestTimezoneSettings): Record<string, unknown> {
  return {
    ...extra,
    ...(settings.mode ? { openai_request_timezone_mode: settings.mode } : {}),
    openai_request_timezone_enabled: settings.enabled,
    openai_request_timezone: settings.timezone,
    openai_request_timezone_sync_search: settings.syncSearch
  }
}
