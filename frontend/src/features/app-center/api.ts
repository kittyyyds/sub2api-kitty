import { apiClient } from '@/api/client'

import type {
  ApplicationDefinition,
  ApplicationRun,
  ApplicationRunList,
  CheckpointApplicationStepRequest,
  CompleteApplicationStepRequest,
  FailApplicationStepRequest,
  StartApplicationRunRequest,
} from './types'

export async function listApplications(signal?: AbortSignal): Promise<ApplicationDefinition[]> {
  const { data } = await apiClient.get<{ items: ApplicationDefinition[] }>('/apps', { signal })
  return data.items || []
}

export async function startApplicationRun(
  slug: string,
  request: StartApplicationRunRequest,
  signal?: AbortSignal,
): Promise<ApplicationRun> {
  const { data } = await apiClient.post<ApplicationRun>(
    `/apps/${encodeURIComponent(slug)}/runs`,
    request,
    { signal },
  )
  return data
}

export async function listApplicationRuns(signal?: AbortSignal): Promise<ApplicationRunList> {
  const { data } = await apiClient.get<ApplicationRunList>('/apps/runs', { signal })
  return { items: data.items || [], total: data.total }
}

export async function getApplicationRun(
  runId: string | number,
  signal?: AbortSignal,
): Promise<ApplicationRun> {
  const { data } = await apiClient.get<ApplicationRun>(`/apps/runs/${encodeURIComponent(String(runId))}`, { signal })
  return data
}

export async function completeApplicationStep(
  runId: string | number,
  stepKey: string,
  request: CompleteApplicationStepRequest,
  signal?: AbortSignal,
): Promise<ApplicationRun> {
  const { data } = await apiClient.post<ApplicationRun>(
    `/apps/runs/${encodeURIComponent(String(runId))}/steps/${encodeURIComponent(stepKey)}/complete`,
    request,
    { signal },
  )
  return data
}

export async function failApplicationStep(
  runId: string | number,
  stepKey: string,
  request: FailApplicationStepRequest,
  signal?: AbortSignal,
): Promise<ApplicationRun> {
  const { data } = await apiClient.post<ApplicationRun>(
    `/apps/runs/${encodeURIComponent(String(runId))}/steps/${encodeURIComponent(stepKey)}/fail`,
    request,
    { signal },
  )
  return data
}

export async function checkpointApplicationStep(
  runId: string | number,
  stepKey: string,
  request: CheckpointApplicationStepRequest,
  signal?: AbortSignal,
): Promise<ApplicationRun> {
  const { data } = await apiClient.post<ApplicationRun>(
    `/apps/runs/${encodeURIComponent(String(runId))}/steps/${encodeURIComponent(stepKey)}/checkpoint`,
    request,
    { signal },
  )
  return data
}

export async function cancelApplicationRun(
  runId: string | number,
  signal?: AbortSignal,
): Promise<ApplicationRun> {
  const { data } = await apiClient.post<ApplicationRun>(
    `/apps/runs/${encodeURIComponent(String(runId))}/cancel`,
    {},
    { signal },
  )
  return data
}

export async function retryApplicationRun(
  runId: string | number,
  signal?: AbortSignal,
): Promise<ApplicationRun> {
  const { data } = await apiClient.post<ApplicationRun>(
    `/apps/runs/${encodeURIComponent(String(runId))}/retry`,
    {},
    { signal },
  )
  return data
}

export const appCenterAPI = {
  listApplications,
  startRun: startApplicationRun,
  listRuns: listApplicationRuns,
  getRun: getApplicationRun,
  completeStep: completeApplicationStep,
  checkpointStep: checkpointApplicationStep,
  failStep: failApplicationStep,
  cancelRun: cancelApplicationRun,
  retryRun: retryApplicationRun,
}
