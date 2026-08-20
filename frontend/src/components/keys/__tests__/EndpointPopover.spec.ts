import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const copyToClipboard = vi.fn().mockResolvedValue(true)

const messages: Record<string, string> = {
  'keys.endpoints.title': 'API 端点',
  'keys.endpoints.default': '默认',
  'keys.endpoints.copied': '已复制',
  'keys.endpoints.copiedHint': '已复制到剪贴板',
  'keys.endpoints.copy': '复制端点',
  'keys.endpoints.speedTestEndpoint': '测试 {name} 延迟',
  'keys.endpoints.speedTestAll': '全部测速',
  'keys.endpoints.testingAll': '测速中',
  'keys.endpoints.notTested': '未测速',
  'keys.endpoints.testing': '测速中',
  'keys.endpoints.timeout': '超时',
  'keys.endpoints.failed': '失败',
  'keys.endpoints.httpError': 'HTTP {status}',
  'keys.endpoints.recommended': '推荐',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      let message = messages[key] ?? key
      for (const [name, value] of Object.entries(params ?? {})) {
        message = message.replace(`{${name}}`, String(value))
      }
      return message
    },
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

import EndpointPopover from '../EndpointPopover.vue'

function response(status: number): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
  } as Response
}

describe('EndpointPopover', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(200)))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('默认端点移除末尾 /v1，自定义端点保持后台配置', () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [
          {
            name: '备用线路',
            endpoint: 'https://backup.example.com/v1',
            description: '自定义说明',
          },
        ],
      },
    })

    expect(wrapper.text()).toContain('API 端点')
    expect(wrapper.text()).toContain('https://default.example.com')
    expect(wrapper.text()).not.toContain('https://default.example.com/v1')
    expect(wrapper.text()).toContain('备用线路')
    expect(wrapper.text()).toContain('https://backup.example.com/v1')
    expect(wrapper.find('button[title="自定义说明"]').exists()).toBe(true)
  })

  it.each([
    'https://default.example.com/v1',
    'https://default.example.com/v1/',
  ])('默认端点 %s 统一显示为不带 /v1 的地址', (apiBaseUrl) => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl,
        customEndpoints: [],
      },
    })

    expect(wrapper.text()).toContain('https://default.example.com')
    expect(wrapper.text()).not.toContain('/v1')
  })

  it('未配置默认端点时回退到当前站点地址', () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: '',
        customEndpoints: [],
      },
    })

    expect(wrapper.text()).toContain(window.location.origin)
    expect(wrapper.text()).toContain('默认')
  })

  it('点击端点后复制并切换为已复制提示', async () => {
    const configuredEndpoint = 'https://default.example.com/v1/'
    const displayedEndpoint = 'https://default.example.com'
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: configuredEndpoint,
        customEndpoints: [],
      },
    })

    await wrapper.find(`button[title="${displayedEndpoint}"]`).trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith(displayedEndpoint, '已复制')
    expect(wrapper.find('button[aria-label="已复制到剪贴板"]').exists()).toBe(true)
  })

  it('复用表格的表头和表体材质，不再渲染蓝色嵌套卡片', () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [],
      },
    })

    expect(wrapper.get('.endpoint-panel').classes()).toContain('card')
    expect(wrapper.get('.endpoint-list').classes()).toContain('dark:bg-dark-900')
    expect(wrapper.get('.endpoint-row').classes()).not.toContain('dark:bg-dark-900/45')
    expect(wrapper.get('.endpoint-panel').classes()).not.toContain('dark:bg-dark-800/90')
  })

  it('单条测速请求同域 /health 并显示延迟和推荐', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [],
      },
    })

    await wrapper.find('button[aria-label="测试 API 端点 延迟"]').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [input, init] = fetchMock.mock.calls[0]
    const probeUrl = new URL(String(input))
    expect(probeUrl.origin).toBe('https://default.example.com')
    expect(probeUrl.pathname).toBe('/health')
    expect(init).toMatchObject({ method: 'GET', cache: 'no-store', credentials: 'omit' })
    expect(wrapper.text()).toMatch(/\d+ ms/)
    expect(wrapper.text()).toContain('推荐')
  })

  it('全部测速并发执行，并显示失败线路', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async input =>
      String(input).includes('backup.example.com') ? response(503) : response(200),
    )
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [
          {
            name: '备用线路',
            endpoint: 'https://backup.example.com/v1',
            description: '',
          },
        ],
      },
    })

    const allButton = wrapper.findAll('button').find(button => button.text().includes('全部测速'))
    expect(allButton).toBeDefined()
    await allButton!.trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('HTTP 503')
    expect(wrapper.text()).toContain('推荐')
  })
})
