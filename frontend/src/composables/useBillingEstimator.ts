import { computed, onMounted, onUnmounted, reactive, shallowRef, watch } from 'vue'

import {
  getBillingCatalog,
  previewBilling,
  type BillingCatalogGroup,
  type BillingCatalogModel,
  type BillingPreviewRequest,
  type BillingPreviewResult,
} from '@/api/billingPreview'

const PREVIEW_DEBOUNCE_MS = 320
const MAX_PREVIEW_TOKENS = 1_000_000_000

export interface BillingEstimatorTokens {
  input: number
  output: number
  cacheWrite5m: number
  cacheWrite1h: number
  cacheRead: number
}

export interface BillingWorkloadPreset {
  id: 'chat' | 'coding' | 'document'
  input: number
  output: number
}

type TokenField = keyof BillingEstimatorTokens
type ServiceTier = BillingPreviewRequest['service_tier']

function modelKey(model: BillingCatalogModel): string {
  return JSON.stringify([model.platform, model.name])
}

function toToken(value: unknown): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return 0
  return Math.min(MAX_PREVIEW_TOKENS, Math.max(0, Math.floor(parsed)))
}

function isCanceled(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const candidate = error as { code?: string; name?: string }
  return candidate.code === 'ERR_CANCELED' || candidate.name === 'CanceledError' || candidate.name === 'AbortError'
}

export function useBillingEstimator() {
  const groups = shallowRef<BillingCatalogGroup[]>([])
  const selectedGroupId = shallowRef<number | null>(null)
  const selectedModelKey = shallowRef<string | null>(null)
  const serviceTier = shallowRef<ServiceTier>('')
  const advancedOpen = shallowRef(false)
  const catalogLoading = shallowRef(true)
  const estimating = shallowRef(false)
  const catalogError = shallowRef(false)
  const previewError = shallowRef(false)
  const result = shallowRef<BillingPreviewResult | null>(null)

  const tokens = reactive<BillingEstimatorTokens>({
    input: 1_000_000,
    output: 500_000,
    cacheWrite5m: 0,
    cacheWrite1h: 0,
    cacheRead: 0,
  })

  const selectedGroup = computed(() =>
    groups.value.find((group) => group.id === selectedGroupId.value) ?? null,
  )
  const selectedModel = computed(() =>
    selectedGroup.value?.models.find((model) => modelKey(model) === selectedModelKey.value) ?? null,
  )
  const hasCatalog = computed(() => groups.value.length > 0)
  const canEstimate = computed(() => selectedGroup.value !== null && selectedModel.value !== null)

  let catalogController: AbortController | null = null
  let previewController: AbortController | null = null
  let debounceTimer: ReturnType<typeof setTimeout> | null = null
  let previewSequence = 0

  function ensureSelection(): void {
    let group = selectedGroup.value
    if (!group) {
      group = groups.value[0] ?? null
      selectedGroupId.value = group?.id ?? null
    }
    if (!group) {
      selectedModelKey.value = null
      return
    }
    const currentModel = group.models.find((model) => modelKey(model) === selectedModelKey.value)
    if (!currentModel) {
      selectedModelKey.value = group.models[0] ? modelKey(group.models[0]) : null
    }
  }

  async function loadCatalog(): Promise<void> {
    catalogController?.abort()
    const controller = new AbortController()
    catalogController = controller
    catalogLoading.value = true
    catalogError.value = false
    try {
      const response = await getBillingCatalog({ signal: controller.signal })
      groups.value = response.groups ?? []
      ensureSelection()
      scheduleEstimate()
    } catch (error) {
      if (isCanceled(error)) return
      catalogError.value = true
      groups.value = []
      selectedGroupId.value = null
      selectedModelKey.value = null
      result.value = null
    } finally {
      if (catalogController === controller) catalogLoading.value = false
    }
  }

  function buildRequest(): BillingPreviewRequest | null {
    const group = selectedGroup.value
    const model = selectedModel.value
    if (!group || !model) return null
    const cacheWrite5m = toToken(tokens.cacheWrite5m)
    const cacheWrite1h = toToken(tokens.cacheWrite1h)
    return {
      group_id: group.id,
      model: model.name,
      model_platform: model.platform,
      input_tokens: toToken(tokens.input),
      output_tokens: toToken(tokens.output),
      cache_creation_tokens: toToken(cacheWrite5m + cacheWrite1h),
      cache_read_tokens: toToken(tokens.cacheRead),
      cache_creation_5m_tokens: cacheWrite5m,
      cache_creation_1h_tokens: cacheWrite1h,
      service_tier: serviceTier.value,
    }
  }

  async function estimateNow(): Promise<void> {
    const request = buildRequest()
    if (!request) {
      result.value = null
      previewError.value = false
      estimating.value = false
      return
    }

    previewController?.abort()
    const controller = new AbortController()
    previewController = controller
    const sequence = ++previewSequence
    estimating.value = true
    previewError.value = false
    try {
      const nextResult = await previewBilling(request, { signal: controller.signal })
      if (sequence === previewSequence) result.value = nextResult
    } catch (error) {
      if (isCanceled(error)) return
      if (sequence === previewSequence) previewError.value = true
    } finally {
      if (sequence === previewSequence) {
        estimating.value = false
        previewController = null
      } else if (previewController === controller) {
        previewController = null
      }
    }
  }

  function scheduleEstimate(): void {
    if (debounceTimer) clearTimeout(debounceTimer)
    previewController?.abort()
    previewController = null
    previewSequence += 1
    previewError.value = false
    estimating.value = canEstimate.value
    debounceTimer = setTimeout(() => {
      debounceTimer = null
      void estimateNow()
    }, PREVIEW_DEBOUNCE_MS)
  }

  function sanitizeToken(field: TokenField): void {
    tokens[field] = toToken(tokens[field])
  }

  function applyPreset(preset: BillingWorkloadPreset): void {
    tokens.input = preset.input
    tokens.output = preset.output
  }

  function reset(): void {
    tokens.input = 1_000_000
    tokens.output = 500_000
    tokens.cacheWrite5m = 0
    tokens.cacheWrite1h = 0
    tokens.cacheRead = 0
    serviceTier.value = ''
  }

  function toggleAdvanced(): void {
    advancedOpen.value = !advancedOpen.value
  }

  watch(selectedGroupId, () => {
    ensureSelection()
  })

  watch(
    [
      selectedGroupId,
      selectedModelKey,
      serviceTier,
      () => tokens.input,
      () => tokens.output,
      () => tokens.cacheWrite5m,
      () => tokens.cacheWrite1h,
      () => tokens.cacheRead,
    ],
    scheduleEstimate,
    { flush: 'post' },
  )

  onMounted(() => {
    void loadCatalog()
  })

  onUnmounted(() => {
    catalogController?.abort()
    previewController?.abort()
    if (debounceTimer) clearTimeout(debounceTimer)
  })

  return {
    groups,
    selectedGroupId,
    selectedModelKey,
    selectedGroup,
    selectedModel,
    serviceTier,
    advancedOpen,
    catalogLoading,
    estimating,
    catalogError,
    previewError,
    result,
    tokens,
    hasCatalog,
    canEstimate,
    modelKey,
    loadCatalog,
    estimateNow,
    sanitizeToken,
    applyPreset,
    reset,
    toggleAdvanced,
  }
}
