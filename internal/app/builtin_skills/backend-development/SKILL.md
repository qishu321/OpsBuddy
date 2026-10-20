---
name: backend-development
description: 开发 Go、Node.js 或 Python 的标准库服务与脚本，生成多文件代码项目并符合本机运行约定。
metadata:
  display_name: 后端与脚本开发
  category: 开发
  kinds: code
---
# 后端与脚本开发

遵守当前代码项目 JSON 交付结构，选择用户指定语言；未指定后端语言时优先 Go。提供完整入口和实际依赖文件，不把多个文件拼在一个代码围栏内。HTTP 服务用 server 模式，命令行脚本用 command 模式。

使用标准库，不自动安装依赖或生成任意 shell 启动命令。服务读取 PORT 环境变量，只监听 127.0.0.1；GET / 返回可访问主页或 JSON 说明。前端文件与接口由同一服务提供，fetch、资源与表单使用 ./api 等相对路径。

要求 Vue 时，用宿主内置的 Vue 3 全局构建 `./__opsbuddy_runtime__/vue.global.prod.js` 或 ESM 构建 `./__opsbuddy_runtime__/vue.esm-browser.prod.js`，不要引用 CDN 或重复生成运行库。使用 HTML 与 createApp，无需额外安装；不要生成尚无构建环境的 .vue 空壳。分别要求多个版本时，每个实现都有实际入口与使用说明。

校验输入、限制请求体，错误返回适当状态码与清晰信息。文件操作仅针对任务所需的项目数据，不读取无关个人文件或模型凭据。日志记录运行状态，避免输出密钥。

Go 提供 go.mod，Node 模块类型与入口一致，Python 文件按 UTF-8 编写。清楚区分源码生成、进程启动和服务就绪；没有运行证据时不能声称已成功启动。技能不代替运行器，也不增加依赖安装或终端权限。
