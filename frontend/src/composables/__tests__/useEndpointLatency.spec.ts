import { effectScope, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { endpointHealthUrl, useEndpointLatency } from '../useEndpointLatency'

function response(status: number): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
  } as Response
}

describe('useEndpointLatency', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('把带 /v1 的 API 地址转换为根路径健康检查地址', () => {
    const url = new URL(endpointHealthUrl('https://api.example.com/v1?foo=bar#hash'))

    expect(url.origin).toBe('https://api.example.com')
    expect(url.pathname).toBe('/health')
    expect(url.searchParams.has('_latency_probe')).toBe(true)
    expect(url.searchParams.has('foo')).toBe(false)
    expect(url.hash).toBe('')
  })

  it('记录成功延迟，并将最快成功端点标记为推荐', async () => {
    const scope = effectScope()
    const fetcher = vi.fn().mockResolvedValue(response(200))
    const timestamps = [0, 80, 100, 120]
    const endpoints = ref(['https://slow.example.com/v1', 'https://fast.example.com/v1'])
    const latency = scope.run(() => useEndpointLatency(endpoints, {
      fetcher,
      now: () => timestamps.shift() ?? 120,
    }))!

    await latency.testEndpoint(endpoints.value[0])
    await latency.testEndpoint(endpoints.value[1])

    expect(latency.states[endpoints.value[0]]).toMatchObject({
      status: 'success',
      latencyMs: 80,
      statusCode: 200,
    })
    expect(latency.states[endpoints.value[1]]).toMatchObject({
      status: 'success',
      latencyMs: 20,
      statusCode: 200,
    })
    expect(latency.recommendedEndpoint.value).toBe(endpoints.value[1])
    scope.stop()
  })

  it('全部测速会请求所有端点且不推荐 HTTP 失败项', async () => {
    const scope = effectScope()
    const endpoints = ref(['https://ok.example.com/v1', 'https://bad.example.com/v1'])
    const fetcher = vi.fn(async input =>
      String(input).includes('bad.example.com') ? response(502) : response(200),
    )
    const latency = scope.run(() => useEndpointLatency(endpoints, { fetcher }))!

    await latency.testAll()

    expect(fetcher).toHaveBeenCalledTimes(2)
    expect(latency.states[endpoints.value[0]].status).toBe('success')
    expect(latency.states[endpoints.value[1]]).toMatchObject({ status: 'error', statusCode: 502 })
    expect(latency.recommendedEndpoint.value).toBe(endpoints.value[0])
    scope.stop()
  })

  it('网络异常显示失败', async () => {
    const scope = effectScope()
    const endpoint = 'https://offline.example.com/v1'
    const latency = scope.run(() => useEndpointLatency([endpoint], {
      fetcher: vi.fn().mockRejectedValue(new TypeError('Failed to fetch')),
    }))!

    await latency.testEndpoint(endpoint)

    expect(latency.states[endpoint]).toMatchObject({
      status: 'error',
      latencyMs: null,
      statusCode: null,
    })
    scope.stop()
  })

  it('超过超时时间会中止请求并显示超时', async () => {
    vi.useFakeTimers()
    const scope = effectScope()
    const endpoint = 'https://timeout.example.com/v1'
    const fetcher = vi.fn((_input: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => {
          reject(new DOMException('Aborted', 'AbortError'))
        })
      }),
    )
    const latency = scope.run(() => useEndpointLatency([endpoint], {
      fetcher,
      timeoutMs: 100,
    }))!

    const pending = latency.testEndpoint(endpoint)
    await vi.advanceTimersByTimeAsync(101)
    await pending

    expect(latency.states[endpoint].status).toBe('timeout')
    scope.stop()
  })
})
