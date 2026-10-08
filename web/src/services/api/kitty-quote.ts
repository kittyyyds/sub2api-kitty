import { kittyCapability, kittyHeaders, kittyRequest } from "@/lib/kitty";
import type { AiConfig } from "@/stores/use-config-store";
import { kittyResolution } from '@/lib/kitty-unified-video';

export type KittyQuote = { billing_mode: "video" | "per_request"; unit: string; unit_price: number; seconds: number; count: number; estimated_cost: number; final_unit_price: boolean; quoted_at: string; note: string; reference_seconds?: number; reference_per_second?: number; reference_cost?: number; output_cost?: number; reference_videos?: string[] };
let confirmQuote: ((quote: KittyQuote, model: string) => Promise<boolean>) | null = null;

export function registerKittyQuoteConfirmation(confirm: typeof confirmQuote) {
    confirmQuote = confirm;
    return () => { if (confirmQuote === confirm) confirmQuote = null; };
}

export async function fetchKittyVideoQuote(config: AiConfig, request?: Record<string, unknown>): Promise<KittyQuote> {
    const key = kittyHeaders(config)["X-Canvas-Key-ID"];
    const seconds = kittyCapability(config.model, config.videoChannelId)?.family === "sd25" ? 0 : Number(config.videoSeconds);
    const cap = kittyCapability(config.model, config.videoChannelId);
    const body = cap?.family === 'unified' ? request || { model: config.model, seconds, resolution: kittyResolution(config.vquality,cap), aspect_ratio: cap.aspect_ratios?.includes(config.size) ? config.size : '16:9' } : { model: config.model, seconds, count: 1 };
    const quote = await kittyRequest<KittyQuote>(`/api/v1/canvas/keys/${key}/quote`, { method: "POST", body });
    if (!quote.final_unit_price || !Number.isFinite(quote.unit_price) || !Number.isFinite(quote.estimated_cost) || quote.unit_price < 0 || quote.estimated_cost < 0) throw new Error("报价无效，请刷新模型与 Key 后重试。");
    return quote;
}

export async function confirmKittyVideoQuote(config: AiConfig, body?: Record<string, unknown>) {
    const quote = await fetchKittyVideoQuote(config,body);
    if (body && quote.reference_videos?.length) body.videos = quote.reference_videos;
    if (!confirmQuote) throw new Error("报价确认尚未就绪，请刷新页面后重试。");
    if (!await confirmQuote(quote, config.model)) throw new Error("已取消视频生成，尚未提交任务。");
    // Recheck session/Key and current pricing after the user has read the quote.
    kittyHeaders(config);
    const current = await fetchKittyVideoQuote(config,body);
    if (current.unit_price !== quote.unit_price || current.estimated_cost !== quote.estimated_cost || current.billing_mode !== quote.billing_mode) throw new Error("报价已变化，请重新查看费用并确认后再生成。");
}
