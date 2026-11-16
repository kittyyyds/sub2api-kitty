import { beforeAll, describe, expect, it } from 'vitest'
import { config, mount } from '@vue/test-utils'
import MediaPricingCatalog from '../MediaPricingCatalog.vue'
import { mediaEntries, mediaPriceSummary, estimateMediaPrice } from '../mediaPricing'
import { type MediaCatalogPolicy, type MediaCatalogModel } from '../mediaCatalogPolicy'
import { fixture } from './mediaPricing.fixture'

beforeAll(() => {
  config.global.stubs = { ...config.global.stubs, teleport: true }
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
})

function policy(overrides: Partial<MediaCatalogModel> = {}): MediaCatalogPolicy {
  return { sourceUrl: '', title: '价格表', capturedOn: '2026-10-31', categories: [{ id: 'configured', label: '其他已接入模型' }], models: [{
    id: '渠道/原始模型-ID', kind: 'video', categoryId: 'configured', sourceRows: [], description: '本站已接入型号。',
    billingUnits: ['秒'], pricingNotes: [], warnings: [], conditionalReferencePricing: false,
    referenceRule: 'unspecified', ...overrides,
  }] }
}

describe('effective prices with refreshed source annotations', () => {
  it.each([
    { id: '经典系列/MiniMax-H3-GO', mode: 'video' as const, source: ['条'], unit: '秒' },
    { id: '002系列/sd2.0(933满血不卡脸1080P)', mode: 'video' as const, source: ['秒', '条'], unit: '秒' },
    { id: '004系列/sd2.5(优质/30-10-10音/5-15秒/720P)', mode: 'per_request' as const, source: ['秒'], unit: '条' },
  ])('shows the backend final price for a reviewed source-unit conflict: $id', async ({ id, mode, source, unit }) => {
    const data = fixture()
    data.groups[0].models[0].name = id
    data.groups[0].models[0].pricing!.billing_mode = mode
    const p = policy({ id, sourceUnitConflict: true, billingUnits: source, sourceRows: [93] })
    const entry = mediaEntries(data, p)[0]
    expect(entry.status).toBe('priced')
    expect(entry.catalogWarnings?.length).toBe(1)
    expect(entry.catalogPriceVerified).toBe(true)
    expect(entry.tiers[0].price).toBe(0.3)
    expect(mediaPriceSummary(entry)).toBe(`$0.3/${unit}`)
    expect(entry.estimateBlocked).toBe(true)
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 10, 0)).toBeNull()
    const w = mount(MediaPricingCatalog, { props: { response: data, catalogPolicy: p, initialKind: 'video' } })
    expect(w.get('.series-prices').text()).toContain('$0.3')
    expect(w.get('.series-prices').text()).toContain(unit)
    expect(w.get('.card-warning').text()).toContain('实际售价')
    await w.get('.table-actions .details-button').trigger('click')
    expect(w.find('.estimate-panel').exists()).toBe(false)
    w.unmount()
  })

  it('still blocks missing final-price evidence, real pricing conflicts, and classification conflicts', () => {
    const p = policy({ sourceUnitConflict: true, billingUnits: ['条'] })
    const missingFinal = fixture()
    delete missingFinal.groups[0].models[0].pricing!.final_unit_price
    const first = mediaEntries(missingFinal, p)[0]
    expect(first.catalogPriceVerified).toBe(false)
    expect(first.tiers.every(t => t.price === null)).toBe(true)
    const conflict = fixture(), model = conflict.groups[0].models[0]
    conflict.groups[0].models.push({ ...model, pricing: { ...model.pricing!, per_request_price: 9 } })
    const second = mediaEntries(conflict, p)[0]
    expect(second.pricingConflict).toBe(true)
    expect(second.catalogPriceVerified).toBe(false)
    expect(second.tiers.every(t => t.price === null)).toBe(true)
    const third = mediaEntries(fixture(), policy({ kind: 'image', sourceUnitConflict: true }))[0]
    expect(third.catalogPriceVerified).toBe(false)
    expect(third.tiers.every(t => t.price === null)).toBe(true)
  })

  it('shows per-request resolution prices without multiplying by seconds or group rates', () => {
    const data = fixture(), model = data.groups[0].models[0]
    model.pricing!.billing_mode = 'per_request'
    model.pricing!.resolution_prices = { '480p': 6.35, '720p': 8.35 }
    const entry = mediaEntries(data, policy({ billingUnits: ['条'], pricedResolutions: ['480p', '720p'] }))[0]
    expect(entry.unit).toBe('条')
    expect(entry.tiers.map(t => t.price)).toEqual([6.35, 8.35])
    expect(mediaPriceSummary(entry)).toBe('$6.35/条 起')
    expect(estimateMediaPrice(entry, entry.tiers[1].key, 1, 10, 0)?.total).toBe(8.35)
  })

  it('shows a neutral source for configured models without inventing Feishu rows', async () => {
    const w = mount(MediaPricingCatalog, { props: { response: fixture(), catalogPolicy: policy(), initialKind: 'video' } })
    await w.get('.table-actions .details-button').trigger('click')
    expect(w.get('.document-rules').text()).toContain('本站已接入型号')
    expect(w.get('.document-rules').text()).not.toContain('第')
    expect(w.get('.tier-table').text()).toContain('$0.3')
    w.unmount()
  })

  it('displays canonical 2k/4k selling tiers and uses the same quoted duration', () => {
    const data = fixture(), model = data.groups[0].models[0]
    model.pricing!.resolution_prices = { '720p': 0.35, '1080p': 0.45, '2k': 0.6, '4k': 1.1 }
    const entry = mediaEntries(data, policy({ pricedResolutions: ['720p', '1080p', '2k', '4k'] }))[0]
    expect(entry.tiers.map(t => t.price)).toEqual([0.35, 0.45, 0.6, 1.1])
    expect(estimateMediaPrice(entry, entry.tiers[3].key, 1, 10, 0)?.total).toBe(11)
  })
})
