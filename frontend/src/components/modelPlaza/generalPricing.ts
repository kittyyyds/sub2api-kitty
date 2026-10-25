import type { ModelPlazaResponse } from '@/api/modelPlaza'
import { productFor } from './catalogTable'

/** General display only; preserve raw response and independent media catalog. */
export function generalPricingResponse(response: ModelPlazaResponse | null): ModelPlazaResponse | null {
  if (!response) return null
  return { ...response, groups: response.groups.map(group => ({ ...group, models: group.models.filter(model => {
    const product = productFor(model)
    if (product === 'image') return false
    if (product === 'video' || group.platform === 'video') {
      return model.platform === 'grok' || group.platform === 'grok'
    }
    return true
  }) })).filter(group => group.models.length > 0) }
}
