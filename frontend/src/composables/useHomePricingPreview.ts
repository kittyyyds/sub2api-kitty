import { computed, onBeforeUnmount, readonly, shallowRef, toValue, watch } from 'vue'
import type { MaybeRefOrGetter } from 'vue'

import { getModelPlaza, type ModelPlazaGroup } from '@/api/modelPlaza'

export type HomePricingPlatform = 'openai' | 'anthropic' | 'grok'

export interface HomePricingSummary {
  platform: HomePricingPlatform
  label: string
  iconModel: string
  rateMultiplier: number
  groupName: string
  subscriptionType: string
  modelCount: number
  featuredModel: string
  imageRateMultiplier: number | null
  videoRateMultiplier: number | null
}

interface PlatformBlueprint {
  platform: HomePricingPlatform
  label: string
  iconModel: string
  preferredModels: RegExp[]
}

interface UseHomePricingPreviewOptions {
  enabled?: MaybeRefOrGetter<boolean>
}

const platformBlueprints: PlatformBlueprint[] = [
  {
    platform: 'openai',
    label: 'GPT / Codex',
    iconModel: 'gpt-5.6',
    preferredModels: [/gpt-5\.6-sol/i, /gpt-5\.6/i, /gpt-5\.5/i],
  },
  {
    platform: 'anthropic',
    label: 'Claude / Kiro',
    iconModel: 'claude-sonnet',
    preferredModels: [/claude-sonnet-5/i, /claude-sonnet/i, /claude-opus/i],
  },
  {
    platform: 'grok',
    label: 'Grok',
    iconModel: 'grok-4.5',
    preferredModels: [/grok-4\.5/i, /grok-4\.3/i, /grok-4/i],
  },
]

function effectiveRate(group: ModelPlazaGroup): number {
  return group.user_rate_multiplier ?? group.rate_multiplier
}

function preferredModel(group: ModelPlazaGroup, patterns: RegExp[]): string {
  for (const pattern of patterns) {
    const matched = group.models.find((model) => pattern.test(model.name))
    if (matched) return matched.name
  }
  return group.models[0]?.name ?? ''
}

function supportsBillingMode(group: ModelPlazaGroup, mode: 'image' | 'video'): boolean {
  return group.models.some((model) => model.pricing?.billing_mode === mode)
}

function effectiveMediaRate(group: ModelPlazaGroup, mode: 'image' | 'video'): number | null {
  if (!supportsBillingMode(group, mode)) return null

  const independent = mode === 'image'
    ? group.image_rate_independent
    : group.video_rate_independent
  const rate = independent
    ? (mode === 'image' ? group.image_rate_multiplier : group.video_rate_multiplier)
    : effectiveRate(group)

  return Number.isFinite(rate) && rate >= 0 ? rate : null
}

export function buildHomePricingSummaries(groups: ModelPlazaGroup[]): HomePricingSummary[] {
  return platformBlueprints.flatMap((blueprint) => {
    const candidates = groups
      .filter((group) => group.platform === blueprint.platform && effectiveRate(group) >= 0)
      .sort((left, right) => effectiveRate(left) - effectiveRate(right))

    const group = candidates[0]
    if (!group) return []

    return [{
      platform: blueprint.platform,
      label: blueprint.label,
      iconModel: blueprint.iconModel,
      rateMultiplier: effectiveRate(group),
      groupName: group.name,
      subscriptionType: group.subscription_type,
      modelCount: group.models.length,
      featuredModel: preferredModel(group, blueprint.preferredModels),
      imageRateMultiplier: effectiveMediaRate(group, 'image'),
      videoRateMultiplier: effectiveMediaRate(group, 'video'),
    }]
  })
}

export function useHomePricingPreview(options: UseHomePricingPreviewOptions = {}) {
  const groups = shallowRef<ModelPlazaGroup[]>([])
  const loading = shallowRef(false)
  const failed = shallowRef(false)
  let controller: AbortController | null = null

  const summaries = computed(() => buildHomePricingSummaries(groups.value))
  const groupCount = computed(() => groups.value.length)
  const modelCount = computed(() => new Set(
    groups.value.flatMap((group) => group.models.map((model) => model.name)),
  ).size)

  async function load(): Promise<void> {
    if (toValue(options.enabled ?? true) === false) return

    controller?.abort()
    const requestController = new AbortController()
    controller = requestController
    loading.value = true
    failed.value = false

    try {
      const response = await getModelPlaza({ signal: requestController.signal })
      groups.value = response.groups
    } catch (error) {
      if ((error as { name?: string }).name !== 'CanceledError' &&
          (error as { name?: string }).name !== 'AbortError') {
        failed.value = true
      }
    } finally {
      if (controller === requestController) {
        controller = null
        loading.value = false
      }
    }
  }

  watch(
    () => toValue(options.enabled ?? true),
    (enabled) => {
      if (enabled) {
        void load()
        return
      }

      controller?.abort()
      controller = null
      groups.value = []
      loading.value = false
      failed.value = false
    },
    { immediate: true },
  )
  onBeforeUnmount(() => controller?.abort())

  return {
    summaries,
    groupCount,
    modelCount,
    loading: readonly(loading),
    failed: readonly(failed),
    reload: load,
  }
}
