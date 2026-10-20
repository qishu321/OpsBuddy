# 0.12.0 验证记录

日期：2026-10-16。Windows 桌面版，Go + Wails + 内嵌 React。

## 自动检查

- `scripts/test.ps1` 通过：全部 Go 测试、`go vet ./...`、TypeScript 与 Vite 构建。
- `npm run format:check` 通过。
- 新增 `TestProjectConversationContextIsolation`：A/B 项目通过本机模拟模型端点实际生成回复，检查请求正文；B 不含 A 的历史、项目说明或项目记忆，同项目追问继承正确历史；跨项目父会话、文件、知识库引用返回 400。
- 新增 `TestDeleteConversationCannotCrossProjectEvenWithLegacyIDCollision`：旧会话标识相同、协作成员引用错误时，删除 A 不影响 B。原删除持久化、成果保留、运行中拒绝与失败回滚测试继续通过。

## 桌面集成

新版 EXE 使用两个隔离数据目录启动 `--smoke-test`；正常窗口与 880×600 窗口均通过。证据：

- `.cache/smoke-projects-0120/smoke-result.json`
- `.cache/smoke-projects-0120-small/smoke-result.json`

`projectConversations` 七项均为 true：项目分组、打开时切换项目、项目内新建、草稿与预览清空、异步上传隔离、文件范围、搜索结果所属项目。

既有截图粘贴、Skills、知识库、工作流画布、定时任务、记忆、会话删除、Markdown/HTML 预览及真实后端服务运行检查也通过。测试在实际 WebView2 DOM 中触发控件及合成事件；不宣称完成物理鼠标、键盘或视觉截图检查。

## 交付

- `release/OpsBuddy/OpsBuddy.exe`，Windows 文件版本 0.12.0。
- `release/OpsBuddy-Windows-amd64.zip`，仅包含 EXE 与说明，不含用户数据。
- 替换前正常关闭旧版并备份 SQLite 到用户数据目录下的 `state-before-0.12.0-*.db`。未删除或重新归类用户会话，未读取或调用用户模型密钥。
