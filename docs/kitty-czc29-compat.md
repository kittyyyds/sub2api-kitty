# Kitty .29：四项 OpenAI 兼容修复

日期：2026-11-17；来源：codex。

基于已经上线的 `0.2.21-custom.1-kitty.28` 定向吸收 zuchengchen/sub2api
`czc-v2026.11.17.1` / `czc-v2026.11.17.2`（0.2.19.49 / 0.2.19.50）相关修复。
不是整体替换该项目。原 Kitty VM、智能 WS、codexwire/TLS、OAuth 身份隔离、
有界 401 恢复、容量重选和模型别名保护继续保留。

## 1. HTTP 请求体压缩独立开关

- 新增 `gateway.openai_request_compression_enabled`，**默认 false**。
- 环境变量：`GATEWAY_OPENAI_REQUEST_COMPRESSION_ENABLED`。
- 不设置即可使用新版默认策略，无需修改客户端或 API 地址。
- 只影响最终符合条件的 Codex OAuth/SetupToken Responses HTTP 出站。
  VM、智能 WS、Excel、API Key、其他平台、不启用 exact fidelity 或显式禁用 TLS
  profile 的路径不被强行压缩，也不为启用压缩而改道。
- TLS/HTTPS、身份头和业务 JSON 不因关闭请求压缩而关闭或删减；原生 WS 帧
  不受这个 HTTP 开关控制。WS 大帧桥接到 HTTP 后遵守相同最终 HTTP 策略。
- opt-in 为 true 时编码 zstd，并同步 Body、GetBody 和 ContentLength。
  读体或编码失败返回错误，不发送残缺/空的替代请求。
- 默认不压缩可能增加上行流量；并不等于关闭 TLS，也不影响 token 计量规则。

## 2. JSON 输出与隐式搜索工具兼容

`text.format.type=json_object/json_schema` 时，不再为了模拟 Codex 而隐式添加
`web_search`。Chat Completions 的 `response_format` 转换为 Responses 后同样适用。
调用者显式提供的工具、schema、已有搜索历史和工具结果不被粗暴删除。
没有新增面板开关，符合条件的请求自动生效。

## 3. 脱敏且有界的 400 结构诊断

在原运维上游错误事件中增加固定分类摘要，帮助区分通用 Bad Request、
消息 ID 校验和 JSON/search 冲突。包含输入类型/数量/大小、工具配对计数、
搜索来源、实际 HTTP 编码/长度/协议和尝试编号。

- 结构摘要不超过 4 KiB；未知类别只记为 `other`。
- 工具 call_id 只在内存中使用有界 hash 配对，不记录原始 ID。
- 摘要不记录提示词、对话正文、schema 正文、工具结果或授权凭据。
- 这不等于开启原系统的完整请求/响应正文日志，也不替代既有日志隐私设置。
- 只添加诊断，不把所有 400 自动重试、换账号或忽略错误。

## 4. 显式关闭推理的记录和分组策略一致性

请求显式 `reasoning.effort=none`、`reasoning_effort=none` 或 Messages 的
`thinking.type=disabled` 时，保留原始请求意图 `none`，与实际发送强度分开记录。
`thinking.disabled` 优先于 Messages 的 `output_config.effort`。

Kitty .28 已有较完整的 Messages 策略绑定和复合平台保护，本轮保留而不是覆盖它，
并补充记录与转发回归。**本轮不新增或修改任何实际分组映射**。
如果特定模型不接受 none，管理员可按业务需要，在目标分组保留原规则并追加：

```json
{"from":"none","to":"low","match_type":"exact","model":"gpt-6-astra"}
```

只有明确配置该规则才转换；low 会启用推理，可能增加耗时和用量。
合法 high、其他模型、未明确指定强度的请求不因上述规则被统一改写。
不支持 none 且没有合法映射的请求仍按原规则报错，不能声称关闭推理已被接受。

## 验证边界与发布

使用合成账号、loopback 模拟上游和隔离数据库验收完整 JSON、工具调用、图片、
Unicode、HTTP 重放、WS 入口桥接和既有 OAuth/VM/TLS 行为。
隔离验收无外网，不使用生产业务账号或收费模型请求；模拟通过不等于已确认真实
上游的 403、账号资格、限流、模型质量或所谓“打票”问题得到解决。

前端静态资源与数据库迁移保持 .28 一致。发布仅替换主应用镜像；保留旧镜像和
上线前全量数据库备份。代码回退只恢复旧应用，不用旧数据库覆盖客户的新写入。
