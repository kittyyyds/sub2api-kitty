# Kitty 本地画布预览

画布单独构建为 Next.js 服务，同源挂载 `/canvas`，入口是 `/canvas/canvas`。Kitty 主站负责 `/api/v1/canvas/*` 与 `/canvas-api/v1/*`；反向代理应完整保留 `/canvas` 前缀转给画布服务。画布原 Go 服务不参与 Kitty 部署。

```powershell
cd web
npm ci --no-audit --no-fund
$env:NEXT_PUBLIC_KITTY_MODE='true'
npx tsc --noEmit
node --test tests/kitty-integration.test.mjs
npm run build
npm run start -- --hostname 127.0.0.1 --port 3101
```

容器候选使用仓库根目录 `docker build -f Dockerfile.kitty -t kitty-canvas:preview .`，内部端口 3101。Kitty 标志必须在构建时设置；原始非 Kitty 构建保持现有模式。

认证从同源主站已有 `localStorage.auth_token` 读取。画布 store 中不保存主站 token，也不接收或保存完整 API Key。Key 元数据与模型目录由 bootstrap 接口读取；频道 ID 固定为 `kitty-key-<id>`。Key 被禁用、删除、过期或不支持当前模型时，当前节点须重新选择，不会自动换 Key。接口仍由 SUB 再次核验权限与计费条件。

工程、素材、技能和生成历史写入 `kitty-canvas-user-<userID>` 的 IndexedDB；所有数据库操作等待 bootstrap 完成后再确定用户。浏览器内退出或切换本站会话会重新加载并停止旧会话的请求与存储操作。画布原工程、配置、素材和 S3/WebDAV 云同步在此模式关闭。页面提供返回 Kitty 与本站视频任务链接。

网页 API Agent 默认模式不变，默认不自动提交媒体生成。本机 Codex 保留为高级可选连接。音频生成功能尚未接入，不列为可用模型。本机 Codex 不保存连接 token，账号切换或刷新后需显式重新连接。

九个算力视频型号按能力目录显示分辨率、画面比例、时长与引用上限，提交 JSON 保持三族各自字段。模型需要可公开读取的图片/音频 URL；可在视频节点设置中每行填写一条链接，或连接有公开链接的素材节点。浏览器 Blob、data URL、本地文件不会被秘密上传到图床。

视频设置按 Key、模型与时长显示本站最终单价和估费。点击生成后重新读取报价并确认，确认后再次检查价格；报价失败、取消、参数变化后的旧报价或价格变化均不会提交任务。

视频创建后保存任务 ID 与创建 Key ID，查询固定使用 `/canvas-api/v1/videos/:id`。生成完成立即显示“视频已生成”，点击节点“下载 / 取消”才开始读取 `.../:id/content`，下载完成校验字节并保存本地 Blob 后才标记本地已保存。取消、失败或重试下载不会重新提交视频。下载目前在浏览器内组装 Blob，尚非磁盘流式大文件下载或 Range 续传。

工程导出遇到引用的 Blob 缺失或未保存时明确失败，不生成静默缺素材的压缩包。导入同样检查压缩包内声明的素材。工程导出剔除 token/API Key 字段，仅保留 Key 引用。

后续验收使用 mock 响应或已有任务，不把类型检查、JSON 测试、页面预览当作九型号真实付费生成已通过。新增付费生成应另行确认型号与预算。
