import type { ModelPlazaResponse } from '@/api/modelPlaza'
import { productFor } from './catalogTable'
import { normalizeModelId } from './modelCatalog'

/** General display only; preserve raw response and independent media catalog. */
export function generalPricingResponse(response: ModelPlazaResponse | null): ModelPlazaResponse | null {
  if (!response) return null
  return { ...response, groups: response.groups.map(group => ({ ...group, models: group.models.filter(model => {
    // Hide the older Sol block in the customer price list; leave API availability intact.
    if (normalizeModelId(model.name).toLowerCase() === 'gpt-6-sol') return false
    const product = productFor(model)
    if (product === 'image') return false
    if (product === 'video' || group.platform === 'video') {
      return model.platform === 'grok' || group.platform === 'grok'
    }
    return true
  }) })).filter(group => group.models.length > 0) }
}
