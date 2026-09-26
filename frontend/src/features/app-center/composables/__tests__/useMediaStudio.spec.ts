import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  useMediaStudio,
  validateCustomImageSize,
} from '../useMediaStudio'

type Studio = ReturnType<typeof useMediaStudio>

interface StudioFixture {
  wrapper: VueWrapper
  studio: Studio
  platform: Ref<string>
  running: Ref<boolean>
}

function mountStudio(options: { platform?: string; running?: boolean } = {}): StudioFixture {
  const platform = ref(options.platform ?? 'openai')
  const running = ref(options.running ?? false)
  let studio: Studio | undefined
  const Harness = defineComponent({
    setup() {
      studio = useMediaStudio((key) => key, {
        mediaPlatform: () => platform.value,
        isRunning: () => running.value,
      })
      return () => h('div')
    },
  })
  const wrapper = mount(Harness)
  return { wrapper, studio: studio!, platform, running }
}

function imageFile(name: string, type = 'image/png'): File {
  return new File(['image'], name, { type })
}

describe('useMediaStudio', () => {
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

  it('builds a canonical OpenAI text-to-image payload and clamps the count', () => {
    const { wrapper, studio } = mountStudio()
    studio.form.prompt = '  titanium product photograph  '
    studio.form.count = 99
    studio.form.ratio = '5:4'
    studio.form.quality = 'high'

    expect(studio.canGenerate.value).toBe(true)
    expect(studio.buildApplicationPayload()).toEqual({
      values: {
        prompt: 'titanium product photograph',
        mode: 'text_to_image',
        model: 'gpt-image-2',
        n: 8,
        reference_count: 0,
        size: '1280x1024',
        quality: 'high',
      },
      files: [],
    })

    wrapper.unmount()
  })

  it('uses platform-specific image counts and narrows the value when the platform changes', async () => {
    const { wrapper, studio, platform } = mountStudio({ platform: 'grok' })
    studio.form.prompt = 'platform count contract'
    studio.form.count = 8

    expect(studio.imageCountOptions.value).toEqual([1, 2, 3, 4])
    expect(studio.buildApplicationPayload()?.values.n).toBe(4)

    platform.value = 'openai'
    await nextTick()
    expect(studio.imageCountOptions.value).toEqual([1, 2, 3, 4, 5, 6, 7, 8])
    studio.form.count = 8
    expect(studio.buildApplicationPayload()?.values.n).toBe(8)

    platform.value = 'grok'
    await nextTick()
    expect(studio.form.count).toBe(4)

    wrapper.unmount()
  })

  it('sends an official Grok aspect ratio for text-to-image generation', () => {
    const { wrapper, studio } = mountStudio({ platform: 'grok' })
    studio.form.prompt = 'wide cinematic skyline'
    studio.form.ratio = '19.5:9'

    expect(studio.buildApplicationPayload()?.values).toEqual(expect.objectContaining({
      mode: 'text_to_image',
      aspect_ratio: '19.5:9',
      resolution: '1k',
    }))

    wrapper.unmount()
  })

  it('validates custom image dimensions before creating a payload', () => {
    const { wrapper, studio } = mountStudio()
    studio.form.prompt = 'custom canvas'
    studio.form.ratio = 'custom'
    studio.form.customWidth = 1000
    studio.form.customHeight = 1024

    expect(studio.canGenerate.value).toBe(false)
    expect(studio.buildApplicationPayload()).toBeNull()
    expect(studio.generationError.value).toBe('native.studio.error.sizeMultiple')

    studio.form.customWidth = 1024
    expect(studio.buildApplicationPayload()?.values).toEqual(expect.objectContaining({
      mode: 'text_to_image',
      size: '1024x1024',
    }))

    wrapper.unmount()
  })

  it('filters invalid references, caps OpenAI image edits at five, and preserves File identity', () => {
    const { wrapper, studio } = mountStudio()
    studio.setMode('image')
    studio.form.prompt = 'compose these references'

    const invalid = new File(['text'], 'notes.txt', { type: 'text/plain' })
    const oversized = imageFile('oversized.png')
    Object.defineProperty(oversized, 'size', { configurable: true, value: 20 * 1024 * 1024 + 1 })
    const first = imageFile('first.png')
    const extensionOnly = imageFile('extension-only.webp', '')
    studio.addReferenceFiles([invalid, oversized, first, extensionOnly])

    expect(studio.references.value.map((reference) => reference.file)).toEqual([first, extensionOnly])
    expect(studio.referenceError.value).toBe('native.studio.error.referenceSize')

    const extra = Array.from({ length: 4 }, (_, index) => imageFile(`extra-${index}.jpg`, 'image/jpeg'))
    studio.addReferenceFiles(extra)
    expect(studio.references.value).toHaveLength(5)
    expect(studio.referenceError.value).toBe('native.studio.error.referenceTrimmed')

    const payload = studio.buildApplicationPayload()
    expect(payload?.values).toEqual(expect.objectContaining({
      mode: 'image_edit',
      model: 'gpt-image-2',
      reference_count: 5,
    }))
    expect(payload?.files).toEqual([first, extensionOnly, ...extra.slice(0, 3)])
    expect(payload?.files[0]).toBe(first)

    wrapper.unmount()
    expect(URL.revokeObjectURL).toHaveBeenCalledTimes(5)
  })

  it('lets a single Grok image edit follow the source image without sending aspect_ratio', () => {
    const { wrapper, studio } = mountStudio({ platform: 'grok' })
    studio.setMode('image')
    studio.form.prompt = 'keep the source framing'
    studio.form.ratio = '16:9'
    const source = imageFile('single-source.png')
    studio.addReferenceFiles([source])

    expect(studio.usesSourceImageRatio.value).toBe(true)
    expect(studio.requestSize.value).toContain('native.studio.ratioOption.source')
    const payload = studio.buildApplicationPayload()
    expect(payload?.values).toEqual(expect.objectContaining({
      mode: 'image_edit',
      reference_count: 1,
      resolution: '1k',
    }))
    expect(payload?.values).not.toHaveProperty('aspect_ratio')
    expect(payload?.files).toEqual([source])

    wrapper.unmount()
  })

  it('builds a multi-reference Grok image-edit payload with the selected official ratio and limit', async () => {
    const { wrapper, studio, platform } = mountStudio({ platform: 'grok' })
    studio.setMode('image')
    studio.form.prompt = 'restyle the product'
    studio.form.imageModel = 'grok-imagine-image'
    studio.form.imageResolution = '2k'
    studio.form.ratio = '2:1'
    const files = Array.from({ length: 5 }, (_, index) => imageFile(`grok-${index}.png`))
    studio.addReferenceFiles(files)
    await nextTick()

    expect(studio.availableImageModels.value.map((model) => model.model)).toEqual([
      'grok-imagine-image-quality',
      'grok-imagine-image',
    ])
    expect(studio.referenceLimit.value).toBe(3)
    expect(studio.references.value.map((reference) => reference.file)).toEqual(files.slice(0, 3))
    expect(studio.usesSourceImageRatio.value).toBe(false)
    expect(studio.form.ratio).toBe('2:1')
    expect(studio.buildApplicationPayload()).toEqual({
      values: {
        prompt: 'restyle the product',
        mode: 'image_edit',
        model: 'grok-imagine-image',
        n: 1,
        reference_count: 3,
        aspect_ratio: '2:1',
        resolution: '2k',
      },
      files: files.slice(0, 3),
    })

    platform.value = 'openai'
    await nextTick()
    expect(studio.availableImageModels.value.map((model) => model.model)).toEqual(['gpt-image-2'])

    wrapper.unmount()
  })

  it('builds canonical Grok text-to-video fields and allows 1080p for model 1.5', async () => {
    const { wrapper, studio } = mountStudio({ platform: 'grok' })
    studio.setMode('text-video')
    studio.form.prompt = 'slow orbital camera move'
    studio.form.videoModel = 'grok-imagine-video-1.5'
    studio.form.videoResolution = '1080p'
    studio.form.videoDuration = 22
    await nextTick()

    expect(studio.availableVideoResolutions.value).toEqual(['480p', '720p', '1080p'])
    expect(studio.form.videoResolution).toBe('1080p')
    expect(studio.buildApplicationPayload()).toEqual({
      values: {
        prompt: 'slow orbital camera move',
        mode: 'text_to_video',
        model: 'grok-imagine-video-1.5',
        n: 1,
        reference_count: 0,
        duration: 15,
        resolution: '1080p',
        aspect_ratio: '16:9',
      },
      files: [],
    })

    wrapper.unmount()
  })

  it('builds image-to-video with one original File and only allows 1080p for model 1.5', async () => {
    const { wrapper, studio } = mountStudio({ platform: 'grok' })
    studio.setMode('image-video')
    studio.form.prompt = 'animate this first frame'
    studio.form.videoResolution = '1080p'
    studio.form.videoDuration = 1
    studio.form.ratio = '9:16'
    const files = [imageFile('first-frame.png'), imageFile('ignored-frame.png')]
    studio.addReferenceFiles(files)

    expect(studio.form.videoModel).toBe('grok-imagine-video-1.5')
    expect(studio.availableVideoResolutions.value).toEqual(['480p', '720p', '1080p'])
    expect(studio.references.value).toHaveLength(1)
    expect(studio.references.value[0]?.file).toBe(files[0])
    expect(studio.buildApplicationPayload()).toEqual({
      values: {
        prompt: 'animate this first frame',
        mode: 'image_to_video',
        model: 'grok-imagine-video-1.5',
        n: 1,
        reference_count: 1,
        duration: 1,
        resolution: '1080p',
        aspect_ratio: '9:16',
      },
      files: [files[0]],
    })

    studio.form.videoModel = 'grok-imagine-video'
    expect(studio.availableVideoResolutions.value).toEqual(['480p', '720p'])
    expect(studio.buildApplicationPayload()?.values.resolution).toBe('480p')
    expect(studio.form.videoResolution).toBe('480p')

    wrapper.unmount()
  })

  it('rejects missing inputs and non-Grok video before producing a payload', async () => {
    const { wrapper, studio, platform } = mountStudio({ platform: '' })

    expect(studio.buildApplicationPayload()).toBeNull()
    expect(studio.generationError.value).toBe('native.studio.error.prompt')

    studio.form.prompt = 'missing platform'
    expect(studio.buildApplicationPayload()).toBeNull()
    expect(studio.generationError.value).toBe('native.studio.error.key')

    platform.value = 'openai'
    await nextTick()
    studio.setMode('text-video')
    expect(studio.buildApplicationPayload()).toBeNull()
    expect(studio.generationError.value).toBe('native.studio.error.videoKey')

    platform.value = 'grok'
    await nextTick()
    studio.setMode('image-video')
    expect(studio.buildApplicationPayload()).toBeNull()
    expect(studio.generationError.value).toBe('native.studio.error.videoReferenceRequired')

    studio.setMode('image')
    expect(studio.buildApplicationPayload()).toBeNull()
    expect(studio.generationError.value).toBe('native.studio.error.referenceRequired')

    wrapper.unmount()
  })

  it('locks mode and reference mutations while the application runtime is running', () => {
    const { wrapper, studio, running } = mountStudio({ running: true })
    studio.form.prompt = 'locked request'
    const reference = imageFile('locked.png')

    expect(studio.canGenerate.value).toBe(false)
    studio.setMode('image')
    studio.addReferenceFiles([reference])
    expect(studio.form.mode).toBe('text')
    expect(studio.references.value).toEqual([])

    running.value = false
    studio.setMode('image')
    studio.addReferenceFiles([reference])
    const referenceId = studio.references.value[0]!.id
    running.value = true
    studio.removeReference(referenceId)
    expect(studio.references.value[0]?.file).toBe(reference)

    wrapper.unmount()
  })

  it('validates the verified custom-size boundaries', () => {
    expect(validateCustomImageSize(1024, 1024)).toBe('')
    expect(validateCustomImageSize(1000, 1024)).toContain('16 的倍数')
    expect(validateCustomImageSize(3840, 1024)).toContain('3:1')
    expect(validateCustomImageSize(512, 512)).toContain('总像素')
    expect(validateCustomImageSize(4096, 2048)).toContain('3840px')
  })
})
