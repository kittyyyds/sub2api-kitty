import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import ReferenceUpload from '../ReferenceUpload.vue'

describe('ReferenceUpload', () => {
  beforeEach(() => {
    vi.stubGlobal('URL', {
      createObjectURL: vi.fn(() => 'blob:preview'),
      revokeObjectURL: vi.fn(),
    })
  })

  afterEach(() => vi.unstubAllGlobals())

  it('rejects a drag-and-dropped file that does not match accept', async () => {
    const wrapper = mount(ReferenceUpload, {
      props: {
        modelValue: [],
        accept: 'image/png,image/jpeg,image/webp',
      },
    })
    const textFile = new File(['notes'], 'notes.txt', { type: 'text/plain' })

    await wrapper.find('.reference-drop').trigger('drop', {
      dataTransfer: { files: [textFile] },
    })

    expect(wrapper.text()).toContain('不支持文件 notes.txt 的格式')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([])
  })

  it('accepts a matching drag-and-dropped image', async () => {
    const wrapper = mount(ReferenceUpload, {
      props: {
        modelValue: [],
        accept: 'image/png,image/jpeg,image/webp',
      },
    })
    const image = new File(['image'], 'source.png', { type: 'image/png' })

    await wrapper.find('.reference-drop').trigger('drop', {
      dataTransfer: { files: [image] },
    })

    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([image])
    expect(wrapper.text()).not.toContain('不支持文件')
  })
})
