import { beforeAll, describe, expect, it } from 'vitest'
import { mount, config } from '@vue/test-utils'
import { fixture } from './mediaPricing.fixture'
import MediaPricingCatalog from '../MediaPricingCatalog.vue'
import { suanliMediaCatalog, type MediaCatalogPolicy } from '../mediaCatalogPolicy'

beforeAll(() => {
  config.global.stubs = { ...config.global.stubs, teleport: true }
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
})
const policy: MediaCatalogPolicy = { sourceUrl: '', title: 'test', capturedOn: '2026-10-27',
  categories: [{ id: 'direct', label: '官方直连系列' }, { id: 'value', label: '优质特价系列' }],
  models: ['渠道/原始模型-ID', '另一型号'].map((id, i) => ({ id, kind: 'video', categoryId: i ? 'value' : 'direct',
    sourceRows: [i + 10], description: '支持参考图', billingUnits: ['秒'], pricingNotes: [], warnings: [],
    conditionalReferencePricing: false, referenceRule: 'unspecified' })),
}
function response() {
  const data = fixture()
  data.groups[0].models.push({ ...data.groups[0].models[0], name: '另一型号' })
  return data
}
describe('categorized customer video tables', () => {
  it('groups exact models by source series and keeps effective final prices', async () => {
    const w = mount(MediaPricingCatalog, { props: { response: response(), catalogPolicy: policy, initialKind: 'video' } })
    expect(w.findAll('[data-testid=media-series-section]')).toHaveLength(2)
    expect(w.findAll('[data-testid=media-model-row]')).toHaveLength(2)
    expect(w.get('.series-prices').text()).toContain('$0.3')
    expect(w.text()).not.toContain('$2.7')
    expect(w.text()).not.toContain('上游成本')
    await w.get('.category-tabs button:nth-child(3)').trigger('click')
    expect(w.findAll('[data-testid=media-model-row]')).toHaveLength(1)
    expect(w.get('.series-model').text()).toContain('另一型号')
    await w.get('input[aria-label=搜索模型名称]').setValue('原始模型-ID')
    expect(w.findAll('[data-testid=media-model-row]')).toHaveLength(1)
    expect(w.get('.series-model').text()).toContain('原始模型-ID')
    w.unmount()
  })
  it('preserves table/card switching and exact-group detail entry', async () => {
    const w = mount(MediaPricingCatalog, { props: { response: response(), catalogPolicy: policy, initialKind: 'video' } })
    await w.get('.series-table .details-button').trigger('click')
    expect(w.get('.dialog-header').text()).toContain('渠道/原始模型-ID')
    await w.get('.view-toggle button:nth-child(2)').trigger('click')
    expect(w.findAll('[data-testid=media-model-card]')).toHaveLength(2)
    expect(w.find('[data-testid=media-model-row]').exists()).toBe(false)
    w.unmount()
  })
  it('has explicit verified categories for every document model', () => {
    const ids = new Set(suanliMediaCatalog.categories?.map(c => c.id))
    expect(ids.size).toBe(15)
    expect(suanliMediaCatalog.models.every(m => ids.has(m.categoryId!))).toBe(true)
    expect(suanliMediaCatalog.models.filter(m => m.kind === 'image')).toHaveLength(34)
    expect(suanliMediaCatalog.models.filter(m => m.kind === 'video')).toHaveLength(158)
    expect(suanliMediaCatalog.models.find(m => m.id === '官转特价sd2.0-480p')?.billingUnits).toEqual(['条'])
    expect(suanliMediaCatalog.models.some(m => m.id === '全能sd2.5(30图10音内置过人脸720P)')).toBe(false)
  })
})
