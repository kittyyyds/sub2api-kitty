import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { keysAPI } from '@/api/keys'
import { imageGenerationAPI } from '@/api/imageGeneration'
import * as gateway from '@/features/ai-workbench/gateway'
import type { ApiKey } from '@/types'
import { useKittyStudio } from '../useKittyStudio'
import { generateStudioAsset } from '../studioGateway'

const keys = [1, 2].map((id) => ({
  id, key: 'test-key-' + id, status: 'active',
  group: { id, platform: 'openai', allow_image_generation: true },
})) as ApiKey[]
const models = (id: string) => [{ id, displayName: id }]
let unmount: (() => void) | undefined

function setup() {
  let studio!: ReturnType<typeof useKittyStudio>
  const wrapper = mount(defineComponent({
    setup() { studio = useKittyStudio(); return () => null },
  }))
  unmount = () => wrapper.unmount()
  vi.spyOn(keysAPI, 'list').mockResolvedValue({ items: keys, total: 2, page: 1, page_size: 100, pages: 1 })
  return studio
}

afterEach(() => { unmount?.(); vi.restoreAllMocks(); localStorage.clear() })

describe('Kitty Studio Image 2.5 flow', () => {
  it('restores the selected variant after a page reload', async () => {
    localStorage.setItem('kitty-studio:draft:v1', JSON.stringify({ keyId: '1', model: 'gpt-image-2.5-flare' }))
    const studio = setup()
    vi.spyOn(gateway, 'listGatewayModels').mockResolvedValue([
      ...models('gpt-image-2'), ...models('gpt-image-2.5-flare'),
    ])
    await studio.loadKeys()
    await nextTick()
    expect(studio.form.model).toBe('gpt-image-2.5-flare')
  })

  it('clears previous group options and ignores its late response when changing Keys', async () => {
    const studio = setup()
    const list = vi.spyOn(gateway, 'listGatewayModels').mockResolvedValue(models('gpt-image-2.5-flare'))
    await studio.loadKeys()
    studio.form.prompt = 'a red cup'
    expect(studio.canGenerate.value).toBe(true)
    let resolveOld!: (value: Awaited<ReturnType<typeof gateway.listGatewayModels>>) => void
    list.mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
    const oldRequest = studio.loadModels()
    expect(studio.modelOptions.value).toEqual([])
    expect(studio.canGenerate.value).toBe(false)
    list.mockResolvedValueOnce(models('gpt-image-2'))
    await studio.setKey('2')
    resolveOld(models('gpt-image-2.5-sunburst'))
    await oldRequest
    expect(studio.form.model).toBe('gpt-image-2')
    expect(studio.modelOptions.value.map((item) => item.id)).toEqual(['gpt-image-2'])
  })

  it.each(['gpt-image-2.5', 'gpt-image-2.5-flare', 'gpt-image-2.5-sunburst'])('submits the exact %s model for generation and reference edits', async (id) => {
    const studio = setup()
    vi.spyOn(gateway, 'listGatewayModels').mockResolvedValue(models(id))
    await studio.loadKeys()
    const submit = vi.spyOn(imageGenerationAPI, 'submitAsync').mockResolvedValue({ taskId: 'test-task', status: 'processing', pollUrl: '/v1/images/tasks/test-task' })
    vi.spyOn(imageGenerationAPI, 'pollAsync').mockResolvedValue([{ url: 'data:image/png;base64,YQ==' }])
    for (const mode of ['text-image', 'image-image'] as const) {
      const references = mode === 'image-image' ? [new File(['image'], 'reference.png', { type: 'image/png' })] : []
      await generateStudioAsset({
        apiKey: keys[0]!.key, model: studio.selectedModel.value!,
        form: { ...studio.form, mode, prompt: 'a red cup', resolution: '1k' },
        references, signal: new AbortController().signal,
      })
      expect(submit).toHaveBeenLastCalledWith(
        keys[0]!.key, mode === 'image-image' ? '/v1/images/edits/async' : '/v1/images/generations/async',
        expect.objectContaining({ model: id, prompt: 'a red cup', size: '1024x1024', n: 1 }),
        expect.objectContaining({ referenceImages: references }),
      )
    }
  })
})
