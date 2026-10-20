后台开发一个轻量的中文办公待办看板 Demo，实际前端 + Go 后端，不打开浏览器或桌面窗口。
只规划3位成员：front（worker，无依赖，kind=code，仅负责index.html），back（worker，无依赖，kind=code，仅负责main.go、go.mod），review（reviewer，依赖front和back）。前后端并行，审阅后主任务集成并在后台启动。没有明确资料，不引用历史会话。所有成员沿用当前主模型。不要生成ZIP、Word、额外部门或设计方案节点。

固定接口契约：
GET ./api/health -> 200 {"ok":true}
GET ./api/todos -> 200 {"items":[{"id":1,"title":"待办标题","done":false}]}
POST ./api/todos 接收 {"title":"字符串"} -> 201 {"item":{"id":递增整数,"title":"去首尾空白后的标题","done":false}}。
空白标题或超过120个Unicode字符 -> 400 JSON错误；只接受JSON，使用标准库，不用外部依赖。
PATCH ./api/todos/{整数id} 接收 {"done":true或false} -> 200 {"item":更新对象}；不存在的ID -> 404。
GET / 提供index.html。Go绑定127.0.0.1，从PORT环境变量读取端口，默认18086。go.mod使用module opsbuddy-todo-demo、go 1.22。
持久化到当前源码目录下data/todos.json，首次为空列表，服务器重启保留数据。使用mutex保护读写，保存失败返回明确500，不伪装成功。

前端：独立单文件index.html，内联CSS/JS，零CDN、零外部图标/字体和依赖。现代清晰办公风：背景#F6F8FC，白色卡片，主色#4361D8，文字#26334A，完成色#27886D。Segoe UI/Microsoft YaHei，正文16px，标题28px，辅助14px。包括标题“今天，做完一件小事。”、任务输入/回车新增、完成切换、全部/进行中/已完成筛选、搜索、总数/完成数、空列表提示和真实错误。所有待办数据从后端读取和保存，不能用localStorage或假数据代替API；用户标题必须用textContent等安全方式显示。接口均用./api/...相对URL，加载失败可重试，防止重复提交。不要做删除功能。

审阅阶段只做接口契约、源码、安全和错误路径的静态审阅，明确说明尚未执行测试；动态接口测试会在集成后由外层程序执行，不因本阶段不能运行而直接blocked。仅当代码有具体问题时rework并指明front/back。前后端文件职责不得重叠。
主任务最终mode=server，runtime=go，entry=main.go，原样集成已审阅的候选源码，自动在后台启动，不输出启动教程代替真实项目。