export type GrokMediaProvider = '' | 'suanli_special'

export function readGrokMediaProvider(credentials: Record<string, unknown> | undefined): GrokMediaProvider {
  return credentials?.grok_media_provider === 'suanli_special' ? 'suanli_special' : ''
}

export function applyGrokMediaProvider(credentials: Record<string, unknown>, provider: GrokMediaProvider): void {
  if (provider === 'suanli_special') credentials.grok_media_provider = provider
  else delete credentials.grok_media_provider
}
