import { afterEach, describe, expect, it, vi } from 'vitest'
import { dailyQuoteIndex, greetingPeriod } from '../dashboardGreeting'

afterEach(() => { vi.restoreAllMocks(); localStorage.clear() })

describe('daily welcome text', () => {
  it('keeps the same sentence on refresh and changes it across local midnight', () => {
    const random = vi.spyOn(Math, 'random').mockReturnValue(0)
    const today = dailyQuoteIndex(81001, new Date(2026, 9, 8, 8))
    expect(dailyQuoteIndex(81001, new Date(2026, 9, 8, 23, 59))).toBe(today)
    expect(random).toHaveBeenCalledTimes(1)
    expect(dailyQuoteIndex(81001, new Date(2026, 9, 9, 0, 0))).not.toBe(today)
  })
  it('isolates accounts and accepts a previously saved daily selection', () => {
    localStorage.setItem('kitty.home.daily-quote.v1:81002', JSON.stringify({ day: '2026-10-08', index: 7 }))
    vi.spyOn(Math, 'random').mockReturnValue(0)
    expect(dailyQuoteIndex(81002, new Date(2026, 9, 8))).toBe(7)
    expect(dailyQuoteIndex(81003, new Date(2026, 9, 8))).toBe(0)
  })
  it('survives denied browser storage without changing on every render', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('denied') })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('denied') })
    const first = dailyQuoteIndex(81004, new Date(2026, 9, 8))
    expect(dailyQuoteIndex(81004, new Date(2026, 9, 8, 23))).toBe(first)
  })
  it('uses local greeting boundaries at 06:00, 12:00 and 18:00', () => {
    expect([0, 5, 6, 11, 12, 17, 18, 23].map(hour => greetingPeriod(new Date(2026, 9, 8, hour))))
      .toEqual(['lateNight', 'lateNight', 'morning', 'morning', 'afternoon', 'afternoon', 'evening', 'evening'])
  })
})
