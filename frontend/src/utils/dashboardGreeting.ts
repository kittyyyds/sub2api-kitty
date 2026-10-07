// Local calendar day, matching the visitor's time-of-day greeting. No account
// token or other credentials are used in the key or stored value.
const fallback = new Map<string, { day: string; index: number }>()
export const DAILY_QUOTE_COUNT = 12

export function localDay(now: Date): string {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
}

export function dailyQuoteIndex(account: number | string, now: Date): number {
  const key = `kitty.home.daily-quote.v1:${account}`
  const day = localDay(now)
  let previous = fallback.get(key)
  try {
    const parsed = JSON.parse(localStorage.getItem(key) || 'null')
    if (parsed && typeof parsed.day === 'string' && Number.isInteger(parsed.index) && parsed.index >= 0 && parsed.index < DAILY_QUOTE_COUNT) previous = parsed
  } catch { /* Private browsing or disabled storage still supports this visit. */ }
  if (previous?.day === day) return previous.index
  // Exclude the previous phrase so a new day visibly brings a new sentence.
  const choices = Array.from({ length: DAILY_QUOTE_COUNT }, (_, i) => i).filter(i => i !== previous?.index)
  const selected = { day, index: choices[Math.floor(Math.random() * choices.length)]! }
  fallback.set(key, selected)
  try { localStorage.setItem(key, JSON.stringify(selected)) } catch { /* In-memory fallback. */ }
  return selected.index
}

export function greetingPeriod(now: Date): 'lateNight' | 'morning' | 'afternoon' | 'evening' {
  const hour = now.getHours()
  return hour < 6 ? 'lateNight' : hour < 12 ? 'morning' : hour < 18 ? 'afternoon' : 'evening'
}
