/** Match the image adapter's confirmed origin; unrelated OpenAI relays keep rate probes. */
export function usesImagePriceProbe(platform: string | undefined, baseURL: unknown): boolean {
  if (platform !== 'openai' || typeof baseURL !== 'string') return false
  try {
    const u = new URL(baseURL.trim())
    return u.protocol === 'https:' && u.hostname === 'suanliai.top' && !u.port &&
      !u.username && !u.password && !u.search && !u.hash && ['', '/', '/v1', '/v1/'].includes(u.pathname)
  } catch { return false }
}

export function usesModelPriceProbe(account: { platform?: string; type?: string; credentials?: Record<string, unknown>; extra?: { upstream_billing_probe?: { kind?: string } } }): boolean {
  return account.platform === 'video' || account.extra?.upstream_billing_probe?.kind === 'video_model_prices' ||
    account.extra?.upstream_billing_probe?.kind === 'image_model_prices' ||
    (account.type === 'apikey' && usesImagePriceProbe(account.platform, account.credentials?.base_url))
}
