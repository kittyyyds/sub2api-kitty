import { describe, expect, it } from 'vitest'
import { fixture } from './mediaPricing.fixture'
import { mediaEntries, mediaMatches, mediaMoney, mediaPriceSummary, estimateMediaPrice } from '../mediaPricing'
describe('customer media pricing presentation', () => {
  it('keeps final video price independent of group and user multipliers', () => {
    const entry = mediaEntries(fixture())[0]
    expect(entry.tiers[0].price).toBe(0.3)
    expect(entry.id).toBe('渠道/原始模型-ID')
    expect(mediaPriceSummary(entry)).toBe('$0.3/秒')
  })
  it('adds reference seconds once, without adding markups again', () => {
    const entry = mediaEntries(fixture())[0]
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 10, 3)).toEqual({ generation: 3, reference: 0.30000000000000004, total: 3.3 })
  })
  it.each([0, -1, NaN, 1.5, 101])('rejects invalid quantity %s', quantity => {
    const entry = mediaEntries(fixture())[0]
    expect(estimateMediaPrice(entry, entry.tiers[0].key, quantity, 10, 0)).toBeNull()
  })
  it('rejects unsupported durations and excessive reference duration', () => {
    const entry = mediaEntries(fixture())[0]
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 7, 0)).toBeNull()
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 10, 13)).toBeNull()
  })
  it('never guesses missing prices, and does not fall back to official costs', () => {
    const data = fixture(); data.groups[0].models[0].pricing = null
    const entry = mediaEntries(data)[0]
    expect(mediaPriceSummary(entry)).toBe('价格待确认')
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 10, 0)).toBeNull()
  })
  it('keeps the same model in different groups separate', () => {
    const data = fixture(); data.groups.push({ ...data.groups[0], id: 52, name: '其他分组' })
    const entries = mediaEntries(data)
    expect(entries).toHaveLength(2); expect(entries[0].key).not.toBe(entries[1].key)
  })
  it('deduplicates exact IDs within a group without applying the multiplier to final prices twice', () => {
    const data = fixture(); const model = data.groups[0].models[0]
    data.groups[0].models.push(structuredClone(model))
    expect(mediaEntries(data)).toHaveLength(1)
    expect(mediaEntries(data)[0].tiers[0].price).toBe(0.3)
  })
  it('prefers a complete explicitly final image record over its legacy duplicate', () => {
    const data = fixture(); const g = data.groups[0]; const m = g.models[0]
    g.platform = 'openai'; m.platform = 'openai'; delete m.capabilities
    m.pricing!.billing_mode = 'image'; delete m.pricing!.final_unit_price
    m.pricing!.intervals = [{ min_tokens: 0, max_tokens: null, tier_label: '1K', per_request_price: 0.02, input_price: null, output_price: null, cache_write_price: null, cache_read_price: null }]
    const authoritative = structuredClone(m); authoritative.final_image_price_tiers = ['1K']
    g.models.push(authoritative)
    const entries = mediaEntries(data)
    expect(entries).toHaveLength(1); expect(entries[0].tiers[0].price).toBe(0.02)
    expect(entries[0].pricingConflict).toBe(false)
  })
  it('blocks estimation when equally authoritative duplicates disagree rather than picking a cheaper price', () => {
    const data = fixture(); const other = structuredClone(data.groups[0].models[0])
    other.pricing!.per_request_price = 0.4; data.groups[0].models.push(other)
    const entry = mediaEntries(data)[0]
    expect(mediaPriceSummary(entry)).toBe('价格冲突 · 待确认')
    expect(entry.tiers.every(t => t.price === null)).toBe(true)
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 10, 0)).toBeNull()
  })
  it('preserves case-sensitive and platform-specific original identities', () => {
    const data = fixture(); const model = data.groups[0].models[0]
    data.groups[0].models.push({ ...model, name: model.name.toLowerCase() }, { ...model, platform: 'grok' })
    expect(mediaEntries(data)).toHaveLength(3)
  })
  it('searches exact Chinese ID and group without changing permissions', () => {
    const entry = mediaEntries(fixture())[0]
    expect(mediaMatches(entry, ' 渠道/原始模型-ID ')).toBe(true)
    expect(mediaMatches(entry, '视频模型 图生视频')).toBe(true)
    expect(mediaMatches(entry, '未授权的模型')).toBe(false)
  })
  it('keeps image final tiers and multiplies image count only', () => {
    const data = fixture(); const g = data.groups[0]; const m = g.models[0]
    g.platform = 'openai'; m.platform = 'openai'; delete m.capabilities; m.final_image_price_tiers = ['1K'];
    m.pricing!.billing_mode = 'image'; m.pricing!.intervals = [{ min_tokens: 0, max_tokens: null, tier_label: '1K', per_request_price: 0.02, input_price: null, output_price: null, cache_write_price: null, cache_read_price: null }]
    const entry = mediaEntries(data)[0]
    expect(entry.tiers[0].price).toBe(0.02)
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 4, 0, 0)?.total).toBe(0.08)
  })
  it('does not estimate models explicitly unsupported by the gateway', () => {
    const data = fixture(); data.groups[0].models[0].capabilities!.supported = false
    const entry = mediaEntries(data)[0]
    expect(entry.status).toBe('unsupported')
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 1, 10, 0)).toBeNull()
  })
  it('does not mix by-second and by-request video estimates', () => {
    const data = fixture(); data.groups[0].models[0].pricing!.billing_mode = 'per_request'
    const entry = mediaEntries(data)[0]
    expect(entry.unit).toBe('条')
    expect(estimateMediaPrice(entry, entry.tiers[0].key, 2, 15, 0)?.total).toBe(0.6)
  })
  it('does not label missing values as free or leak tiny price precision', () => {
    expect(mediaMoney(null)).toBe('价格待确认'); expect(mediaMoney(NaN)).toBe('价格待确认')
    expect(mediaMoney(0)).toBe('$0'); expect(mediaMoney(0.000001)).toBe('$0.000001'); expect(mediaMoney(10)).toBe('$10')
  })
})
