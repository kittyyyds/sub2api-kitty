import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { computed, reactive, ref, type Component } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { videoGenerationAPI } from '@/api/videoGeneration'
import { SUANLI_VIDEO_MODELS } from '@/features/app-center/suanliVideo'
import type { ApiKey } from '@/types'
import KittyStudioView from '../KittyStudioView.vue'
import type { StudioForm } from '../types'

vi.mock('@/api/videoGeneration', () => ({
  videoGenerationAPI: { submitGateway: vi.fn(), status: vi.fn(), createContentURL: vi.fn() },
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ checkAuth: vi.fn(), isAuthenticated: true, isAdmin: true, isSimpleMode: false }),
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ siteName: 'Kitty', publicSettingsLoaded: true, brandLogoUrl: '', backendModeEnabled: false, cachedPublicSettings: null }),
}))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/composables/useDocumentDarkMode', () => ({ useDocumentDarkMode: () => ({ isDarkMode: ref(false) }) }))
vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn(), isLoading: { value: false } }),
}))
vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({ triggerPrefetch: vi.fn(), cancelPendingPrefetch: vi.fn(), resetPrefetchState: vi.fn() }),
}))
vi.mock('../useKittyStudio', () => ({ useKittyStudio: () => studio }))

const modelIDs = SUANLI_VIDEO_MODELS.map((model) => model.id)
const wrappers: VueWrapper[] = []
let studio: ReturnType<typeof createStudio>

function createStudio() {
  const form = reactive<StudioForm>({
    keyId: '1', mode: 'text-image', model: '', prompt: '', count: 1, ratio: '1:1',
    resolution: '2k', videoResolution: '720p', videoDuration: 8, format: 'png', background: 'auto',
  })
  const availableKeys = ref([
    { id: 1, name: 'Private video', key: 'private-fixture-key', group: { name: '算力私有组', platform: 'grok', is_exclusive: true } },
    { id: 2, name: 'Normal Grok', key: 'normal-fixture-key', group: { name: '普通组', platform: 'grok', is_exclusive: false } },
  ] as ApiKey[])
  const selectedKey = computed(() => availableKeys.value.find((key) => String(key.id) === form.keyId) || null)
  const gatewayModelIds = ref([...modelIDs])
  return {
    form, availableKeys, selectedKey, gatewayModelIds,
    selectedPlatform: computed(() => selectedKey.value?.group?.platform || ''),
    modelOptions: ref([]), referencePreviews: ref([]), gallery: ref([]), progress: ref([]),
    loadingKeys: ref(false), loadingModels: ref(false), loadingGallery: ref(false), generating: ref(false),
    canGenerate: ref(false), needsReference: ref(false), videoMode: ref(false), error: ref(''), notice: ref(''),
    loadKeys: vi.fn().mockResolvedValue(undefined), loadGallery: vi.fn().mockResolvedValue(undefined),
    setKey: vi.fn(async (value: string) => {
      form.keyId = value
      gatewayModelIds.value = value === '1' ? [...modelIDs] : ['grok-imagine-image']
    }),
    setMode: vi.fn(), updateForm: vi.fn(), addReferences: vi.fn(), removeReference: vi.fn(), generate: vi.fn(),
    stop: vi.fn(), clearGallery: vi.fn(), downloadResult: vi.fn(), copyPrompt: vi.fn(), removeResult: vi.fn(),
  }
}

async function mountAppsRoute(): Promise<VueWrapper> {
  const { default: router } = await import('@/router')
  const route = router.getRoutes().find((item) => item.name === 'AppCenter')
  expect(route?.path).toBe('/apps')
  const loadComponent = route?.components?.default as () => Promise<{ default: Component }>
  const component = await loadComponent()
  // This assertion catches accidentally wiring a component that /apps never imports.
  expect(component.default).toBe(KittyStudioView)
  const wrapper = mount(component.default, {
    global: { stubs: {
      RouterLink: { template: '<a><slot /></a>' },
      StudioCanvas: { template: '<div data-testid="ordinary-studio-canvas" />' },
    } },
  })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}

describe('/apps private Suanli video entry', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    studio = createStudio()
  })
  afterEach(() => wrappers.splice(0).forEach((wrapper) => wrapper.unmount()))

  it('renders all nine models through the actual lazy route without a paid request', async () => {
    const wrapper = await mountAppsRoute()
    expect(wrapper.get('[data-testid="suanli-model"]').findAll('option').map((item) => item.attributes('value'))).toEqual(modelIDs)
    expect(wrapper.find('.studio-controls').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ordinary-studio-canvas"]').exists()).toBe(false)
    expect(studio.loadKeys).toHaveBeenCalledOnce()
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
    expect(videoGenerationAPI.status).not.toHaveBeenCalled()
  })

  it('uses the same runner for video-platform groups without requiring an exclusive flag', async () => {
    studio.availableKeys.value[0].group!.platform = 'video'
    studio.availableKeys.value[0].group!.is_exclusive = false
    const wrapper = await mountAppsRoute()
    expect(wrapper.find('[data-testid="suanli-special-video-runner"]').exists()).toBe(true)
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
  })

  it.each([
    ['public group', false, 'grok', modelIDs],
    ['another platform', true, 'openai', modelIDs],
    ['mixed model catalog', true, 'grok', [...modelIDs, 'grok-imagine-video']],
    ['empty model catalog', true, 'grok', []],
  ] as const)('keeps the existing Studio for %s', async (_name, exclusive, platform, catalog) => {
    studio.availableKeys.value[0].group!.is_exclusive = exclusive
    studio.availableKeys.value[0].group!.platform = platform
    studio.gatewayModelIds.value = [...catalog]
    const wrapper = await mountAppsRoute()
    expect(wrapper.find('[data-testid="suanli-special-video-runner"]').exists()).toBe(false)
    expect(wrapper.find('.studio-controls').exists()).toBe(true)
    expect(wrapper.find('[data-testid="ordinary-studio-canvas"]').exists()).toBe(true)
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
  })

  it('switches between private video and ordinary Studio using the existing key selector flow', async () => {
    const wrapper = await mountAppsRoute()
    await wrapper.get('[data-testid="studio-suanli-key"]').setValue('2')
    await flushPromises()
    expect(studio.setKey).toHaveBeenLastCalledWith('2')
    expect(wrapper.find('.studio-controls').exists()).toBe(true)
    await wrapper.get('.studio-controls select').setValue('1')
    await flushPromises()
    expect(studio.setKey).toHaveBeenLastCalledWith('1')
    expect(wrapper.find('[data-testid="suanli-special-video-runner"]').exists()).toBe(true)
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
  })

  it('locks key changes and refresh while the real runner is awaiting its task ID', async () => {
    let completeSubmission!: (result: { requestId: string }) => void
    vi.mocked(videoGenerationAPI.submitGateway).mockReturnValue(new Promise((resolve) => { completeSubmission = resolve }))
    vi.mocked(videoGenerationAPI.status).mockResolvedValue({ requestId: 'route-task', status: 'failed', videoUrl: '', errorMessage: 'fixture complete' })
    const wrapper = await mountAppsRoute()
    await wrapper.get('textarea').setValue('鹈鹕骑自行车')
    await wrapper.get('[data-testid="suanli-images"]').setValue('https://example.com/reference.jpg')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('任务正在提交，请等待返回任务 ID。')
    const keySelect = wrapper.get('[data-testid="studio-suanli-key"]')
    expect(keySelect.attributes()).toHaveProperty('disabled')
    expect(wrapper.get('[data-testid="studio-suanli-refresh"]').attributes()).toHaveProperty('disabled')
    await keySelect.setValue('2')
    await wrapper.get('[data-testid="studio-suanli-refresh"]').trigger('click')
    expect(studio.form.keyId).toBe('1')
    expect(studio.setKey).not.toHaveBeenCalled()
    expect(studio.loadKeys).toHaveBeenCalledOnce()
    completeSubmission({ requestId: 'route-task' })
    await flushPromises()
    expect(videoGenerationAPI.status).toHaveBeenCalledWith('private-fixture-key', 'route-task', expect.any(AbortSignal))
    expect(keySelect.attributes()).not.toHaveProperty('disabled')
  })
})
