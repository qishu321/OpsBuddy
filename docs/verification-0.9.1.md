# 0.9.1 输入快捷键验证

2026-10-16，Windows 桌面。

- Enter 发送，Shift+Enter 保留原生换行行为，Ctrl+Enter 继续兼容。
- isComposing 或 keyCode 229 的回车不触发提交，避免输入法选词误发送。
- 长按回车的重复 keydown 不再次提交；发送按钮禁用时不提交。
- TypeScript/Vite、Prettier、桌面 Go 编译通过。
- 最终 EXE 的 WebView2 集成检查 passed=true，结果 `.cache/smoke-enter-091/smoke-result.json`。
- 使用真实表单 requestSubmit 并在捕获阶段拦截，验证 Enter/Ctrl+Enter 提交次数、Shift+Enter/IME 不提交、长按不重复、空内容不提交；未调用付费模型。输入法检查为浏览器合成按键事件，不等同于所有系统输入法人工操作验收。
- 原有后端项目预览、截图粘贴、工作流与文件等桌面集成回归同时通过。
