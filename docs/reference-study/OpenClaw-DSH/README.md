# OpenClaw / DSH 参考与 OpsBuddy 改进方案

核对日期：2026-10-15。OpsBuddy 基线：0.7.0 Windows 桌面版。用户要求现有及后续功能均可参考这两个项目。

“龙虾”按 OpenClaw 理解；用户已明确确认 DSH 指 DeepSeek Harness。这次没有安装它们、启动外部服务、迁移用户数据或替换现有技术栈。

## 阅读顺序

1. [功能对照](01-功能对照.md)：现有、部分实现、尚未实现及对应源码。
2. [交互与运行设计](02-交互与运行设计.md)：OpsBuddy 自身的产品设计，包含输入、模型、工具、协作、工作流与成果。
3. [实施与验收](03-实施与验收.md)：依赖、开发顺序、具体完成标准和不应误判的成功信号。
4. 新增需求：[29 统一 Agent 运行与工具执行](../../requirements/29-统一Agent运行与工具执行.md)、[30 模型能力与会话交互](../../requirements/30-模型能力与会话交互.md)。已有文档 02、10–14、22–28 继续有效。

## 核实的参考事实

OpenClaw 将会话输入、上下文组装、模型推理、工具执行、流式和持久化放在 Agent 运行循环中；子 Agent 文档区分内部协作与可见独立会话，并提供后续指令和结果回传的设计入口。[Agent loop](https://docs.openclaw.ai/concepts/agent-loop)、[Sub-agents](https://docs.openclaw.ai/tools/subagents)。

DSH 官方将模型适配、工具注册、会话日志和 Agent loop 分为可替换插件，并用持久事件与实时流区分存档事实和界面增量。[官方仓库](https://github.com/deepseek-ai/deepseek-harness)、[架构](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/architecture.md)。官方展示插件管理和桌面入口，部分扩展标为实验功能，不能直接视为我们的稳定能力。[官方产品页](https://www.deepseek.com/en/harness/)。

下述需求是对 OpsBuddy 的设计建议，不是这两个项目逐项功能清单，也不声称它们具有相同的可视化工作流、办公排版、知识库或媒体编辑功能。

## 技术方向

保留 Go + Wails + React 和本地 SQLite；保留用户要求的桌面入口、模型优先、无“本地整理”聊天选项。借鉴接口分层与使用体验，不把 DSH 的 Node/Cordis 运行环境或 OpenClaw 网关直接塞入程序。插件格式和运行协议分别评估，不宣称既有插件可跨项目直接运行。

研究只产出了参考和待开发需求；没有发布新 EXE，当前可运行版本仍为 0.7.0。

本机 WorkBuddy 的桌面观察与参考事项另外记录在 [WorkBuddy 参考](../WorkBuddy/README.md)，三者共同作为后续需求依据，不改变桌面与 Go 后端约束。
