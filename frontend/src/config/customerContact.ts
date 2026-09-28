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

export const telegramGroupContact = {
  href: 'https://t.me/kittyapi1',
  qrSrc: `${kittySite.assetPrefix}/images/heyroute/community/telegram-qr.png`,
} as const satisfies CustomerGroupContact
