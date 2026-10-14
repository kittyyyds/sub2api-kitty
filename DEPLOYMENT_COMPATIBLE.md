# Kitty 兼容版部署与更新

此候选版本为 2.8.18-kitty.4，在已部署 2.8.18-kitty.3 / 340510bc63f95021ef34161601c786ab50175921 基础上合入 SUB 内置 Prism 手动 Cookie 实验协议，保留 ranxi v2.8.18 的非观察者更新，保留 Kitty 充值及永久赠送、图片与视频计费、Canvas 和媒体、账户请求策略及客户界面。管理后台使用相同配色与字体。旧 Kitty 智能测试、采票和 Grok 分组提示词实现退役；已有历史数据与迁移身份保留。

本轮不引入观察者角色、权限或初始化，也不引入上游新增但从未部署的观察者迁移 252_user_observer_groups.sql。相对 2.8.15-kitty.2 / 6e3949df120b57ee87b7d1362596a0a40d242e40，迁移文件清单及内容全部相同；部署前仍需现场核对已应用迁移与 checksum，不能跳过现有保护。

## 普通更新与回退

Prism 默认全局关闭且账号关闭，只有显式启用全局开关、逐账号填写 Cookie 并选择模型后才会进入此协议。仅有本地 mock、单元回归与构建验证；未完成真实 Prism 工作区认证、端到端推理或真实账单验证，不代表任何账号已经获得工作区/模型资格。本版不提供 Prism Bearer 模式、自动登录、浏览器 helper 或自动绑定。详细范围和限制见 `docs/PRISM_RELEASE_20260928.md`。来源：codex，2026-09-28。

本次 `2.8.18-kitty.4` 交付限于兼容包和源码上传服务器独立目录、禁网隔离验证；不切换生产运行版本。生产仍为 `2.8.18-kitty.3`，上传与运行状态必须分别记录。

更新页面使用原生 Release 流程，来源固定为私有仓库 `kittyyyds/sub2api-kittyyyds`。只接受版本格式 `X.Y.Z-kitty.N`、包含 `kitty-compatibility.json` 与 `checksums.txt` 的兼容包；原版 ranxi 包不能覆盖现有二开。

私有下载使用服务器现有只读发布凭据 `UPDATE_GITHUB_TOKEN`，不向浏览器下发。凭据只发到 GitHub API，资产重定向不携带凭据。

Linux 容器须将**完整可写目录**绑定到 `/runtime`，UID/GID 为 `1000:1000`，入口为 `/runtime/sub2api`，重启策略为 `unless-stopped` 或 `always`。单文件 bind mount 无法支持原子更新。运行文件、兼容 `.backup` 及证明文件均在此持久目录中，容器重建后仍保留更新结果。

更新后先等待活动请求完成再点击重启。Linux 重启发出 SIGTERM，执行服务已有关闭和账务清理链；普通关闭仍继承上游超时语义，不保证强行重启时任意长流或 WebSocket 完成。生产版本切换应先维护并自然排空，再启动新实例。

回退仅使用相同数据契约的 Kitty 兼容后端，不恢复旧数据库快照。融合后不可直接返回旧 `.4` 或 `.3` 后端，以免破坏新增的账号/分组限制与永久赠送语义。

## kittyyyds 运行约定

本次一次性切换会停用旧托管 updater，旧签名 current/history 保留为历史证据。新的普通运行规范与回执位于 `/opt/kittyyyds/runtime-deployment`；实际运行版本以进程版本接口和 `/runtime/sub2api --version` 为准。旧 `/opt/kitty-tb` Compose 和历史 bootstrap 均不得启动。

运维仅通过现有 1Panel HTTPS API。保持 PostgreSQL、Redis、配置与密钥、pages/plugins、视频引用、Canvas、CPA、现有域名及 ACME 路由。现有数据完整时使用原库；迁移前需完整备份、离机核验及真实隔离演练。

本文描述部署契约，不是上线成功回执。实际 UTC、精确源码和产物 SHA、备份及验收见任务离机证据。
