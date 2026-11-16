export type KittySiteId = 'kittyapi' | 'kittyyyds' | 'custom'

export interface KittySiteIdentity {
  id: KittySiteId
  primaryUrl: string
  domesticUrl: string
  assetPrefix: string
  afterSalesGroupUrl: string
}

const sites: Record<KittySiteId, KittySiteIdentity> = {
  custom: {
    id: 'custom',
    primaryUrl: 'https://custom.kittyapi.xyz',
    domesticUrl: 'https://custom.kittyapi.xyz',
    assetPrefix: '/sites/kittyapi',
    afterSalesGroupUrl: 'https://qm.qq.com/q/oRR8GUdEPK',
  },
  kittyapi: {
    id: 'kittyapi',
    primaryUrl: 'https://kittyapi.xyz',
    domesticUrl: 'https://cdn.kittyapi.xyz',
    assetPrefix: '/sites/kittyapi',
    afterSalesGroupUrl: 'https://qm.qq.com/q/oRR8GUdEPK',
  },
  kittyyyds: {
    id: 'kittyyyds',
    primaryUrl: 'https://kittyyyds.xyz',
    domesticUrl: 'https://cdn.kittyyyds.xyz',
    // Keep the existing B deployment's public asset URLs compatible.
    assetPrefix: '',
    afterSalesGroupUrl: 'https://qm.qq.com/q/Wr3pNr9zMe',
  },
}

export function resolveKittySite(value?: string, currentOrigin?: string): KittySiteIdentity {
  const id = value?.trim() || 'custom'
  if (id !== 'kittyapi' && id !== 'kittyyyds' && id !== 'custom') {
    throw new Error(`Invalid VITE_KITTY_SITE "${id}"; expected custom, kittyapi or kittyyyds`)
  }
  if (id === 'custom' && currentOrigin && /^https?:\/\//.test(currentOrigin)) {
    const origin = new URL(currentOrigin).origin
    return { ...sites.custom, primaryUrl: origin, domesticUrl: origin }
  }
  return sites[id]
}
