export interface VideoUsageRow {
  video_count?: number
  video_resolution?: string | null
  video_duration_seconds?: number | null
  billing_mode?: string | null
  total_cost?: number | null
}

export function isVideoUsage(row: VideoUsageRow | null | undefined): boolean {
  return (row?.video_count ?? 0) > 0 || row?.billing_mode === 'video'
}

// The effective historical unit price comes from this receipt, never today's
// model price. Missing units stay unknown, including old video-only receipts.
export function videoUnitPrice(row: VideoUsageRow | null | undefined): number | null {
  if (!row || !Number.isFinite(row.total_cost) || (row.total_cost ?? -1) < 0) return null
  const count = row.video_count ?? 0
  if (!Number.isFinite(count) || count <= 0) return null
  let units = count
  if (row.billing_mode === 'video') {
    const seconds = row.video_duration_seconds ?? 0
    if (!Number.isFinite(seconds) || seconds <= 0) return null
    units *= seconds
  } else if (row.billing_mode !== 'per_request') {
    return null
  }
  return row.total_cost! / units
}
