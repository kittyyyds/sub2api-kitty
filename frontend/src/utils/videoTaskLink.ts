/** Validate a supplier URL without fetching bytes or forwarding site credentials. */
export function validateVideoTaskDownloadURL(raw: unknown): string {
  if (typeof raw !== 'string' || !raw || raw.length > 16384) throw new Error('上游尚未提供该视频的下载链接。')
  let url: URL
  try { url = new URL(raw) } catch { throw new Error('上游视频下载链接无效。') }
  const host = url.hostname.toLowerCase().replace(/^\[|\]$/g, '')
  const parts = host.split('.').map(Number)
  const ipv4 = parts.length === 4 && parts.every(n => Number.isInteger(n) && n >= 0 && n <= 255)
  const privateIPv4 = ipv4 && (parts[0] === 0 || parts[0] === 10 || parts[0] === 127 || parts[0] >= 224
    || (parts[0] === 169 && parts[1] === 254) || (parts[0] === 172 && parts[1]! >= 16 && parts[1]! <= 31)
    || (parts[0] === 192 && parts[1] === 168) || (parts[0] === 100 && parts[1]! >= 64 && parts[1]! <= 127))
  if (url.protocol !== 'https:' || url.username || url.password || url.hash || (url.port && url.port !== '443')
    || host === window.location.hostname.toLowerCase() || host === 'localhost' || !host.includes('.')
    || /\.(localhost|local|internal)$/.test(host) || host.includes(':') || privateIPv4) {
    throw new Error('上游未提供可安全打开的外部视频下载链接。')
  }
  return url.href
}
