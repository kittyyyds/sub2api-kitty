import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))
import { getOperationsRechargeOrders, getOperationsRechargeSummary } from '@/api/admin/dashboard'

describe('operations recharge request boundaries', () => {
  beforeEach(() => get.mockReset().mockResolvedValue({ data: { items: [], total: 0, page: 1, pages: 1 } }))

  it('uses site midnight for both rows and totals and keeps page/status out of totals', async () => {
    const filter = { start_date: '2026-11-11', end_date: '2026-11-11', timezone: 'Asia/Shanghai', search: 'alice', channel: 'code' as const }
    await getOperationsRechargeOrders({ ...filter, page: 2, page_size: 25, status: 'unpaid' })
    expect(get.mock.calls[0]).toEqual(['/admin/payment/recharge-activity', { params: {
      page: 2, page_size: 25, email: 'alice', channel: 'redeem', status: 'unpaid',
      start_time: '2026-11-10T16:00:00.000Z', end_time: '2026-11-11T16:00:00.000Z'
    } }])
    await getOperationsRechargeSummary(filter)
    expect(get.mock.calls[1]).toEqual(['/admin/payment/recharge-activity/summary', { params: {
      email: 'alice', channel: 'redeem', start_time: '2026-11-10T16:00:00.000Z', end_time: '2026-11-11T16:00:00.000Z'
    } }])
  })

  it('keeps an exclusive next-day end on a 23-hour DST day', async () => {
    await getOperationsRechargeSummary({ start_date: '2026-03-08', end_date: '2026-03-08', timezone: 'America/New_York' })
    expect(get.mock.calls[0][1].params).toMatchObject({ start_time: '2026-03-08T05:00:00.000Z', end_time: '2026-03-09T04:00:00.000Z' })
  })
})
