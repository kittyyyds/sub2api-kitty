import { beforeAll, describe, expect, it, vi } from 'vitest'
import { config, mount } from '@vue/test-utils'
import MediaPricingCatalog from '../MediaPricingCatalog.vue'
import MediaSourceBadges from '../MediaSourceBadges.vue'
import { suanliMediaCatalog, type MediaCatalogModel, type MediaCatalogPolicy } from '../mediaCatalogPolicy'
import { mediaEntries, mediaMatches } from '../mediaPricing'
import { fixture } from './mediaPricing.fixture'

beforeAll(() => {
  config.global.stubs = { ...config.global.stubs, teleport: true }
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
})
function imageFixture() {
  const data = fixture(), group = data.groups[0], model = group.models[0]
  group.id = 50; group.name = '图片模型'; group.platform = 'openai'; model.platform = 'openai'; delete model.capabilities
  model.name = 'gpt-image-2'; model.pricing!.billing_mode = 'image'; model.final_image_price_tiers = ['1K']
  model.pricing!.intervals = [{ min_tokens: 0, max_tokens: null, tier_label: '1K', per_request_price: 0.02, input_price: null, output_price: null, cache_write_price: null, cache_read_price: null }]
  return data
}
const imageDocument = suanliMediaCatalog.models.find(m => m.id === 'gpt-image-2')!
function policy(model: MediaCatalogModel = imageDocument): MediaCatalogPolicy {
  return { sourceUrl: '', title: 'test', capturedOn: '2026-10-28', categories: [{ id: model.categoryId!, label: '稳定出图模型' }], models: [model] }
}
describe('image tables and source annotations', () => {
  it('defaults images to the same categorized table, retaining final per-image prices', () => {
    const w = mount(MediaPricingCatalog, { props: { response: imageFixture(), catalogPolicy: policy() } })
    expect(w.findAll('[data-testid=media-model-row]')).toHaveLength(1)
    expect(w.get('thead').text()).toContain('标注 / 提醒')
    expect(w.get('thead').text()).toContain('优势与说明')
    expect(w.get('.series-prices').text()).toContain('$0.02')
    expect(w.get('.series-prices').text()).toContain('张')
    expect(w.text()).toContain('长期稳定')
    expect(w.text()).toContain('🌟🌟🌟🌟🌟')
    expect(w.text()).toContain('并非本站实时检测')
    w.unmount()
  })
  it('preserves annotations when toggling image table/cards and opening details', async () => {
    const w = mount(MediaPricingCatalog, { props: { response: imageFixture(), catalogPolicy: policy() } })
    await w.get('.view-toggle button:nth-child(2)').trigger('click')
    expect(w.get('[data-testid=media-model-card]').text()).toContain('长期稳定')
    await w.get('.card-actions .details-button').trigger('click')
    expect(w.get('.dialog-body').text()).toContain('🌟🌟🌟🌟🌟')
    expect(w.get('.estimate-result').text()).toContain('$0.02')
    w.unmount()
  })
  it('searches source labels without changing exact copyable model IDs', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const w = mount(MediaPricingCatalog, { props: { response: imageFixture(), catalogPolicy: policy() } })
    await w.get('input[aria-label=搜索模型名称]').setValue('长期稳定')
    expect(w.findAll('[data-testid=media-model-row]')).toHaveLength(1)
    await w.get('button[aria-label="复制模型名称 gpt-image-2"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith('gpt-image-2')
    w.unmount()
  })
  it('keeps exact advantages and separate descriptions visible without truncation', async () => {
    const model = { ...imageDocument, sourceAdvantages: '原生真4K\n保留原文第二行', sourceDescription: '建议凌晨跑', sourceBadges: ['推荐', '特惠'] }
    const w = mount(MediaPricingCatalog, { props: { response: imageFixture(), catalogPolicy: policy(model) } })
    expect(w.get('[data-testid=source-advantages]').text()).toContain(model.sourceAdvantages)
    expect(w.get('[data-testid=source-description]').text()).toContain('建议凌晨跑')
    await w.get('.table-actions .details-button').trigger('click')
    expect(w.get('.detail-description').text()).toContain('原生真4K')
    expect(mediaMatches(mediaEntries(imageFixture(), policy(model))[0], '建议凌晨跑')).toBe(true)
    w.unmount()
  })
  it('does not infer missing labels or render source text as markup', () => {
    const empty = mount(MediaSourceBadges, { props: { document: { ...imageDocument, sourceBadges: [], sourceRecommendation: '' }, showEmpty: true } })
    expect(empty.text()).toBe('未标注'); expect(empty.findAll('[data-testid=source-badge]')).toHaveLength(0); empty.unmount()
    const w = mount(MediaSourceBadges, { props: { document: { ...imageDocument, sourceBadges: ['<img src=x onerror=alert(1)>'] } } })
    expect(w.find('img').exists()).toBe(false); expect(w.text()).toContain('<img'); w.unmount()
  })
  it('uses warning colors for adverse labels, without overriding customer prices or availability', () => {
    const labels = ['不稳定', '不可用', '维护', '白天经常不能用', '建议凌晨跑', '特惠', '可用']
    const document = { ...imageDocument, sourceBadges: labels }
    const w = mount(MediaSourceBadges, { props: { document } })
    expect(w.findAll('.badge-warning')).toHaveLength(4)
    expect(w.findAll('.badge-timing')).toHaveLength(1)
    expect(w.findAll('.badge-offer')).toHaveLength(1)
    expect(w.findAll('.badge-positive')).toHaveLength(1)
    const [before] = mediaEntries(imageFixture(), policy()), [after] = mediaEntries(imageFixture(), policy(document))
    expect(after.status).toBe(before.status); expect(after.tiers).toEqual(before.tiers); w.unmount()
  })
  it('has source labels for all 34 documented image models, including maintenance warnings', () => {
    const images = suanliMediaCatalog.models.filter(m => m.kind === 'image')
    expect(images).toHaveLength(34)
    expect(images.every(m => m.annotationRows?.length)).toBe(true)
    expect(images.find(m => m.id === 'nano-banana-pro(原生高质量支持真4K)')?.sourceBadges).toContain('维护')
  })
  it('copies the observed quality-series recommendations and timing warnings exactly', () => {
    const fast = suanliMediaCatalog.models.find(m => m.id === '高质量sd2.5/(全参30-10-10原生过真人速度快720P)')!
    expect(fast.sourceBadges).toEqual(['推荐非常快', '高质量速度极快'])
    expect(fast.sourceDescription).toBe('出视频速度非常快 不是dola')
    expect(fast.sourceRecommendation).toBe('🌟🌟🌟🌟🌟')
    const night = suanliMediaCatalog.models.find(m => m.id === '高质量sd2.5(满血全参/720P/按次)')!
    expect(night.sourceBadges).toEqual(['白天经常不能用', '建议凌晨跑', '福利'])
  })
  it('retains specification numbers but never supplier money in advantages', () => {
    const model = suanliMediaCatalog.models.find(m => m.id === 'SVIP系列/官方直连-高质量/SD2.0mini')!
    expect(model.sourceAdvantages).toContain('480P')
    expect(model.sourceAdvantages).toContain('720P有参考图时')
    expect(model.sourceAdvantages).toContain('价格见本页价格栏')
    expect(model.sourceAdvantages).not.toContain('0.18/秒')
    expect(suanliMediaCatalog.models.flatMap(m => m.sourceBadges ?? []).every(label => !/文档|\.md|接口\//.test(label))).toBe(true)
  })
})
