import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '../../client'
import { create, deletePlan, listByAccount, listResults, update } from '../scheduledTests'
import type { CreateScheduledTestPlanRequest, ScheduledTestPlan, UpdateScheduledTestPlanRequest } from '@/types'

vi.mock('../../client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() }
}))

const plan: ScheduledTestPlan = {
  id: 17,
  account_id: 42,
  model_id: 'test-model',
  cron_expression: '*/30 * * * *',
  enabled: false,
  max_results: 50,
  auto_recover: false,
  last_run_at: null,
  next_run_at: null,
  created_at: '',
  updated_at: ''
}

beforeEach(() => {
  vi.resetAllMocks()
})

describe('scheduled account connectivity tests API', () => {
  it('lists only the requested account and accepts an empty result', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data: [plan] }).mockResolvedValueOnce({ data: null })

    expect(await listByAccount(42)).toEqual([plan])
    expect(apiClient.get).toHaveBeenNthCalledWith(1, '/admin/accounts/42/scheduled-test-plans')
    expect(await listByAccount(99)).toEqual([])
    expect(apiClient.get).toHaveBeenNthCalledWith(2, '/admin/accounts/99/scheduled-test-plans')
  })

  it('creates a plan without enabling tests or auto recovery implicitly', async () => {
    const body: CreateScheduledTestPlanRequest = {
      account_id: plan.account_id,
      model_id: plan.model_id,
      cron_expression: plan.cron_expression,
      enabled: false,
      max_results: plan.max_results,
      auto_recover: false
    }
    vi.mocked(apiClient.post).mockResolvedValueOnce({ data: plan })

    expect(await create(body)).toEqual(plan)
    expect(apiClient.post).toHaveBeenCalledTimes(1)
    expect(apiClient.post).toHaveBeenCalledWith('/admin/scheduled-test-plans', body)
    expect(body).toMatchObject({ account_id: 42, enabled: false, auto_recover: false })
  })

  it('preserves explicit false values and sends only the requested plan patch', async () => {
    const patch: UpdateScheduledTestPlanRequest = { enabled: false, auto_recover: false }
    vi.mocked(apiClient.put).mockResolvedValueOnce({ data: plan })

    expect(await update(17, patch)).toEqual(plan)
    expect(apiClient.put).toHaveBeenCalledTimes(1)
    expect(apiClient.put).toHaveBeenCalledWith('/admin/scheduled-test-plans/17', patch)
    expect(patch).toEqual({ enabled: false, auto_recover: false })
    expect(apiClient.post).not.toHaveBeenCalled()
    expect(apiClient.delete).not.toHaveBeenCalled()
  })

  it('scopes history to one plan and retains the optional result limit', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data: [] }).mockResolvedValueOnce({ data: null })

    expect(await listResults(17, 10)).toEqual([])
    expect(apiClient.get).toHaveBeenNthCalledWith(1, '/admin/scheduled-test-plans/17/results', { params: { limit: 10 } })
    expect(await listResults(18)).toEqual([])
    expect(apiClient.get).toHaveBeenNthCalledWith(2, '/admin/scheduled-test-plans/18/results', { params: undefined })
  })

  it('deletes only the selected plan', async () => {
    vi.mocked(apiClient.delete).mockResolvedValueOnce({ data: null })

    await deletePlan(17)
    expect(apiClient.delete).toHaveBeenCalledTimes(1)
    expect(apiClient.delete).toHaveBeenCalledWith('/admin/scheduled-test-plans/17')
    expect(apiClient.post).not.toHaveBeenCalled()
    expect(apiClient.put).not.toHaveBeenCalled()
  })
})
