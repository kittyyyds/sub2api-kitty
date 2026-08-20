/**
 * formatScaled formats a per-token (or per-request) USD price scaled by `scale`.
 *
 *   formatScaled(0.000003, 1_000_000)    → "$3"      // per 1M tokens
 *   formatScaled(0.5,        1)          → "$0.5"    // per request
 *   formatScaled(null,       1_000_000)  → "-"
 *   formatScaled(0.000003, 1_000_000, 2) → "$3.00"   // pad to ≥2 decimals
 *   formatScaled(1.25e-8,  1_000_000, 2) → "$0.0125" // longer decimals kept as-is
 *
 * Uses toPrecision(10) then strips trailing zeros to avoid IEEE 754 display noise.
 * `minFractionDigits` pads the result back up to a minimum number of decimals.
 */
export function formatScaled(value: number | null, scale: number, minFractionDigits = 0): string {
  if (value == null) return '-'
  let s = (value * scale).toPrecision(10).replace(/\.?0+$/, '')
  if (minFractionDigits > 0 && !s.includes('e')) {
    const dot = s.indexOf('.')
    const digits = dot === -1 ? 0 : s.length - dot - 1
    if (digits < minFractionDigits) {
      s = (dot === -1 ? `${s}.` : s) + '0'.repeat(minFractionDigits - digits)
    }
  }
  return `$${s}`
}

/** Display-only exchange rate used by the customer-facing model price catalog. */
export const DISPLAY_USD_TO_CNY_RATE = 7

/** Formats a value that is already denominated in CNY. */
export function formatCnyAmount(
  value: number | null,
  scale = 1,
  minFractionDigits = 2
): string {
  if (value == null) return '-'
  const formatted = formatScaled(value, scale, minFractionDigits)
  return `¥${formatted.slice(1)}`
}

/**
 * Converts a USD source price to the catalog's RMB display value.
 * Billing remains based on the backend USD price and configured multiplier.
 */
export function formatCnyScaled(
  value: number | null,
  scale: number,
  minFractionDigits = 2
): string {
  return formatCnyAmount(
    value == null ? null : value * DISPLAY_USD_TO_CNY_RATE,
    scale,
    minFractionDigits
  )
}
