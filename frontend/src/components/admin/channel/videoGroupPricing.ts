import type { PricingFormEntry } from './types'
import { createDefaultTimePricingForm } from './types'
import type { VideoModelConfig, VideoRuntimeSettings, VideoSupplierPrice } from '@/api/admin/videoUpstream'

export function newVideoGroupPricing(): PricingFormEntry {
  return {
    models: [''], billing_mode: 'video', per_request_price: null,
    input_price: null, output_price: null, cache_write_price: null, cache_read_price: null,
    image_input_price: null, image_output_price: null, intervals: [], time_pricing: createDefaultTimePricingForm(),
  }
}

// Missing video metadata denotes a legacy fixed price, even if the ID is in the catalog.
export function videoPriceSource(video?: VideoModelConfig): 'upstream' | 'manual' {
  return video?.price_source ?? (video?.auto_price ? 'upstream' : 'manual')
}

export function videoConfigToAPI(video?: VideoModelConfig): VideoModelConfig | undefined {
  if (!video) return undefined
  const price_source = videoPriceSource(video)
  return { ...JSON.parse(JSON.stringify(video)), price_source, auto_price: price_source === 'upstream' && video.auto_price }
}

export function isAvailableVideoSupplier(price: VideoSupplierPrice): boolean {
  return !!price.model.trim() && !!price.spec?.supported && !price.unavailable_reason
    && (price.billing_mode === 'video' || price.billing_mode === 'per_request')
    && validPrice(price.unit_price) && validPrice(price.reference_per_second)
    && Object.values(price.resolution_prices || {}).every(validPrice)
}

export function videoPricingFromCatalog(price: VideoSupplierPrice): PricingFormEntry | null {
  if (!isAvailableVideoSupplier(price) || !price.spec) return null
  return {
    ...newVideoGroupPricing(),
    models: [price.model],
    billing_mode: price.billing_mode,
    video: {
      price_source: 'upstream', auto_price: true,
      reference_per_second: price.reference_per_second,
      resolution_prices: { ...price.resolution_prices },
      spec: JSON.parse(JSON.stringify(price.spec)),
      cost: JSON.parse(JSON.stringify(price)),
    },
  }
}

export function videoCustomerPrices(
  entry: PricingFormEntry,
  settings?: VideoRuntimeSettings,
  cost: VideoSupplierPrice | null | undefined = entry.video?.cost,
) {
  const video = entry.video
  const cached = validPrice(entry.per_request_price)
  // Saved prices remain effective until the server accepts and recalculates the form.
  if (cached || videoPriceSource(video) === 'manual' || !video || !cost || !settings) {
    return {
      unit: cached ? Number(entry.per_request_price) : null,
      reference: video?.reference_per_second ?? 0,
      resolutions: video?.resolution_prices ?? {},
      preview: false,
    }
  }
  const markup = video.markup ?? settings.markup
  const add = entry.billing_mode === 'per_request' ? markup.per_request : markup.output_per_second
  const money = (value: number) => Number(value.toFixed(8))
  return {
    unit: money(cost.unit_price + add),
    reference: entry.billing_mode === 'video' && (video.spec.reference_billed || cost.reference_price_known || cost.reference_per_second > 0)
      ? money(cost.reference_per_second + markup.reference_per_second) : 0,
    resolutions: Object.fromEntries(Object.entries(cost.resolution_prices || {}).map(([key, value]) => [key, money(value + markup.output_per_second)])),
    preview: true,
  }
}

export function videoManualPricePatch(
  entry: PricingFormEntry, value: string, settings?: VideoRuntimeSettings,
  cost: VideoSupplierPrice | null | undefined = entry.video?.cost,
): Partial<PricingFormEntry> {
  if (!entry.video) return { per_request_price: value }
  if (!value.trim()) {
    // Manual dimension inputs disappear in upstream mode. Replace their draft
    // values (including NaN from cleared inputs) with finite cost-cache values.
    // These are display caches only; the server owns the authoritative repricing.
    const upstreamCost = cost ?? entry.video.cost
    return {
      per_request_price: validPrice(entry.per_request_price) ? entry.per_request_price : null,
      video: {
        ...entry.video, price_source: 'upstream', auto_price: true,
        reference_per_second: validPrice(upstreamCost?.reference_per_second) ? Number(upstreamCost!.reference_per_second) : 0,
        resolution_prices: Object.fromEntries(Object.entries(upstreamCost?.resolution_prices || {})
          .filter(([, price]) => validPrice(price))),
      },
    }
  }
  const effective = videoCustomerPrices(entry, settings, cost)
  const resolutions = new Set([...Object.keys(effective.resolutions), ...(entry.video.spec.resolutions || [])])
  return {
    per_request_price: value,
    video: {
      ...entry.video, price_source: 'manual', auto_price: false,
      reference_per_second: effective.reference,
      // Quotes prefer resolution prices over the base price. A uniform manual
      // price must replace those overrides too; reference pricing is independent.
      resolution_prices: Object.fromEntries([...resolutions].map(resolution => [resolution, Number(value)])),
    },
  }
}

function validPrice(value: number | string | null | undefined): boolean {
  return value != null && String(value).trim() !== '' && Number.isFinite(Number(value)) && Number(value) >= 0
}

export function validateVideoGroupPricing(entries: PricingFormEntry[]): string | null {
  const seen = new Set<string>()
  for (const entry of entries) {
    const model = entry.models[0]?.trim() || ''
    if (entry.models.length !== 1 || !model || /[*?]/.test(model)) return 'videoModule.invalidModel'
    if (seen.has(model)) return 'videoModule.duplicateModel'
    seen.add(model)
    if (entry.billing_mode !== 'video' && entry.billing_mode !== 'per_request') return 'videoModule.invalidBilling'
    const upstream = videoPriceSource(entry.video) === 'upstream'
    const blank = entry.per_request_price == null || String(entry.per_request_price).trim() === ''
    if (!(upstream && blank) && !validPrice(entry.per_request_price)) return 'videoModule.invalidPrice'
    if (entry.video) {
      if (!validPrice(entry.video.reference_per_second)
        || !Object.values(entry.video.resolution_prices || {}).every(validPrice)) return 'videoModule.invalidPrice'
      if (entry.video.markup && !Object.values(entry.video.markup).every(validPrice)) return 'videoModule.invalidMarkup'
    }
  }
  return null
}
