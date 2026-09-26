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
    qrSrc: '/images/heyroute/support/qq-qr.png',
  },
  wechat: {
    account: 'ALe990411',
    qrSrc: '/images/heyroute/support/wechat-qr.png',
  },
} as const satisfies Record<'qq' | 'wechat', CustomerContactChannel>

// This station has its own after-sales group. Do not share this with the other deployment.
export const afterSalesGroupContact = {
  href: 'https://qm.qq.com/q/Wr3pNr9zMe',
  qrSrc: '/qq-group-qr.png',
} as const satisfies CustomerGroupContact
