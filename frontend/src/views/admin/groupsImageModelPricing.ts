export const grokImagePriceResolutions = [
  { key: '1K', label: '1K' },
  { key: '2K', label: '2K' },
  { key: '4K', label: '4K' }
] as const

export const grokImagePriceModels = [
  { key: 'grok-imagine', label: 'grok-imagine' },
  { key: 'grok-imagine-image', label: 'grok-imagine-image' },
  { key: 'grok-imagine-image-2.0', label: 'grok-imagine-image-2.0' },
  { key: 'grok-imagine-image-quality', label: 'grok-imagine-image-quality' }
] as const

export const openAIImagePriceModels = [
  { key: 'gpt-image-1', label: 'gpt-image-1' },
  { key: 'gpt-image-1.5', label: 'gpt-image-1.5' },
  { key: 'gpt-image-2', label: 'gpt-image-2' },
  { key: 'gpt-image-2.5', label: 'gpt-image-2.5' },
  { key: 'gpt-image-2.5-flare', label: 'gpt-image-2.5-flare' },
  { key: 'gpt-image-2.5-sunburst', label: 'gpt-image-2.5-sunburst' }
] as const

const imagePriceModelsByPlatform: Record<
  string,
  ReadonlyArray<{ key: string; label: string }>
> = {
  grok: grokImagePriceModels,
  openai: openAIImagePriceModels
}

const allImagePriceModels = [
  ...grokImagePriceModels,
  ...openAIImagePriceModels
]

export function supportsImageModelPricingPlatform(platform: string): boolean {
  return platform in imagePriceModelsByPlatform
}

export type ImageModelPrices = Record<string, Record<string, number>>
export type ImageModelPricesForm = Record<string, Record<string, number | string | null>>

// All prices are USD. Populated model cells are final customer prices.
const defaultPrices: ImageModelPrices = {
  'grok-imagine': { '1K': 0.05, '2K': 0.07, '4K': 0.07 },
  'grok-imagine-image': { '1K': 0.02, '2K': 0.02, '4K': 0.02 },
  'grok-imagine-image-2.0': { '1K': 0.06, '2K': 0.08, '4K': 0.08 },
  'grok-imagine-image-quality': { '1K': 0.05, '2K': 0.07, '4K': 0.07 }
}

function normalizeModel(value: string): string {
  return value.trim().toLowerCase()
}

function normalizeResolution(value: string): string {
  return value.trim().toUpperCase()
}

function normalizePrice(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const price = Number(value)
  return Number.isFinite(price) && price >= 0 ? price : null
}

function emptyTiers(): Record<string, number | string | null> {
  return Object.fromEntries(grokImagePriceResolutions.map(({ key }) => [key, null]))
}

// Unknown model rows are retained so editing another group field never drops
// prices for a model introduced by a newer backend.
export function createImageModelPricesForm(
  prices?: ImageModelPrices | null
): ImageModelPricesForm {
  const form: ImageModelPricesForm = {}

  for (const [rawModel, rawTiers] of Object.entries(prices ?? {})) {
    const model = normalizeModel(rawModel)
    if (!model || !rawTiers || typeof rawTiers !== 'object') continue
    form[model] = emptyTiers()
    for (const [rawResolution, rawPrice] of Object.entries(rawTiers)) {
      const resolution = normalizeResolution(rawResolution)
      const price = normalizePrice(rawPrice)
      if (resolution && price !== null) form[model][resolution] = price
    }
  }

  for (const { key } of allImagePriceModels) {
    form[key] ??= emptyTiers()
  }
  return form
}

export function serializeImageModelPrices(form: ImageModelPricesForm): ImageModelPrices {
  const result: ImageModelPrices = {}
  for (const [rawModel, tiers] of Object.entries(form)) {
    const model = normalizeModel(rawModel)
    if (!model || !tiers || typeof tiers !== 'object') continue

    const normalizedTiers: Record<string, number> = {}
    for (const [rawResolution, rawPrice] of Object.entries(tiers)) {
      const resolution = normalizeResolution(rawResolution)
      const price = normalizePrice(rawPrice)
      if (resolution && price !== null) normalizedTiers[resolution] = price
    }
    if (Object.keys(normalizedTiers).length > 0) result[model] = normalizedTiers
  }
  return result
}

export function imageModelPriceRows(
  form: ImageModelPricesForm,
  platform = 'grok'
) {
  const platformModels = imagePriceModelsByPlatform[platform] ?? []
  const known = new Set<string>(allImagePriceModels.map(({ key }) => key))
  const extra = Object.keys(form)
    .map(normalizeModel)
    .filter((model) => model && !known.has(model))
    .sort()
    .map((key) => ({ key, label: key }))
  return [...platformModels, ...extra]
}

export function getImageModelPricePlaceholder(model: string, resolution: string): string {
  const price = defaultPrices[normalizeModel(model)]?.[normalizeResolution(resolution)]
  if (price === undefined) return ''
  return String(price)
}

export function getConfiguredImageModelPrice(
  form: ImageModelPricesForm,
  model: string,
  resolution: string
): number | null {
  return normalizePrice(
    form[normalizeModel(model)]?.[normalizeResolution(resolution)]
  )
}

export function resolveImageModelPrice(
  form: ImageModelPricesForm,
  model: string,
  resolution: string,
  flatPrice: unknown,
  defaultFlatPrice?: unknown
): number | null {
  const normalizedModel = normalizeModel(model)
  const normalizedResolution = normalizeResolution(resolution)
  const configuredPrice = getConfiguredImageModelPrice(form, model, resolution)
  if (configuredPrice !== null) return configuredPrice

  const normalizedFlatPrice = normalizePrice(flatPrice)
  if (normalizedFlatPrice !== null) return normalizedFlatPrice

  return defaultPrices[normalizedModel]?.[normalizedResolution]
    ?? normalizePrice(defaultFlatPrice)
}

export function resolveImageModelCustomerPrice(
  form: ImageModelPricesForm,
  model: string,
  resolution: string,
  flatPrice: unknown,
  multiplier: number,
  defaultFlatPrice?: unknown
): number | null {
  const configuredPrice = getConfiguredImageModelPrice(form, model, resolution)
  if (configuredPrice !== null) return configuredPrice

  const inheritedPrice = resolveImageModelPrice(
    form,
    model,
    resolution,
    flatPrice,
    defaultFlatPrice
  )
  if (inheritedPrice === null) return null

  const effectiveMultiplier = Number.isFinite(multiplier) && multiplier >= 0 ? multiplier : 0
  return inheritedPrice * effectiveMultiplier
}
