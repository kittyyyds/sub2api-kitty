import { describe, expect, it } from 'vitest'
import { applyGrokMediaProvider, readGrokMediaProvider } from '../grokMediaProvider'

describe('Grok media provider selection', () => {
  it('persists the explicit provider and round-trips the edit value', () => {
    const credentials: Record<string, unknown> = { api_key: 'test-value', base_url: 'https://suanliai.top' }
    applyGrokMediaProvider(credentials, 'suanli_special')
    expect(readGrokMediaProvider(credentials)).toBe('suanli_special')
    applyGrokMediaProvider(credentials, '')
    expect(credentials).toEqual({ api_key: 'test-value', base_url: 'https://suanliai.top' })
  })
  it('keeps existing Grok credentials free of new provider fields by default', () => {
    expect(readGrokMediaProvider(undefined)).toBe('')
    const credentials = { base_url: 'https://api.x.ai/v1' }
    applyGrokMediaProvider(credentials, '')
    expect(credentials).toEqual({ base_url: 'https://api.x.ai/v1' })
  })
})
