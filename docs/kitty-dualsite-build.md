# Kitty 双站前端构建

日期：2026-09-29；来源：codex。

`VITE_KITTY_SITE` 是构建时站点选择，支持 `kittyapi`（A）和 `kittyyyds`（B）。未设置时保持 B 兼容；其他值会在 Vite 启动时直接报错。部署时必须选择目标站点并使用对应构建产物。

| 站点 | 默认主入口 | 默认国内入口 | 售后 QQ 群 |
| --- | --- | --- | --- |
| A / kittyapi | https://kittyapi.xyz | https://cdn.kittyapi.xyz | https://qm.qq.com/q/oRR8GUdEPK |
| B / kittyyyds | https://kittyyyds.xyz | https://cdn.kittyyyds.xyz | https://qm.qq.com/q/Wr3pNr9zMe |

在 `frontend` 目录使用 PowerShell：

```powershell
$env:VITE_KITTY_SITE = 'kittyapi'
pnpm build

# 完成 A 包的后端嵌入/归档后，再构建 B。
$env:VITE_KITTY_SITE = 'kittyyyds'
pnpm build
Remove-Item Env:VITE_KITTY_SITE
```

POSIX shell 对应命令为 `VITE_KITTY_SITE=kittyapi pnpm build` 或 `VITE_KITTY_SITE=kittyyyds pnpm build`。开发模式同样支持该变量。也可将 `frontend/.env.example` 复制为 `.env.local` 并设置站点。

两次默认构建都会写入 `backend/internal/web/dist`，因此必须按站点分别嵌入/归档，不能并行写该目录。仅需独立前端验证产物时，可先运行 `pnpm run check:i18n` 和 `pnpm exec vue-tsc -b`，再用 `pnpm exec vite build --outDir <站点独立目录>`。

后台公开设置的优先级不变：`site_name`、`site_logo`、`contact_info`、`frontend_url` 继续由已有逻辑处理；文档示例优先使用 `api_base_url`，说明优先使用 `api_base_url_description`，额外入口优先使用有效 `custom_endpoints`。无额外入口配置时才显示本地站点的国内入口。OpenAI 示例将根地址规范化后添加一次 `/v1`。

新手页面优先嵌入后台有效的 HTTP(S) `doc_url`。未配置时直接显示本地接入文档，包含本站主入口及国内入口，不再默认跳转 `tb.kittyyyds.xyz`。`/docs` 和 `/custom/docs` 均保留现有路由。

客服 QQ `1722345668`（https://qm.qq.com/q/rU9mJYhImA）、微信 `ALe990411` 和 Telegram `https://t.me/kittyapi1` 沿用原配置。A 首页社区 QQ 群与控制台售后群独立：社区弹窗保留原群号文字、复制群号、原群链接和 `/sites/kittyapi/images/heyroute/community/qq-qr.png`；售后入口继续使用本站售后群和 `/sites/kittyapi/qq-group-qr.png`。B 首页社区和售后入口继续共同引用 B 售后群配置，语义不变。

A 的 5 张原图从旧 A 源码 `bb27f883a99b7c5ebcba91bc23a5bf659a6d4907` 逐字节复制至 `frontend/public/sites/kittyapi/`，保持原相对路径。B 原图及现有公开路径不变，来源 B 源码 `63a41ef3c1d259f7fe8f7c16ccd8a0e0bfd11cf1`。`src/config/__tests__/siteIdentity.spec.ts` 固定检查两套客服、售后群、首页社区 QQ 和 Telegram 图片的原始 SHA-256。

此配置不改写 `api.kittyyyds.xyz` 的运行时含义，不迁移数据库配置，不执行 DNS 或线上部署。
