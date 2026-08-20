import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import AppResultPanel from '../AppResultPanel.vue'

const IconStub = {
  name: 'Icon',
  props: ['name'],
  template: '<span class="icon-stub" :data-icon="name" />',
}

describe('AppResultPanel video result', () => {
  it('disables playback and keeps the authenticated download available', () => {
    const wrapper = mount(AppResultPanel, {
      props: {
        running: false,
        result: {
          type: 'video',
          videoUrl: 'blob:authenticated-current-video',
          requestId: 'video-current',
        },
      },
      global: { stubs: { Icon: IconStub } },
    })

    expect(wrapper.find('video').exists()).toBe(false)
    const playButton = wrapper.get('button[aria-label="在线播放已关闭，请下载后查看"]')
    expect(playButton.attributes('disabled')).toBeDefined()
    const download = wrapper.get('a[download="app-video.mp4"]')
    expect(download.attributes('href')).toBe('blob:authenticated-current-video')
  })
})
