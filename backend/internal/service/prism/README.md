# Prism 协议适配器（本地试验）

2026-09-27：宿主已增加 GPT OAuth 账号编辑绑定、加密存储、文本 Responses/Chat
转发和账务边界。完整用法见仓库 `docs/PRISM_LOCAL_PILOT.md`。默认关闭，尚无真实
Prism 会话验收，也未部署。下文的宿主责任是边界说明，不表示功能全部已上线。

此包把 `prism.openai.com` 的浏览器型 start/status 流程隔离为一个可替换的
传输适配器，供后续 SUB 网关按账号协议分流使用。它只接收已经选定的单个
账号；账号调度、计费、失败转移和数据库仍由宿主服务负责。

当前协议包支持：

- `response_with_tools_start` + `response_with_tools_status` 的有界轮询；
- 可注入 `DoFunc`，由宿主复用自己的代理、TLS 和超时策略；
- Cookie 只写入出站请求头，错误不会回显 Cookie 或上游响应体；
- Project/YSweet/sandbox 初始化，并缓存当前客户端的 sandbox；
- 从 Prism 进度事件提取 token usage，完成响应转换为文本结果。

宿主职责与生产前验收：

1. 把账号的 `extra.openai_prism`、模型范围和解密后的会话映射到 `prism.Config`；
2. 将 `Result` 转换为 SUB 的 Responses/Chat Completions SSE 和统一计费事件；
3. 处理图片、工具事件、会话黏性和账号级熔断；
4. 在隔离环境用真实协议回执验证端点和字段，之后才允许上线。

测试既包含完整 bootstrap，也包含独立 start/status 的合成 `DoFunc`；不会连接 Prism
或读取真实 Cookie。`UsageSource=payload` 才表示响应级用量；累计/最后内层调用的
用量不能直接计费。`Error.RequestStarted` 为真时禁止盲重放；同一 client 会隔离
未知结果，宿主也必须在不同 client 间保留此状态并限制同账号并发。
