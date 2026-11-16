export interface MonitorTimelineBucketLike {
  bucket_start: string
}

export interface MonitorTimelineSlot<T extends MonitorTimelineBucketLike> {
  key: string
  startMs: number | null
  bucket: T | null
}

export function latestMonitorWindowBucketStart(
  requestedEnd: string | undefined,
  dataThrough: string | undefined,
  bucketSeconds: number | undefined,
): number | null {
  const requestedEndMs = Date.parse(requestedEnd || '')
  const dataThroughMs = Date.parse(dataThrough || '')
  const endMs = Number.isFinite(requestedEndMs) ? requestedEndMs : dataThroughMs
  const bucketMs = Number(bucketSeconds) * 1000
  if (!Number.isFinite(endMs) || !Number.isFinite(bucketMs) || bucketMs <= 0) return null
  return Math.floor((endMs - 1) / bucketMs) * bucketMs
}

export function isLatestMonitorBucketCurrent(
  bucketStart: string | undefined,
  requestedEnd: string | undefined,
  dataThrough: string | undefined,
  bucketSeconds: number | undefined,
): boolean {
  const expectedStart = latestMonitorWindowBucketStart(requestedEnd, dataThrough, bucketSeconds)
  const actualStart = Date.parse(bucketStart || '')
  if (expectedStart == null || !Number.isFinite(actualStart)) return false
  return actualStart === expectedStart
}

export interface MonitorCoverageFreshnessInput {
  dataThrough?: string
  computedAt?: string
  aggregationLagSeconds?: number
  refreshIntervalSeconds?: number
  observedAtMs?: number
  nowMs?: number
}

/**
 * A watermark is considered current for at most two normal aggregation cycles.
 * `observedAtMs` lets the status age locally between API refreshes instead of
 * freezing the server-reported lag at response time.
 */
export function isMonitorCoverageFresh(input: MonitorCoverageFreshnessInput): boolean {
  const dataThroughMs = Date.parse(input.dataThrough || '')
  const computedAtMs = Date.parse(input.computedAt || '')
  const lagSeconds = Number(input.aggregationLagSeconds)
  const refreshSeconds = Number(input.refreshIntervalSeconds)
  const observedAtMs = Number(input.observedAtMs)
  const nowMs = Number(input.nowMs)
  if (
    !Number.isFinite(dataThroughMs) || dataThroughMs <= 0 ||
    !Number.isFinite(computedAtMs) || computedAtMs <= 0 ||
    !Number.isFinite(lagSeconds) || lagSeconds < 0 ||
    !Number.isFinite(refreshSeconds) || refreshSeconds <= 0 ||
    !Number.isFinite(observedAtMs) || !Number.isFinite(nowMs)
  ) return false

  const elapsedSeconds = Math.max(0, (nowMs - observedAtMs) / 1000)
  const staleAfterSeconds = Math.max(120, refreshSeconds * 2)
  return lagSeconds + elapsedSeconds <= staleAfterSeconds
}

export function buildMonitorTimelineSlots<T extends MonitorTimelineBucketLike>(
  buckets: T[],
  requestedEnd: string | undefined,
  dataThrough: string | undefined,
  bucketSeconds: number | undefined,
  length: number,
): Array<MonitorTimelineSlot<T>> {
  const safeLength = Math.max(0, Math.floor(length))
  const expectedStart = latestMonitorWindowBucketStart(requestedEnd, dataThrough, bucketSeconds)
  const bucketMs = Number(bucketSeconds) * 1000

  if (expectedStart == null || !Number.isFinite(bucketMs) || bucketMs <= 0) {
    const real = [...buckets]
      .filter(bucket => Number.isFinite(Date.parse(bucket.bucket_start)))
      .sort((a, b) => Date.parse(a.bucket_start) - Date.parse(b.bucket_start))
      .slice(-safeLength)
    const emptyCount = Math.max(0, safeLength - real.length)
    return [
      ...Array.from({ length: emptyCount }, (_, index) => ({
        key: `unknown-${index}`,
        startMs: null,
        bucket: null,
      })),
      ...real.map(bucket => ({
        key: bucket.bucket_start,
        startMs: Date.parse(bucket.bucket_start),
        bucket,
      })),
    ]
  }

  const byStart = new Map<number, T>()
  for (const bucket of buckets) {
    const start = Date.parse(bucket.bucket_start)
    if (Number.isFinite(start)) byStart.set(start, bucket)
  }

  return Array.from({ length: safeLength }, (_, index) => {
    const startMs = expectedStart - (safeLength - index - 1) * bucketMs
    return {
      key: new Date(startMs).toISOString(),
      startMs,
      bucket: byStart.get(startMs) ?? null,
    }
  })
}
