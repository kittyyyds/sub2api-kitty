import { describe, expect, it } from 'vitest'

import { CUSTOMER_NAV_GROUPS, mergeCustomerNavGroups } from '../navGroups'

const item = (path: string) => ({ path, label: path, icon: path })
const groupLabel = (group: { id: string }) => `group:${group.id}`

describe('customer navigation groups', () => {
  it('collapses each group to its first member and keeps the other paths for highlighting', () => {
    const merged = mergeCustomerNavGroups(
      ['/dashboard', '/keys', '/monitor', '/available-channels', '/usage', '/billing', '/purchase', '/orders', '/coupons', '/invoices', '/redeem', '/affiliate'].map(item),
      groupLabel,
    )
    expect(merged.map(entry => entry.path)).toEqual(['/dashboard', '/keys', '/monitor', '/usage', '/billing', '/purchase', '/affiliate'])
    expect(merged.find(entry => entry.path === '/monitor')).toMatchObject({ label: 'group:channels', icon: '/monitor', activePaths: ['/monitor', '/available-channels'] })
    expect(merged.find(entry => entry.path === '/billing')).toMatchObject({ label: 'group:billing-records', activePaths: ['/billing', '/orders', '/invoices'] })
    expect(merged.find(entry => entry.path === '/purchase')).toMatchObject({ label: 'group:recharge-center', activePaths: ['/purchase', '/redeem', '/coupons'] })
    expect(merged.find(entry => entry.path === '/keys')).toEqual(item('/keys'))
  })

  it('opens the channel entry on the status page', () => {
    expect(CUSTOMER_NAV_GROUPS.find(group => group.id === 'channels')!.members.map(member => member.path)).toEqual(['/monitor', '/available-channels'])
  })

  it('falls back to the next visible member when the first one is hidden', () => {
    const merged = mergeCustomerNavGroups(['/orders', '/invoices', '/redeem', '/coupons'].map(item), groupLabel)
    expect(merged.map(entry => entry.path)).toEqual(['/orders', '/redeem'])
    expect(merged[0]).toMatchObject({ label: 'group:billing-records', activePaths: ['/orders', '/invoices'] })
    expect(merged[1]).toMatchObject({ label: 'group:recharge-center', activePaths: ['/redeem', '/coupons'] })
  })

  it('leaves a lone member under its own name and does not touch the input', () => {
    const items = ['/monitor', '/billing', '/custom/docs'].map(item)
    const merged = mergeCustomerNavGroups(items, groupLabel)
    expect(merged).toEqual(items)
    expect(merged.every(entry => !('activePaths' in entry))).toBe(true)
    expect(items).toEqual(['/monitor', '/billing', '/custom/docs'].map(item))
  })

  it('lists every page in one group only', () => {
    const paths = CUSTOMER_NAV_GROUPS.flatMap(group => group.members.map(member => member.path))
    expect(new Set(paths).size).toBe(paths.length)
  })
})
