# 0.13.0 验证记录

日期：2026-10-16，Windows amd64，Go 原生桌宠 + Wails 桌面版。

## 代码与桌面回归

- `scripts/test.ps1` 通过：全部 Go 测试、Go vet、TypeScript 与 Vite 构建。
- `npm run format:check` 通过。
- 新增测试验证：状态通道只保留最新事件且不阻塞任务、未变化状态不重复发送、启动不回放历史完成通知、退订释放通道；像素帧透明且各状态有差异；关闭桌宠不创建原生窗口、配置和位置保留。
- 正常窗口和 880×600 窗口的实际 EXE 集成检查通过：桌宠设置开关、关闭动画、真实文档任务完成事件送达、气泡结束后定时器停用且重绘数不增长。既有项目边界、截图、Skills、知识库、工作流、定时任务、记忆、文件与代码运行回归通过。
- 记录：`.cache/smoke-pet-0130/smoke-result.json`、`.cache/smoke-pet-0130-small/smoke-result.json`。

## 原生窗口与资源短测

显式设置 `OPSBUDDY_PET_NATIVE_TEST=1` 后运行 `TestNativeCompanionLifecycle`。测试创建自己的临时配置与原生窗口，验证透明层实际绘制成功、双击消息回调、窗口拖出工作区后的收回与位置保存、状态变化、重复启用与关闭。

最终代码的独立原生测试进程记录如下：

| 指标 | 测量结果 |
| --- | --- |
| 静止采样时长 | 3.0006 秒 |
| 采样期间重绘 | 0 次 |
| 动画定时器 | 已停止 |
| 进程 CPU 时间增量 | 0 秒（Windows GetProcessTimes 采样） |
| 启用前私有内存 | 48,848,896 字节 |
| 启用后私有内存 | 54,427,648 字节 |
| 增量 | 5,578,752 字节，约 5.32 MiB |
| GDI 对象 | 启用前 0，关闭后 1；重复开关三轮后仍为 1，没有逐轮增长 |

这些是本机短测，内存增量包含 Go 堆、字体及 Win32 首次初始化，不能当作桌宠的跨机器固定上限，也不是整个 OpsBuddy 的内存占用。CPU 时间计量有系统精度限制，0 增量不能证明永远不消耗 CPU。普通模式的动画和状态变化仍会消耗少量资源。

证据：`.cache/pet-native-report.json`；实际原生绘图缓冲区预览：`.cache/pet-native-preview.png`；代码像素画多状态预览：`.cache/pet-preview.png`。交互检查使用本应用控件与窗口消息，不宣称完成物理鼠标双击或右键菜单的人工操作检查。

## 交付与保留

`release/OpsBuddy/OpsBuddy.exe` 与 `release/OpsBuddy-Windows-amd64.zip` 均为 0.13.0。替换前正常关闭旧窗口并备份 SQLite 到 `state-before-0.13.0-*.db`。用户项目、会话、成果和模型配置保留；桌宠只新增独立的 `pet.json` 设置文件，不使用用户的模型密钥。
