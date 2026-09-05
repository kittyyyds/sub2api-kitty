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
  | 'self_report_consistent'
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
  source?: string
  attempt_count?: number
  success_count?: number
  failure_count?: number
  tool_name?: string
  response_summary?: string
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
  self_reported?: number
  suspect: number
  unverified: number
  passive_requests: number
  active_checks: number
}

export interface CapabilityAuditSummary {
  days: number
  generated_at: string
  total_models: number
  returned_models?: number
  rows_truncated?: boolean
  capability_verified: number
  identity_consistent: number
  self_report_consistent?: number
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
  preview_token?: string
}

export interface CapabilityAuditEstimate {
  target_count: number
  protocol_count: number
  request_count: number
  estimated_max_seconds: number
  max_items: number
  preview_token: string
  expires_at: string
}

export interface BehaviorAuditRequest {
  target_account_ids?: number[]
  model_id: string
  case_count?: number
  reasoning_effort?: 'minimal' | 'low' | 'medium' | 'high' | 'xhigh'
  preview_token?: string
}

export interface BehaviorAuditEstimate {
  target_count: number
  available_target_count: number
  anchor_count: number
  pair_count: number
  request_count: number
  estimated_max_seconds: number
  can_run: boolean
  targets_truncated: boolean
  reason?: string
  preview_token: string
  expires_at: string
}

export interface BehaviorAuditRun {
  id: number
  status: string
  requested_by?: number | null
  model_id: string
  case_count: number
  reasoning_effort: string
  total_pairs: number
  completed_pairs: number
  matched_count: number
  diverged_count: number
  inconclusive_count: number
  error_count: number
  total_requests: number
  attempt_count: number
  error_message?: string
  started_at: string
  finished_at?: string | null
  created_at: string
}

export interface BehaviorAuditResult {
  id: number
  target_account_id: number
  target_account_name: string
  anchor_account_id: number
  anchor_account_name: string
  case_key: string
  case_label: string
  requested_model: string
  target_mapped_model: string
  anchor_mapped_model: string
  target_response_model?: string
  anchor_response_model?: string
  comparison_status: 'matched' | 'diverged' | 'inconclusive' | 'error' | string
  target_correct: boolean
  anchor_correct: boolean
  target_http_status: number
  anchor_http_status: number
  target_latency_ms: number
  anchor_latency_ms: number
  target_output_hash?: string
  anchor_output_hash?: string
  expected_output_hash: string
  target_status: string
  anchor_status: string
  attempt_count: number
  success_count: number
  failure_count: number
  error?: string
  checked_at: string
}

export interface BehaviorAuditDetail {
  run: BehaviorAuditRun
  results: BehaviorAuditResult[]
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

export async function estimateProbes(
  request: CapabilityAuditProbeRequest = {}
): Promise<CapabilityAuditEstimate> {
  const { data } = await apiClient.post<CapabilityAuditEstimate>(
    '/admin/model-capability-audit/probe-preview',
    request
  )
  return data
}

export async function estimateBehavior(request: BehaviorAuditRequest): Promise<BehaviorAuditEstimate> {
  const { data } = await apiClient.post<BehaviorAuditEstimate>(
    '/admin/model-capability-audit/behavior-preview',
    request
  )
  return data
}

export async function runBehavior(request: BehaviorAuditRequest): Promise<BehaviorAuditRun> {
  const { data } = await apiClient.post<BehaviorAuditRun>(
    '/admin/model-capability-audit/behavior',
    request
  )
  return data
}

export async function listBehaviorRuns(limit = 20): Promise<BehaviorAuditRun[]> {
  const { data } = await apiClient.get<BehaviorAuditRun[]>(
    '/admin/model-capability-audit/behavior-runs',
    { params: { limit } }
  )
  return data ?? []
}

export async function getBehaviorRun(id: number): Promise<BehaviorAuditDetail> {
  const { data } = await apiClient.get<BehaviorAuditDetail>(
    `/admin/model-capability-audit/behavior-runs/${id}`
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
  estimateProbes,
  estimateBehavior,
  runBehavior,
  listBehaviorRuns,
  getBehaviorRun,
  summary,
  runs,
  run,
  probe,
}

export default modelCapabilityAuditAPI
