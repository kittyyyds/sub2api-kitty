import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import type { ApplicationRun } from '../../types'
import AppRunStatus from '../AppRunStatus.vue'

const IconStub = {
  name: 'Icon',
  props: ['name'],
  template: '<span class="icon-stub" :data-icon="name" />',
}

function mountStatus(run: ApplicationRun, running = true) {
  return mount(AppRunStatus, {
    props: { run, steps: run.steps || [], running },
    global: { stubs: { Icon: IconStub } },
  })
}

describe('AppRunStatus submitted media controls', () => {
  it('labels a submitted video action as stop waiting', async () => {
    const run: ApplicationRun = {
      id: 'run-video',
      app_slug: 'free-media-studio',
      status: 'running',
      steps: [{
        step_key: 'media',
        executor: 'video_generation',
        status: 'running',
        gateway_request_id: 'video-123',
        output: { request_id: 'video-123', status: 'processing' },
      }],
    }
    const wrapper = mountStatus(run)

    expect(wrapper.get('header button').text()).toContain('停止等待')
    await wrapper.get('header button').trigger('click')
    expect(wrapper.emitted('cancel')).toHaveLength(1)
  })

  it('keeps cancel wording before any video task is submitted', () => {
    const wrapper = mountStatus({
      id: 'run-new',
      app_slug: 'free-media-studio',
      status: 'running',
      steps: [{ step_key: 'media', executor: 'video_generation', status: 'pending' }],
    })

    expect(wrapper.get('header button').text()).toContain('取消运行')
  })

  it('labels a submitted image task as stop waiting', () => {
    const wrapper = mountStatus({
      id: 'run-image',
      app_slug: 'free-media-studio',
      status: 'running',
      steps: [{
        step_key: 'media',
        executor: 'image_generation',
        status: 'running',
        gateway_request_id: 'image-123',
        output: { task_id: 'image-123', status: 'processing' },
      }],
    })

    expect(wrapper.get('header button').text()).toContain('停止等待')
  })
})
