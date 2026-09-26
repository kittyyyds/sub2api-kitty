export interface OpenAIRequestTimezoneSettings {
  enabled: boolean
  timezone: string
  syncSearch: boolean
}

export function readOpenAIRequestTimezone(extra?: Record<string, unknown> | null): OpenAIRequestTimezoneSettings {
  return {
    enabled: extra?.openai_request_timezone_enabled === true,
    timezone: typeof extra?.openai_request_timezone === 'string' && extra.openai_request_timezone
      ? extra.openai_request_timezone : 'Asia/Singapore',
    syncSearch: extra?.openai_request_timezone_sync_search === true
  }
}

export function withOpenAIRequestTimezone(extra: Record<string, unknown> | null | undefined, settings: OpenAIRequestTimezoneSettings): Record<string, unknown> {
  return {
    ...extra,
    openai_request_timezone_enabled: settings.enabled,
    openai_request_timezone: settings.timezone,
    openai_request_timezone_sync_search: settings.syncSearch
  }
}
