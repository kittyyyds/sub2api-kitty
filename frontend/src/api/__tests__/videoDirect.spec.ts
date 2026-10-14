import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchVideoContent } from '../videoDirect'

vi.mock('../url', () => ({ buildGatewayUrl: (path: string) => path }))
afterEach(() => vi.unstubAllGlobals())
const delivery = (data: object) => new Response(JSON.stringify(data), { status: 200 })

describe('video supplier direct delivery', () => {
  it('sends the site Key only to the status endpoint and only range headers to the CDN', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(delivery({status:'completed',delivery_mode:'upstream_direct',url:'https://cdn.example/video.mp4?ticket=fixture'})).mockResolvedValueOnce(new Response('video'))
    vi.stubGlobal('fetch',fetch)
    await fetchVideoContent('site-fixture','task', {Range:'bytes=10-', 'If-Range':'etag',Authorization:'must-not-forward','X-Canvas-Key-ID':'private'})
    expect(fetch.mock.calls[0][1].headers).toEqual({Authorization:'Bearer site-fixture'})
    expect(fetch.mock.calls[1][1]).toMatchObject({headers:{Range:'bytes=10-','If-Range':'etag'},credentials:'omit',referrerPolicy:'no-referrer'})
    expect(fetch).toHaveBeenCalledTimes(2)
  })
  it('never falls back to OVH on CORS failure', async () => {
    const fetch=vi.fn().mockResolvedValueOnce(delivery({status:'completed',delivery_mode:'upstream_direct',url:'https://cdn.example/video.mp4'})).mockRejectedValueOnce(new TypeError('CORS'))
    vi.stubGlobal('fetch',fetch)
    await expect(fetchVideoContent('key','task')).rejects.toThrow('CORS')
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(fetch.mock.calls.some(c=>String(c[0]).endsWith('/content'))).toBe(false)
  })
  it.each([
    {status:'submission_unknown',delivery_mode:'upstream_direct'},
    {status:'failed',delivery_mode:'upstream_direct'},
    {status:'completed',delivery_mode:'upstream_direct',url:'http://cdn.example/video.mp4'},
    {status:'completed',delivery_mode:'upstream_direct',url:'https://user:password@cdn.example/video.mp4'},
    {status:'completed',delivery_mode:'upstream_direct',delivery_status:'expired',url:'https://cdn.example/video.mp4'}
  ])('rejects unready, expired, and unsafe direct delivery %j', async data => {
    const fetch=vi.fn().mockResolvedValueOnce(delivery(data)); vi.stubGlobal('fetch',fetch)
    await expect(fetchVideoContent('key','task')).rejects.toThrow()
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('preserves the existing authenticated content endpoint for legacy tasks',async()=>{
    const fetch=vi.fn().mockResolvedValueOnce(delivery({status:'done'})).mockResolvedValueOnce(new Response('legacy'))
    vi.stubGlobal('fetch',fetch); await fetchVideoContent('key','old-task')
    expect(fetch.mock.calls[1][0]).toBe('/v1/videos/old-task/content')
    expect(fetch.mock.calls[1][1].headers.Authorization).toBe('Bearer key')
  })
})
