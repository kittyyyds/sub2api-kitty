import { nextTick } from 'vue'
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import { GROK_VIDEO_15_MODEL, GROK_VIDEO_MODEL } from '@/api/videoGeneration'

import type { ApplicationDefinition } from '../../../types'
import ReferenceUpload from '../../ReferenceUpload.vue'
import GrokVideoDirectorRunner from '../GrokVideoDirectorRunner.vue'

const definition: ApplicationDefinition = {
  slug: 'grok-video-director',
  revision: 1,
  version: '1.0',
  name: 'Grok 视频导演',
  description: 'test',
  category: 'video',
  status: 'available',
  tags: ['Grok'],
  supported_modes: ['text_to_video', 'image_to_video', 'reference_to_video', 'video_edit', 'video_extend'],
  form: {
    fields: [
      { key: 'mode', label: '模式', type: 'select', options: [
        { label: '文生视频', value: 'text_to_video' },
        { label: '图生视频', value: 'image_to_video' },
        { label: '多图参考生视频', value: 'reference_to_video' },
        { label: '视频编辑', value: 'video_edit' },
        { label: '视频续写', value: 'video_extend' },
      ] },
      { key: 'model', label: '模型', type: 'select', options: [
        { label: '1.5', value: GROK_VIDEO_15_MODEL },
        { label: '经典', value: GROK_VIDEO_MODEL },
      ] },
      { key: 'resolution', label: '清晰度', type: 'select', options: [
        { label: '480p', value: '480p' },
        { label: '720p', value: '720p' },
        { label: '1080p', value: '1080p' },
      ] },
      { key: 'reference_files', label: '素材', type: 'file', max_items: 5 },
      { key: 'duration', label: '时长', type: 'number', min: 1, max: 15 },
    ],
  },
}

describe('GrokVideoDirectorRunner', () => {
  it('links source type, model, and 1080p availability to the selected mode', async () => {
    const wrapper = mount(GrokVideoDirectorRunner, {
      props: { disabled: false, definition },
    })
    const selects = wrapper.findAll('select')
    const modeSelect = selects[0]
    const modelSelect = selects[1]

    expect(modelSelect.findAll('option').map((option) => option.attributes('value'))).toEqual([
      GROK_VIDEO_15_MODEL,
      GROK_VIDEO_MODEL,
    ])
    expect(wrapper.text()).toContain('1080p')

    await modeSelect.setValue('video_edit')
    await nextTick()
    expect(modelSelect.element.value).toBe(GROK_VIDEO_MODEL)
    expect(modelSelect.findAll('option').map((option) => option.attributes('value'))).toEqual([GROK_VIDEO_MODEL])
    expect(wrapper.text()).not.toContain('1080p')
    expect(wrapper.findComponent(ReferenceUpload).props('accept')).toBe('video/mp4,video/webm')

    await modeSelect.setValue('image_to_video')
    await modelSelect.setValue(GROK_VIDEO_15_MODEL)
    await nextTick()
    expect(wrapper.text()).toContain('1080p')
    expect(wrapper.findComponent(ReferenceUpload).props('accept')).toBe('image/png,image/jpeg,image/webp')
  })
})
