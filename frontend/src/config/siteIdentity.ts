export type KittySiteId = 'kittyapi' | 'kittyyyds'

export interface KittySiteIdentity {
  id: KittySiteId
  primaryUrl: string
  domesticUrl: string
  assetPrefix: string
  afterSalesGroupUrl: string
}

const sites: Record<KittySiteId, KittySiteIdentity> = {
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

export function resolveKittySite(value?: string): KittySiteIdentity {
  const id = value?.trim() || 'kittyyyds'
  if (id !== 'kittyapi' && id !== 'kittyyyds') {
    throw new Error(`Invalid VITE_KITTY_SITE "${id}"; expected kittyapi or kittyyyds`)
  }
  return sites[id]
}
