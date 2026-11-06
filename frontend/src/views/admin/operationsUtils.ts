export interface ProfitAmounts { user_cost: number; upstream_cost: number; unknown_cost?: number }

export const profitOf = (row: ProfitAmounts): number => row.unknown_cost ? Number.NaN : row.user_cost - row.upstream_cost

// Put the largest losses first, followed by the largest profits.
export function compareProfit(a: ProfitAmounts, b: ProfitAmounts): number {
  const aProfit = profitOf(a)
  const bProfit = profitOf(b)
  if (!Number.isFinite(aProfit) || !Number.isFinite(bProfit)) return Number.isFinite(aProfit) ? -1 : Number.isFinite(bProfit) ? 1 : 0
  if ((aProfit < 0) !== (bProfit < 0)) return aProfit < 0 ? -1 : 1
  return aProfit < 0 ? aProfit - bProfit : bProfit - aProfit
}

export function localDate(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

export function dateRange(days: number, now = new Date()): { start: string; end: string } {
  const start = new Date(now)
  start.setDate(start.getDate() - days + 1)
  return { start: localDate(start), end: localDate(now) }
}

export function formatMoney(value: number | null | undefined, currency = 'USD'): string {
  if (value == null || !Number.isFinite(value)) return '—'
  const safeValue = Number.isFinite(value) ? value : 0
  try {
    return new Intl.NumberFormat('en-US', {
      style: 'currency', currency, minimumFractionDigits: 2,
      maximumFractionDigits: Math.abs(safeValue) > 0 && Math.abs(safeValue) < 0.01 ? 4 : 2
    }).format(safeValue)
  } catch {
    return `${currency} ${safeValue.toFixed(2)}`
  }
}
