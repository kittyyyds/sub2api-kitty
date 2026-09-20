import { kittyCapability, kittyHeaders, kittyRequest } from "@/lib/kitty";
import type { AiConfig } from "@/stores/use-config-store";

export type KittyQuote = { billing_mode: "video" | "per_request"; unit: string; unit_price: number; seconds: number; count: number; estimated_cost: number; final_unit_price: boolean; quoted_at: string; note: string };
let confirmQuote: ((quote: KittyQuote, model: string) => Promise<boolean>) | null = null;

export function registerKittyQuoteConfirmation(confirm: typeof confirmQuote) {
    confirmQuote = confirm;
    return () => { if (confirmQuote === confirm) confirmQuote = null; };
}

export async function fetchKittyVideoQuote(config: AiConfig): Promise<KittyQuote> {
    const key = kittyHeaders(config)["X-Canvas-Key-ID"];
    const seconds = kittyCapability(config.model, config.videoChannelId)?.family === "sd25" ? 0 : Number(config.videoSeconds);
    const quote = await kittyRequest<KittyQuote>(`/api/v1/canvas/keys/${key}/quote`, { method: "POST", body: { model: config.model, seconds, count: 1 } });
    if (!quote.final_unit_price || !Number.isFinite(quote.unit_price) || !Number.isFinite(quote.estimated_cost) || quote.unit_price < 0 || quote.estimated_cost < 0) throw new Error("报价无效，请刷新模型与 Key 后重试。");
    return quote;
}

export async function confirmKittyVideoQuote(config: AiConfig) {
    const quote = await fetchKittyVideoQuote(config);
    if (!confirmQuote) throw new Error("报价确认尚未就绪，请刷新页面后重试。");
    if (!await confirmQuote(quote, config.model)) throw new Error("已取消视频生成，尚未提交任务。");
    // Recheck session/Key and current pricing after the user has read the quote.
    kittyHeaders(config);
    const current = await fetchKittyVideoQuote(config);
    if (current.unit_price !== quote.unit_price || current.estimated_cost !== quote.estimated_cost || current.billing_mode !== quote.billing_mode) throw new Error("报价已变化，请重新查看费用并确认后再生成。");
}
