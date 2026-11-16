import { describe, expect, it } from 'vitest'
import { dateRange, zonedDayStart, formatMoney, profitOf, supportedTimeZone } from '../operationsUtils'
describe('operations site calendar and unknown costs', () => {
  it('uses the site calendar across UTC midnight', () => {
    expect(dateRange(7, new Date('2026-11-10T18:00:00Z'), 'Asia/Shanghai')).toEqual({ start: '2026-11-05', end: '2026-11-11' })
    expect(zonedDayStart('2026-11-11', 'Asia/Shanghai').toISOString()).toBe('2026-11-10T16:00:00.000Z')
  })
  it('keeps complete days across both DST transitions', () => {
    const spring = zonedDayStart('2026-03-09', 'America/New_York').getTime() - zonedDayStart('2026-03-08', 'America/New_York').getTime()
    const autumn = zonedDayStart('2026-11-02', 'America/New_York').getTime() - zonedDayStart('2026-11-01', 'America/New_York').getTime()
    expect(spring).toBe(23 * 3600000)
    expect(autumn).toBe(25 * 3600000)
    expect(supportedTimeZone('invalid-zone')).toBeUndefined()
  })
  it('does not turn missing cost or profit into zero', () => {
    expect(formatMoney(null)).toBe('—')
    expect(formatMoney(profitOf({ user_cost: 100, upstream_cost: 0, unknown_cost: 1 }))).toBe('—')
    expect(formatMoney(0)).toBe('$0.00')
  })
})
