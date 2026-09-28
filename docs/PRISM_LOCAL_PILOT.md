# SUB 原账号池的 Prism 协议：本地试验版

> 历史本地验证记录（2026-09-27）。当前 `2.8.18-kitty.4` 候选的范围与上线边界以 `PRISM_RELEASE_20260928.md` 为准；此文不作为部署或真实工作区认证回执。

日期：2026-09-27；来源：codex。

这份实现把 Prism 作为 SUB 已有 GPT OAuth 账号的可选上游协议，沿用 SUB 的客户 API Key、分组、模型映射、账号选择和计费入口。没有另建站点、客户系统或独立号池。默认关闭，未部署到 kittyyyds，也未操作 kittyapi。

## 请求如何走

```text
管理员把自己的 GPT OAuth 账号导入 SUB
  → 在原账号编辑页绑定该账号的 Prism 网页会话、选择模型
客户拿 SUB API Key 发请求
  → SUB 原有鉴权/分组/模型与账号选择
  → 对选中的 Prism 模型：Project/YSweet/Sandbox → start/status
  → 验证上游完成状态、模型声明、本次响应 usage
  → 转成 Responses / Chat Completions，交给 SUB 原有用量记录
```

现有 Excel/BPS 能用 OAuth bearer 和 ChatGPT account ID；所参考的 Prism 源码使用 `prism.openai.com` Cookie，未提供从普通 GPT OAuth 凭据自动换取 Prism 会话的实现。因此本版复用**同一条 SUB GPT 账号记录**，但需要管理员补绑 Prism 会话。导入 OAuth 后不补绑不能走 Prism。当前没有通过上游身份接口证实 Cookie 和 OAuth 属于同一用户；请绑定自己的对应会话，不宣称自动验证或额度跨产品一致。

## 已实现的范围

- 已有 GPT OAuth 母账号编辑页新增 Prism 开关、映射后模型范围、Cookie 密码输入和可选 User ID。
- Cookie 以 AES-GCM 密文保存；从配置的持久 TOTP 根密钥进行 HMAC 用途隔离派生，错误密钥/密文拒绝。前端仅看到是否绑定，不返回明文或密文；保留原 OAuth 凭据。
- 只对明确选择的模型进入 Prism，其余模型按原路径处理。Prism 与 Excel 同模型配置冲突会拒绝；UI 要求先关另一协议再启用。
- `/v1/responses` 与 `/v1/chat/completions` 的完整文本历史、system/developer 指令、明确模型和 reasoning effort。
- 非流式 JSON、上游完成后再转换为 SSE。此 SSE 是**完成后输出**，不是实时逐 Token 转发；首字时间不会伪造为 0。
- 同一进程内按账号串行；每次请求新建 Project/Sandbox/Conversation，提交完整历史，防止不同客户共享上下文。
- 固定官方 HTTPS origin，禁止重定向；不转发客户 Authorization、Codex 票据或 BPS 认证头。
- 任何无法确认完成的模型请求不自动重放、不自动换号。已发起但结果不明时，该账号的 Prism 通道在当前进程内隔离，后续请求返回 `prism_outcome_unknown`。
- 使用 SUB 原本的实际模型/用量/计费管线，不包含作者仓库写死的价格或“必定满血”判定。

## 本地如何验证

首先跑纯本地回归，不需要账号，不消费额度，不连接 Prism：

```powershell
cd D:\AI-Storage\Codex\Workspaces\kitty-ranxi-local-20260926\backend
go test ./internal/service/prism -count=1
go test ./internal/service ./internal/handler/dto -run 'TestPrism|TestAccountPrism' -count=1
```

测试通过可证明我们自己的转换、分流和保护逻辑符合 mock 合同，不能证明未公开的 Prism 接口已被真实账号接受。

需要做真实隔离验收时，在**独立本地 SUB 实例**中沿用正常数据库/Redis 配置，并准备持久且妥善备份的 `TOTP_ENCRYPTION_KEY`；不能换掉已有实例的根密钥。该值由正常 SUB 配置加载，必须是明确配置的密钥而非启动时随机生成。仅在该本地进程设置：

```powershell
$env:SUB2API_PRISM_ENABLED = 'true'
```

然后用这份源码构建/启动本地实例（不等于已经重新编译或重启现有 4319 预览），登录管理员后台：

1. 按原方式导入自己的 GPT OAuth 账号。
2. 打开该账号「编辑」，关闭此账号的 Excel/BPS 开关，才能在当前 UI 开启 Prism。后端支持不重叠模型范围，但当前表单采用两项开关互斥。
3. 启用「Prism 协议（实验）」，明确选择允许的模型或全部范围。
4. 绑定同账号登录 Prism 后的会话 Cookie。不要把 Cookie 发给客户、提交 Git 或贴进日志。
5. 按原方式把账号放入测试分组，为自己的本地测试用户创建 SUB API Key。
6. 客户端 Base URL 填本地 SUB 的 `/v1` 地址，API Key 填 SUB Key，模型填刚刚配置且该账号确实有权限的名称。

请求示例（API Key 仅是占位符）：

```http
POST /v1/chat/completions
Authorization: Bearer <本地SUB测试Key>
Content-Type: application/json

{"model":"<已验证的模型名>","messages":[{"role":"user","content":"回复一个简短的问候"}],"stream":false}
```

## 当前拒绝的能力和账务边界

图片/文件、客户工具、WebSocket、Anthropic Messages、compact、previous_response_id、远程会话续接、结构化输出、采样和输出长度限制等尚未实现的字段会明确拒绝，不能通过丢字段伪装成功。不要把此首版当作 Codex 工具调用的完整替代。

`service_tier` 先遵循 SUB 已有 Fast 过滤/阻断策略。只有管理员明确配置的 filter 才会按原策略移除该字段；处理后仍指定 tier，或分组强制 Priority 的请求会明确拒绝，因为本版尚不能保证 Prism 的服务档位。

必须同时满足：上游明确完成、返回的 `payload.model` 与实际发送模型一致、存在本次响应的 `payload.usage` 且输入/缓存/输出数字有效。只读取上游模型声明，不能据此证实模型权重或所谓“降智”。

`codex_live_progress` 中的 `total_token_usage` 可能是会话累计值，`last_token_usage` 也未证实覆盖整个客户请求；它们不能进入客户账单。缺少可验证 usage、缺失/不符模型或不完整响应时返回明确错误和 nil 用量结果，不扣客户余额，也不自动重试。上游账号仍可能已经消耗额度。

若返回 `prism_outcome_unknown`，必须先核实上游旧任务状态。不要通过自动重启解除隔离后盲发；当前隔离状态仅在进程内，生产化还需要持久任务回执、跨进程账号锁和可审计的恢复操作。

每请求独立项目避免共享状态，但会产生额外的上游工作区和初始化开销；尚未添加自动删除项目，避免错误清理真实资料。需要完成真实协议/容量/工作区生命周期验收之后再设计复用和清理。

## 开关、回退与未完成项

关闭账号的 Prism 开关或取消 `SUB2API_PRISM_ENABLED=true` 可停止该通道；全局关闭时已选择 Prism 的请求会返回明确禁用错误，不偷偷回到 Codex。关闭账号开关会恢复其原协议设置，绑定会话密文保留。没有新增数据库表/迁移，没有改旧迁移 checksum；配置存入现有 JSONB。

上线前仍需：真实 Prism 会话端到端验证；真实客户 API Key 的完整鉴权/账单/余额不变量验证；实际模型和 usage 回执；跨实例锁/未知任务恢复；会话到期更新；项目生命周期；图片/工具/真实流式支持的单独验收。本轮不执行真实付费推理、不更改生产配置。

## 2026-09-27 本地验证回执

- 协议 mock：15 个顶层测试通过，含子例共 50 个通过。
- 宿主、DTO、仓储与 Excel/账号策略回归：113 个顶层测试通过，含子例共 317 个通过；真实 PostgreSQL 用例 1 个跳过。
- 账号编辑组件 92 项、i18n 3 项通过；前端 typecheck、4 个改动文件 ESLint 通过。
- 四个后端包的 `go vet`、`cmd/server` 编译检查和 `git diff --check` 通过。
- 最后补齐 Fast 策略后，Prism 宿主回归 20 个顶层通过（含子例56），涵盖两个入口的强制档位/阻断/显式过滤；重新 `go vet ./internal/service` 通过。此批包含前轮重复用例，不直接相加。

上述为本地实现验证，未使用真实 Prism 会话。仓储 SQL 的 mock 回归已通过，但自动审批拒绝启动本地隔离 PostgreSQL（`pg_ctl start` 返回 `blocked by policy`），没有真实 PG 事务回执。保留 `TestPrismOAuthRefreshPreservesCurrentBindingPostgres`，通过专用本地 `PRISM_TEST_POSTGRES_DSN` 启用；测试只使用连接内 TEMP 表。详细时间和原始日志见 D 盘证据目录的 `validation-summary.md`。

## 参考和实现

协议参考：[FaFengFei1961/prism-ai-gateway](https://github.com/FaFengFei1961/prism-ai-gateway/tree/2df3e7314273b559f9aa732dfceef8331ccc777e)，固定提交 `2df3e7314273b559f9aa732dfceef8331ccc777e`。只研究 HTTP 契约，Go 适配/安全和 SUB 代码独立实现，未整体复制作者的号池、管理后台、价格表、提示词注入或本地工具执行功能。

关键文件：`backend/internal/service/prism/client.go`、`account_prism.go`、`openai_prism.go`，以及 `frontend/src/components/account/EditAccountModal.vue`。测试回执保存在 D 盘 `Reviews/prism-sub2api-20260927`。
