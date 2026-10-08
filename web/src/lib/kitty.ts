import type { AiConfig, LocalModelChannel, ModelCapability } from "@/stores/use-config-store";

export const KITTY_MODE = process.env.NEXT_PUBLIC_KITTY_MODE === "true";
export const canvasAssetPath = (path: string) => `${KITTY_MODE ? "/canvas" : ""}${path}`;
export type KittyCapability = { media_type: "text" | "image" | "video"; supported: boolean; provider?: string; family?: "seedance" | "sd25" | "h3" | "unified"; seconds?: { min: number; max: number; fixed?: number }; second_options?: number[]; resolutions?: string[]; aspect_ratios?: string[]; reference_images?: { min: number; max: number }; reference_audios?: { min: number; max: number }; reference_videos?: { min: number; max: number }; max_reference_seconds?: number; face?: boolean; generate_audio?: boolean; operations?: string[] };
export type KittyModel = { id: string; capabilities: KittyCapability };
export type KittyKey = { id: number; name: string; key_display: string; status: string; group_id: number; group?: { id: number; name: string; platform: string }; expires_at?: string | null };
export type KittyBootstrap = { user: { id: number; username: string; email: string; balance: number }; keys: KittyKey[]; storage_mode: "local"; capabilities_version: string };

let session: KittyBootstrap | null = null;
let sessionToken = "";
let bootstrapPromise: Promise<KittyBootstrap> | null = null;
const catalogs = new Map<string, KittyModel[]>();
export const kittyToken = () => typeof window === "undefined" ? "" : window.localStorage.getItem("auth_token") || "";

export async function kittyRequest<T>(path: string, options: { method?: "POST"; body?: unknown } = {}): Promise<T> {
    const token = kittyToken();
    if (!token) throw new Error("请先登录 Kitty 主站，再进入画布。");
    const response = await fetch(path, { method: options.method, body: options.body === undefined ? undefined : JSON.stringify(options.body), headers: { Authorization: `Bearer ${token}`, ...(options.body === undefined ? {} : { "Content-Type": "application/json" }) }, cache: "no-store" });
    const payload = await response.json().catch(() => null);
    if (!response.ok || !payload || payload.code !== 0) throw new Error(payload?.message || payload?.msg || `读取画布权限失败（${response.status}）`);
    if (token !== kittyToken()) throw new Error("登录账号已变化，请重新打开画布。");
    return payload.data as T;
}

export function loadKittyBootstrap(force = false): Promise<KittyBootstrap> {
    if (!KITTY_MODE) return Promise.reject(new Error("Kitty 模式未启用"));
    const token = kittyToken();
    if (sessionToken && sessionToken !== token) return Promise.reject(new Error("登录会话已变化，请重新打开画布。"));
    if (!bootstrapPromise || force) {
        bootstrapPromise = kittyRequest<KittyBootstrap>("/api/v1/canvas/bootstrap").then((value) => {
            if (!value.user?.id || value.storage_mode !== "local") throw new Error("画布身份或本地存储配置无效");
            session = value;
            sessionToken = token;
            return value;
        }).catch((error) => { bootstrapPromise = null; throw error; });
    }
    return bootstrapPromise;
}

export function assertKittySession() {
    if (!session || !sessionToken || sessionToken !== kittyToken()) throw new Error("登录会话已变化，请返回 Kitty 重新登录并打开画布。");
    return session;
}

export async function kittyStorageOwner() {
    await loadKittyBootstrap();
    return String(assertKittySession().user.id);
}

export function kittyCapability(model: string, channelId?: string): KittyCapability | undefined {
    if (channelId) return catalogs.get(channelId)?.find((item) => item.id === model)?.capabilities;
    for (const models of catalogs.values()) {
        const match = models.find((item) => item.id === model);
        if (match) return match.capabilities;
    }
}

export function kittyModelMatches(model: string, capability?: ModelCapability, channelId?: string) {
    const value = kittyCapability(model, channelId);
    return Boolean(value?.supported && (!capability || value.media_type === capability));
}

export function kittyKeyUsable(key: KittyKey, now = Date.now()) {
    return key.status === "active" && (!key.expires_at || Date.parse(key.expires_at) > now);
}

export async function loadKittyChannels(force = false): Promise<LocalModelChannel[]> {
    const data = await loadKittyBootstrap(force);
    const channels = await Promise.all(data.keys.map(async (key): Promise<LocalModelChannel> => {
        const id = `kitty-key-${key.id}`;
        let models: KittyModel[] = [];
        let problem = "";
        if (kittyKeyUsable(key)) {
            try { models = (await kittyRequest<{ models: KittyModel[] }>(`/api/v1/canvas/keys/${key.id}/models`)).models || []; }
            catch (error) { problem = error instanceof Error ? error.message : "目录读取失败"; }
        } else problem = "停用或过期";
        catalogs.set(id, models);
        return { id, protocol: "openai", name: `${key.name || "Key"} · ${key.key_display} · ${key.group?.name || "未分组"}${problem ? `（${problem}）` : ""}`, baseUrl: "/canvas-api/v1", apiKey: "", models: models.filter((model) => model.capabilities?.supported && model.capabilities.media_type !== ("audio" as string)).map((model) => model.id) };
    }));
    return channels;
}

export function kittySelectedChannelId(config: AiConfig): string {
    const capability = kittyCapability(config.model)?.media_type;
    return capability === "video" ? config.videoChannelId : capability === "image" ? config.imageChannelId : capability === "text" ? config.textChannelId : config.activeChannelId;
}

export function kittyHeaders(config: AiConfig, contentType?: string): Record<string, string> {
    const data = assertKittySession();
    const id = kittySelectedChannelId(config);
    const key = data.keys.find((item) => `kitty-key-${item.id}` === id);
    if (!key || !kittyKeyUsable(key)) throw new Error("所选 Key 已删除、停用或过期，请为当前节点重新选择 Key。");
    if (!kittyModelMatches(config.model, undefined, id)) throw new Error("当前 Key 不支持这个模型，请为当前节点重新选择模型。");
    return { Authorization: `Bearer ${sessionToken}`, "X-Canvas-Key-ID": String(key.id), ...(contentType ? { "Content-Type": contentType } : {}) };
}

export function mergeKittyConfig(config: AiConfig, channels: LocalModelChannel[]): AiConfig {
    const next = { ...config, channelMode: "local" as const, baseUrl: "/canvas-api/v1", apiKey: "", localChannels: channels, publicChannels: [], models: [...new Set(channels.flatMap((item) => item.models))], syncStorageConfig: false, syncWebDAVStorageConfig: false, audioModel: "", audioChannelId: "", audioModels: [] };
    for (const capability of ["image", "video", "text"] as const) {
        const modelKey = `${capability}Model` as const;
        const channelKey = `${capability}ChannelId` as const;
        next[`${capability}Models`] = next.models.filter((model) => kittyModelMatches(model, capability));
        // A saved selection is an explicit spending route; never replace it implicitly.
        if (!config[channelKey]?.startsWith("kitty-key-")) {
            const channel = channels.find((item) => item.models.some((model) => kittyModelMatches(model, capability, item.id)));
            next[channelKey] = channel?.id || "";
            next[modelKey] = channel?.models.find((model) => kittyModelMatches(model, capability, channel.id)) || "";
        }
    }
    return next;
}
