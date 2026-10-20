# OpsBuddy

**轻量 AI 办公与开发桌面助手 · Go + React + Wails**

简体中文 | [English](README.en.md)

[下载预览版](https://github.com/qishu321/OpsBuddy/releases) · [构建状态](https://github.com/qishu321/OpsBuddy/actions/workflows/release.yml) · [实现状态](docs/implementation-status.md)

OpsBuddy 把对话、日常文档、代码项目与预览放在一个桌面工作空间。配置自己的模型服务，让多个角色处理同一项目，保留执行记录和源码。应用使用系统 WebView，不捆绑 Electron 或独立浏览器，也不需要常驻 Python 服务。

当前版本：**v0.18.0，开发预览版**。支持 Windows x64 与 Linux x64；原生桌宠暂仅 Windows 支持。

## 常用能力

- **对话与项目**：会话按项目分组，项目之间隔离上下文；历史搜索、删除、归档和继续修改。
- **添加材料**：选择文件/文件夹，粘贴截图或拖入图片；选择知识库、Skills 和声明式插件。图片理解取决于模型能力。
- **日常办公**：生成可编辑 DOCX、PPTX、XLSX，读取与处理 PDF，整理文件，生成和编辑 Markdown。
- **开发与预览**：单文件 HTML 和多文件 Go / Node.js / Python 项目，右侧文件树、源码编辑、启动/停止、日志和网页预览。运行代码需要本机相应语言环境。
- **多 Agent 编排**：主模型制定计划或手动指定角色；成员独立模型与上下文，按依赖并行、审阅、有限重试/返工、集成最终代码。提供真实编排画布、暂停调整和恢复。
- **工作流与知识库**：可视化连线与条件分支，项目文档切片、关键词检索和来源引用。
- **定时任务与记忆**：本机运行计划、独立会话、偏好和项目经验管理。定时执行需要应用保持运行。
- **模型接入**：OpenAI-compatible Chat Completions、Anthropic Messages、Gemini GenerateContent；使用自己的 API Key。
- **Windows 桌宠**：同进程原生像素小猫、低帧率动画和任务事件反馈，无额外模型请求。

## 下载与启动

在 [Releases](https://github.com/qishu321/OpsBuddy/releases) 选择平台，校验文件可见 `SHA256SUMS.txt`。

| 平台 | 下载与要求 |
| --- | --- |
| Windows 10/11 x64 | `OpsBuddy.exe` 或 `OpsBuddy-Windows-amd64.zip`；需要 Microsoft Edge WebView2 Runtime |
| Linux x64 | `OpsBuddy-Linux-amd64.tar.gz`；基于 Ubuntu 24.04 构建，需要 GTK3、WebKit2GTK 4.1、图形桌面和 `xdg-open` |

Windows：解压到可写目录，双击 `OpsBuddy.exe`。应用本身无需 Node.js、Go 或浏览器启动；只有运行生成代码才需要对应工具链。未签名的开发预览版可能触发系统信誉提示。

Ubuntu 24.04 / 兼容系统：

```bash
sudo apt install libgtk-3-0t64 libwebkit2gtk-4.1-0 xdg-utils
tar -xzf OpsBuddy-Linux-amd64.tar.gz
./opsbuddy
```

Linux 包不是静态通用二进制，其他发行版需要对应运行库；当前不提供 ARM64、macOS 或 AppImage。`opsbuddy.desktop` 可在将可执行文件加入 PATH 后复制至个人应用菜单目录。

首次启动：**设置 → 添加模型 → 填写协议、Base URL、模型 ID 和 API Key → 测试**，返回工作台发送任务。Enter 发送、Shift+Enter 换行。代码和文件成果在右侧预览；ZIP 仅在主动导出项目或分发程序时使用。

## 多 Agent 示例

在项目内输入「做一个办公待办看板，前端与 Go 后端分工实现，再独立审阅」，从输入框的「更多任务选项 → 派发子任务」进入。可让主模型规划，或指定 worker、researcher、explorer、reviewer 的职责和模型。角色不是另一套专家库。

主会话「编排画布」展示实际依赖、成员任务和回执；支持暂停后修改计划再继续。最多6位成员、全局3并发、1层委派，节点最多1次自动重试、最多2轮审阅返工。模型请求会产生所选服务的费用。

仓库包含 [办公待办示例源码](demos/multi-agent-todo/project) 和历史项目包，可在「项目 → 导入项目」选择 `demos/multi-agent-todo/todo-demo.opsbuddy-project.json`。导入不重新调用模型或自动执行代码；在右侧点击「本机运行」后预览。详见 [协作指南](docs/multi-agent-guide.md)。

## 从源码构建

需要 **Go 1.26+、Node.js 22+、npm**；首次安装依赖需要联网。

```bash
git clone https://github.com/qishu321/OpsBuddy.git
cd OpsBuddy
npm ci
```

Windows PowerShell：

```powershell
./scripts/build-desktop.ps1
# 输出 release/OpsBuddy/OpsBuddy.exe
# 可选：./scripts/build-desktop.ps1 -Package
```

Linux（Ubuntu 24.04）：

```bash
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
bash scripts/build-desktop.sh
# 输出 release/linux/opsbuddy 和 release/OpsBuddy-Linux-amd64.tar.gz
```

Linux 必须在 Linux 上构建，使用 CGO、GTK3 和 `webkit2_41` 构建标签。依赖依据 [Wails v2 安装说明](https://v2.wails.io/docs/gettingstarted/installation/)。`npm run build` 只构建嵌入界面，不启动网页服务；`npm run dev` 为 Windows 桌面开发入口。应用没有面向用户的独立 Web 版本。

## 数据与凭据

- Windows 默认数据目录：EXE 旁 `.opsbuddy/`。
- Linux 默认数据目录：`${XDG_CONFIG_HOME:-~/.config}/opsbuddy/`。
- 两个平台都可用 `--data /绝对路径` 指定目录。退出应用后备份整个数据目录；不要让多个实例同时打开同一数据库。
- Windows 的模型密钥使用当前用户 DPAPI 加密。
- Linux 使用 AES-256-GCM，随机用户密钥保存在 `${XDG_CONFIG_HOME:-~/.config}/opsbuddy-credentials/key`，目录0700、密钥0600。它是文件级本地加密，不是硬件密钥库；能读取数据库和该密钥的人可以解密。迁移 Linux 数据时需要一并安全备份该密钥。
- 跨平台搬迁后重新配置 API Key，Windows 密文不能在 Linux 解密。任务快照不包含 API Key；主动提交的内容和所选附件会发送给选择的模型服务。
- 个人会话、数据库、缓存、密钥、运行产物不会包含在 GitHub 源码或发行包中。示例包只包含演示记录和源码。

## 验证与发布

Windows：`powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test.ps1`。

Linux：`go test -tags webkit2_41 ./...`、`go vet -tags webkit2_41 ./...`、`npm run build`。原生界面验收可运行 `bash scripts/test-linux-desktop.sh`（需已构建及安装 `xvfb dbus-x11`），使用独立目录和虚拟显示，不调用真实模型。

GitHub Actions 对 Windows/Linux 分别测试、构建；推送与版本匹配的 `v*` tag 后，全部通过才发布双平台预览版及 SHA-256 校验。详情见 [发布说明](docs/releases/v0.18.0.md)。

## 边界

Office 当前是基础内容生成和内容预览，不是完整 Office 排版渲染器。知识库使用关键词检索，暂不提供向量检索或 OCR。模型适配器目前为非流式；MCP 支持初始化与工具发现，尚未接入工具执行。未实现通用工具循环、任意自动安装依赖、无限委派、关闭应用后的系统级定时服务，或操作系统级代码沙箱。Linux 暂无原生桌宠。

## 目录

```text
cmd/opsbuddy-desktop/  Windows/Linux 桌面入口与嵌入界面
internal/app/         Go 任务、模型、存储、办公与编排
internal/desktop/     Wails 原生文件操作与请求桥接
internal/pet/         Windows 原生桌宠和共享图像资源
src/                  React / TypeScript 界面
scripts/              构建、启动和验收脚本
.github/workflows/    双平台测试、构建和 tag 发布
docs/                 需求、指南与验证记录
demos/                示例源码和可导入演示项目
```

问题或建议请提交到 [Issues](https://github.com/qishu321/OpsBuddy/issues)，附平台、版本和已脱敏的复现步骤；请勿上传 API Key、个人数据库或敏感附件。
