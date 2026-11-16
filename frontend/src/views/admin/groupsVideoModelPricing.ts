export const grokVideoPriceResolutions = [
  { key: '480p', label: '480p' },
  { key: '720p', label: '720p' },
  { key: '1080p', label: '1080p' }
] as const

export const grokVideoPriceFamilies = [
  { key: 'grok-imagine-video', label: 'grok-imagine-video' },
  { key: 'grok-imagine-video-1.5', label: 'grok-imagine-video-1.5' }
] as const

export type VideoModelPrices = Record<string, Record<string, number>>
export type VideoModelPricesForm = Record<string, Record<string, number | string | null>>

// All prices are USD/s. Populated model cells are final customer rates.
const defaultPrices: VideoModelPrices = {
  'grok-imagine-video': { '480p': 0.05, '720p': 0.07, '1080p': 0.07 },
  'grok-imagine-video-1.5': { '480p': 0.08, '720p': 0.14, '1080p': 0.25 }
}

function normalizeFamily(value: string): string {
  return value.trim().toLowerCase()
}

function normalizePrice(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const price = Number(value)
  return Number.isFinite(price) && price >= 0 ? price : null
}

function emptyTiers(): Record<string, number | string | null> {
  return Object.fromEntries(grokVideoPriceResolutions.map(({ key }) => [key, null]))
}

// Keep unknown families from an existing group so a future backend catalog is
// not silently discarded when an operator edits another group setting.
export function createVideoModelPricesForm(
  prices?: VideoModelPrices | null
): VideoModelPricesForm {
  const form: VideoModelPricesForm = {}

  for (const [rawFamily, rawTiers] of Object.entries(prices ?? {})) {
    const family = normalizeFamily(rawFamily)
    if (!family || !rawTiers || typeof rawTiers !== 'object') continue
    form[family] = emptyTiers()
    for (const [rawResolution, rawPrice] of Object.entries(rawTiers)) {
      const price = normalizePrice(rawPrice)
      if (price !== null) form[family][rawResolution.trim().toLowerCase()] = price
    }
  }

  for (const { key } of grokVideoPriceFamilies) {
    form[key] ??= emptyTiers()
  }
  return form
}

export function serializeVideoModelPrices(form: VideoModelPricesForm): VideoModelPrices {
  const result: VideoModelPrices = {}
  for (const [rawFamily, tiers] of Object.entries(form)) {
    const family = normalizeFamily(rawFamily)
    if (!family || !tiers || typeof tiers !== 'object') continue

    const normalizedTiers: Record<string, number> = {}
    for (const [rawResolution, rawPrice] of Object.entries(tiers)) {
      const resolution = rawResolution.trim().toLowerCase()
      const price = normalizePrice(rawPrice)
      if (resolution && price !== null) normalizedTiers[resolution] = price
    }
    if (Object.keys(normalizedTiers).length > 0) result[family] = normalizedTiers
  }
  return result
}

export function videoModelPriceFamilyRows(form: VideoModelPricesForm) {
  const known = new Set<string>(grokVideoPriceFamilies.map(({ key }) => key))
  const extra = Object.keys(form)
    .map(normalizeFamily)
    .filter((family) => family && !known.has(family))
    .sort()
    .map((key) => ({ key, label: key }))
  return [...grokVideoPriceFamilies, ...extra]
}

export function getConfiguredVideoModelPrice(
  form: VideoModelPricesForm,
  model: string,
  resolution: string
): number | null {
  return normalizePrice(
    form[normalizeFamily(model)]?.[resolution.trim().toLowerCase()]
  )
}

export function resolveVideoModelCustomerPrice(
  form: VideoModelPricesForm,
  model: string,
  resolution: string,
  flatPerSecondPrice: unknown,
  multiplier: number,
  defaultFlatPerSecondPrice?: unknown
): number | null {
  const family = normalizeFamily(model)
  const tier = resolution.trim().toLowerCase()
  const configuredPrice = getConfiguredVideoModelPrice(form, family, tier)
  if (configuredPrice !== null) return configuredPrice

  const inheritedPerSecondPrice = normalizePrice(flatPerSecondPrice)
    ?? defaultPrices[family]?.[tier]
    ?? normalizePrice(defaultFlatPerSecondPrice)
  if (inheritedPerSecondPrice === null) return null

  const effectiveMultiplier = Number.isFinite(multiplier) && multiplier >= 0 ? multiplier : 0
  return inheritedPerSecondPrice * effectiveMultiplier
}
