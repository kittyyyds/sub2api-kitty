# 2.8.18-kitty.4：Prism 手动 Cookie 实验协议

日期：2026-09-28；来源：codex。本文描述候选源码与功能边界，部署结果由单独的服务器回执确认。

本次交付范围是构建完整兼容包与源码，上传服务器独立目录并进行禁网隔离验证。上传文件不等于切换运行版本；生产继续运行 `2.8.18-kitty.3`，不启用 Prism 或改变真实账号。上传路径、制品校验与隔离验证结果以单独回执为准。

## 基线与保留范围

以生产 `2.8.18-kitty.3 / 340510bc63f95021ef34161601c786ab50175921` 为基线，三方合入已确认的本地 Prism 手动 Cookie 改动。保留 2.8.18 的修复、Excel/BPS 403 迁组配置、Kitty 二开、原账号数据、路由、计费和客户界面。不引入观察者用户角色或迁移 `252_user_observer_groups.sql`，不修改或新增数据库迁移。

Prism 作为现有普通 GPT/OpenAI OAuth 母账号的可选协议，客户继续使用 SUB API Key。此版本不是第三方 free-gpt-api，也没有另建客户系统或号池。

## 管理员操作

全局开关 `SUB2API_PRISM_ENABLED` 和每个账号的 Prism 开关都默认关闭。只有显式开启全局功能，并在原账号编辑页启用「Prism 协议（实验）」、手动填写 Cookie、选择模型范围后，选中的请求才进入 Prism。Prism User ID 可选；已有绑定时，Cookie 留空表示保留。

Cookie 使用现有持久 TOTP 根密钥经过用途隔离派生的 AES-GCM 密钥加密保存，详情接口不返回明文或密文。不能替换现有实例的根密钥。原 OAuth 凭据继续保留，但导入 OAuth 不会自动取得 Prism 会话；没有 Prism Bearer 模式、自动登录、一次性命令、浏览器 helper 或自动绑定路由。

手动保存 Cookie 与 OAuth 刷新并发时，仓储在锁定当前账号行后合并管理员明确变更的凭据字段，保留最新 OAuth token 与到期时间；反向刷新同样保留最新 Prism 绑定。显式解绑、并发解绑后的启用校验与事务失败回滚另有本地隔离 PostgreSQL 回归。

保存绑定只说明本地加密与保存成功，不能证明 Cookie 当前有效、与 OAuth 属于同一用户或具备工作区与模型权限。本次没有改变任何上游工作区资格或账号权限。

## 请求、账务与限制

- 支持文本 `/v1/responses` 与 `/v1/chat/completions`，保留文本历史、system/developer 指令和 reasoning effort。SSE 在上游完成后转换输出，并非实时逐 Token 流式。
- 明确选择的映射后模型才使用 Prism；Excel/BPS 与 Prism 的重叠模型配置会拒绝，当前编辑页要求两项开关互斥。关闭 Prism 不会自动重新打开之前关闭的 Excel/BPS。
- 每个请求新建上游 Project/Sandbox/Conversation，进程内按账号串行。未实现跨实例锁、持久任务回执和自动项目清理，尚不适用于多实例启用；额外项目会产生工作区与初始化开销。
- 只对明确完成、模型声明匹配且具备有效本次响应 usage 的结果进入原 SUB 计费管线。模型字段只是上游声明，不能证明模型权重。缺失可信 usage 或响应状态不明时不向客户计费，上游仍可能已消耗额度。
- 无法确认完成的请求不自动重放或换号，当前进程隔离该账号的 Prism 通道。`prism_outcome_unknown` 必须核实旧任务状态；不能靠重启后盲发解决。
- 未支持的工具、图片/文件、WebSocket、Anthropic Messages、compact、远程会话续接、结构化输出、采样及输出长度等能力明确拒绝。原 SUB Fast 过滤/阻断策略继续适用；无法保证的服务档位明确拒绝。

关闭账号 Prism 开关可停止该账号分流，绑定密文保留。关闭全局功能时，仍选中 Prism 的请求返回禁用错误，不会暗中切换协议。

## 验证边界

本次只执行本地 mock、现有单元回归、隔离 PostgreSQL 事务/并发回归、静态检查与前后端构建，不使用真实会话，不发真实付费推理。没有完成真实 Prism 工作区认证、端到端推理、实际模型/usage 回执或真实客户账单与余额不变量验证。上传默认关闭的实验代码不等于上述验证已完成。

历史本地设计与 2026-09-27 验证记录见 `PRISM_LOCAL_PILOT.md`；手动操作说明见 `PRISM_COOKIE_BINDING.md`。这些历史记录中的“未部署”“本轮不更改生产”仅描述当时状态，不替代当前发布回执。

候选集成、原始差异、迁移哈希、测试和构建日志保存在 D 盘 `Reviews/kitty-prism-deploy-20260928/integration`。发行工具从 `backend/cmd/server/VERSION` 生成 compatibility manifest 的版本，保留现有 Kitty 兼容能力和迁移校验合同。
