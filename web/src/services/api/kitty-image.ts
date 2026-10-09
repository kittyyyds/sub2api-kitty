import { kittyHeaders, kittyToken } from "@/lib/kitty";
import type { AiConfig } from "@/stores/use-config-store";

export type KittyImagePayload = { data?: Array<Record<string, unknown>>; error?: { message?: string }; id?: string; status?: string; result?: KittyImagePayload };

// One paid POST, followed only by owner-authenticated local task GETs. Capture
// the selected Key and session so changing channels never redirects a task.
export async function requestKittyImage(config: AiConfig, prompt: string, images: string[], size?: string): Promise<KittyImagePayload> {
    const session = kittyToken();
    const headers = kittyHeaders(config, "application/json");
    const idempotencyKey = crypto.randomUUID();
    const body = { model: config.model, prompt, n: 1, stream: false, response_format: "b64_json", ...(size ? { size } : {}), ...(images.length ? { image: images } : {}) };
    const response = await fetch("/canvas-api/v1/images/generations", { method: "POST", headers: { ...headers, "Idempotency-Key": idempotencyKey, Prefer: "respond-async" }, body: JSON.stringify(body), signal: AbortSignal.timeout(330000) });
    let data: KittyImagePayload = await response.json();
    if (!response.ok) throw new Error(data.error?.message || "图片提交未确认，请查看任务记录，勿重复生成");
    if (data.data?.length) return data;
    const id = data.id;
    if (!id || !/^simg_[a-f0-9-]{36}$/.test(id)) throw new Error("图片任务编号无效");
    const deadline = Date.now() + 30 * 60 * 1000;
    let errors = 0;
    while (Date.now() < deadline) {
        await new Promise(resolve => setTimeout(resolve, 3000));
        if (kittyToken() !== session) throw new Error("登录账号已变化，请在原账号查询图片任务");
        let poll: Response;
        try {
            poll = await fetch(`/canvas-api/v1/images/tasks/${id}`, { headers, cache: "no-store", signal: AbortSignal.timeout(25000) });
            if (poll.status >= 500) throw new Error("图片状态暂时不可用");
            data = await poll.json();
        } catch (error) {
            if (++errors >= 5) throw new Error(`图片查询暂时失败，保留任务 ${id}，请勿重新生成`);
            continue;
        }
        errors = 0;
        if (!poll.ok) throw new Error(data.error?.message || "图片任务查询失败");
        if (data.status === "completed") {
            if (!data.result?.data?.length) throw new Error(data.error?.message || "图片结果已过期");
            return data.result;
        }
        if (data.status === "failed" || data.status === "needs_review") throw new Error(data.error?.message || "图片任务未完成");
    }
    throw new Error(`图片仍在生成，任务 ${id}，请勿重复生成`);
}

export function kittyImageSize(quality: string, size: string): string | undefined {
    if (/^\d+x\d+$/.test(size)) return size;
    const ratio = /^(\d+):(\d+)$/.exec(size);
    if (!ratio) return undefined;
    const w = Number(ratio[1]), h = Number(ratio[2]);
    if (!w || !h) return undefined;
    const edge = /^(high|4k)$/i.test(quality) ? 4096 : /^(medium|hd|2k)$/i.test(quality) ? 2048 : 1024;
    const scale = edge / Math.max(w, h);
    return `${Math.max(64, Math.round(w * scale / 64) * 64)}x${Math.max(64, Math.round(h * scale / 64) * 64)}`;
}
