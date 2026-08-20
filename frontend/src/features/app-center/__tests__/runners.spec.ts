import { mount, type VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GROK_IMAGE_ASPECT_RATIOS } from '@/api/imageGeneration'
import EcommerceVisualRunner from '../components/runners/EcommerceVisualRunner.vue'
import GrokVideoDirectorRunner from '../components/runners/GrokVideoDirectorRunner.vue'
import InspirationImageRunner from '../components/runners/InspirationImageRunner.vue'

async function setFiles(wrapper: VueWrapper, files: File[]): Promise<void> {
  const input = wrapper.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { configurable: true, value: files })
  await input.trigger('change')
}

function selectByLabel(wrapper: VueWrapper, label: string) {
  return wrapper.findAll('label').find((item) => item.get('span').text() === label)!.get('select')
}

describe('application center runners', () => {
  beforeEach(() => {
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:test') })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
  })

  it('submits canonical image planning fields', async () => {
    const wrapper = mount(InspirationImageRunner, { props: { disabled: false, definition: null } })
    await wrapper.get('textarea').setValue('雨夜里的霓虹书店，作为文章头图')
    await wrapper.get('form').trigger('submit')

    const payload = wrapper.emitted('run')?.[0]?.[0] as { values: Record<string, unknown> }
    expect(payload.values).toEqual(expect.objectContaining({
      brief: '雨夜里的霓虹书店，作为文章头图',
      mode: 'text_to_image',
      model: 'grok-imagine-image-quality',
      aspect_ratio: '1:1',
      n: 1,
    }))
    expect(payload.values).not.toHaveProperty('idea')
    expect(payload.values).not.toHaveProperty('operation')
    expect(payload.values).not.toHaveProperty('output_count')
  })

  it('uses the source ratio for one Grok edit and enables all official ratios for multiple references', async () => {
    const wrapper = mount(InspirationImageRunner, { props: { disabled: false, definition: null } })
    await wrapper.get('textarea').setValue('将参考图改成电影感夜景并保持主体特征')
    await selectByLabel(wrapper, '生成方式').setValue('image_edit')
    await setFiles(wrapper, [new File(['one'], 'one.png', { type: 'image/png' })])

    expect(selectByLabel(wrapper, '画面比例').attributes('disabled')).toBeDefined()
    expect(selectByLabel(wrapper, '画面比例').text()).toBe('跟随原图')
    await wrapper.get('form').trigger('submit')
    const singlePayload = wrapper.emitted('run')?.[0]?.[0] as { values: Record<string, unknown> }
    expect(singlePayload.values).not.toHaveProperty('aspect_ratio')

    await setFiles(wrapper, [new File(['two'], 'two.png', { type: 'image/png' })])
    const ratio = selectByLabel(wrapper, '画面比例')
    expect(ratio.attributes('disabled')).toBeUndefined()
    expect(ratio.findAll('option').map((option) => option.attributes('value')))
      .toEqual([...GROK_IMAGE_ASPECT_RATIOS])
    await ratio.setValue('20:9')
    await wrapper.get('form').trigger('submit')
    const multiPayload = wrapper.emitted('run')?.[1]?.[0] as { values: Record<string, unknown> }
    expect(multiPayload.values.aspect_ratio).toBe('20:9')
  })

  it('does not expose Grok 4K or unsupported image model options', async () => {
    const wrapper = mount(InspirationImageRunner, { props: { disabled: false, definition: {
      slug: 'inspiration-image-refiner',
      revision: 1,
      version: '1.0',
      name: 'test',
      description: 'test',
      category: 'image',
      status: 'available',
      tags: ['Grok'],
      supported_modes: ['text_to_image', 'image_edit'],
      form: { fields: [
        { key: 'model', label: '模型', type: 'select', options: [
          { label: 'Grok Image', value: 'grok-imagine-image' },
          { label: '4K legacy', value: 'grok-imagine-image-4k' },
        ] },
        { key: 'aspect_ratio', label: '画面比例', type: 'select', options: [
          { label: '1:1 legacy', value: '1:1' },
        ] },
        { key: 'resolution', label: '清晰度', type: 'select', options: [
          { label: '4K', value: '4k' },
          { label: '2K', value: '2k' },
        ] },
      ] },
    } } })

    const selects = wrapper.findAll('select')
    expect(selects[1]?.findAll('option').map((option) => option.attributes('value'))).toEqual([
      'grok-imagine-image',
    ])
    expect(selectByLabel(wrapper, '画面比例').findAll('option').map((option) => option.attributes('value')))
      .toEqual([...GROK_IMAGE_ASPECT_RATIOS])
    expect(selects[5]?.findAll('option').map((option) => option.attributes('value'))).toEqual([
      '2k',
    ])
  })

  it('submits structured ecommerce constraints with the product reference', async () => {
    const wrapper = mount(EcommerceVisualRunner, { props: { disabled: false, definition: null } })
    const textInputs = wrapper.findAll('input').filter((input) => input.attributes('type') !== 'file')
    await textInputs[0].setValue('轻量防晒外套')
    await wrapper.get('textarea').setValue('UPF50+，轻薄透气，适合城市通勤')
    await setFiles(wrapper, [new File(['image'], 'coat.png', { type: 'image/png' })])
    await wrapper.get('form').trigger('submit')

    const payload = wrapper.emitted('run')?.[0]?.[0] as { values: Record<string, unknown>; files: File[] }
    expect(payload.values).toEqual(expect.objectContaining({
      product_name: '轻量防晒外套',
      placement: 'hero',
      marketing_goal: expect.any(String),
      must_keep: expect.stringContaining('商品外观'),
      mode: 'image_edit',
      model: 'grok-imagine-image-quality',
      reference_count: 1,
    }))
    expect(payload.values).not.toHaveProperty('aspect_ratio')
    expect(selectByLabel(wrapper, '画面比例').attributes('disabled')).toBeDefined()
    expect(selectByLabel(wrapper, '画面比例').text()).toBe('跟随原图')
    expect(payload.files).toHaveLength(1)

    await setFiles(wrapper, [new File(['second'], 'coat-detail.png', { type: 'image/png' })])
    const ratio = selectByLabel(wrapper, '画面比例')
    expect(ratio.findAll('option').map((option) => option.attributes('value')))
      .toEqual([...GROK_IMAGE_ASPECT_RATIOS])
    await ratio.setValue('9:20')
    await wrapper.get('form').trigger('submit')
    const multiPayload = wrapper.emitted('run')?.[1]?.[0] as { values: Record<string, unknown> }
    expect(multiPayload.values.aspect_ratio).toBe('9:20')
  })

  it.each([
    ['灵感精修生图', InspirationImageRunner],
    ['电商视觉策划', EcommerceVisualRunner],
  ])('%s follows the Grok 1-4 and OpenAI 1-8 count contract', async (_name, Runner) => {
    const wrapper = mount(Runner, {
      props: { disabled: false, definition: null, mediaPlatform: 'grok' },
    })
    const countControl = () => wrapper.findAll('label')
      .find((label) => label.text().includes('生成数量'))!
      .get('select')

    expect(countControl().findAll('option').map((option) => option.attributes('value')))
      .toEqual(['1', '2', '3', '4'])

    await wrapper.setProps({ mediaPlatform: 'openai' })
    expect(countControl().findAll('option').map((option) => option.attributes('value')))
      .toEqual(['1', '2', '3', '4', '5', '6', '7', '8'])
    await countControl().setValue('8')

    await wrapper.setProps({ mediaPlatform: 'grok' })
    expect((countControl().element as HTMLSelectElement).value).toBe('4')

    wrapper.unmount()
  })

  it('binds each video mode to the supported model, resolution and source type', async () => {
    const wrapper = mount(GrokVideoDirectorRunner, { props: { disabled: false, definition: null } })
    await wrapper.get('textarea').setValue('产品从暗处进入聚光灯，镜头稳定推进并在标志处停住')
    const selects = wrapper.findAll('select')
    const mode = selects[0]
    const model = selects[1]
    const resolution = selects[4]

    expect((model.element as HTMLSelectElement).value).toBe('grok-imagine-video-1.5')
    expect(resolution.findAll('option').map((option) => option.text())).toEqual(['480p', '720p', '1080p'])

    await mode.setValue('image_to_video')
    expect(resolution.findAll('option').map((option) => option.text())).toEqual(['480p', '720p', '1080p'])
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await setFiles(wrapper, [new File(['video'], 'wrong.mp4', { type: 'video/mp4' })])
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await setFiles(wrapper, [new File(['image'], 'start.png', { type: 'image/png' })])
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()

    await mode.setValue('reference_to_video')
    expect((model.element as HTMLSelectElement).value).toBe('grok-imagine-video')
    expect(resolution.findAll('option').map((option) => option.text())).toEqual(['480p', '720p'])
    await setFiles(wrapper, [new File(['one'], 'one.png', { type: 'image/png' })])
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await setFiles(wrapper, [
      new File(['one'], 'one.png', { type: 'image/png' }),
      new File(['two'], 'two.png', { type: 'image/png' }),
    ])
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()

    await mode.setValue('video_edit')
    await setFiles(wrapper, [new File(['image'], 'wrong.png', { type: 'image/png' })])
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await setFiles(wrapper, [new File(['video'], 'source.mp4', { type: 'video/mp4' })])
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })
})
