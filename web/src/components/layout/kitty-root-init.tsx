"use client";

import { App } from "antd";
import { registerKittyQuoteConfirmation } from "@/services/api/kitty-quote";

import { useEffect, useState, type ReactNode } from "react";
import { kittyToken, loadKittyBootstrap, loadKittyChannels, mergeKittyConfig } from "@/lib/kitty";
import { useConfigStore } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";

export function KittyRootInit({ children }: { children: ReactNode }) {
    const { modal } = App.useApp();
    useEffect(() => registerKittyQuoteConfirmation((quote, model) => new Promise<boolean>((resolve) => {
        modal.confirm({ title: "确认视频生成费用", content: <div className="space-y-2"><p>{model}</p><p>生成单价 ${quote.unit_price} / {quote.unit}，本次 {quote.billing_mode === "video" ? `${quote.seconds} 秒` : `${quote.count} 条`}</p>{quote.output_cost !== undefined && <p>生成费用：${quote.output_cost}</p>}{(quote.reference_seconds || 0) > 0 && <p>参考视频 {quote.reference_seconds} 秒 × ${quote.reference_per_second} / 秒，参考费用 ${quote.reference_cost}</p>}<p>预估费用：<strong>${quote.estimated_cost}</strong></p><p className="text-xs opacity-70">{quote.note}</p></div>, okText: "确认并生成", cancelText: "取消", onOk: () => resolve(true), onCancel: () => resolve(false) });
    })), [modal]);
    const [ready, setReady] = useState(false);
    const [error, setError] = useState("");
    useEffect(() => {
        let canceled = false;
        const token = kittyToken();
        const checkSession = () => { if (kittyToken() !== token) window.location.reload(); };
        window.addEventListener("storage", checkSession);
        window.addEventListener("focus", checkSession);
        const interval = window.setInterval(checkSession, 1000);
        void (async () => {
            const bootstrap = await loadKittyBootstrap();
            await useConfigStore.persist.rehydrate();
            const channels = await loadKittyChannels();
            if (canceled) return;
            useConfigStore.setState((state) => ({ config: mergeKittyConfig(state.config, channels) }));
            // The host JWT stays in its existing auth_token slot, never in canvas state.
            useUserStore.setState({ token: "", isReady: true, user: { id: String(bootstrap.user.id), username: bootstrap.user.username, displayName: bootstrap.user.username, avatarUrl: "", role: "user", credits: bootstrap.user.balance, createdAt: "", updatedAt: "" } });
            setReady(true);
        })().catch((value) => { if (!canceled) setError(value instanceof Error ? value.message : "画布初始化失败"); });
        return () => { canceled = true; clearInterval(interval); window.removeEventListener("storage", checkSession); window.removeEventListener("focus", checkSession); };
    }, []);
    if (!ready) return <div className="flex min-h-dvh flex-col items-center justify-center gap-4 bg-background p-6 text-foreground"><p>{error || "正在读取 Kitty 身份与模型权限…"}</p>{error ? <><a href="/login">返回 Kitty 登录</a><button type="button" onClick={() => window.location.reload()}>重新读取</button></> : null}</div>;
    return children;
}
