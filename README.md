<p align="center"><img src="frontend/public/logo.svg" alt="KittyAPI" width="88" /></p>

# KittyAPI

**AI API 网关与运营平台**

KittyAPI 是 `kittyapi.xyz` 独立维护的二次开发项目。使用者通过统一的 API Key 调用已配置的模型，运营者在同一个后台管理上游账号、渠道、用量与采购成本。

[访问 KittyAPI](https://kittyapi.xyz/) · [版本发布](https://github.com/kittyyyds/sub2api-kitty/releases) · [问题反馈](https://github.com/kittyyyds/sub2api-kitty/issues)

中文 · [English](README_EN.md) · [日本語](README_JA.md)

## 核心功能

| 功能 | 说明 |
| --- | --- |
| 模型调用 | 支持 OpenAI 兼容的 Responses、Chat Completions 与流式响应。Responses 能否使用 WebSocket 取决于客户端和网关配置。 |
| 账号与密钥 | 管理已授权的 OAuth 或 API Key 上游，支持模型映射、分组、账号调度和并发控制。 |
| 用量与计费 | 按模型和分组配置计价，查看请求记录、Token 用量和费用。 |
| 渠道与运营 | 通过仪表盘和运营面板查看渠道状态、请求趋势和费用；按已配置的供应商及采购成本规则分析毛利。 |

## 使用、发布与反馈

- **使用 API**：在 [KittyAPI](https://kittyapi.xyz/) 创建或管理 API Key，选择有权限的分组与模型，按站内接入说明配置客户端。
- **查看源码或升级**：以本仓库的 [Release](https://github.com/kittyyyds/sub2api-kitty/releases) 和对应源码包为准。默认分支不等同于线上版本，不要用其他分支的安装脚本或产物覆盖 KittyAPI 部署。
- **提交问题**：提供版本、客户端、错误提示和脱敏后的复现步骤；不要粘贴 API Key、OAuth 凭据或客户数据。

## 源码来源与许可证

KittyAPI 基于 [Sub2API](https://github.com/Wei-Shaw/sub2api) 的开源代码二次开发，由 KittyAPI 独立维护和发布。保留的开源组件继续适用各自的版权声明与许可证，见 [LICENSE](LICENSE) 和 [第三方组件](third_party/)。请仅接入有权使用的上游服务，并遵守其使用条款。

Copyright (c) 2026 Wesley Liddick
