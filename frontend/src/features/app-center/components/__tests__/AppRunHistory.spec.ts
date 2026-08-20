import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import type { ApplicationRun } from '../../types'
import AppRunHistory from '../AppRunHistory.vue'

const IconStub = {
  name: 'Icon',
  props: ['name'],
  template: '<span class="icon-stub" :data-icon="name" />',
}

function runWithVideo(status: string, overrides: Partial<ApplicationRun> = {}): ApplicationRun {
  const runStatus = ['failed', 'expired'].includes(status)
    ? 'failed'
    : status === 'cancelled'
      ? 'cancelled'
      : ['done', 'completed', 'succeeded'].includes(status)
        ? 'completed'
        : 'running'
  const stepStatus = ['failed', 'expired'].includes(status)
    ? 'failed'
    : status === 'cancelled'
      ? 'cancelled'
      : ['done', 'completed', 'succeeded'].includes(status)
        ? 'completed'
        : 'running'
  return {
    id: `run-${status}`,
    app_slug: 'free-media-studio',
    status: runStatus,
    steps: [{
      step_key: 'media',
      status: stepStatus,
      executor: 'video_generation',
      gateway_request_id: `video-${status}`,
      output: { request_id: `video-${status}`, status },
    }],
    ...overrides,
  }
}

function mountHistory(selectedRun: ApplicationRun, preview: {
  url?: string
  loading?: boolean
  error?: string
} = {}) {
  return mount(AppRunHistory, {
    props: {
      runs: [selectedRun],
      selectedRun,
      loading: false,
      videoPreviewUrl: preview.url || '',
      videoPreviewLoading: preview.loading || false,
      videoPreviewError: preview.error || '',
    },
    global: { stubs: { Icon: IconStub } },
  })
}

function runWithImages(output: unknown, steps: ApplicationRun['steps'] = []): ApplicationRun {
  return {
    id: 'run-images',
    app_slug: 'free-media-studio',
    status: 'completed',
    output,
    steps,
  }
}

describe('AppRunHistory video lifecycle', () => {
  it.each(['failed', 'expired', 'cancelled', 'done', 'completed', 'succeeded'])('does not offer resume for terminal %s tasks', (status) => {
    const wrapper = mountHistory(runWithVideo(status))

    expect(wrapper.find('.resume-button').exists()).toBe(false)
    expect(wrapper.text()).toContain(`video-${status}`)
  })

  it.each(['pending', 'queued', 'submitted', 'processing', 'in_progress'])('offers resume for %s tasks', (status) => {
    const wrapper = mountHistory(runWithVideo(status))

    expect(wrapper.get('.resume-button').text()).toContain('继续查询视频')
  })

  it('treats a cancelled run as terminal even when its last checkpoint said processing', () => {
    const wrapper = mountHistory(runWithVideo('processing', { status: 'cancelled' }))

    expect(wrapper.find('.resume-button').exists()).toBe(false)
    expect(wrapper.get('.history-video span').text()).toBe('cancelled')
  })

  it('restores a completed video as a download with playback disabled', () => {
    const run = runWithVideo('completed', {
      output: {
        request_id: 'video-completed',
        status: 'completed',
        video_url: 'https://cdn.test/history-video.mp4',
      },
      steps: [{
        step_key: 'media',
        status: 'completed',
        executor: 'video_generation',
        gateway_request_id: 'video-completed',
        output: {
          request_id: 'video-completed',
          status: 'completed',
          video_url: 'https://cdn.test/history-video.mp4',
        },
      }],
    })
    const wrapper = mountHistory(run, { url: 'blob:authenticated-history-video' })

    expect(wrapper.find('video').exists()).toBe(false)
    const playButton = wrapper.get('button[aria-label="在线播放已关闭，请下载后查看"]')
    expect(playButton.attributes('disabled')).toBeDefined()
    const download = wrapper.get('a[download="app-history-video.mp4"]')
    expect(download.attributes('href')).toBe('blob:authenticated-history-video')
    expect(wrapper.get('.history-video code').text()).toBe('video-completed')
    expect(wrapper.find('.resume-button').exists()).toBe(false)
  })

  it('does not send a protected persisted URL directly to the video element', () => {
    const run = runWithVideo('completed', {
      output: {
        request_id: 'video-completed',
        status: 'completed',
        video_url: 'https://gateway.test/v1/videos/video-completed/content',
      },
    })
    const loading = mountHistory(run, { loading: true })
    expect(loading.find('video').exists()).toBe(false)
    expect(loading.get('.history-video-message').text()).toContain('正在加载')

    const failed = mountHistory(run, { error: '历史视频加载失败。' })
    expect(failed.find('video').exists()).toBe(false)
    expect(failed.get('.history-video-message.error').text()).toContain('加载失败')
  })
})

describe('AppRunHistory persisted images', () => {
  it('offers image polling only for a persisted processing task', () => {
    const wrapper = mountHistory({
      id: 'run-image-processing',
      app_slug: 'free-media-studio',
      status: 'running',
      steps: [{
        step_key: 'media',
        status: 'running',
        executor: 'image_generation',
        gateway_request_id: 'image-123',
        output: { task_id: 'image-123', status: 'processing' },
      }],
    })

    expect(wrapper.get('.history-image-task code').text()).toBe('image-123')
    expect(wrapper.get('.resume-button').text()).toContain('继续查询图片')
  })

  it('renders images from selectedRun.output with open and download actions', () => {
    const wrapper = mountHistory(runWithImages({
      count: 2,
      images: [
        { url: 'https://cdn.test/history-one.png', revised_prompt: 'one' },
        { url: 'http://cdn.test/history-two.webp', revised_prompt: 'two' },
      ],
    }))

    expect(wrapper.findAll('.history-image')).toHaveLength(2)
    expect(wrapper.get('.history-image img').attributes('src')).toBe('https://cdn.test/history-one.png')
    const links = wrapper.get('.history-image').findAll('a')
    expect(links[0]?.attributes('target')).toBe('_blank')
    expect(links[0]?.attributes('href')).toBe('https://cdn.test/history-one.png')
    expect(links[1]?.attributes('download')).toBe('app-history-1.png')
  })

  it('falls back to the last media step output when run output has no images', () => {
    const wrapper = mountHistory(runWithImages(
      { count: 0, images: [] },
      [
        {
          step_key: 'plan',
          status: 'completed',
          executor: 'chat_completions',
          credential_role: 'planner',
          output: { images: [{ url: 'https://cdn.test/not-media.png' }] },
        },
        {
          step_key: 'media',
          status: 'completed',
          executor: 'image_generation',
          credential_role: 'media',
          output: { images: [{ url: 'https://cdn.test/from-step.png', revised_prompt: 'step' }] },
        },
      ],
    ))

    expect(wrapper.findAll('.history-image')).toHaveLength(1)
    expect(wrapper.get('.history-image img').attributes('src')).toBe('https://cdn.test/from-step.png')
  })

  it('filters non-http URLs and caps the preview at eight images', () => {
    const validImages = Array.from({ length: 9 }, (_, index) => ({
      url: `https://cdn.test/image-${index + 1}.png`,
    }))
    const wrapper = mountHistory(runWithImages({
      images: [
        { url: 'data:image/png;base64,AAAA' },
        { url: 'blob:https://app.test/task' },
        { url: '/v1/images/private' },
        { url: 'ftp://cdn.test/file.png' },
        { url: 'not-a-url' },
        ...validImages,
      ],
    }))

    const images = wrapper.findAll('.history-image img')
    expect(images).toHaveLength(8)
    expect(images.every((image) => /^https?:\/\//.test(image.attributes('src')))).toBe(true)
    expect(images.at(-1)?.attributes('src')).toBe('https://cdn.test/image-8.png')
  })
})
