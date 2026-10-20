# 0.15.0 验证记录

日期：2026-10-16，Windows amd64，Go 原生桌宠与 Wails 桌面版。

## 本次变化

- 气泡标题为 22 px 半粗字，正文 20 px 深色字，两行短句；本机 125% 缩放时正文为 25 物理像素。字体使用原生 GDI 按 DPI 绘制，像素猫保持像素风格。
- 原生桌宠线程使用独立的 Per Monitor V2 DPI 上下文，换屏消息重建 DIB 与字体，并采用系统建议位置后收回可见工作区。
- 待机约每秒 2 帧，加入呼吸起伏、眨眼、摆尾、左右探头、挥爪及伸懒腰。工作和交互约每秒 6 帧；关闭动画后，气泡结束即停止定时器与持续重绘。
- 下方“打开”返回工作台；“静止/活跃”切换动画。保留摸摸、拖动、双击、右键菜单；只在宠物窗口内处理鼠标消息，不监听全局鼠标。
- 设计参考用户截图与 [官方 Pets 文档](https://learn.chatgpt.com/docs/pets?surface=app) 的浮动入口和工作状态，不复制 Codex 的角色素材，也未增加未实现的语音按钮。

## 代码与桌面验证

- `scripts/test.ps1` 通过：全部 Go 测试、Go vet、TypeScript 与 Vite；后续原生 DPI 调整的 `go test ./internal/pet ./internal/desktop` 和 `go vet ./internal/pet` 通过。
- `npm run format:check` 通过。
- 原生 `TestNativeCompanionLifecycle` 通过：实际分层窗口绘制、大字号、待机低帧率、工作动画持续、关闭动画静止、摸摸与悬停分别产生反应、打开和恢复动画按钮、双击、拖动位置收回与保存、重复开关释放。
- 同步发送带有效 RECT 的 WM_DPICHANGED，验证 96/144/192 DPI 的字体与绘图缓冲区重建。测试线程与宠物线程均使用 Per Monitor V2，避免坐标在不同 DPI 上下文间被系统转换。该项是窗口消息验证，并非多块物理显示器的人工拖动检查。
- 原生测试证据：`.cache/pet-native-0150-report.json`；实际 DIB 预览：`.cache/pet-native-0150-preview.png`。动作演示 `.cache/pet-animation-0150.gif` 由同一像素绘制代码生成，不是桌面录屏。

最终 EXE 的标准窗口与 880×600 窗口均通过办公、工作流、知识库、Markdown/HTML、源码项目、离线 Vue、项目会话边界、截图、定时任务、记忆、子 Agent 和桌宠集成检查。活跃待机有持续重绘；关闭动画后重绘数不变，正文字号为 25 物理像素（本机 120 DPI）。证据：.cache/smoke-pet-0150-final/smoke-result.json 与 .cache/smoke-pet-0150-final-small/smoke-result.json，均 passed=true。

## 资源短测

| 指标 | 独立原生测试进程的本机结果 |
| --- | --- |
| 活跃待机采样 | 3 秒，6 次重绘（约 2 帧/秒） |
| 活跃待机 CPU 时间增量 | 0 秒（系统采样精度下） |
| 关闭动画后的静止采样 | 3.0013 秒，0 次重绘，定时器停用 |
| 关闭动画后的 CPU 时间增量 | 0 秒（系统采样精度下） |
| 启用前私有内存 | 49,803,264 字节 |
| 启用后私有内存 | 55,353,344 字节 |
| 增量 | 5,550,080 字节，约 5.29 MiB |
| GDI 对象 | 初始 0，关闭后 1；反复开关后仍为 1 |

这些是短测数据，不能把计时精度下的 0 增量理解为持续动画完全不消耗 CPU，也不是整个 OpsBuddy 的占用或固定跨机器上限。活跃模式用小型缓冲区和低帧率绘制；无需额外 WebView、浏览器进程、模型调用或服务。

## 交付

保留用户会话、模型、项目及宠物位置和动画偏好；升级前正常关闭旧实例，数据库备份为 `state-before-0.15.0-*.db`。交付仍为 Windows 桌面版 `release/OpsBuddy/OpsBuddy.exe`，版本 0.15.0。