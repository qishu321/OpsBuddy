# 0.10.0 内置 Skills 验证

2026-10-16，Windows 桌面版。

## 交付

12 个独立 SKILL.md 经 Go embed 编入 EXE；内置内容按 OpsBuddy 现有能力编写。扩展页可搜索、查看、停用及复制，输入材料选择器可显式引用，任务显示实际读取记录。

## 验证

- `scripts/test.ps1`：全量 Go 测试、go vet、TypeScript/Vite 构建通过；最终格式检查通过；桌面 Go 编译通过。
- Skill Creator 的 `quick_validate.py` 对 12 个技能目录全部返回 Skill is valid。校验用 PyYAML 仅安装在 `.cache/skill-validator`，不进入桌面程序或发行包。
- 新增 Go 测试覆盖目录解析、唯一名称、正文范围、已有数据补齐、禁用保留、自定义内容保留、历史快照不改写、内置只读/删除保护及来源字段防伪。
- 验证按任务类型与关键词仅选择相关项，问候不误选，停用项不读取，明确绑定优先。
- 模拟模型任务证明：Word 任务实际收到所选内置技能正文，未收到无关后端技能；只发一次内容生成请求，没有额外技能选择请求；保存实际使用 ID 与冻结哈希。自定义技能原有按需选择测试继续通过。
- 最终 EXE 在普通和 880×600 窗口 WebView2 中均 passed=true。验证 12 项目录、后端搜索、SKILL.md 说明展开、停用落库、内置无删除按钮、自定义副本保留正文且可编辑。
- 同轮回归 Enter/Shift+Enter/IME、截图粘贴、HTML/Markdown、前后端真实接口与按钮交互、停止服务、工作流、知识库、定时与记忆等已有检查。
- 结果：`.cache/smoke-skills-0100/smoke-result.json` 与 `.cache/smoke-skills-0100-small/smoke-result.json`。

本轮模型使用模拟服务，桌面验证使用隔离目录；没有向真实供应商发送付费请求，也没有把说明型 Skill 宣称为新增 OCR、语音、MCP 执行或任意脚本能力。

更新前已正常关闭旧窗口并备份真实数据库。发布包只含 EXE 与说明，技能嵌入程序；用户数据目录保持原位置。内置目录升级只维护内置项并保留停用开关，自定义项和历史冻结内容不被改写。

参考：[OpenAI Skills](https://developers.openai.com/plugins/concepts/skills)、[Codex 技能按需加载](https://developers.openai.com/blog/skills-agents-sdk)。
