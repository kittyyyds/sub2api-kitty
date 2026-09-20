"use client";

import { useState } from "react";
import { App, Button, Form, Modal } from "antd";
import { ModelPicker } from "@/components/model-picker";
import { loadKittyChannels, mergeKittyConfig } from "@/lib/kitty";
import { useConfigStore } from "@/stores/use-config-store";

export function KittyConfigModal() {
    const { message } = App.useApp();
    const { config, isConfigOpen, setConfigDialogOpen, updateConfig } = useConfigStore();
    const [loading, setLoading] = useState(false);
    const refresh = async () => {
        setLoading(true);
        try { const channels = await loadKittyChannels(true); useConfigStore.setState((state) => ({ config: mergeKittyConfig(state.config, channels) })); message.success("Key 与模型权限已更新"); }
        catch (error) { message.error(error instanceof Error ? error.message : "读取失败"); }
        finally { setLoading(false); }
    };
    return <Modal title="Kitty 画布设置" open={isConfigOpen} onCancel={() => setConfigDialogOpen(false)} footer={<Button type="primary" onClick={() => setConfigDialogOpen(false)}>完成</Button>}>
        <p className="mb-4 text-sm text-muted-foreground">工程、素材和历史仅保存在当前浏览器，按本站账号隔离。每个节点可以单独选择 Key 与模型。</p>
        <Form layout="vertical">
            {([ ["text", "默认文本模型"], ["image", "默认生图模型"], ["video", "默认视频模型"] ] as const).map(([kind, label]) => <Form.Item key={kind} label={label}><ModelPicker config={config} capability={kind} value={config[`${kind}Model`]} channelId={config[`${kind}ChannelId`]} fullWidth onChange={(model, channelId) => { updateConfig(`${kind}Model`, model); updateConfig(`${kind}ChannelId`, channelId || ""); }} /></Form.Item>)}
        </Form>
        <p className="mb-3 text-sm text-muted-foreground">音频生成暂不支持。网页 API Agent 默认启用；本机 Codex 可在 Agent 面板中按需连接。</p>
        <Button loading={loading} onClick={() => void refresh()}>刷新我的 Key 与模型</Button>
        <a className="ml-4" href="/keys" target="_blank" rel="noreferrer">管理本站 Key</a>
    </Modal>;
}
