import type { PricingFormEntry } from './types'
import { createDefaultTimePricingForm } from './types'

export function newVideoGroupPricing(): PricingFormEntry {
  return {
    models: [''], billing_mode: 'video', per_request_price: null,
    input_price: null, output_price: null, cache_write_price: null, cache_read_price: null,
    image_input_price: null, image_output_price: null, intervals: [], time_pricing: createDefaultTimePricingForm(),
  }
}

export function validateVideoGroupPricing(entries: PricingFormEntry[]): string | null {
  if (!entries.length) return 'videoModule.emptyPricing'
  const seen = new Set<string>()
  for (const entry of entries) {
    const model = entry.models[0]?.trim() || ''
    if (entry.models.length !== 1 || !model || /[*?]/.test(model)) return 'videoModule.invalidModel'
    if (seen.has(model)) return 'videoModule.duplicateModel'
    seen.add(model)
    if (entry.billing_mode !== 'video' && entry.billing_mode !== 'per_request') return 'videoModule.invalidBilling'
    if (entry.per_request_price == null || entry.per_request_price === ''
      || !Number.isFinite(Number(entry.per_request_price)) || Number(entry.per_request_price) < 0) return 'videoModule.invalidPrice'
  }
  return null
}
