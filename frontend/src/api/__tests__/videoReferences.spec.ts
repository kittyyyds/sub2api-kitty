import { afterEach, describe, expect, it, vi } from 'vitest'
import { webcrypto } from 'node:crypto'
import { uploadVideoReference, validateVideoReference } from '../videoReferences'

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })

describe('temporary video references', () => {
  it('rejects active formats, wrong kind and oversized files before network', () => {
    expect(() => validateVideoReference(new Blob(['<svg/>'], { type: 'image/svg+xml' }), 'image')).toThrow()
    expect(() => validateVideoReference(new Blob(['file'], { type: 'audio/mpeg' }), 'image')).toThrow()
    expect(() => validateVideoReference(new Blob([new Uint8Array(50 * 1024 * 1024 + 1)], { type: 'image/png' }), 'image')).toThrow('50MB')
  })
  it('authenticates multipart upload with a header and never starts model generation', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-22T12:00:00Z'))
    vi.stubGlobal('crypto', webcrypto)
    const fetcher = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ id: 'fixture', url: 'https://media.example/reference', expires_at: '2026-09-23T00:00:00Z' }) })
    vi.stubGlobal('fetch', fetcher)
    const file = new File(['fixture'], 'local.png', { type: 'image/png' })
    Object.defineProperty(file, 'arrayBuffer', { value: async () => new TextEncoder().encode('fixture').buffer })
    const result = await uploadVideoReference('fixture-only-key', file, 'image', new AbortController().signal)
    expect(result.url).toBe('https://media.example/reference')
    expect(fetcher).toHaveBeenCalledTimes(1)
    const [path, options] = fetcher.mock.calls[0]
    expect(path).toContain('/v1/video-references')
    expect(path).not.toContain('fixture-only-key')
    expect(options.headers).toEqual({ Authorization: 'Bearer fixture-only-key' })
    expect(options.body.get('file')).toBe(file)
    expect(options.headers['Content-Type']).toBeUndefined()
    await uploadVideoReference('fixture-only-key', file, 'image', new AbortController().signal)
    expect(fetcher).toHaveBeenCalledTimes(1)
    await uploadVideoReference('different-fixture-key', file, 'image', new AbortController().signal)
    expect(fetcher).toHaveBeenCalledTimes(2)
  })
})
