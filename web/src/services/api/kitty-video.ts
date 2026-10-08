import { assertKittySession, kittyToken, type KittyKey } from "@/lib/kitty";
import { uploadMediaFile } from "@/services/file-storage";
import type { VideoResponse } from "./video";
import { downloadVideoToFile, type VideoFileDownloadSession, type VideoDownloadProgress } from "@/lib/video-download";

export async function kittyVideoDelivery(task: VideoResponse, signal?: AbortSignal) {
    const path = `/canvas-api/v1/videos/${encodeURIComponent(task.task_id || task.id)}`;
    const response = await fetch(path, { headers: kittyVideoTaskHeaders(task), signal, cache: 'no-store' });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error?.message || '视频交付状态暂时不可用');
    if (data.delivery_status === 'expired' || data.status === 'expired') throw new Error('上游视频已过期');
    if (data.delivery_mode !== 'upstream_direct') return { url: path + '/content', direct: false };
    if (data.status !== 'completed' || typeof data.url !== 'string') throw new Error('视频尚未交付，请继续查询原任务');
    const url = new URL(data.url);
    if (url.protocol !== 'https:' || url.username || url.password) throw new Error('视频直链无效');
    return { url: url.href, direct: true };
}
async function fetchKittyVideoContent(task: VideoResponse, rangeHeaders: Record<string, string>, signal: AbortSignal) {
    const delivery = await kittyVideoDelivery(task, signal);
    const headers: Record<string, string> = {};
    for (const [name, value] of Object.entries(rangeHeaders)) if (/^(range|if-range)$/i.test(name)) headers[name] = value;
    // The site's session token must never be attached to a supplier/CDN URL.
    if (!delivery.direct) Object.assign(headers, kittyVideoTaskHeaders(task));
    return fetch(delivery.url, { headers, signal, cache: 'no-store', credentials: 'omit', referrerPolicy: 'no-referrer', redirect: delivery.direct ? 'follow' : 'error' });
}

export function kittyVideoTaskHeaders(task: VideoResponse) {
    const session = assertKittySession();
    const keyId = task.kittyKeyId;
    const key: KittyKey | undefined = session.keys.find((item) => String(item.id) === keyId);
    if (!key || !keyId) throw new Error("任务绑定的 Key 不可用，请到本站视频任务页查询，画布不会切换其他 Key。");
    return { Authorization: `Bearer ${kittyToken()}`, "X-Canvas-Key-ID": keyId };
}

export function kittyVideoDownloadIdentity(task: VideoResponse): string {
    const owner = assertKittySession().user.id;
    const taskId = task.task_id || task.id;
    if (!taskId || !task.kittyKeyId) throw new Error("视频任务或绑定 Key 缺失");
    kittyVideoTaskHeaders(task);
    return `${owner}:${task.kittyKeyId}:${taskId}`;
}

export async function downloadKittyVideoToFile(task: VideoResponse, session: VideoFileDownloadSession, signal: AbortSignal, onProgress: (progress: VideoDownloadProgress) => void) {
    const identity = kittyVideoDownloadIdentity(task);
    const result = await downloadVideoToFile({
        identity, session, signal,
        request: (headers, requestSignal) => fetchKittyVideoContent(task, headers, requestSignal),
        onProgress: (progress) => {
            if (kittyVideoDownloadIdentity(task) !== identity) throw new Error("任务身份已变化，下载已停止。");
            onProgress(progress);
        },
    });
    assertKittySession();
    return result;
}

export async function downloadKittyVideoBlob(task: VideoResponse, signal: AbortSignal, onProgress: (received: number, total?: number) => void): Promise<Blob> {
    const taskId = task.task_id || task.id;
    if (!taskId) throw new Error("视频任务 ID 缺失");
    const controller = new AbortController();
    const abort = () => controller.abort(signal.reason);
    signal.addEventListener("abort", abort, { once: true });
    if (signal.aborted) abort();
    let timer = setTimeout(() => controller.abort(new Error("下载读取超时，可重新下载已有结果")), 60_000);
    try {
        const response = await fetchKittyVideoContent(task, {}, controller.signal);
        if (!response.ok) throw new Error(response.status === 410 ? "视频结果已过期" : `视频下载失败（${response.status}），可重试下载已有任务`);
        if (response.status === 206) {
            const range = response.headers.get("content-range")?.match(/^bytes 0-(\d+)\/(\d+)$/);
            if (!range || Number(range[1]) + 1 !== Number(range[2])) throw new Error("视频接口只返回部分内容，请重新下载完整文件。");
        }
        const type = response.headers.get("content-type") || "video/mp4";
        if (/json|text\//i.test(type)) throw new Error("视频内容接口返回了错误信息，请重试下载或查看本站任务。");
        const total = Number(response.headers.get("content-length")) || undefined;
        if (total && total > 512 * 1048576) { await response.body?.cancel(); throw new Error('视频超过512MB，请使用保存到文件'); }
        const chunks: Uint8Array<ArrayBuffer>[] = [];
        const reader = response.body?.getReader();
        if (!reader) throw new Error("浏览器未提供视频下载流");
        let received = 0;
        for (;;) {
            const { done, value } = await reader.read();
            clearTimeout(timer);
            if (done) break;
            timer = setTimeout(() => controller.abort(new Error("下载读取超时，可重新下载已有结果")), 60_000);
            chunks.push(new Uint8Array(value));
            received += value.byteLength;
            if (received > 512 * 1048576) { await reader.cancel(); throw new Error('视频超过512MB，请使用保存到文件'); }
            onProgress(received, total);
        }
        if (!received || (total && received !== total)) throw new Error("视频下载不完整，请重新下载已有任务。");
        assertKittySession();
        return new Blob(chunks, { type });
    } finally { clearTimeout(timer); signal.removeEventListener("abort", abort); }
}

export async function downloadKittyVideo(task: VideoResponse, signal: AbortSignal, onProgress: (received: number, total?: number) => void) {
    const blob = await downloadKittyVideoBlob(task, signal, onProgress);
    const stored = await uploadMediaFile(blob, "kitty-video");
    return { ...stored, blob };
}
