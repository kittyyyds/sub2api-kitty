import { describe, expect, it } from 'vitest'
import { buildPelicanPosterDocument, readPelicanPosterMessage } from '../pelicanPoster'

describe('pelican poster safety', () => {
  it('keeps restrictive CSP before instrumentation and authored scripts', () => {
    const html = buildPelicanPosterDocument('<html><body><script>window.bad=1</script><svg/></body></html>', 'channel', 'validNonce')
    expect(html.indexOf('Content-Security-Policy')).toBeLessThan(html.indexOf('<script'))
    expect(html).toContain("connect-src 'none'")
    expect(html).toContain("frame-src 'none'")
    expect(html).toContain('nonce="validNonce"')
    expect(html).not.toContain('allow-same-origin')
  })
  it('rejects oversized and unrecognized artwork', () => {
    expect(buildPelicanPosterDocument('abc', 'c', '')).toBe('')
    expect(buildPelicanPosterDocument('<svg>' + 'a'.repeat(600000) + '</svg>', 'c', '')).toBe('')
  })
  it('accepts only this frame/channel and a bounded PNG, not SVG or arbitrary payloads', () => {
    const frame = {} as Window
    const buffer = new ArrayBuffer(40), view = new DataView(buffer)
    for (const [offset, value] of [[0,0x89504e47],[4,0x0d0a1a0a],[8,13],[12,0x49484452],[16,800],[20,600]]) view.setUint32(offset,value)
    const event = { source:frame, data:{type:'kitty-pelican-poster-v1',channel:'c',buffer} } as unknown as MessageEvent
    expect(readPelicanPosterMessage(event,frame,'c')).toBe(buffer)
    expect(readPelicanPosterMessage(event,frame,'wrong')).toBeNull()
    expect(readPelicanPosterMessage(event,{} as Window,'c')).toBeNull()
    view.setUint32(16,90000)
    expect(readPelicanPosterMessage(event,frame,'c')).toBe(false)
    expect(readPelicanPosterMessage({...event,data:{...event.data,buffer:'<svg/>'}},frame,'c')).toBe(false)
  })
})
