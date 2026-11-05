import { authorizePelicanInlineScripts, extractPelicanHtml, readPelicanPreviewNonce } from './pelicanHtml'
import { createPelicanPreviewChannel } from './pelicanPreview'

const MESSAGE = 'kitty-pelican-poster-v1'
const MAX_BYTES = 4 * 1024 * 1024
const MAX_HTML = 512 * 1024
const WIDTH = 800
const HEIGHT = 600
// Session-only raster cache: never bypass the gallery's authorized HTML fetch.
const cache = new Map<string, Blob | null>()
let tail: Promise<unknown> = Promise.resolve()

// Executed only in the opaque-origin sandbox. The generated artwork is untrusted.
export function installPelicanPoster(target: Window & typeof globalThis, channel: string, message: string) {
  const doc = target.document
  const later = target.setTimeout.bind(target)
  const send = target.parent.postMessage.bind(target.parent)
  const serializer = new target.XMLSerializer()
  const ImageClass = target.Image
  later(async () => {
    try {
      if (!doc.body || doc.querySelectorAll('*').length > 1500) throw new Error('too_large')
      for (const animation of doc.getAnimations?.() ?? []) animation.pause()
      for (const svg of doc.querySelectorAll('svg')) svg.pauseAnimations?.()
      const original = doc.body
      const copy = original.cloneNode(true) as HTMLElement
      const sources = [original, ...original.querySelectorAll('*')]
      const targets = [copy, ...copy.querySelectorAll('*')]
      sources.forEach((node, i) => {
        const clone = targets[i] as HTMLElement
        const style = target.getComputedStyle(node)
        // Snapshot computed styles instead of importing authored styles/scripts.
        const styles = Array.from(style).filter(k => !k.startsWith('animation') && !k.startsWith('transition'))
        if (clone.style) for (const key of styles) clone.style.setProperty(key, style.getPropertyValue(key))
        for (const attr of Array.from(clone.attributes)) {
          if (/^on/i.test(attr.name) || ['nonce', 'srcset', 'srcdoc', 'href', 'xlink:href'].includes(attr.name.toLowerCase())) clone.removeAttribute(attr.name)
        }
        if (node instanceof target.HTMLCanvasElement) {
          const img = doc.createElement('img')
          img.src = node.toDataURL('image/png')
          img.style.cssText = clone.style.cssText
          clone.replaceWith(img)
        }
      })
      copy.querySelectorAll('script,style,link,meta,iframe,object,embed,video,audio,animate,animateTransform,animateMotion,set').forEach(n => n.remove())
      copy.querySelectorAll('[src]').forEach(n => { if (!n.getAttribute('src')?.startsWith('data:image/')) n.removeAttribute('src') })
      copy.style.margin = '0'
      copy.setAttribute('xmlns', 'http://www.w3.org/1999/xhtml')
      const image = new ImageClass()
      const markup = `<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="768"><foreignObject width="1024" height="768">${serializer.serializeToString(copy)}</foreignObject></svg>`
      image.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(markup)
      await image.decode()
      const canvas = doc.createElement('canvas')
      canvas.width = 800; canvas.height = 600
      const context = canvas.getContext('2d')
      if (!context) throw new Error('canvas')
      context.fillStyle = target.getComputedStyle(doc.documentElement).backgroundColor || '#ffffff'
      context.fillRect(0, 0, 800, 600)
      context.drawImage(image, 0, 0, 800, 600)
      const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/png'))
      if (!blob) throw new Error('encode')
      const buffer = await blob.arrayBuffer()
      send({ type: message, channel, buffer }, '*', [buffer])
    } catch { send({ type: message, channel, failed: true }, '*') }
  }, 1200)
}

export function buildPelicanPosterDocument(html: string, channel: string, nonce: string): string {
  const extracted = extractPelicanHtml(html)
  if (!extracted || html.length > MAX_HTML) return ''
  const protectedHtml = authorizePelicanInlineScripts(extracted, nonce)
  const safeChannel = channel.replace(/[^a-zA-Z0-9_-]/g, '')
  const nonceAttribute = /^[A-Za-z0-9+/_=-]+$/.test(nonce) ? ` nonce="${nonce}"` : ''
  const script = `<script${nonceAttribute}>(${installPelicanPoster.toString()})(window,${JSON.stringify(safeChannel)},${JSON.stringify(MESSAGE)});<\/script>`
  // Our restrictive CSP remains before instrumentation AND authored content.
  const end = protectedHtml.indexOf('>', protectedHtml.indexOf('<meta')) + 1
  return protectedHtml.slice(0, end) + script + protectedHtml.slice(end)
}

export function readPelicanPosterMessage(event: MessageEvent, source: Window | null, channel: string): ArrayBuffer | false | null {
  if (!source || !channel || event.source !== source || event.data?.type !== MESSAGE || event.data?.channel !== channel) return null
  const buffer = event.data.buffer
  // Validate PNG header and dimensions before decoding untrusted bytes.
  if (!(buffer instanceof ArrayBuffer) || buffer.byteLength < 33 || buffer.byteLength > MAX_BYTES) return false
  const b = new DataView(buffer)
  if (b.getUint32(0) !== 0x89504e47 || b.getUint32(4) !== 0x0d0a1a0a || b.getUint32(8) !== 13 || b.getUint32(12) !== 0x49484452) return false
  if (b.getUint32(16) !== WIDTH || b.getUint32(20) !== HEIGHT) return false
  return buffer
}

async function normalize(buffer: ArrayBuffer): Promise<Blob | null> {
  const url = URL.createObjectURL(new Blob([buffer], { type: 'image/png' }))
  try {
    const image = new Image()
    image.src = url
    await image.decode()
    if (image.naturalWidth !== WIDTH || image.naturalHeight !== HEIGHT) return null
    const canvas = document.createElement('canvas'); canvas.width = WIDTH; canvas.height = HEIGHT
    const context = canvas.getContext('2d'); if (!context) return null
    context.drawImage(image, 0, 0)
    return await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/png'))
  } catch { return null } finally { URL.revokeObjectURL(url) }
}

function render(html: string, signal: AbortSignal): Promise<Blob | null> {
  return new Promise(resolve => {
    if (signal.aborted) { resolve(null); return }
    const frame = document.createElement('iframe')
    const channel = createPelicanPreviewChannel()
    frame.setAttribute('sandbox', 'allow-scripts')
    frame.setAttribute('referrerpolicy', 'no-referrer')
    frame.setAttribute('aria-hidden', 'true'); frame.tabIndex = -1
    frame.style.cssText = 'position:fixed;left:-12000px;top:0;width:1024px;height:768px;border:0;pointer-events:none;'
    let done = false
    const finish = (value: Blob | null) => {
      if (done) return
      done = true
      clearTimeout(timer)
      signal.removeEventListener('abort', abort)
      window.removeEventListener('message', receive)
      frame.remove()
      resolve(value)
    }
    const abort = () => finish(null)
    const receive = async (event: MessageEvent) => {
      const result = readPelicanPosterMessage(event, frame.contentWindow, channel)
      if (result === null || done) return
      // Only one reply can be accepted, even if generated code floods messages.
      window.removeEventListener('message', receive)
      finish(result === false ? null : await normalize(result))
    }
    const timer = setTimeout(() => finish(null), 6500)
    signal.addEventListener('abort', abort, { once: true })
    window.addEventListener('message', receive)
    frame.srcdoc = buildPelicanPosterDocument(html, channel, readPelicanPreviewNonce())
    if (!frame.srcdoc) { finish(null); return }
    document.body.appendChild(frame)
  })
}

export async function getPelicanPoster(html: string, signal: AbortSignal): Promise<Blob | null> {
  if (signal.aborted || html.length > MAX_HTML || !crypto.subtle) return null
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(html))
  const key = Array.from(new Uint8Array(digest), n => n.toString(16).padStart(2, '0')).join('')
  const job = tail.then(async () => {
    if (signal.aborted) return null
    if (cache.has(key)) return cache.get(key) ?? null
    const blob = await render(html, signal)
    if (!signal.aborted) {
      cache.set(key, blob)
      while (cache.size > 32) cache.delete(cache.keys().next().value!)
    }
    return blob
  })
  tail = job.catch(() => null)
  return job
}
