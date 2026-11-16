import { kittySite } from './site'

export interface CustomerContactChannel {
  account: string
  href?: string
  qrSrc: string
}

export interface CustomerGroupContact {
  href: string
  qrSrc: string
}

export const customerSupportContacts = {
  qq: {
    account: '1722345668',
    href: 'https://qm.qq.com/q/rU9mJYhImA',
    qrSrc: `${kittySite.assetPrefix}/images/heyroute/support/qq-qr.png`,
  },
  wechat: {
    account: 'ALe990411',
    qrSrc: `${kittySite.assetPrefix}/images/heyroute/support/wechat-qr.png`,
  },
} as const satisfies Record<'qq' | 'wechat', CustomerContactChannel>

// This station has its own after-sales group. Do not share this with the other deployment.
export const afterSalesGroupContact = {
  href: kittySite.afterSalesGroupUrl,
  qrSrc: `${kittySite.assetPrefix}/qq-group-qr.png`,
} as const satisfies CustomerGroupContact

// A's homepage community and after-sales groups have different original identities.
// Keep B's existing homepage and after-sales group shared.
export const communityGroupContact = kittySite.id === 'kittyapi'
  ? {
      href: 'https://qm.qq.com/q/aP8eapBPpu',
      qrSrc: `${kittySite.assetPrefix}/images/heyroute/community/qq-qr.png`,
      value: '494038',
      valueKind: 'number',
    } as const
  : {
      ...afterSalesGroupContact,
      value: afterSalesGroupContact.href,
      valueKind: 'link',
    } as const

export const telegramGroupContact = {
  href: 'https://t.me/kittyapi1',
  qrSrc: `${kittySite.assetPrefix}/images/heyroute/community/telegram-qr.png`,
} as const satisfies CustomerGroupContact
