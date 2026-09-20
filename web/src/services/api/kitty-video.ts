import { assertKittySession, kittyToken, type KittyKey } from "@/lib/kitty";
import { uploadMediaFile } from "@/services/file-storage";
import type { VideoResponse } from "./video";

export function kittyVideoTaskHeaders(task: VideoResponse) {
    const session = assertKittySession();
    const keyId = task.kittyKeyId;
    const key: KittyKey | undefined = session.keys.find((item) => String(item.id) === keyId);
    if (!key || !keyId) throw new Error("任务绑定的 Key 不可用，请到本站视频任务页查询，画布不会切换其他 Key。");
    return { Authorization: `Bearer ${kittyToken()}`, "X-Canvas-Key-ID": keyId };
}

export async function downloadKittyVideo(task: VideoResponse, signal: AbortSignal, onProgress: (received: number, total?: number) => void) {
    const taskId = task.task_id || task.id;
    if (!taskId) throw new Error("视频任务 ID 缺失");
    const controller = new AbortController();
    const abort = () => controller.abort(signal.reason);
    signal.addEventListener("abort", abort, { once: true });
    if (signal.aborted) abort();
    let timer = setTimeout(() => controller.abort(new Error("下载读取超时，可重新下载已有结果")), 60_000);
    try {
        const response = await fetch(`/canvas-api/v1/videos/${encodeURIComponent(taskId)}/content`, { headers: kittyVideoTaskHeaders(task), signal: controller.signal });
        if (!response.ok) throw new Error(response.status === 410 ? "视频结果已过期" : `视频下载失败（${response.status}），可重试下载已有任务`);
        if (response.status === 206) {
            const range = response.headers.get("content-range")?.match(/^bytes 0-(\d+)\/(\d+)$/);
            if (!range || Number(range[1]) + 1 !== Number(range[2])) throw new Error("视频接口只返回部分内容，请重新下载完整文件。");
        }
        const type = response.headers.get("content-type") || "video/mp4";
        if (/json|text\//i.test(type)) throw new Error("视频内容接口返回了错误信息，请重试下载或查看本站任务。");
        const total = Number(response.headers.get("content-length")) || undefined;
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
            onProgress(received, total);
        }
        if (!received || (total && received !== total)) throw new Error("视频下载不完整，请重新下载已有任务。");
        assertKittySession();
        const blob = new Blob(chunks, { type });
        const stored = await uploadMediaFile(blob, "kitty-video");
        return { ...stored, blob };
    } finally { clearTimeout(timer); signal.removeEventListener("abort", abort); }
}
