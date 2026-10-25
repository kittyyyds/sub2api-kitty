import { beforeAll, describe, expect, it, vi } from 'vitest'
import { mount, config, DOMWrapper } from '@vue/test-utils'
import MediaPricingCatalog from '../MediaPricingCatalog.vue'
import { fixture } from './mediaPricing.fixture'
beforeAll(() => {
  config.global.stubs = { ...config.global.stubs, teleport: true }
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
  HTMLDialogElement.prototype.close = function () { this.removeAttribute('open'); this.dispatchEvent(new Event('close')) }
})
describe('media catalog customer UI', () => {
  it('filters by media type and pasted original ID', async () => {
    const wrapper = mount(MediaPricingCatalog, { props: { response: fixture(), preview: true } })
    expect(wrapper.findAll('[data-testid=media-model-card]')).toHaveLength(0)
    await wrapper.get('.media-tabs button:nth-child(2)').trigger('click')
    expect(wrapper.findAll('[data-testid=media-model-card]')).toHaveLength(1)
    await wrapper.get('input[aria-label=搜索模型名称]').setValue('原始模型-ID')
    expect(wrapper.findAll('[data-testid=media-model-card]')).toHaveLength(1)
    await wrapper.get('input[aria-label=搜索模型名称]').setValue('不存在')
    expect(wrapper.text()).toContain('没有找到匹配的模型'); wrapper.unmount()
  })
  it('shows breakdown and copies the original ID without any generation action', async () => {
    const wrapper = mount(MediaPricingCatalog, { attachTo: document.body, global: { stubs: { teleport: false } }, props: { response: fixture(), initialKind: 'video' } })
    await wrapper.get('.card-actions .details-button').trigger('click')
    const modal = new DOMWrapper(document.body.querySelector('dialog')!)
    expect(modal.attributes('open')).toBeDefined()
    expect(modal.text()).toContain('参考视频附加费用')
    await modal.get('input[aria-label=参考视频秒数]').setValue(3)
    await modal.get('select[aria-label=生成时长]').setValue(10)
    expect(modal.get('.estimate-result').text()).toContain('$3.3')
    expect(modal.text()).toContain('仅试算，不提交任务')
    await modal.get('button[aria-label=关闭模型详情]').trigger('click')
    expect(modal.find('.dialog-body').exists()).toBe(false); wrapper.unmount()
  })
  it('does not advertise public price listings as user authorization', () => {
    const wrapper = mount(MediaPricingCatalog, { props: { response: fixture() } })
    expect(wrapper.text()).toContain('当前为公开价格')
    expect(wrapper.text()).toContain('价格目录不代表已获得调用权限'); wrapper.unmount()
  })
  it('keeps the last catalog but clearly marks errors as stale', async () => {
    const wrapper = mount(MediaPricingCatalog, { props: { response: fixture(), initialKind: 'video', error: true } })
    expect(wrapper.get('[role=alert]').text()).toContain('当前不是最新核验结果')
    expect(wrapper.findAll('[data-testid=media-model-card]')).toHaveLength(1)
    await wrapper.get('.refresh-button').trigger('click'); expect(wrapper.emitted('refresh')).toHaveLength(1); wrapper.unmount()
  })
})


describe('canvas exact-group detail entry', () => {
  it('opens only the requested original model in the requested group', async () => {
    const data = fixture(); data.groups.push({ ...data.groups[0], id: 52, name: '另外分组' })
    const wrapper = mount(MediaPricingCatalog, { attachTo: document.body, global: { stubs: { teleport: false } }, props: { response: data, initialKind: 'video', initialGroupId: 52, initialDetail: '渠道/原始模型-ID' } })
    const modal = new DOMWrapper(document.body.querySelector('dialog')!)
    await vi.waitFor(() => expect(modal.attributes('open')).toBeDefined())
    expect(modal.get('.dialog-header').text()).toContain('另外分组')
    expect(wrapper.get('select[aria-label=筛选分组]').element.value).toBe('52')
    await modal.get('button[aria-label=关闭模型详情]').trigger('click')
    await wrapper.setProps({ response: structuredClone(data) })
    expect(document.body.querySelector('.dialog-body')).toBeNull()
    wrapper.unmount()
  })
  it.each([undefined, 999])('does not guess another group when target group is %s', async group => {
    const wrapper = mount(MediaPricingCatalog, { props: { response: fixture(), initialKind: 'video', initialGroupId: group, initialDetail: '渠道/原始模型-ID' } })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.dialog-body').exists()).toBe(false)
    wrapper.unmount()
  })
})
