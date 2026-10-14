/** Optional disk downloads of existing tasks. Sessions never contain credentials or media bytes. */
interface VideoWritable {
  write(data: Uint8Array): Promise<void>
  seek(position: number): Promise<void>
  truncate(size: number): Promise<void>
  close(): Promise<void>
  abort(): Promise<void>
}

interface VideoFileHandle {
  name: string
  getFile(): Promise<{ size: number; lastModified: number }>
  createWritable(options: { keepExistingData: boolean }): Promise<VideoWritable>
}

type VideoFilePicker = (options: {
  suggestedName: string
  types: Array<{ description: string; accept: Record<string, string[]> }>
}) => Promise<VideoFileHandle>

export interface VideoFileDownloadSession {
  readonly identity: string
  readonly handle: VideoFileHandle
  bytes: number
  total?: number
  etag?: string
  lastModified?: number
  complete: boolean
  active: boolean
}

export interface VideoDownloadProgress {
  received: number
  total?: number
  resumed: boolean
  restarted: boolean
}

function filePicker(): VideoFilePicker | undefined {
  return typeof window === 'undefined' ? undefined
    : (window as unknown as { showSaveFilePicker?: VideoFilePicker }).showSaveFilePicker
}

export function supportsVideoFileDownload(): boolean {
  return typeof filePicker() === 'function'
}

/** Invoke directly from the button handler, before unrelated awaits consume user activation. */
export async function pickVideoDownloadFile(identity: string, suggestedName: string): Promise<VideoFileDownloadSession> {
  const picker = filePicker()
  if (!picker) throw new Error('当前浏览器不支持直接写入文件，请使用普通下载（会先缓存完整视频）。')
  if (!identity) throw new Error('视频任务标识缺失')
  const handle = await picker.call(window, {
    suggestedName,
    types: [{ description: '视频文件', accept: { 'video/mp4': ['.mp4'] } }],
  })
  return { identity, handle, bytes: 0, complete: false, active: false }
}

function strongETag(value: string | null): string | undefined {
  return value && /^"[\x21\x23-\x7e\x80-\xff]*"$/.test(value) ? value : undefined
}

function size(value: string | null): number | undefined {
  if (value === null) return undefined
  if (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value))) throw new Error('视频长度无效，请重新下载已有任务。')
  return Number(value)
}

function contentRange(value: string | null): { start: number; end: number; total: number } {
  const match = value?.match(/^bytes (\d+)-(\d+)\/(\d+)$/)
  if (!match) throw new Error('视频分段范围无效，已停止拼接；请重试完整下载。')
  const [start, end, total] = match.slice(1).map(Number)
  if (![start, end, total].every(Number.isSafeInteger) || end < start || total <= end) {
    throw new Error('视频分段范围无效，已停止拼接；请重试完整下载。')
  }
  return { start, end, total }
}

/**
 * One GET of the existing result; a rejected range can cause one full GET, never task creation.
 * A retry resumes only this in-memory session, with an unchanged local file and strong ETag.
 */
export async function downloadVideoToFile(options: {
  identity: string
  session: VideoFileDownloadSession
  request: (headers: Record<string, string>, signal: AbortSignal) => Promise<Response>
  signal: AbortSignal
  onProgress?: (progress: VideoDownloadProgress) => void
  idleTimeoutMs?: number
}): Promise<{ fileName: string; bytes: number; resumed: boolean }> {
  const { session, signal, onProgress } = options
  if (!options.identity || session.identity !== options.identity) throw new Error('下载任务已变化，请重新选择保存文件。')
  if (session.active) throw new Error('该文件正在下载，请先暂停当前下载。')
  if (signal.aborted) throw signal.reason ?? new DOMException('下载已暂停', 'AbortError')
  session.active = true
  const controller = new AbortController()
  const abort = () => controller.abort(signal.reason ?? new DOMException('下载已暂停', 'AbortError'))
  signal.addEventListener('abort', abort, { once: true })
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined
  let writable: VideoWritable | undefined
  let written = 0
  let modified = false
  let transferTotal: number | undefined
  let transferETag: string | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  let preservePartial = true

  const bounded = async <T>(operation: () => Promise<T>): Promise<T> => {
    if (controller.signal.aborted) throw controller.signal.reason
    let onAbort: () => void = () => {}
    const interrupted = new Promise<never>((_, reject) => {
      onAbort = () => reject(controller.signal.reason)
      controller.signal.addEventListener('abort', onAbort, { once: true })
      timer = setTimeout(() => controller.abort(new Error('下载读取超时，已保留可用片段；可重试下载已有任务。')), options.idleTimeoutMs ?? 60_000)
    })
    try { return await Promise.race([operation(), interrupted]) }
    finally { clearTimeout(timer); controller.signal.removeEventListener('abort', onAbort) }
  }
  const checkpoint = async () => {
    if (!writable || !modified) return
    await writable.truncate(written)
    await writable.close()
    writable = undefined
    const saved = await session.handle.getFile()
    if (saved.size !== written) throw new Error('本地文件写入不完整，请重新下载已有任务。')
    session.bytes = written
    session.lastModified = saved.lastModified
    session.total = transferTotal
    session.etag = transferETag
  }

  try {
    const local = await session.handle.getFile()
    let offset = !session.complete && session.etag && session.bytes > 0
      && local.size === session.bytes && local.lastModified === session.lastModified ? session.bytes : 0
    let restarted = session.bytes > 0 && offset === 0
    const headers: Record<string, string> = offset ? { Range: `bytes=${offset}-`, 'If-Range': session.etag! } : {}
    let response = await bounded(() => options.request(headers, controller.signal))
    if (response.status === 416 && offset > 0) {
      // A stale/invalid offset never proves completion. Start one full transfer instead.
      await response.body?.cancel()
      offset = 0
      restarted = true
      session.etag = undefined
      response = await bounded(() => options.request({}, controller.signal))
    }
    if (response.status !== 200 && response.status !== 206) {
      await response.body?.cancel()
      throw new Error(response.status === 410 ? '视频结果已过期，请查看任务记录。' : `视频下载失败（${response.status}），可重试下载已有任务。`)
    }
    const type = response.headers.get('content-type') || 'video/mp4'
    const encoding = response.headers.get('content-encoding')
    if (/json|text\/|html/i.test(type) || (encoding && encoding.toLowerCase() !== 'identity')) {
      await response.body?.cancel()
      throw new Error('视频内容或编码不适合直接保存，请使用普通完整下载。')
    }
    const length = size(response.headers.get('content-length'))
    transferETag = strongETag(response.headers.get('etag'))
    let expectedSegment = length
    if (response.status === 206) {
      try {
        const range = contentRange(response.headers.get('content-range'))
        if (range.start !== offset || range.end !== range.total - 1
          || (length !== undefined && length !== range.end - range.start + 1)
          || (offset > 0 && (!transferETag || transferETag !== session.etag || (session.total !== undefined && session.total !== range.total)))) {
          throw new Error('视频分段或版本发生变化，已停止拼接；再次点击将从头下载已有任务。')
        }
        transferTotal = range.total
        expectedSegment = range.end - range.start + 1
      } catch (error) {
        session.etag = undefined
        await response.body?.cancel()
        throw error
      }
    } else {
      // If-Range can legitimately return 200 after a version change or without range support.
      restarted ||= offset > 0
      offset = 0
      transferTotal = length
    }
    reader = response.body?.getReader()
    if (!reader) throw new Error('浏览器未提供视频下载流。')
    if (controller.signal.aborted) throw controller.signal.reason
    writable = await session.handle.createWritable({ keepExistingData: true })
    written = offset
    session.complete = false
    if (offset) await writable.seek(offset)
    else await writable.truncate(0)
    modified = true
    onProgress?.({ received: written, total: transferTotal, resumed: offset > 0, restarted })
    let segment = 0
    for (;;) {
      const { done, value } = await bounded(() => reader!.read())
      if (done) break
      if (!value.byteLength) continue
      if (expectedSegment !== undefined && segment + value.byteLength > expectedSegment) {
        preservePartial = false
        throw new Error('视频实际长度超过声明长度，已停止写入；请重新下载已有任务。')
      }
      await writable.write(value)
      written += value.byteLength
      segment += value.byteLength
      onProgress?.({ received: written, total: transferTotal, resumed: offset > 0, restarted })
    }
    if (!written || (expectedSegment !== undefined && segment !== expectedSegment)
      || (transferTotal !== undefined && written !== transferTotal)) {
      throw new Error('视频下载中断，已保留片段；可重试下载已有任务。')
    }
    if (controller.signal.aborted) throw controller.signal.reason
    await checkpoint()
    session.complete = true
    return { fileName: session.handle.name, bytes: written, resumed: offset > 0 }
  } catch (error) {
    if (writable && modified && preservePartial) {
      try { await checkpoint() }
      catch { session.bytes = 0; session.etag = undefined; await writable?.abort().catch(() => {}) }
    } else if (writable) {
      await writable.abort().catch(() => {})
      if (!preservePartial) { session.bytes = 0; session.etag = undefined }
    }
    throw error
  } finally {
    clearTimeout(timer)
    controller.abort()
    await reader?.cancel().catch(() => {})
    reader?.releaseLock()
    signal.removeEventListener('abort', abort)
    session.active = false
  }
}
