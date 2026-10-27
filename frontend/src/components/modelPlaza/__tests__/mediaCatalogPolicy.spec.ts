import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { fixture } from './mediaPricing.fixture'
import { mediaEntries, mediaMatches, mediaPriceSummary, estimateMediaPrice } from '../mediaPricing'
import { suanliMediaCatalog, type MediaCatalogPolicy, type MediaCatalogModel } from '../mediaCatalogPolicy'
import MediaPricingCatalog from '../MediaPricingCatalog.vue'

function policy(overrides: Partial<MediaCatalogModel> = {}): MediaCatalogPolicy {
  return { sourceUrl: 'https://example.invalid/source', title: '指定价表', capturedOn: '2026-10-25', models: [{
    id: '渠道/原始模型-ID', kind: 'video', sourceRows: [13], description: '参考图 9 张，4–15 秒',
    billingUnits: ['秒'], pricingNotes: ['按生成视频时长计费。'], warnings: [],
    conditionalReferencePricing: false, referenceRule: 'unspecified', ...overrides,
  }] }
}

describe('document scoped media presentation', () => {
  it('uses 193 verified IDs, merging duplicate rows without choosing a cheaper price', () => {
    expect(suanliMediaCatalog.models).toHaveLength(193)
    expect(new Set(suanliMediaCatalog.models.map(m => m.id)).size).toBe(193)
    const duplicate = suanliMediaCatalog.models.find(m => m.sourceRows.includes(209))!
    expect(duplicate.sourceRows).toEqual([209, 219])
    expect(duplicate.warnings.length).toBeGreaterThan(0)
  })
  it('does not ship upstream cost amounts in the public editorial policy', () => {
    expect(JSON.stringify(suanliMediaCatalog)).not.toMatch(/\d+(?:\.\d+)?\s*(?:元)?\s*[/]\s*(?:秒|条|张图?|次)/)
  })
  it('filters unlisted models and never invents entries absent from the site response', () => {
    const data = fixture()
    data.groups[0].models.push({ ...data.groups[0].models[0], name: '表外隐藏模型' })
    const p = policy(); p.models.push({ ...p.models[0], id: '文档有但未接入' })
    expect(mediaEntries(data, p).map(e => e.id)).toEqual(['渠道/原始模型-ID'])
    expect(mediaEntries(null, p)).toHaveLength(0)
  })
  it('does not fuzzy-match spelling or casing', () => {
    expect(mediaEntries(fixture(), policy({ id: '渠道/原始模型-id' }))).toHaveLength(0)
  })
  it('takes descriptions from the document but keeps final customer prices unchanged', () => {
    const entry = mediaEntries(fixture(), policy())[0]
    expect(entry.model.description).toBe('参考图 9 张，4–15 秒')
    expect(entry.tiers[0].price).toBe(0.3)
    expect(mediaPriceSummary(entry)).toBe('$0.3/秒')
    expect(mediaMatches(entry, '参考图')).toBe(true)
  })
  it('preserves different group identities', () => {
    const data = fixture(); data.groups.push({ ...data.groups[0], id: 500 })
    expect(mediaEntries(data, policy())).toHaveLength(2)
  })
  it.each([
    { warnings: ['价表价格栏与说明栏冲突'] },
    { billingUnits: ['条'] },
    { kind: 'image' as const },
    { referenceRule: 'free' as const },
  ])('blocks unreliable prices and estimates: %s', overrides => {
    const entry = mediaEntries(fixture(), policy(overrides))[0]
    expect(entry.catalogWarnings?.length).toBeGreaterThan(0)
    expect(entry.tiers[0].price).toBeNull()
    expect(mediaPriceSummary(entry)).toBe('价格／规格待核对')
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 10, 0)).toBeNull()
  })
  it('does not fake a conditional-reference quote from a basic price', () => {
    const entry = mediaEntries(fixture(), policy({ conditionalReferencePricing: true }))[0]
    expect(entry.tiers[0].price).toBe(0.3)
    expect(entry.estimateBlocked).toBe(true)
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 10, 0)).toBeNull()
  })
  it('does not retain generic gateway capability tags as documented model promises', () => {
    const entry = mediaEntries(fixture(), policy())[0]
    expect(entry.tags).toEqual([])
  })
  it('rejects durations outside the explicit document range', () => {
    const entry = mediaEntries(fixture(), policy({ seconds: { min: 4, max: 15 } }))[0]
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 30, 0)).toBeNull()
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 10, 0)?.generation).toBe(3)
  })
  it('does not display inferred reference-video charges absent from the document', () => {
    const entry = mediaEntries(fixture(), policy({ referenceVideoMentioned: false }))[0]
    expect(entry.hasReferenceVideo).toBe(false)
    expect(entry.referencePrice).toBeNull()
  })
  it('does not expose generic unpriced resolution placeholders as document price tiers', () => {
    const data = fixture()
    data.groups[0].models[0].pricing!.resolution_prices = { '480p': 0.3, '720p': null, '4k': null }
    const entry = mediaEntries(data, policy({ pricedResolutions: ['480p'] }))[0]
    expect(entry.tiers.every(t => t.name === '480p' && t.price === 0.3)).toBe(true)
    expect(entry.tiers.length).toBeGreaterThan(0)
  })
  it('shows only the whitelist and document explanation in the customer card list', () => {
    const data = fixture(); data.groups[0].models.push({ ...data.groups[0].models[0], name: '表外隐藏模型' })
    const wrapper = mount(MediaPricingCatalog, { props: { response: data, initialKind: 'video', catalogPolicy: policy() } })
    expect(wrapper.findAll('[data-testid=media-model-card]')).toHaveLength(1)
    expect(wrapper.text()).toContain('参考图 9 张，4–15 秒')
    expect(wrapper.text()).toContain('详细规格、计费方式与费用试算')
    expect(wrapper.text()).not.toContain('加价')
    expect(wrapper.text()).not.toContain('上游成本')
    expect(wrapper.text()).not.toContain('表外隐藏模型')
    wrapper.unmount()
  })
})
