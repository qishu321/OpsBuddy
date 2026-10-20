# v0.18.0 双平台发布验证

日期：2026-10-20。仓库：https://github.com/qishu321/OpsBuddy 。版本0.18.0，Windows与Linux x64开发预览。

## 本机 Windows

- 全量 Go 测试、go vet、TypeScript/Vite 构建和 Prettier 检查通过。
- 新生成 `release/OpsBuddy/OpsBuddy-0.18.0.exe`；SHA-256 `4E6AD5797B7D96EEE11A784340314BFE02AD85676C4F7DA433CC336F9FC5F613`。这是本机验收构建，GitHub构建哈希以Release的SHA256SUMS为准。
- `.cache/release-0180-windows/`：隐藏的独立桌面导入真实Demo，项目分组/会话、5节点编排画布、3源码、显式Go启动/健康检查/停止通过，0次模型请求。
- `.cache/release-0180-regression/`：隐藏完整原生自检通过，含知识库、Office、工作流、项目隔离、多Agent实际调度、暂停修改继续和停止；17次本机模拟请求，无真实供应商调用。
- 用户现有窗口未重启、未操作；用户数据没有加入Git或发布档案。

## GitHub Linux

首轮提交 `a854a48` 的 Ubuntu24.04任务已实际成功，运行：https://github.com/qishu321/OpsBuddy/actions/runs/39378319695 。

- Go1.26、Node22、GTK3和WebKit2GTK4.1，真实Linux原生构建，不是交叉编译的占位程序。
- `go test -tags webkit2_41 ./... -count=1` 和 `go vet -tags webkit2_41 ./...`通过，含Linux凭据加解密、随机nonce、篡改/权限/符号链接拒绝及模型API不泄露密钥的回归。
- 真实Wails GTK/WebKit进程在独立D-Bus/Xvfb会话运行：项目/主会话/5节点编排画布/3源码文件/无静默执行/显式启动实际Go服务/健康检查/停止全部通过；保留6个历史任务和5条实际历史回执，未调用真实模型。
- Linux tar.gz构建并上传CI artifact成功。原生桌宠是明确的Windows专属，不为Linux创建占位渲染进程。

## CI修正与发布门禁

首轮Windows托管CI在Prettier检查阶段因Git默认CRLF检出失败，本机对应回归已通过。`.gitattributes`现统一文本LF，避免相同源码在Windows/Linux获得不同格式结果。生成的嵌入index不再入Git，保留embed占位文件，干净检出可先测试再构建。

tag `v0.18.0`必须与package.json版本匹配。发布作业依赖Windows和Linux两项成功；只有全部通过才生成GitHub prerelease、EXE、Windows ZIP、Linux tar.gz及SHA256SUMS。最终CI结果和发布资产以该tag关联的Actions/Release为准。

源码剔除本机演示runs/测试EXE，不删除本机文件；不重写已有提交历史。保留安全的示例源码与可导入演示包。README中英文分别说明依赖、数据目录、Linux文件加密的边界、跨平台重新配置密钥、构建验证和未实现能力。
