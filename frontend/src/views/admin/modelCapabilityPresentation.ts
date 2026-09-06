/** Normalize persisted per-protocol values to the public summary vocabulary. */
export function normalizeCapabilityIdentityStatus(value?: string): string {
  if (value === 'mismatch') return 'mismatch_suspected'
  return value || 'unverified'
}

export function passiveCapabilityStatus(passiveRequests: number): string {
  return passiveRequests > 0 ? 'observed' : 'unverified'
}

export function passiveIdentityStatus(
  passiveRequests: number,
  passiveMismatches: number,
  responseModels: string[] = []
): string {
  if (passiveMismatches > 0) return 'mismatch_suspected'
  if (passiveRequests > 0 && responseModels.length > 0) return 'self_report_consistent'
  return 'unverified'
}

export function passiveConfidence(
  passiveRequests: number,
  passiveMismatches: number,
  responseModels: string[] = []
): string {
  if (passiveMismatches > 0) return 'high'
  if (passiveRequests > 0 && responseModels.length > 0) return 'medium'
  return 'low'
}

export function auditIdentityDecision(
  identityStatus?: string,
  confidence?: string,
  catalogDeclared?: boolean | null,
  hasResponseModel = false
): string {
  const normalized = normalizeCapabilityIdentityStatus(identityStatus)
  if (normalized === 'mismatch_suspected') {
    return confidence === 'high'
      ? '重复自报不匹配，高度疑似模型替换'
      : '单次自报不匹配，需要复测'
  }
  if (normalized === 'consistent' || normalized === 'self_report_consistent') {
    return '目录与自报一致，身份仍未验真'
  }
  if (catalogDeclared && !hasResponseModel) {
    return '仅模型目录声明，尚无调用或自报证据'
  }
  if (normalized === 'evidence_only') {
    return '能力证据有效，身份仍未验真'
  }
  return '身份未验证'
}

export function auditEvidenceLevel(identityStatus?: string, confidence?: string): string {
  const normalized = normalizeCapabilityIdentityStatus(identityStatus)
  if (normalized === 'mismatch_suspected') {
    return confidence === 'high' ? '强异常证据' : '中等异常证据'
  }
  if (normalized === 'consistent' || normalized === 'self_report_consistent') {
    return '弱一致性证据'
  }
  return '证据不足'
}

export function auditCatalogLabel(status?: string, declared?: boolean | null): string {
  if (status === 'checked') return declared ? '/v1/models 已声明该模型' : '/v1/models 未声明该模型'
  if (status === 'unsupported') return '上游未提供 /v1/models'
  if (status === 'timeout') return '/v1/models 检查超时'
  if (status === 'error') return '/v1/models 检查失败'
  return '/v1/models 尚未检查'
}
