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
        defaultEndpointDescription: '国内直连',
        customEndpoints: [
          {
            name: '备用线路',
            endpoint: 'https://backup.example.com/v1',
            description: '自定义说明',
          },
        ],
      },
    })

    expect(wrapper.get('section').attributes('aria-label')).toBe('API 端点')
    expect(wrapper.text()).toContain('https://default.example.com')
    expect(wrapper.text()).toContain('国内直连')
    expect(wrapper.text()).toContain('推荐')
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
        autoTest: false,
      },
    })

    await wrapper.find(`button[title="${displayedEndpoint}"]`).trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith(displayedEndpoint, '已复制')
    expect(wrapper.find('button[aria-label="已复制到剪贴板"]').exists()).toBe(true)
  })

  it('端点独立成紧凑短条，保留默认和可配置的推荐线路', () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://api.kittyyyds.xyz',
        defaultEndpointDescription: '回国线路',
        autoTest: false,
        customEndpoints: [{ name: '国际路线', endpoint: 'https://kittyyyds.xyz', description: '', recommended: true }],
      },
    })
    const rows = wrapper.findAll('.endpoint-row')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('回国线路')
    expect(rows[0].text()).toContain('默认')
    expect(rows[0].text()).not.toContain('推荐')
    expect(rows[1].text()).toContain('国际路线')
    expect(rows[1].text()).toContain('推荐')
    expect(rows[1].text()).not.toContain('默认')
    for (const row of rows) {
      expect(row.find('button[aria-label="复制端点"]').exists()).toBe(true)
      expect(row.find('button[aria-label$="延迟"]').exists()).toBe(true)
      expect(row.find('[role="status"]').text()).toBe('未测速')
    }
  })

  it('单条测速请求同域 /health 并显示延迟和推荐', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [],
        autoTest: false,
      },
    })

    await wrapper.find('button[aria-label="测试 API 端点 延迟"]').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [input, init] = fetchMock.mock.calls[0]
    const probeUrl = new URL(String(input))
    expect(probeUrl.origin).toBe('https://default.example.com')
    expect(probeUrl.pathname).toBe('/health')
    expect(init).toMatchObject({ method: 'GET', mode: 'cors', redirect: 'error', cache: 'no-store', credentials: 'omit' })
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
        autoTest: false,
        customEndpoints: [
          {
            name: '备用线路',
            endpoint: 'https://backup.example.com/v1',
            description: '',
          },
        ],
      },
    })

    await wrapper.get('button[aria-label="全部测速"]').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('HTTP 503')
    expect(wrapper.text()).toContain('推荐')
  })

  it('站点配置就绪后自动测速一次，并按本地延迟显示颜色等级', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: '',
        customEndpoints: [],
      },
    })

    await flushPromises()
    expect(fetchMock).not.toHaveBeenCalled()

    await wrapper.setProps({
      apiBaseUrl: 'https://api.kittyyyds.xyz',
      customEndpoints: [
        {
          name: '海外线路',
          endpoint: 'https://kittyyyds.xyz',
          description: '需要梯子',
        },
      ],
    })
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('[data-latency-level="fast"]')).toHaveLength(2)
    expect(wrapper.find('button[title="需要梯子"]').exists()).toBe(true)
  })
})
