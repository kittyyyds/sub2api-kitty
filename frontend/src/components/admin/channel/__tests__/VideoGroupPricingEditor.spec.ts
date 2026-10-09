import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import VideoGroupPricingEditor from '../VideoGroupPricingEditor.vue'
import {
  newVideoGroupPricing, validateVideoGroupPricing, videoConfigToAPI,
  videoCustomerPrices, videoPricingFromCatalog,
} from '../videoGroupPricing'
import type { PricingFormEntry } from '../types'
import type { VideoCatalog, VideoRuntimeSettings, VideoSupplierPrice } from '@/api/admin/videoUpstream'

const api = vi.hoisted(() => ({ get: vi.fn(), probe: vi.fn(), sync: vi.fn(), import: vi.fn() }))
vi.mock('@/api/admin/videoUpstream', () => ({ videoUpstreamAPI: api }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const settings: VideoRuntimeSettings = {
  auto_sync: false, follow_increase: true, follow_decrease: false, interval_seconds: 60,
  markup: { output_per_second: 0.1, reference_per_second: 0.03, per_request: 0.2 },
  reference_retention_hours: 24, max_concurrent_tasks: 5, reference_user_mb: 100, reference_total_mb: 1000,
}
const supplier: VideoSupplierPrice = {
  model: 'video-a', billing_mode: 'video', unit_price: 0.25, reference_per_second: 0.05,
  resolution_prices: { '720p': 0.3, '1080p': 0.6 },
  spec: {
    supported: true, min_seconds: 3, max_seconds: 10, resolutions: ['720p', '1080p'], ratios: ['16:9'],
    min_images: 0, max_images: 2, max_videos: 1, max_audios: 0, max_reference_seconds: 10,
    face: false, generate_audio: false, reference_billed: true,
  },
}
const catalog: VideoCatalog = {
  version: '1', last_attempt: '2026-10-09T01:00:00Z', last_success: '2026-10-09T01:00:00Z', last_error: '', changes: [],
  models: {
    'video-a': supplier,
    'no-spec': { ...supplier, model: 'no-spec', spec: undefined },
    'unavailable': { ...supplier, model: 'unavailable', unavailable_reason: 'Unverified billing' },
    'unsupported': { ...supplier, model: 'unsupported', spec: { ...supplier.spec!, supported: false } },
  },
}
const wrappers: ReturnType<typeof mountEditor>[] = []
function mountEditor(modelValue: PricingFormEntry[] = []) {
  const wrapper = mount(VideoGroupPricingEditor, {
    props: {
      modelValue,
      'onUpdate:modelValue': (entries: PricingFormEntry[]) => { void wrapper.setProps({ modelValue: entries }) },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}
function savedEntry(auto_price = true): PricingFormEntry {
  const entry = videoPricingFromCatalog(supplier)!
  delete entry.video!.price_source
  return { ...entry, per_request_price: 0.8, video: { ...entry.video!, auto_price, reference_per_second: 0.12, resolution_prices: { '720p': 0.9, '1080p': 1.1 } } }
}

describe('video group customer prices', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    api.get.mockResolvedValue({ settings, catalog })
    api.probe.mockResolvedValue(catalog)
  })
  afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

  it('edits exact custom IDs and fixed prices without token or tier controls', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('[data-testid="video-pricing-add"]').trigger('click')
    expect(wrapper.findAll('[data-testid="video-pricing-billing"] option').map(item => item.attributes('value'))).toEqual(['per_request', 'video'])
    await wrapper.get('[data-testid="video-pricing-model"]').setValue('特价/官方h3-2k')
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('0.25')
    expect(validateVideoGroupPricing(wrapper.props('modelValue'))).toBeNull()
    await wrapper.get('[data-testid="video-pricing-billing"]').setValue('per_request')
    expect(wrapper.props('modelValue')[0]).toMatchObject({ models: ['特价/官方h3-2k'], billing_mode: 'per_request', per_request_price: '0.25', intervals: [] })
    expect(wrapper.props('modelValue')[0].video).toBeUndefined()
  })

  it('allows empty groups but rejects incomplete rows, invalid prices and duplicate IDs', () => {
    const entry = { ...newVideoGroupPricing(), models: ['Video-A'], per_request_price: 0 }
    expect(validateVideoGroupPricing([])).toBeNull()
    expect(validateVideoGroupPricing([newVideoGroupPricing()])).toBe('videoModule.invalidModel')
    expect(validateVideoGroupPricing([entry])).toBeNull()
    expect(validateVideoGroupPricing([entry, { ...entry, models: ['video-a'] }])).toBeNull()
    expect(validateVideoGroupPricing([entry, entry])).toBe('videoModule.duplicateModel')
    expect(validateVideoGroupPricing([{ ...entry, models: ['Video-*'] }])).toBe('videoModule.invalidModel')
    expect(validateVideoGroupPricing([{ ...entry, models: ['Video-A', 'Video-B'] }])).toBe('videoModule.invalidModel')
    expect(validateVideoGroupPricing([{ ...entry, billing_mode: 'token' }])).toBe('videoModule.invalidBilling')
    for (const price of [null, '', ' ', -1, NaN, Infinity]) {
      expect(validateVideoGroupPricing([{ ...entry, per_request_price: price }])).toBe('videoModule.invalidPrice')
    }
  })

  it('loads the public catalog and global settings, then adds a supported model with a blank manual price before group creation', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    expect(api.get).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="video-global-sync"]').text()).toContain('videoModule.disabled')
    const tableRow = wrapper.findAll('[data-testid="video-catalog-row"]').find(row => row.text().includes('video-a'))!
    expect(tableRow.get('[data-testid="video-catalog-cost"]').text()).toContain('0.25 /')
    expect(tableRow.get('[data-testid="video-catalog-customer"]').text()).toContain('0.35 /')
    await tableRow.get('input').setValue(true)
    await wrapper.get('[data-testid="video-catalog-add"]').trigger('click')
    const entry = wrapper.props('modelValue')[0]
    expect(entry).toMatchObject({ models: ['video-a'], per_request_price: null, video: { price_source: 'upstream', auto_price: true, spec: { supported: true } } })
    expect(entry.video!.markup).toBeUndefined()
    expect(entry.video!.follow_increase).toBeUndefined()
    expect(entry.video!.synced_at).toBeUndefined()
    expect((wrapper.get('[data-testid="video-pricing-price"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.get('[data-testid="video-pricing-price"]').attributes('required')).toBeUndefined()
    expect(validateVideoGroupPricing(wrapper.props('modelValue'))).toBeNull()
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('0.35 /')
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('0.08 /')
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('0.7 /')
    expect(tableRow.get('input').attributes('disabled')).toBeDefined()
    expect(api.import).not.toHaveBeenCalled()
    expect(api.sync).not.toHaveBeenCalled()
  })

  it('blocks catalog selection when the spec or pricing is unavailable', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    for (const id of ['no-spec', 'unavailable', 'unsupported']) {
      expect(wrapper.get(`input[value="${id}"]`).attributes('disabled')).toBeDefined()
      expect(videoPricingFromCatalog(catalog.models[id])).toBeNull()
    }
  })

  it('keeps an entered zero manual and restores upstream periodic repricing when cleared', async () => {
    const wrapper = mountEditor([videoPricingFromCatalog(supplier)!])
    await flushPromises()
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('0')
    let entry = wrapper.props('modelValue')[0]
    expect(entry).toMatchObject({ per_request_price: '0', video: { price_source: 'manual', auto_price: false, reference_per_second: 0.08, resolution_prices: { '720p': 0, '1080p': 0 } } })
    expect(validateVideoGroupPricing([entry])).toBeNull()
    expect(wrapper.get('[data-testid="video-auto-price"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('0 /')
    await wrapper.get('[data-testid="video-reference-price"]').setValue('0')
    await wrapper.get('[data-testid="video-resolution-1080p"]').setValue('0.9')
    expect(wrapper.props('modelValue')[0].video).toMatchObject({ price_source: 'manual', auto_price: false, reference_per_second: 0, resolution_prices: { '1080p': 0.9 } })
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('')
    entry = wrapper.props('modelValue')[0]
    expect(entry.video).toMatchObject({ price_source: 'upstream', auto_price: true })
    expect((wrapper.get('[data-testid="video-pricing-price"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.find('[data-testid="video-reference-price"]').exists()).toBe(false)
    expect(validateVideoGroupPricing([entry])).toBeNull()
    expect(videoConfigToAPI(entry.video)).toMatchObject({ price_source: 'upstream', auto_price: true })
  })

  it.each([true, false])('preserves legacy video prices while mapping auto_price=%s to its source', async auto => {
    const entry = savedEntry(auto)
    const original = structuredClone(entry)
    const wrapper = mountEditor([entry])
    await flushPromises()
    expect(wrapper.props('modelValue')[0]).toEqual(original)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    const input = wrapper.get('[data-testid="video-pricing-price"]').element as HTMLInputElement
    expect(input.value).toBe(auto ? '' : '0.8')
    expect(videoConfigToAPI(entry.video)).toMatchObject({ price_source: auto ? 'upstream' : 'manual', auto_price: auto, reference_per_second: 0.12, resolution_prices: { '720p': 0.9, '1080p': 1.1 } })
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('0.8 /')
  })

  it('keeps upstream mode when its periodic switch is turned off', async () => {
    const wrapper = mountEditor([savedEntry(true)])
    await flushPromises()
    await wrapper.get('[data-testid="video-auto-price"]').setValue(false)
    expect(wrapper.props('modelValue')[0]).toMatchObject({ per_request_price: 0.8, video: { price_source: 'upstream', auto_price: false } })
    expect((wrapper.get('[data-testid="video-pricing-price"]').element as HTMLInputElement).value).toBe('')
  })

  it.each(['catalog', 'saved', 'none'])('discards cleared manual dimensions when returning upstream with %s cost data', async source => {
    const entry = savedEntry(false)
    if (source === 'none') delete entry.video!.cost
    const latest = { ...supplier, reference_per_second: 0.2, resolution_prices: { '720p': 0.7, '1080p': 1.2 } }
    api.get.mockResolvedValue({ settings, catalog: { ...catalog, models: source === 'catalog' ? { 'video-a': latest } : {} } })
    const wrapper = mountEditor([entry])
    await flushPromises()
    await wrapper.get('[data-testid="video-reference-price"]').setValue('')
    await wrapper.get('[data-testid="video-resolution-720p"]').setValue('')
    expect(wrapper.props('modelValue')[0].video!.reference_per_second).toBeNaN()
    expect(wrapper.props('modelValue')[0].video!.resolution_prices!['720p']).toBeNaN()
    expect(validateVideoGroupPricing(wrapper.props('modelValue'))).toBe('videoModule.invalidPrice')

    await wrapper.get('[data-testid="video-pricing-price"]').setValue('')
    const restored = wrapper.props('modelValue')[0]
    expect(restored).toMatchObject({ per_request_price: 0.8, video: { price_source: 'upstream', auto_price: true } })
    expect(wrapper.find('[data-testid="video-reference-price"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="video-resolution-720p"]').exists()).toBe(false)
    expect(validateVideoGroupPricing([restored])).toBeNull()
    const expectedCost = source === 'catalog' ? latest : source === 'saved' ? supplier : undefined
    expect(restored.video!.reference_per_second).toBe(expectedCost?.reference_per_second ?? 0)
    expect(restored.video!.resolution_prices).toEqual(expectedCost?.resolution_prices ?? {})
    expect(restored.video!.cost).toEqual(entry.video!.cost)
    expect(videoConfigToAPI(restored.video)!.reference_per_second).not.toBeNull()
  })

  it('clears an invalid manual base price cache when returning upstream', async () => {
    const wrapper = mountEditor([savedEntry(false)])
    await flushPromises()
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('-1')
    expect(validateVideoGroupPricing(wrapper.props('modelValue'))).toBe('videoModule.invalidPrice')
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('')
    expect(wrapper.props('modelValue')[0]).toMatchObject({ per_request_price: null, video: { price_source: 'upstream', auto_price: true } })
    expect(validateVideoGroupPricing(wrapper.props('modelValue'))).toBeNull()
  })

  it('does not convert a legacy fixed price when its model matches the catalog', async () => {
    const wrapper = mountEditor([{ ...newVideoGroupPricing(), models: [supplier.model], per_request_price: 0.4 }])
    await flushPromises()
    await wrapper.get('[data-testid="video-pricing-probe"]').trigger('click')
    await flushPromises()
    expect(wrapper.props('modelValue')[0].video).toBeUndefined()
    expect(wrapper.find('[data-testid="video-auto-price"]').exists()).toBe(false)
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('0')
    expect(wrapper.props('modelValue')[0]).toMatchObject({ per_request_price: '0' })
    expect(wrapper.props('modelValue')[0].video).toBeUndefined()
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('')
    expect(validateVideoGroupPricing(wrapper.props('modelValue'))).toBe('videoModule.invalidPrice')
  })

  it('inherits global row rules and restores inheritance after an override', async () => {
    const wrapper = mountEditor([videoPricingFromCatalog(supplier)!])
    await flushPromises()
    await wrapper.get('[data-testid="video-markup-override"]').setValue(true)
    expect(wrapper.props('modelValue')[0].video!.markup).toEqual(settings.markup)
    await wrapper.get('[data-testid="video-markup-output_per_second"]').setValue('0.5')
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('0.75 /')
    await wrapper.get('[data-testid="video-follow-increase"]').setValue('false')
    expect(wrapper.props('modelValue')[0].video!.follow_increase).toBe(false)
    await wrapper.get('[data-testid="video-follow-increase"]').setValue('')
    await wrapper.get('[data-testid="video-markup-override"]').setValue(false)
    expect(wrapper.props('modelValue')[0].video!.markup).toBeUndefined()
    expect(wrapper.props('modelValue')[0].video!.follow_increase).toBeUndefined()
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('0.35 /')
  })

  it('refreshes only the catalog on probe without repricing or replacing saved and unsaved rows', async () => {
    const entries = [savedEntry(), newVideoGroupPricing()]
    const wrapper = mountEditor(entries)
    await flushPromises()
    api.probe.mockResolvedValue({ ...catalog, models: { 'video-a': { ...supplier, unit_price: 2 } } })
    await wrapper.get('[data-testid="video-pricing-probe"]').trigger('click')
    await flushPromises()
    expect(api.probe).toHaveBeenCalledOnce()
    expect(api.sync).not.toHaveBeenCalled()
    expect(wrapper.props('modelValue')).toEqual(entries)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.findAll('[data-testid="video-pricing-row"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="video-row-cost"]')[0].text()).toContain('2 /')
    expect(wrapper.findAll('[data-testid="video-row-customer"]')[0].text()).toContain('0.8 /')
  })

  it('preserves prices, catalog and incomplete rows when reading prices fails', async () => {
    const entries = [savedEntry(false), newVideoGroupPricing()]
    const wrapper = mountEditor(entries)
    await flushPromises()
    api.probe.mockRejectedValue(new Error('offline'))
    await wrapper.get('[data-testid="video-pricing-probe"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('videoModule.probeFailed')
    expect(wrapper.props('modelValue')).toEqual(entries)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.findAll('[data-testid="video-pricing-row"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="video-catalog-row"]')).toHaveLength(4)
    expect(api.sync).not.toHaveBeenCalled()
  })

  it('retains legacy rows and lets the group remain editable if initial catalog loading fails', async () => {
    api.get.mockRejectedValue(new Error('offline'))
    const wrapper = mountEditor([savedEntry(false)])
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('videoModule.catalogLoadFailed')
    expect(wrapper.findAll('[data-testid="video-pricing-row"]')).toHaveLength(1)
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('1')
    expect(validateVideoGroupPricing(wrapper.props('modelValue'))).toBeNull()
  })

  it('validates all manual dimensions and row markups', () => {
    const entry = savedEntry(false)
    entry.video!.reference_per_second = NaN
    expect(validateVideoGroupPricing([entry])).toBe('videoModule.invalidPrice')
    entry.video!.reference_per_second = 0
    entry.video!.resolution_prices = { '720p': -1 }
    expect(validateVideoGroupPricing([entry])).toBe('videoModule.invalidPrice')
    entry.video!.resolution_prices = { '720p': 0 }
    entry.video!.markup = { ...settings.markup, output_per_second: -1 }
    expect(validateVideoGroupPricing([entry])).toBe('videoModule.invalidMarkup')
  })

  it('uses per-video markup without reference markup for request billing', () => {
    const entry = videoPricingFromCatalog({ ...supplier, billing_mode: 'per_request', reference_per_second: 0, resolution_prices: {} })!
    expect(videoCustomerPrices(entry, settings)).toMatchObject({ unit: 0.45, reference: 0, preview: true })
  })

  it('applies a uniform manual price to the official 720p tier before allowing a separate override', async () => {
    const official = { ...supplier, model: 'official-720p', unit_price: 0.68, resolution_prices: { '720p': 0.68 }, spec: { ...supplier.spec!, resolutions: ['720p'] } }
    api.get.mockResolvedValue({ settings, catalog: { ...catalog, models: { 'official-720p': official } } })
    const wrapper = mountEditor([videoPricingFromCatalog(official)!])
    await flushPromises()
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('1.0')
    let effective = videoCustomerPrices(wrapper.props('modelValue')[0], settings)
    expect(effective).toMatchObject({ unit: 1, reference: 0.08, resolutions: { '720p': 1 } })
    expect(wrapper.props('modelValue')[0].video).toMatchObject({ price_source: 'manual', auto_price: false, resolution_prices: { '720p': 1 } })
    await wrapper.get('[data-testid="video-resolution-720p"]').setValue('1.2')
    effective = videoCustomerPrices(wrapper.props('modelValue')[0], settings)
    expect(effective).toMatchObject({ unit: 1, reference: 0.08, resolutions: { '720p': 1.2 } })
    await wrapper.get('[data-testid="video-reference-price"]').setValue('0.4')
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('0')
    expect(videoCustomerPrices(wrapper.props('modelValue')[0], settings)).toMatchObject({ unit: 0, reference: 0.4, resolutions: { '720p': 0 } })
  })

  it('shows raw costs without asserting a currency and dates the catalog cost separately from saved pricing', async () => {
    const entry = savedEntry()
    entry.video!.synced_at = '2026-10-08T00:00:00Z'
    const wrapper = mountEditor([entry])
    await flushPromises()
    expect(wrapper.text()).not.toMatch(/USD|CNY|¥|￥/)
    expect(wrapper.get('[data-testid="video-row-cost-date"] time').attributes('datetime')).toBe(catalog.last_success)
    const latest = '2026-10-09T03:00:00Z'
    api.probe.mockResolvedValue({ ...catalog, last_success: latest, models: { 'video-a': { ...supplier, unit_price: 2 } } })
    await wrapper.get('[data-testid="video-pricing-probe"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="video-row-cost-date"] time').attributes('datetime')).toBe(latest)
    expect(wrapper.get('[data-testid="video-row-cost"]').text()).toContain('2 /')
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('0.8 /')
    expect(wrapper.props('modelValue')[0].video!.synced_at).toBe('2026-10-08T00:00:00Z')
  })

  it('labels the saved cost date when the model is absent from the catalog', async () => {
    const entry = savedEntry()
    entry.video!.synced_at = '2026-10-08T00:00:00Z'
    api.get.mockResolvedValue({ settings, catalog: { ...catalog, models: {} } })
    const wrapper = mountEditor([entry])
    await flushPromises()
    const costDate = wrapper.get('[data-testid="video-row-cost-date"]')
    expect(costDate.text()).toContain('videoModule.savedCostRead')
    expect(costDate.get('time').attributes('datetime')).toBe(entry.video!.synced_at)
  })

  it('refreshes unsaved upstream previews from the new catalog without changing form data', async () => {
    const entry = videoPricingFromCatalog(supplier)!
    const wrapper = mountEditor([entry])
    await flushPromises()
    api.probe.mockResolvedValue({ ...catalog, models: { 'video-a': { ...supplier, unit_price: 1, reference_per_second: 0.2, resolution_prices: { '720p': 1, '1080p': 1.5 } } } })
    await wrapper.get('[data-testid="video-pricing-probe"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('1.1 /')
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('0.23 /')
    expect(wrapper.props('modelValue')).toEqual([entry])
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('2')
    expect(wrapper.props('modelValue')[0].video!.reference_per_second).toBe(0.23)
  })

  it('shows probe diagnostics and failed-attempt metadata without falsely advancing the cost date', async () => {
    const wrapper = mountEditor([savedEntry()])
    await flushPromises()
    api.probe.mockRejectedValue({ status: 502, message: '上游默认分组倍率已变化，需要核实成本' })
    api.get.mockResolvedValue({ settings, catalog: { ...catalog, last_attempt: '2026-10-09T05:00:00Z', last_error: '上游默认分组倍率已变化，需要核实成本' } })
    await wrapper.get('[data-testid="video-pricing-probe"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="video-pricing-error-detail"]').text()).toBe('上游默认分组倍率已变化，需要核实成本')
    expect(wrapper.get('[data-testid="video-row-cost-date"] time').attributes('datetime')).toBe(catalog.last_success)
    expect(wrapper.text()).not.toContain('videoModule.probeSuccess')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('keeps the probe error if reading failure metadata also fails', async () => {
    const wrapper = mountEditor([savedEntry()])
    await flushPromises()
    api.probe.mockRejectedValue({ message: '目录存储失败' })
    api.get.mockRejectedValue(new Error('offline'))
    await wrapper.get('[data-testid="video-pricing-probe"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="video-pricing-error-detail"]').text()).toBe('目录存储失败')
    expect(wrapper.findAll('[data-testid="video-pricing-row"]')).toHaveLength(1)
  })

  it('does not claim a fully successful read when the returned catalog carries an error', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    api.probe.mockResolvedValue({ ...catalog, last_error: '部分型号缺价' })
    await wrapper.get('[data-testid="video-pricing-probe"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('部分型号缺价')
    expect(wrapper.text()).not.toContain('videoModule.probeSuccess')
  })

  it.each([true, undefined])('preserves reference markup at zero cost only when the price is known (%s)', known => {
    const price = { ...supplier, reference_per_second: 0, reference_price_known: known, spec: { ...supplier.spec!, reference_billed: false } }
    const entry = videoPricingFromCatalog(price)!
    expect(videoCustomerPrices(entry, settings).reference).toBe(known ? 0.03 : 0)
    expect(entry.video!.cost!.reference_price_known).toBe(known)
    const legacyBilled = videoPricingFromCatalog({ ...price, spec: { ...price.spec, reference_billed: true } })!
    expect(videoCustomerPrices(legacyBilled, settings).reference).toBe(0.03)
  })

  it('retains reference markup when a known reference rate drops from positive to zero on probe', async () => {
    const price = { ...supplier, reference_price_known: true, spec: { ...supplier.spec!, reference_billed: false } }
    api.get.mockResolvedValue({ settings, catalog: { ...catalog, models: { 'video-a': price } } })
    const wrapper = mountEditor([videoPricingFromCatalog(price)!])
    await flushPromises()
    api.probe.mockResolvedValue({ ...catalog, models: { 'video-a': { ...price, reference_per_second: 0 } } })
    await wrapper.get('[data-testid="video-pricing-probe"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="video-row-customer"]').text()).toContain('0.03 /')
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('1')
    expect(wrapper.props('modelValue')[0].video!.reference_per_second).toBe(0.03)
  })
})
