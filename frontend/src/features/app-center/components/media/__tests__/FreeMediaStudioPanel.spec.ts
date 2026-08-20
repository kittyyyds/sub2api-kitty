import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick, ref, unref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { GROK_IMAGE_ASPECT_RATIOS } from '@/api/imageGeneration'
import { createTestI18n } from '@/test/createTestI18n'
import type { ApplicationRun, LocalMediaResult } from '../../../types'
import FreeMediaStudioPanel, {
  type EmbeddedMediaKeyOption,
  type EmbeddedMediaRuntime,
} from '../FreeMediaStudioPanel.vue'

const IconStub = {
  name: 'Icon',
  props: ['name'],
  template: '<span class="icon-stub" :data-icon="name" />',
}

const RouterLinkStub = {
  name: 'RouterLink',
  props: ['to'],
  template: '<a :href="typeof to === \'string\' ? to : undefined"><slot /></a>',
}

const globalStubs = {
  Icon: IconStub,
  RouterLink: RouterLinkStub,
}

const openAIKey: EmbeddedMediaKeyOption = {
  id: 'openai-key',
  label: 'OpenAI media key',
  platform: 'openai',
  groupName: 'OpenAI media',
}

const grokKey: EmbeddedMediaKeyOption = {
  id: 'grok-key',
  label: 'Grok media key',
  platform: 'grok',
  groupName: 'Grok media',
}

function createRuntime(options: {
  keyOptions?: EmbeddedMediaKeyOption[]
  selectedKeyId?: string
  mediaPlatform?: string
  loadingKeys?: boolean
  isRunning?: boolean
  activeRun?: ApplicationRun | null
  localResult?: LocalMediaResult | null
  error?: string
  successMessage?: string
} = {}) {
  const keyOptions = ref(options.keyOptions ?? [openAIKey, grokKey])
  const initialSelectedKeyId = options.selectedKeyId ?? keyOptions.value[0]?.id ?? ''
  const selectedKeyId = ref(initialSelectedKeyId)
  const mediaPlatform = ref(options.mediaPlatform
    ?? keyOptions.value.find((option) => option.id === initialSelectedKeyId)?.platform
    ?? '')
  const loadingKeys = ref(options.loadingKeys ?? false)
  const isRunning = ref(options.isRunning ?? false)
  const activeRun = ref<ApplicationRun | null>(options.activeRun ?? null)
  const localResult = ref<LocalMediaResult | null>(options.localResult ?? null)
  const error = ref(options.error ?? '')
  const successMessage = ref(options.successMessage ?? '')
  const run = vi.fn()
  const resume = vi.fn()
  const stop = vi.fn()
  const refreshKeys = vi.fn()
  const clear = vi.fn()
  const setSelectedKeyId = vi.fn((value: string) => {
    selectedKeyId.value = value
    mediaPlatform.value = keyOptions.value.find((option) => option.id === value)?.platform ?? ''
  })
  const runtime: EmbeddedMediaRuntime = {
    keyOptions,
    selectedKeyId,
    mediaPlatform,
    loadingKeys,
    isRunning,
    activeRun,
    localResult,
    error,
    successMessage,
    setSelectedKeyId,
    run,
    resume,
    stop,
    refreshKeys,
    clear,
  }
  return {
    runtime,
    keyOptions,
    selectedKeyId,
    mediaPlatform,
    loadingKeys,
    isRunning,
    activeRun,
    localResult,
    error,
    successMessage,
    setSelectedKeyId,
    run,
    resume,
    stop,
    refreshKeys,
    clear,
  }
}

function mountPanel(runtime: EmbeddedMediaRuntime): VueWrapper {
  return mount(FreeMediaStudioPanel, {
    props: { runtime },
    global: { plugins: [createTestI18n()], stubs: globalStubs },
  })
}

function findMode(wrapper: VueWrapper, label: string) {
  return wrapper.findAll('button[aria-pressed]').find((button) => button.text().includes(label))
}

async function setReference(wrapper: VueWrapper, file: File): Promise<void> {
  const input = wrapper.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
}

describe('FreeMediaStudioPanel', () => {
  beforeEach(() => {
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      value: vi.fn((file: File) => `blob:${file.name}`),
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      value: vi.fn(),
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders from the required application runtime and starts with submission disabled', async () => {
    const fixture = createRuntime()
    const wrapper = mountPanel(fixture.runtime)
    await flushPromises()

    expect(wrapper.find('h1#free-media-studio-title').exists()).toBe(false)
    expect(wrapper.get('h2#free-media-studio-title').text()).toContain('自由媒体工作台')
    expect(wrapper.get('.key-control select').text()).toContain('OpenAI media key')
    expect(wrapper.get('.key-control select').text()).toContain('Grok media key')
    expect(wrapper.get('.primary-action').attributes('disabled')).toBeDefined()
    expect(wrapper.find('.history-section').exists()).toBe(false)

    wrapper.unmount()
  })

  it('submits a canonical text-to-image payload and delegates key refresh to runtime', async () => {
    const fixture = createRuntime()
    const wrapper = mountPanel(fixture.runtime)
    await flushPromises()

    await wrapper.get('#free-media-prompt').setValue('cyan product photograph')
    await wrapper.get('.primary-action').trigger('click')

    expect(fixture.run).toHaveBeenCalledTimes(1)
    expect(fixture.run).toHaveBeenCalledWith({
      values: {
        prompt: 'cyan product photograph',
        mode: 'text_to_image',
        model: 'gpt-image-2',
        n: 1,
        reference_count: 0,
        size: '1280x1024',
        quality: 'auto',
      },
      files: [],
    })

    await wrapper.get('.key-control .icon-button').trigger('click')
    expect(fixture.refreshKeys).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })

  it('shows every official Grok ratio and follows the source ratio for a single-image edit', async () => {
    const fixture = createRuntime({
      keyOptions: [grokKey],
      selectedKeyId: grokKey.id,
      mediaPlatform: 'grok',
    })
    const wrapper = mountPanel(fixture.runtime)
    await flushPromises()

    const ratioControl = () => wrapper.get('select[aria-label="画面比例"]')
    expect(ratioControl().findAll('option').map((option) => option.attributes('value')))
      .toEqual([...GROK_IMAGE_ASPECT_RATIOS])

    await findMode(wrapper, '图生图')!.trigger('click')
    await setReference(wrapper, new File(['one'], 'source-one.png', { type: 'image/png' }))
    expect(ratioControl().attributes('disabled')).toBeDefined()
    expect(ratioControl().text()).toBe('跟随原图')

    await setReference(wrapper, new File(['two'], 'source-two.png', { type: 'image/png' }))
    expect(ratioControl().attributes('disabled')).toBeUndefined()
    expect(ratioControl().findAll('option').map((option) => option.attributes('value')))
      .toEqual([...GROK_IMAGE_ASPECT_RATIOS])

    wrapper.unmount()
  })

  it('filters to Grok, preserves the original File, and delegates canonical image-to-video', async () => {
    const fixture = createRuntime()
    const wrapper = mountPanel(fixture.runtime)
    await flushPromises()

    const imageVideoMode = findMode(wrapper, '图生视频')
    expect(imageVideoMode).toBeDefined()
    await imageVideoMode!.trigger('click')
    await flushPromises()

    const keyOptions = wrapper.get('.key-control select').findAll('option')
    expect(keyOptions).toHaveLength(1)
    expect(keyOptions[0]?.text()).toContain('Grok media key')
    expect(unref(fixture.runtime.selectedKeyId)).toBe(grokKey.id)
    expect(fixture.setSelectedKeyId).toHaveBeenCalledWith(grokKey.id)

    const reference = new File(['reference'], 'start-frame.png', { type: 'image/png' })
    await setReference(wrapper, reference)
    await wrapper.get('#free-media-prompt').setValue('animate this exact frame')
    await wrapper.get('.primary-action').trigger('click')

    expect(fixture.run).toHaveBeenCalledTimes(1)
    const payload = fixture.run.mock.calls[0]![0]
    expect(payload.values).toEqual(expect.objectContaining({
      prompt: 'animate this exact frame',
      mode: 'image_to_video',
      model: 'grok-imagine-video-1.5',
      reference_count: 1,
    }))
    expect(payload.files).toHaveLength(1)
    expect(payload.files[0]).toBe(reference)

    wrapper.unmount()
  })

  it('clears the selected key and prevents video submission when runtime only has OpenAI', async () => {
    const fixture = createRuntime({
      keyOptions: [openAIKey],
      selectedKeyId: openAIKey.id,
      mediaPlatform: 'openai',
    })
    const wrapper = mountPanel(fixture.runtime)
    await flushPromises()

    const textVideoMode = findMode(wrapper, '文生视频')
    expect(textVideoMode).toBeDefined()
    await textVideoMode!.trigger('click')
    await wrapper.get('#free-media-prompt').setValue('must not submit through OpenAI')
    await flushPromises()

    const keySelect = wrapper.get('.key-control select')
    expect(keySelect.findAll('option')).toHaveLength(1)
    expect(keySelect.get('option').attributes('value')).toBe('')
    expect(keySelect.attributes('disabled')).toBeDefined()
    expect(fixture.selectedKeyId.value).toBe('')
    expect(fixture.mediaPlatform.value).toBe('')
    expect(wrapper.get('.primary-action').attributes('disabled')).toBeDefined()
    expect(fixture.run).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('locks controls while running and delegates stop without cancelling runtime state', async () => {
    const fixture = createRuntime({ isRunning: true })
    const wrapper = mountPanel(fixture.runtime)
    await flushPromises()

    expect(wrapper.get('.composer-fields').attributes('disabled')).toBeDefined()
    expect(wrapper.findAll('button[aria-pressed]')
      .every((button) => button.attributes('disabled') !== undefined)).toBe(true)
    const stopButton = wrapper.findAll('.secondary-action')
      .find((button) => button.text().includes('停止等待'))
    expect(stopButton).toBeDefined()
    await stopButton!.trigger('click')

    expect(fixture.stop).toHaveBeenCalledTimes(1)
    expect(fixture.run).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('renders runtime image results, messages, and delegates clear', async () => {
    const fixture = createRuntime({
      localResult: {
        type: 'images',
        images: [{ url: 'https://cdn.test/runtime-result.png', revisedPrompt: 'refined prompt' }],
      },
      successMessage: '应用运行完成',
    })
    const wrapper = mountPanel(fixture.runtime)
    await flushPromises()

    expect(wrapper.findAll('.result-item')).toHaveLength(1)
    expect(wrapper.get('.result-item img').attributes('src')).toBe('https://cdn.test/runtime-result.png')
    expect(wrapper.get('.studio-message').attributes('role')).toBe('status')
    expect(wrapper.get('.studio-message').text()).toContain('应用运行完成')

    const clearButton = wrapper.findAll('.secondary-action')
      .find((button) => button.text().includes('清空结果'))
    expect(clearButton).toBeDefined()
    await clearButton!.trigger('click')
    expect(fixture.clear).toHaveBeenCalledTimes(1)

    fixture.error.value = '网关运行失败'
    await nextTick()
    expect(wrapper.get('.studio-message').attributes('role')).toBe('alert')
    expect(wrapper.get('.studio-message').text()).toContain('网关运行失败')

    wrapper.unmount()
  })

  it('recovers a checkpointed runtime video task and renders the completed runtime result', async () => {
    const pendingRun: ApplicationRun = {
      id: 'run-video-1',
      app_slug: 'free-media-studio',
      status: 'running',
      steps: [{
        step_key: 'media',
        status: 'running',
        executor: 'video_generation',
        gateway_request_id: 'video-task-123',
      }],
    }
    const fixture = createRuntime({
      keyOptions: [grokKey],
      selectedKeyId: grokKey.id,
      mediaPlatform: 'grok',
      activeRun: pendingRun,
    })
    const wrapper = mountPanel(fixture.runtime)
    await flushPromises()

    const textVideoMode = findMode(wrapper, '文生视频')
    await textVideoMode!.trigger('click')
    await flushPromises()

    expect(wrapper.get('.video-task-state').text()).toContain('video-task-123')
    const resumeButton = wrapper.findAll('.video-task-state button')
      .find((button) => button.text().includes('继续查询'))
    expect(resumeButton).toBeDefined()
    await resumeButton!.trigger('click')
    expect(fixture.resume).toHaveBeenCalledTimes(1)

    fixture.localResult.value = {
      type: 'video',
      videoUrl: 'https://cdn.test/runtime-video.mp4',
      requestId: 'video-task-123',
    }
    await nextTick()
    expect(wrapper.find('video').exists()).toBe(false)
    const playButton = wrapper.get('.video-result button[aria-label="在线播放已关闭，请下载后查看"]')
    expect(playButton.attributes('disabled')).toBeDefined()
    expect(wrapper.get('.video-result a[download="kitty-video.mp4"]').attributes('href')).toBe('https://cdn.test/runtime-video.mp4')
    expect(wrapper.get('.video-caption code').text()).toBe('video-task-123')

    wrapper.unmount()
  })

  it.each(['failed', 'expired'] as const)('shows %s as terminal without a resume action', async (status) => {
    const fixture = createRuntime({
      keyOptions: [grokKey],
      selectedKeyId: grokKey.id,
      mediaPlatform: 'grok',
      activeRun: {
        id: `run-${status}`,
        app_slug: 'free-media-studio',
        status: 'failed',
        steps: [{
          step_key: 'media',
          status: 'failed',
          executor: 'video_generation',
          gateway_request_id: `video-${status}`,
          output: { request_id: `video-${status}`, status },
        }],
      },
    })
    const wrapper = mountPanel(fixture.runtime)
    await flushPromises()
    await findMode(wrapper, '文生视频')!.trigger('click')
    await flushPromises()

    expect(wrapper.get('.video-task-state').text()).toContain(`video-${status}`)
    expect(wrapper.findAll('.video-task-state button')
      .some((button) => button.text().includes('继续查询'))).toBe(false)
    expect(fixture.resume).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('restores a completed video from the persisted application output', async () => {
    const output = {
      request_id: 'video-persisted',
      status: 'completed',
      video_url: 'https://cdn.test/persisted-video.mp4',
    }
    const fixture = createRuntime({
      keyOptions: [grokKey],
      selectedKeyId: grokKey.id,
      mediaPlatform: 'grok',
      activeRun: {
        id: 'run-completed',
        app_slug: 'free-media-studio',
        status: 'completed',
        output,
        steps: [{
          step_key: 'media',
          status: 'completed',
          executor: 'video_generation',
          gateway_request_id: 'video-persisted',
          output,
        }],
      },
    })
    const wrapper = mountPanel(fixture.runtime)
    await flushPromises()
    await findMode(wrapper, '文生视频')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.get('.video-result button[aria-label="在线播放已关闭，请下载后查看"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('.video-result a[download="kitty-video.mp4"]').attributes('href')).toBe(output.video_url)
    expect(wrapper.get('.video-caption code').text()).toBe(output.request_id)
    expect(wrapper.findAll('.video-result button')
      .some((button) => button.text().includes('继续查询'))).toBe(false)
    wrapper.unmount()
  })
})
