"use client";

import { useEffect, useState } from "react";
import { fetchKittyVideoQuote, type KittyQuote } from "@/services/api/kitty-quote";

import { InputNumber, Select } from "antd";
import { kittyCapability } from "@/lib/kitty";
import { normalizeSeedanceRatio } from "@/lib/seedance-video";
import type { AiConfig } from "@/stores/use-config-store";

export function KittyVideoSettings({ config, model, onChange }: { config: AiConfig; model: string; onChange: (key: "size" | "videoSeconds", value: string) => void }) {
    const capabilities = kittyCapability(model, config.videoChannelId);
    const [quote, setQuote] = useState<KittyQuote | null>(null);
    const [error, setError] = useState("");
    useEffect(() => {
        let canceled = false;
        setQuote(null); setError("");
        const timer = setTimeout(() => { void fetchKittyVideoQuote({ ...config, model }).then((value) => { if (!canceled) setQuote(value); }).catch((value) => { if (!canceled) setError(value instanceof Error ? value.message : "报价失败"); }); }, 250);
        return () => { canceled = true; clearTimeout(timer); };
    }, [model, config.videoChannelId, config.videoSeconds]);
    if (!capabilities?.supported) return <p className="text-sm">请先选择当前 Key 支持的视频型号。</p>;
    return <div className="space-y-4 text-sm">
        <div>分辨率：{capabilities.resolutions?.join(" / ") || "由模型决定"}</div>
        <label className="flex items-center justify-between gap-3">画面比例<Select value={normalizeSeedanceRatio(config.size)} options={(capabilities.aspect_ratios || ["16:9", "9:16", "1:1"]).map((value) => ({ label: value, value }))} onChange={(value) => onChange("size", value)} /></label>
        {capabilities.family === "sd25" ? <div>时长：模型固定，提交时不发送 seconds</div> : <label className="flex items-center justify-between gap-3">时长（秒）<InputNumber min={capabilities.seconds?.min || 5} max={capabilities.seconds?.max || 15} precision={0} value={Number(config.videoSeconds)} onChange={(value) => onChange("videoSeconds", String(value || 5))} /></label>}
        <div className="rounded-lg border border-border p-3" role="status">{error ? `报价失败：${error}（暂不能提交）` : quote ? <>单价 ${quote.unit_price} / {quote.unit}<br />预估费用 ${quote.estimated_cost}<p className="mt-1 text-xs opacity-70">{quote.note}</p></> : "正在读取报价…"}</div>
        <p className="text-muted-foreground">参考图片 {capabilities.reference_images?.min ?? 1}–{capabilities.reference_images?.max ?? 1} 张；参考音频最多 {capabilities.reference_audios?.max || 0} 条。可连接本地素材节点或填写公开链接。本地素材仅在确认生成时临时上传，24 小时后清理，每个文件最多 25MB；画布工程继续保存在本机。</p>
    </div>;
}
