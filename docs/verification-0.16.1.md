# 0.16.1 项目协作续开发独立验证

日期：2026-10-17，北京时间 08:00 开始。平台：Windows amd64，Go 后端 + Wails / WebView2 / React。全部模型请求为本机可控模拟请求，未使用用户已配置服务、密钥或付费额度。

## 接续基线

凌晨任务因额度限制中断，交接与 0.16.0 验证文件缺失。上午核对聊天状态、相关进程、源文件时间和已有日志；无 OpsBuddy 实例或同工程开发进程，未抢占其他任务。保留凌晨实现，源码基线：`.cache/before-agent-continuation-20261017-0800`。工程没有 Git 元数据，因此没有伪造 commit/PR。

凌晨 EXE 在约 02:20 构建，早于 02:23 新增的桌面按钮测试。历史证据单独补档于 [0.16.0 记录](verification-0.16.0.md)，本轮对最终新 EXE 独立验收。

## 复现与修复

1. **规划接受尾随内容**：原校验只调用一次 JSON Decode，接受有效计划后附加第二段 JSON 或文本。回归修复前两种输入均被错误执行，4 次模拟请求且组状态 succeeded。修复后要求完整单个 JSON；组 blocked、规划任务 failed，仅有 1 次规划请求，没有成员派发。空/超长摘要也标记规划失败。
2. **旧参考候选误入合并**：引用旧 `main.go` 候选，让新成员交付替代源码，原行为把旧材料和新候选一起合并，错误报冲突。现冻结 `TaskConfig.agentCandidateIds`，只选本轮成员最后成功候选，原始参考继续提供给模型但明确标注不自动合并。不同本轮成员的同路径冲突仍拒绝，原件不改写。
3. **返工链路扩展验证**：前端 v1→审阅 rework→前端 v2→审阅 pass→集成→真实 Go 服务。审阅请求检查到了实际源码；第二次审阅只包含 v2，不混入 v1；前端调用 2 次、后端 1 次、审阅 2 次。旧参考 `main.go` 内容不变，最终源码为 v2 前端与原成功后端。
4. **交互补齐**：首次审阅使用“首次审阅”；手动未设置 reviewer 时明确提示。保留现有计划、成员模型、依赖、状态、验收条件、真实任务回执与文件视图。没有重写 UI、工作流或队列。

修复前后证据：`.cache/agent-continuation-regressions-before.log`（预期失败）、`.cache/agent-continuation-regressions-after.log`（通过）。新增回归位于 `internal/app/agent_continuation_test.go`。

## 后端与构建检查

实际运行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test.ps1
npm.cmd run format:check
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-desktop.ps1
```

- 全部 `go test ./... -count=1` 通过：app 19.472s、desktop 0.843s、pet 0.237s，其余包无测试。`go vet ./...` 通过，TypeScript/Vite 通过。
- 格式检查通过。Vite 保留约 504.38 kB 主包的大小提示，不影响构建；本轮不顺带重构全站资源。
- 新增三个独立 HTTP 监听端点的模型路由验证，分别为 OpenAI-compatible、Anthropic、Gemini。实际断言 URL、协议 payload、模型标识和不同认证头，reviewer/main 请求确实含上游返回内容，不向独立分支发送兄弟结果或密钥。调用数 main=2、anthropic=1、gemini=1。
- 原有多 Agent 测试全部重新通过：DAG/未知节点/跨项目材料拒绝、冻结模型配置、独立分支实际并行、依赖等待、结构化审阅、最多两轮返工与一次重试、追加要求使运行下游失效、取消队列与请求、历史成果保留、旧实体读取及重启中断不重放、文件职责与大小写不敏感冲突。
- 核对 `store.go` 的共享容量 3 信号量、`runTask` 获取/释放与取消检查，所有成员、规划及汇总仍使用同一执行队列。成员数 6、委派层数 1 的校验保留，没有第二套运行时。
- 办公链路生成并解包/读取真正 DOCX，预算 120、数量 3 一致。Go 集成测试实际请求 `/api/add?a=7&b=5` 得到 sum=12、source=go-backend，非法整数返回 400，并验证运行日志和停止接口。

日志：`.cache/agent-continuation-final-tests.log`、`.cache/agent-continuation-format.log`、`.cache/agent-continuation-build-final.log`。

## 最终 EXE 验收

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-agent-desktop.ps1 -RunName smoke-agent-0161-evidence
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-agent-desktop.ps1 -RunName smoke-agent-0161-compact -Compact
```

脚本新建 `.cache/<RunName>`，启动只监听 loopback 的模拟模型，再用 `--data` / `--smoke-test` 启动发行 EXE。嵌入前端直接操作真实 DOM 按钮和进程内 Go API；不是独立的替代网页。成功后应用正常退出，脚本收尾自己的模拟进程。重新执行需使用新目录名，禁止覆盖旧数据。

| 场景 | 完成时间（北京时间） | 结果 |
| --- | --- | --- |
| 默认 1280×840 窗口 | 08:09:24 | passed=true，phase=complete |
| 1000×700 窗口 | 08:10:10 | passed=true，phase=complete |

两次均验证：

- 主会话填写目标、打开协作入口、自动规划、独立模型成员、依赖与审阅记录出现。
- DOCX 是真实 OpenXML 包且可读取，内容包含预算 120、数量 3。
- 前端和 Go 后端源码集成；真实预览按钮发起接口请求并显示 12，来源为 go-backend，预览不能访问桌面桥。
- 文件树包含 `index.html`、`main.go`、`go.mod`；点开源码看到真实函数与 API 路径，日志含 `agent-go-backend-ready`；停止按钮使 Go 运行状态变为 stopped。
- 手动切换成员模型、提交追加指令、停止整组；状态与追加要求持久化，未派发依赖节点。
- 既有工作流、知识库、项目会话、文件预览与桌面设置自检仍通过。

证据目录每次含 `smoke-result.json`、`verification.json`、`model-receipts.json`、真实 DOCX/代码文件和独立数据库；每次记录到 13 个完成的本机模拟请求，不将其换算为 token 或费用。取消挂起请求不计入完成回执。

### 验证过程中发现的环境/脚本问题

- 首次受限环境启动 WebView2 子进程失败，没有自检报告；正常关闭超时后只终止了本轮隔离测试进程。改用允许桌面子进程的环境后 EXE 自检通过，未打开用户真实数据。
- 曾在前端构建尚未结束时使用 SkipFrontend，发现嵌入目录仍是较早产物；已完整顺序重建，再核对全部嵌入文件与 dist。最终自检均针对修正后的 EXE。
- 一次 EXE 已报告 passed=true，但 Windows PowerShell 默认 ANSI 读取中文 JSON 导致脚本解析失败。脚本已显式 UTF-8 读取；随后上表两次均脚本 exit=0。失败尝试保留在 `.cache/smoke-agent-0161-final`、`.cache/smoke-agent-0161-verified` 及对应日志，不算最终通过证据。

## 发行与数据保全

- 最终 EXE：`release/OpsBuddy/OpsBuddy.exe`，FileVersion / ProductVersion **0.16.1**，27,260,928 字节。
- SHA-256：`109EA6CF064190EFA5E5D34C089B4ADD44403FB237E2A8D990E074BF3E8DBB16`。
- 便携 ZIP：`release/OpsBuddy-Windows-amd64.zip`，仅 `OpsBuddy.exe` 与 `README.txt`，没有用户数据。代码项目 ZIP 仍仅由用户主动导出。
- 替换前无旧桌面实例；整个真实数据目录备份至 `.cache/user-data-before-0161-20261017-0800`，414 个文件、71,972,782 字节。旧 EXE 留在 `.cache/OpsBuddy-before-0161.exe`。
- 真实 `.opsbuddy` 的全部 414 个文件 SHA-256 前后相同，证据 `.cache/agent-continuation-user-data-verification.json`。未读取模型明文、未迁移/修改用户数据库、未运行用户定时任务。
- 08:13 最终核对：13 个嵌入前端文件全部与 dist 一致，414 个备份文件哈希匹配；EXE 哈希与两次通过报告完全一致，剩余桌面/fixture 进程为 0。证据 `.cache/agent-continuation-final-verification.json`。

## 边界与后续

上述模型是测试专用 fixture，不作为产品规划/交付兜底；生产执行器不导入 `internal/agentfixture`。尚未验证用户的实际供应商网关、付费模型或任意复杂项目。独立模型审阅是基于提供内容的判断，不等于任意测试命令执行；本轮演示另有真实 Go API 与桌面按钮验收。

DOCX 完成包结构及文字读取验证，未在 Word/WPS 逐页渲染验收复杂排版。没有完整 token/费用预算、通用工具循环、运行中修改 DAG、协作计划自动投影到工作流画布、自动恢复中断或自动修复集成冲突。上限当前固定。正常使用最终代码运行器仍需本机 Go/Node/Python；未提供操作系统级代码沙箱或自动安装依赖。未来应在这些边界内继续，避免再造功能。

## 2026-10-19 续接复核

本轮先核对上午续开发成果，确认当前为已完成验收的 0.16.1；未覆盖源码、重建发行物或重复开发。

- `go test ./internal/app -run 'TestAgent' -count=1` 通过，`ok opsbuddy/internal/app 8.955s`。使用工程内 Go 缓存和独立测试临时目录；日志：`.cache/agent-resume-20261019-tests.log`。
- 当前 EXE SHA-256 与 10 月 17 日两份通过的最终桌面验收报告一致；13 个嵌入前端文件与 dist 全部匹配。证据：`.cache/agent-resume-20261019-audit.json`。
- 复核时没有 OpsBuddy/fixture 遗留进程。本轮没有调用真实付费模型，没有修改用户数据库，没有重新运行桌面自检；桌面交付证据仍是上述 10 月 17 日两次验收。
- 上午轮已完成核心需求，剩余边界继续以上文记录为准。本轮只补充复核记录，不把已有实现记作新开发。
