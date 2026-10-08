"use client";

import { useEffect, useState } from "react";
import { fetchKittyVideoQuote, type KittyQuote } from "@/services/api/kitty-quote";

import { InputNumber, Select } from "antd";
import { kittyCapability } from "@/lib/kitty";
import { normalizeSeedanceRatio } from "@/lib/seedance-video";
import type { AiConfig } from "@/stores/use-config-store";
import { kittyResolution } from '@/lib/kitty-unified-video';

export function KittyVideoSettings({ config, model, onChange }: { config: AiConfig; model: string; onChange: (key: "size" | "videoSeconds" | "vquality" | "videoGenerateAudio", value: string) => void }) {
    const capabilities = kittyCapability(model, config.videoChannelId);
    const [quote, setQuote] = useState<KittyQuote | null>(null);
    const [error, setError] = useState("");
    useEffect(() => {
        if (capabilities?.family !== 'unified') return;
        const value = Number(config.videoSeconds), range = capabilities.seconds, choices = capabilities.second_options;
        if (!Number.isInteger(value) || (range && (value < range.min || value > range.max)) || (choices?.length && !choices.includes(value))) {
            onChange('videoSeconds', String(choices?.[0] || range?.min || 5));
        }
        if (!capabilities.aspect_ratios?.includes(config.size)) onChange('size', capabilities.aspect_ratios?.[0] || '16:9');
    }, [model, config.videoChannelId, config.videoSeconds, config.size, capabilities, onChange]);
    useEffect(() => {
        let canceled = false;
        setQuote(null); setError("");
        const timer = setTimeout(() => { void fetchKittyVideoQuote({ ...config, model }).then((value) => { if (!canceled) setQuote(value); }).catch((value) => { if (!canceled) setError(value instanceof Error ? value.message : "报价失败"); }); }, 250);
        return () => { canceled = true; clearTimeout(timer); };
    }, [model, config.videoChannelId, config.videoSeconds, config.vquality, config.size]);
    if (!capabilities?.supported) return <p className="text-sm">请先选择当前 Key 支持的视频型号。</p>;
    return <div className="space-y-4 text-sm">
        <label className="flex items-center justify-between gap-3">分辨率<Select value={kittyResolution(config.vquality,capabilities)} options={(capabilities.resolutions||[]).map(value=>({label:value,value}))} onChange={value=>onChange('vquality',value)} /></label>
        <label className="flex items-center justify-between gap-3">画面比例<Select value={capabilities.aspect_ratios?.includes(config.size) ? config.size : normalizeSeedanceRatio(config.size)} options={(capabilities.aspect_ratios || ["16:9", "9:16", "1:1"]).map((value) => ({ label: value, value }))} onChange={(value) => onChange("size", value)} /></label>
        {capabilities.family === "sd25" ? <div>时长：模型固定，提交时不发送 seconds</div> : <label className="flex items-center justify-between gap-3">时长（秒）{capabilities.second_options?.length ? <Select value={Number(config.videoSeconds)} options={capabilities.second_options.map(value=>({label:`${value} 秒`,value}))} onChange={value=>onChange('videoSeconds',String(value))} /> : <InputNumber min={capabilities.seconds?.min || 5} max={capabilities.seconds?.max || 15} precision={0} value={Number(config.videoSeconds)} onChange={(value) => onChange("videoSeconds", String(value || 5))} />}</label>}
        {capabilities.generate_audio ? <label className="flex items-center justify-between gap-3">生成声音<Select value={config.videoGenerateAudio} options={[{label:'开启',value:'true'},{label:'关闭',value:'false'}]} onChange={value=>onChange('videoGenerateAudio',value)} /></label> : null}
        <div className="rounded-lg border border-border p-3" role="status">{error ? `报价失败：${error}（暂不能提交）` : quote ? <>单价 ${quote.unit_price} / {quote.unit}<br />预估费用 ${quote.estimated_cost}<p className="mt-1 text-xs opacity-70">{quote.note}</p></> : "正在读取报价…"}</div>
        <p className="text-muted-foreground">参考图片 {capabilities.reference_images?.min ?? 0}–{capabilities.reference_images?.max ?? 0} 张；参考视频最多 {capabilities.reference_videos?.max || 0} 条；音频最多 {capabilities.reference_audios?.max || 0} 条。这里先显示输出费用，提交时测量参考视频并确认完整报价。素材最多50MB，在任务结束后的保留期到期清理，默认24小时；工程仍保存在本机。</p>
    </div>;
}
