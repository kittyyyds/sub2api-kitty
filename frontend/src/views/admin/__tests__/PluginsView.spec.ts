import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import PluginsView from '../PluginsView.vue'

const {
  listPlugins,
  uploadPlugin,
  enablePlugin,
  savePluginConfig,
  createUISession,
  stepUpRun,
  listGroups,
  groupModels,
} = vi.hoisted(() => ({
  listPlugins: vi.fn(),
  uploadPlugin: vi.fn(),
  enablePlugin: vi.fn(),
  savePluginConfig: vi.fn(),
  createUISession: vi.fn(),
  stepUpRun: vi.fn((action: () => Promise<unknown>) => action()),
  listGroups: vi.fn(),
  groupModels: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: { getAllIncludingInactive: listGroups, getAvailableModels: groupModels },
    plugins: {
      list: listPlugins,
      upload: uploadPlugin,
      enable: enablePlugin,
      disable: vi.fn(),
      remove: vi.fn(),
      getConfig: vi.fn().mockResolvedValue({}),
      saveConfig: savePluginConfig,
      test: vi.fn().mockResolvedValue({ success: true, message: 'ok', latency_ms: 1 }),
      createUISession,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: stepUpRun }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

const plugin = {
  id: 7,
  plugin_key: 'local.test.transport',
  name: 'Test Transport',
  version: '1.0.0',
  description: '',
  author: 'test',
  manifest: {
    schema_version: 1,
    id: 'local.test.transport',
    name: 'Test Transport',
    version: '1.0.0',
    requires: {
      sub2api: '>=0.1.0',
      plugin_protocol: 1,
      transport_api: 1,
      ui_bridge: 1,
    },
    capabilities: [],
    ui: { entrypoint: 'ui/index.html' },
  },
  binary_sha256: 'a'.repeat(64),
  signature_status: 'trusted' as const,
  state: 'disabled' as const,
  last_error: '',
  installed_at: '2026-08-22T00:00:00Z',
  updated_at: '2026-08-22T00:00:00Z',
  bindings: [
    {
      id: 1,
      plugin_id: 7,
      capability: 'openai.oauth.outbound_transport.v1',
      platform: 'openai',
      account_type: 'oauth',
      enabled: false,
      rollout_percent: 100,
    },
  ],
  compatibility: {
    compatible: true,
    tested: true,
    status: 'compatible' as const,
    message: '',
    current_sub2api_version: '0.1.0',
    required_sub2api_version: '>=0.1.0',
    recommended_sub2api_version: '0.1.0',
    plugin_protocol: 1,
    transport_api: 1,
    ui_bridge: 1,
  },
  runtime_healthy: false,
  runtime_message: '',
}

function mountView() {
  return mount(PluginsView, {
    attachTo: document.body,
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { template: '<div><slot /></div>' },
        Icon: true,
        TotpStepUpDialog: true,
      },
    },
  })
}

describe('管理员插件页二次验证', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stepUpRun.mockImplementation((action: () => Promise<unknown>) => action())
    listPlugins.mockResolvedValue([plugin])
    uploadPlugin.mockResolvedValue(plugin)
    enablePlugin.mockResolvedValue(plugin)
    savePluginConfig.mockResolvedValue({ enabled: true })
    createUISession.mockResolvedValue({
      url: '/api/v1/plugin-ui/token/index.html#bridge_token=bridge',
      bridge_token: 'bridge',
      ui_bridge_version: 1,
      expires_at: '2026-08-22T01:00:00Z',
    })
  })

  it('启用插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.plugins.enable'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(enablePlugin).toHaveBeenCalledWith(7, 100, false)
  })

  it('上传插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['plugin'], 'transport.s2plugin', { type: 'application/zip' })],
    })

    await input.trigger('change')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(uploadPlugin).toHaveBeenCalledTimes(1)
  })

  it('分组桥接校验窗口与令牌并只返回必要字段', async () => {
    listPlugins.mockResolvedValue([{...plugin, manifest:{...plugin.manifest, capabilities:[{id:'gateway.request_transform.v1',platform:'gateway',account_type:'any'}]}}])
    listGroups.mockResolvedValue([{id:12,name:'测试分组',status:'active',billing_secret:'must-not-leak'}])
    groupModels.mockResolvedValue(['client-model'])
    const wrapper=mountView(); await flushPromises()
    const button=wrapper.findAll('button').find(item=>item.text().includes('admin.plugins.configure'))
    expect(button).toBeDefined(); await button!.trigger('click'); await flushPromises()
    const frame=wrapper.get('iframe').element as HTMLIFrameElement
    const reply=vi.spyOn(frame.contentWindow!, 'postMessage')
    const send=async(type:string,id:string,token='bridge',source:Window|null=frame.contentWindow)=>{
      window.dispatchEvent(new MessageEvent('message',{source,origin:'null',data:{source:'sub2api-plugin-ui',bridge_token:token,type,request_id:id,group_id:12}}));await flushPromises()
    }
    await send('groups.list','bad-token','wrong');await send('groups.list','wrong-frame','bridge',window)
    expect(listGroups).not.toHaveBeenCalled()
    await send('groups.list','list')
    expect(reply).toHaveBeenCalledWith(expect.objectContaining({ok:true,groups:[{id:12,name:'测试分组',status:'active'}]}),'*')
    await send('groups.models','models')
    expect(groupModels).toHaveBeenCalledWith(12)
    expect(reply).toHaveBeenCalledWith(expect.objectContaining({ok:true,models:['client-model']}),'*')
    wrapper.unmount()
  })

  it('未声明改写能力的插件不能读取分组', async () => {
    const wrapper=mountView();await flushPromises()
    await wrapper.findAll('button').find(item=>item.text().includes('admin.plugins.configure'))!.trigger('click');await flushPromises()
    const frame=wrapper.get('iframe').element as HTMLIFrameElement
    const reply=vi.spyOn(frame.contentWindow!,'postMessage')
    window.dispatchEvent(new MessageEvent('message',{source:frame.contentWindow,origin:'null',data:{source:'sub2api-plugin-ui',bridge_token:'bridge',type:'groups.list',request_id:'denied'}}));await flushPromises()
    expect(listGroups).not.toHaveBeenCalled();expect(reply).toHaveBeenCalledWith(expect.objectContaining({ok:false}),'*')
    wrapper.unmount()
  })
})
