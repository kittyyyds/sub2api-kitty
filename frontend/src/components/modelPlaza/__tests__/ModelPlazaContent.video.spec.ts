import { shallowRef } from 'vue'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import zh from '@/i18n/locales/zh/dashboard'
import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'

const { copyToClipboard } = vi.hoisted(() => ({ copyToClipboard: vi.fn() }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: true }) }))
vi.mock('@/composables/useDocumentDarkMode', () => ({ useDocumentDarkMode: () => ({ isDarkMode: shallowRef(false) }) }))

function model(name = '特价/models/新型号-4K', mode: 'video' | 'per_request' = 'video', price = 0.78): PlazaModel {
  return {
    name, platform: 'video', description: '支持图片、视频和音频参考；音频须同时提供视觉素材。',
    capabilities: {
      media_type: 'video', supported: true, seconds: { min: 4, max: 15 }, second_options: [5, 10, 15],
      resolutions: ['720p', '1080p', '4k'], aspect_ratios: ['16:9', '9:16'],
      reference_images: { min: 0, max: 9 }, reference_videos: { min: 0, max: 3 }, reference_audios: { min: 0, max: 3 },
      max_reference_seconds: 15, max_combined_seconds: 20, allow_audio_only: false, requires_visual_or_audio: false,
    },
    pricing: {
      billing_mode: mode, per_request_price: price, final_unit_price: true,
      input_price: null, output_price: null, cache_read_price: null, cache_write_price: null,
      image_input_price: null, image_output_price: null, intervals: [],
    },
    official_pricing: null,
  }
}

function group(id: number, models: PlazaModel[] = [], platform = 'video'): ModelPlazaGroup {
  return {
    id, name: platform === 'video' ? `视频分组${id}` : 'Grok 视频', description: '客户可见分组说明', platform,
    subscription_type: 'standard', rate_multiplier: 9, user_rate_multiplier: 5,
    image_rate_independent: false, image_rate_multiplier: 1,
    video_rate_independent: true, video_rate_multiplier: 11,
    peak_rate_enabled: true, peak_start: '00:00', peak_end: '23:59', peak_rate_multiplier: 20,
    is_exclusive: false, long_context_pricing_enabled: false, models,
  }
}

function mountContent(groups: ModelPlazaGroup[]) {
  return mount(ModelPlazaContent, {
    props: { response: { description: '', groups }, loading: false },
    global: {
      plugins: [createI18n({ legacy: false, locale: 'zh', missingWarn: false, fallbackWarn: false,
        messages: { zh: { ...zh, nav: { billingRules: '计费规则' }, videoModule: { finalPrice: '最终单价' } } } })],
      stubs: { Icon: true, ModelIcon: true },
    },
  })
}

describe('customer video model price catalog', () => {
  beforeEach(() => copyToClipboard.mockReset())

  it('keeps Grok and dedicated video groups in the existing video tab, including empty video groups', async () => {
    const grok = { ...model('grok-imagine-video-1.5'), platform: 'grok', capabilities: undefined, description: undefined }
    grok.pricing!.final_unit_price = undefined
    const selected = model()
    selected.pricing!.resolution_prices = { '720p': 0.78, '4k': 5.55 }
    selected.pricing!.reference_per_second = 0.13
    const wrapper = mountContent([group(1, [grok], 'grok'), group(25), group(26, [selected])])
    expect(wrapper.findAll('.product-tabs button').map(button => button.text())).toEqual(['视频生成'])
    expect(wrapper.findAll('.pricing-group-card')).toHaveLength(3)
    // Grok keeps its independent media multiplier and its own row.
    expect(wrapper.get('.price-primary strong').text()).toBe('$8.58')
    expect(wrapper.get('tbody').text()).toContain('grok-imagine-video-1.5')
    await wrapper.findAll('.pricing-group-card')[1].trigger('click')
    expect(wrapper.get('.state-panel').text()).toBe('该视频分组暂未配置模型售价，请联系管理员配置后使用。')
    expect(wrapper.find('tbody').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('$0')
    await wrapper.findAll('.pricing-group-card')[2].trigger('click')
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(3)
    expect(rows[0].get('.price-primary').text()).toBe('$0.78 / 秒')
    expect(rows[1].get('[data-testid="video-price-unconfigured"]').text()).toBe('未配置售价')
    expect(rows[1].find('.price-primary').exists()).toBe(false)
    expect(rows[2].get('.price-primary').text()).toBe('$5.55 / 秒')
    expect(rows[0].get('.video-reference-price').text()).toBe('参考视频：$0.13 / 秒')
    expect(rows[0].get('.video-model-description').text()).toBe(selected.description)
    expect(rows[0].get('details').text()).toContain('5 / 10 / 15 秒')
    expect(rows[0].get('details').text()).toContain('16:9 / 9:16')
    expect(wrapper.get('.pricing-group-card.active').text()).toContain('分组售价')
    expect(wrapper.get('.pricing-group-card.active').text()).not.toContain('倍率')
    await rows[0].get('.copy-id-button').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith(selected.name, '模型 ID 已复制')
    await wrapper.findAll('.pricing-group-card')[0].trigger('click')
    expect(wrapper.get('.price-primary strong').text()).toBe('$8.58')
    wrapper.unmount()
  })

  it('uses final per-video and per-second selling prices without multipliers, preserving explicit reference zero and missing reference data', () => {
    const perVideo = model('整条计费', 'per_request', 9)
    perVideo.pricing!.reference_per_second = 0
    perVideo.time_pricing = { timezone: 'UTC', periods: [{ start_time: '00:00', end_time: '23:59', multiplier: 100 }] }
    const perSecond = model('每秒计费', 'video', 0.012345)
    const wrapper = mountContent([group(25, [perVideo, perSecond])])
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0].get('.price-primary').text()).toBe('$9.00 / 条')
    expect(rows[0].get('.video-reference-price').text()).toBe('参考视频无需另收费用')
    expect(rows[1].get('.price-primary').text()).toBe('$0.012345 / 秒')
    expect(rows[1].find('.video-reference-price').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('高峰时段')
    expect(wrapper.find('.period-label').exists()).toBe(false)
    wrapper.unmount()
  })

  it('does not infer a selling price from missing pricing, invalid tiers or official reference prices', () => {
    const missing = model('尚未定价')
    missing.pricing = null
    missing.official_pricing = { input_price: 9, output_price: 19, cache_write_price: null, cache_read_price: null }
    const partial = model('部分定价')
    partial.capabilities = undefined
    partial.pricing!.resolution_prices = { '720p': null, '1080p': -1, '4k': 0 }
    const wrapper = mountContent([group(25, [missing, partial])])
    expect(wrapper.findAll('tbody tr')).toHaveLength(4)
    expect(wrapper.findAll('[data-testid="video-price-unconfigured"]')).toHaveLength(3)
    expect(wrapper.findAll('.price-primary').map(cell => cell.text())).toEqual(['$0.00 / 秒'])
    expect(wrapper.text()).not.toContain('$0.78')
    expect(wrapper.text()).not.toContain('$9')
    expect(wrapper.text()).not.toContain('1M tokens')
    wrapper.unmount()
  })

  it.each([
    { max: 0, reference: 0 },
    { max: 0, reference: 0.13 },
    { max: undefined, reference: 0 },
    { max: undefined, reference: 0.13 },
  ])('hides reference fees without known reference-video support (max=$max, price=$reference)', ({ max, reference }) => {
    const video = model('4K-H3')
    video.pricing!.reference_per_second = reference
    if (max == null) video.capabilities = undefined
    else video.capabilities!.reference_videos = { min: 0, max }
    const wrapper = mountContent([group(25, [video])])
    expect(wrapper.get('.price-primary').text()).toBe('$0.78 / 秒')
    expect(wrapper.find('.video-reference-price').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('参考视频无需另收费用')
    expect(wrapper.text()).not.toContain('参考视频：$')
    wrapper.unmount()
  })

  it('renders only returned group models and public fields, and handles a group gaining or losing its selected prices', async () => {
    const wrapper = mountContent([group(25)])
    expect(wrapper.get('.product-tabs').text()).toBe('视频生成')
    expect(wrapper.get('.state-panel').text()).toContain('暂未配置模型售价')
    const selected = model('唯一已选型号')
    selected.description = '<img src=x onerror="alert(1)"> 公开模型描述'
    const response: ModelPlazaResponse = { description: '', groups: [group(25, [selected])] }
    Object.assign(selected.pricing!, {
      cost: { unit_price: 0.12345678 }, markup: { output_per_second: 0.11122233 },
      upstream_url: 'https://private-upstream.invalid',
    })
    Object.assign(response, { supplier_catalog: { models: { '未选型号': { unit_price: 0.12345678 } } } })
    const before = JSON.stringify(response)
    await wrapper.setProps({ response })
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.get('.model-id-cell code').text()).toBe('唯一已选型号')
    expect(wrapper.get('.video-model-description').text()).toBe(selected.description)
    expect(wrapper.find('.video-model-description img').exists()).toBe(false)
    expect(wrapper.get('.price-primary').text()).toBe('$0.78 / 秒')
    expect(wrapper.html()).not.toMatch(/private-upstream|未选型号|0\.12345678|0\.11122233|markup/)
    expect(JSON.stringify(response)).toBe(before)
    await wrapper.findAll('.price-mode-switch button')[1].trigger('click')
    expect(wrapper.text()).toContain('暂无官方参考价')
    expect(wrapper.text()).not.toContain('$0.78')
    await wrapper.setProps({ response: { description: '', groups: [group(25)] } })
    expect(wrapper.get('.state-panel').text()).toContain('暂未配置模型售价')
    expect(wrapper.find('tbody').exists()).toBe(false)
    wrapper.unmount()
  })
})
