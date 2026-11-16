export interface CacheHitRateUsage {
  input_tokens?: number | null
  cache_read_tokens?: number | null
  cache_creation_tokens?: number | null
}

const validTokenCount = (value: number | null | undefined): number =>
  Number.isFinite(value) && Number(value) > 0 ? Number(value) : 0

export const getPromptTokenCount = (usage: CacheHitRateUsage): number =>
  validTokenCount(usage.input_tokens)
  + validTokenCount(usage.cache_read_tokens)
  + validTokenCount(usage.cache_creation_tokens)

export const getCacheHitRate = (usage: CacheHitRateUsage): number | null => {
  const promptTokens = getPromptTokenCount(usage)
  if (promptTokens <= 0) return null

  return (validTokenCount(usage.cache_read_tokens) / promptTokens) * 100
}

export const formatCacheHitRate = (
  usage: CacheHitRateUsage,
  unavailable = '—',
): string => {
  const rate = getCacheHitRate(usage)
  return rate == null ? unavailable : `${rate.toFixed(1)}%`
}
