/**
 * Admin model capability and identity audit API.
 *
 * The server intentionally returns diagnostic metadata only (model names,
 * status and bounded evidence); request/response bodies and credentials are
 * never part of these types.
 */

import { apiClient } from '../client'

export type CapabilityAuditProtocol =
  | 'chat_completions'
  | 'responses'
  | 'responses_tools'
  | (string & {})

export type CapabilityAuditStatus =
  | 'verified'
  | 'model_mismatch'
  | 'unsupported'
  | 'timeout'
  | 'error'
  | 'unknown'
  | (string & {})

export type CapabilityAuditCapabilityStatus =
  | 'verified'
  | 'unsupported'
  | 'unverified'
  | 'unknown'
  | (string & {})

export type CapabilityAuditIdentityStatus =
  | 'consistent'
  | 'mismatch_suspected'
  | 'evidence_only'
  | 'unverified'
  | 'unobserved'
  | (string & {})

export interface CapabilityAuditEvidence {
  id?: number
  run_id?: number
  account_id?: number
  account_name?: string
  requested_model?: string
  mapped_model?: string
  response_model?: string
  protocol: CapabilityAuditProtocol
  status: CapabilityAuditStatus
  capability?: string
  http_status?: number
  latency_ms?: number
  upstream_model?: string
  tool_call_found?: boolean
  tool_arguments_valid?: boolean
  error?: string
  evidence?: string
  evidence_code?: string
  capability_status?: CapabilityAuditCapabilityStatus
  identity_status?: CapabilityAuditIdentityStatus
  confidence?: string
  evidence_source?: string
  checked_at: string
}

export interface CapabilityAuditRow {
  key: string
  account_id: number
  account_name: string
  platform: string
  group_id?: number | null
  group_name?: string
  requested_model: string
  mapped_model?: string
  observed_model?: string
  passive_requests: number
  passive_mismatches: number
  passive_response_models?: string[]
  passive_last_seen?: string | null
  active_last_seen?: string | null
  capability_status: CapabilityAuditCapabilityStatus
  active_status?: string
  identity_status: CapabilityAuditIdentityStatus
  confidence: string
  last_seen?: string | null
  last_checked?: string | null
  evidence_count?: number
  protocols: Record<string, CapabilityAuditEvidence>
}

export interface CapabilityAuditStats {
  models: number
  accounts: number
  verified: number
  suspect: number
  unverified: number
  passive_requests: number
  active_checks: number
}

export interface CapabilityAuditSummary {
  days: number
  generated_at: string
  total_models: number
  capability_verified: number
  identity_consistent: number
  mismatch_suspected: number
  unverified: number
  passive_requests: number
  total_accounts?: number
  stats: CapabilityAuditStats
  recent_evidence?: CapabilityAuditEvidence[]
  rows: CapabilityAuditRow[]
}

export interface CapabilityAuditRun {
  id: number
  trigger?: string
  status: string
  requested_by?: number | null
  created_by?: number | null
  total_count: number
  total_items?: number
  completed_count: number
  completed_items?: number
  verified_count: number
  mismatch_count: number
  unsupported_count: number
  error_count: number
  options?: unknown
  requested_scope?: unknown
  error_message?: string
  error_summary?: string
  started_at: string
  finished_at?: string | null
  created_at: string
}

export interface CapabilityAuditRunDetail {
  run: CapabilityAuditRun
  evidence: CapabilityAuditEvidence[]
}

export interface CapabilityAuditProbeRequest {
  account_ids?: number[]
  model_ids?: string[]
  protocols?: CapabilityAuditProtocol[]
  active_only?: boolean
  max_items?: number
}

export interface CapabilityAuditSummaryParams {
  days?: number
  limit?: number
}

export interface CapabilityAuditRunsParams {
  limit?: number
}

export async function getSummary(
  params: CapabilityAuditSummaryParams | number = {}
): Promise<CapabilityAuditSummary> {
  const query = typeof params === 'number' ? { days: params } : params
  const { data } = await apiClient.get<CapabilityAuditSummary>(
    '/admin/model-capability-audit/summary',
    { params: query }
  )
  return data
}

export async function listRuns(params: CapabilityAuditRunsParams = {}): Promise<CapabilityAuditRun[]> {
  const { data } = await apiClient.get<CapabilityAuditRun[]>(
    '/admin/model-capability-audit/runs',
    { params }
  )
  return data ?? []
}

export async function getRun(id: number): Promise<CapabilityAuditRunDetail> {
  const { data } = await apiClient.get<CapabilityAuditRunDetail>(
    `/admin/model-capability-audit/runs/${id}`
  )
  return data
}

export async function runProbes(
  request: CapabilityAuditProbeRequest = {}
): Promise<CapabilityAuditRun> {
  const { data } = await apiClient.post<CapabilityAuditRun>(
    '/admin/model-capability-audit/probe',
    request
  )
  return data
}

// Short aliases make the module convenient for callers that mirror endpoint
// names while retaining the explicit names used by the audit view.
export const summary = getSummary
export const runs = listRuns
export const run = getRun
export const probe = runProbes

export const modelCapabilityAuditAPI = {
  getSummary,
  listRuns,
  getRun,
  runProbes,
  summary,
  runs,
  run,
  probe,
}

export default modelCapabilityAuditAPI
