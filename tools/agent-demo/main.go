// agent-demo runs a real, isolated collaboration through the production engine.
// It never writes to the running desktop's database or opens any windows.
package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"opsbuddy/internal/app"
)

const goal = `后台开发一个轻量的中文办公待办看板 Demo，实际前端 + Go 后端，不打开浏览器或桌面窗口。
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
主任务最终mode=server，runtime=go，entry=main.go，原样集成已审阅的候选源码，自动在后台启动，不输出启动教程代替真实项目。`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Demo:", err)
		os.Exit(1)
	}
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0600)
}

// An EXCLUSIVE-lock desktop is not opened as a second App. Read a short-lived
// stable filesystem snapshot, reject hot journals/changes, query models only,
// and immediately discard the snapshot containing unrelated local data.
func modelsFromStableSnapshot(source, tempRoot string) ([]app.Model, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := os.Stat(source + "-journal"); err == nil {
			time.Sleep(150 * time.Millisecond)
			continue
		}
		before, err := os.ReadFile(source)
		if err != nil {
			return nil, err
		}
		time.Sleep(150 * time.Millisecond)
		after, err := os.ReadFile(source)
		if err != nil {
			return nil, err
		}
		if sha256.Sum256(before) != sha256.Sum256(after) {
			continue
		}
		if _, err = os.Stat(source + "-journal"); err == nil {
			continue
		}
		f, err := os.CreateTemp(tempRoot, "model-read-*.db")
		if err != nil {
			return nil, err
		}
		snapshot := f.Name()
		if _, err = f.Write(after); err != nil {
			f.Close()
			os.Remove(snapshot)
			return nil, err
		}
		f.Close()
		db, err := sql.Open("sqlite", snapshot)
		if err != nil {
			os.Remove(snapshot)
			return nil, err
		}
		if _, err = db.Exec("PRAGMA query_only=ON"); err != nil {
			db.Close()
			os.Remove(snapshot)
			return nil, err
		}
		models, queryErr := readModels(db)
		db.Close()
		os.Remove(snapshot)
		return models, queryErr
	}
	return nil, errors.New("桌面数据正在写入，未读取配置，也未影响正在运行的窗口")
}
func readModels(db *sql.DB) ([]app.Model, error) {
	rows, err := db.Query("SELECT payload FROM entities WHERE kind='models' ORDER BY ordinal")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := []app.Model{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m app.Model
		if err = json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

func call(a *app.App, method, path string, in, out any) error {
	var body bytes.Buffer
	if in != nil {
		if err := json.NewEncoder(&body).Encode(in); err != nil {
			return err
		}
	}
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, &body)
	r.Header.Set("X-OpsBuddy-Request", "1")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code < 200 || w.Code >= 300 {
		return fmt.Errorf("%s %s: HTTP %d %s", method, path, w.Code, w.Body.String())
	}
	if out != nil {
		return json.Unmarshal(w.Body.Bytes(), out)
	}
	return nil
}

func run() error {
	source := flag.String("source-db", "release/OpsBuddy/.opsbuddy/state.db", "read-only configured model source")
	output := flag.String("out", "demos/multi-agent-todo", "isolated output directory")
	modelID := flag.String("model", "", "configured connection ID; default chooses DeepSeek")
	execute := flag.Bool("execute", false, "make real model requests; may use provider quota")
	flag.Parse()
	if err := os.MkdirAll(*output, 0700); err != nil {
		return err
	}
	models, err := modelsFromStableSnapshot(*source, *output)
	if err != nil {
		return err
	}
	if !*execute {
		for _, m := range models {
			fmt.Printf("连接：%s | %s | %s\n", m.ID, m.Name, m.ModelID)
		}
		return nil
	}
	var model app.Model
	for _, m := range models {
		if (*modelID != "" && m.ID == *modelID) || (*modelID == "" && strings.Contains(strings.ToLower(m.Name+" "+m.ModelID), "deepseek")) {
			model = m
			break
		}
	}
	if model.ID == "" {
		return errors.New("未找到所需已配置模型，没有调用替代服务")
	}
	runRoot := filepath.Join(*output, "runs", time.Now().Format("20060102-150405"))
	if err = os.MkdirAll(runRoot, 0700); err != nil {
		return err
	}
	stateRoot := filepath.Join(runRoot, ".opsbuddy")
	if err = os.Mkdir(stateRoot, 0700); err != nil {
		return err
	}
	seed := app.State{Projects: []app.Project{{ID: "demo-todo", Name: "多Agent Demo · 办公待办", Description: "后台独立演示；不影响主桌面数据", CreatedAt: time.Now().UTC().Format(time.RFC3339)}}, Models: []app.Model{model}, Roles: []app.Role{}, Extensions: []app.Extension{}, Assets: []app.Asset{}, Tasks: []app.Task{}}
	seedPath := filepath.Join(stateRoot, "state.json")
	if err = writeJSON(seedPath, seed); err != nil {
		return err
	}
	a, err := app.New(stateRoot, "")
	if err != nil {
		return err
	}
	os.Remove(seedPath)
	defer a.Close()
	defer func() { _ = call(a, "DELETE", "/api/models/"+model.ID, nil, nil) }() // retain no copied credentials after this run
	if err = os.WriteFile(filepath.Join(runRoot, "goal.md"), []byte(goal), 0600); err != nil {
		return err
	}
	var group app.AgentGroup
	if err = call(a, "POST", "/api/agent-groups", map[string]any{"projectId": "demo-todo", "prompt": goal, "kind": "code", "modelId": model.ID, "allowRemote": true, "useMemory": false, "autoPlan": true, "autoResume": false, "skillIds": []string{}}, &group); err != nil {
		return err
	}
	fmt.Printf("开始真实协作：%s / %s；运行目录 %s\n", model.Name, model.ModelID, runRoot)
	deadline := time.Now().Add(15 * time.Minute)
	previous := ""
	var state app.State
	for time.Now().Before(deadline) {
		if err = call(a, "GET", "/api/state", nil, &state); err != nil {
			return err
		}
		for _, g := range state.AgentGroups {
			if g.ID == group.ID {
				group = g
			}
		}
		progress := group.Status
		for _, m := range group.Members {
			progress += " | " + m.Name + ":" + m.Status
		}
		if progress != previous {
			fmt.Println(progress)
			previous = progress
		}
		_ = writeJSON(filepath.Join(runRoot, "progress.json"), map[string]any{"group": group, "updatedAt": time.Now().UTC()})
		if group.Status != "planning" && group.Status != "running" && group.Status != "aggregating" {
			break
		}
		time.Sleep(750 * time.Millisecond)
	}
	_ = writeJSON(filepath.Join(runRoot, "execution.json"), state) // snapshot excludes stored model secrets
	if group.Status != "succeeded" {
		_ = call(a, "POST", "/api/agent-groups/"+group.ID+"/cancel", nil, nil)
		for _, t := range state.Tasks {
			if t.AgentGroupID == group.ID && t.Error != "" {
				fmt.Printf("任务 %s: %s\n", t.Title, t.Error)
			}
		}
		return fmt.Errorf("协作未通过：%s %s；真实记录已保存", group.Status, group.Error)
	}
	var root app.Task
	for _, t := range state.Tasks {
		if t.ID == group.RootTaskID {
			root = t
		}
	}
	if len(root.OutputIDs) == 0 {
		return errors.New("主任务没有最终代码成果")
	}
	assetID := root.OutputIDs[len(root.OutputIDs)-1]
	var project app.CodeProject
	if err = call(a, "GET", "/api/assets/"+assetID+"/code", nil, &project); err != nil {
		return err
	}
	projectRoot := filepath.Join(runRoot, "project")
	if err = os.Mkdir(projectRoot, 0700); err != nil {
		return err
	}
	for _, f := range project.Files {
		path := filepath.Join(projectRoot, filepath.FromSlash(f.Path))
		rel, e := filepath.Rel(projectRoot, path)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return errors.New("拒绝项目外路径")
		}
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err = os.WriteFile(path, []byte(f.Content), 0600); err != nil {
			return err
		}
	}
	_ = writeJSON(filepath.Join(runRoot, "agent-plan.json"), group.Plan)
	var runtime app.CodeRunStatus
	for i := 0; i < 120; i++ {
		if err = call(a, "GET", "/api/assets/"+assetID+"/runtime", nil, &runtime); err != nil {
			return err
		}
		if runtime.Status == "running" {
			break
		}
		if runtime.Status == "failed" || runtime.Status == "exited" {
			return fmt.Errorf("实际运行失败：%s", runtime.Error)
		}
		time.Sleep(time.Second)
	}
	_ = os.WriteFile(filepath.Join(runRoot, "runtime.log"), []byte(runtime.Logs), 0600)
	if runtime.Status != "running" {
		return errors.New("后台服务未就绪")
	}
	checks, err := verifyAPI(runtime.URL)
	_ = writeJSON(filepath.Join(runRoot, "verification.json"), map[string]any{"passed": err == nil, "realModels": true, "model": model.ModelID, "checks": checks, "groupId": group.ID, "assetId": assetID, "project": projectRoot, "at": time.Now().UTC()})
	if err != nil {
		return err
	}
	_ = call(a, "POST", "/api/assets/"+assetID+"/runtime/stop", nil, nil)
	_ = os.WriteFile(filepath.Join(*output, "LATEST.txt"), []byte(runRoot), 0600)
	fmt.Println("通过：真实模型编排、Go编译启动、接口新增/完成/错误路径和静态前端。测试服务已停止，未打开窗口。")
	fmt.Println("成果：", projectRoot)
	return nil
}

func verifyAPI(base string) ([]string, error) {
	checks := []string{}
	client := &http.Client{Timeout: 10 * time.Second}
	check := func(method, path, body string, want int, out any) error {
		r, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, strings.NewReader(body))
		if err != nil {
			return err
		}
		r.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(r)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return err
		}
		if resp.StatusCode != want {
			return fmt.Errorf("%s %s 得到HTTP %d，期望%d", method, path, resp.StatusCode, want)
		}
		if out != nil {
			if err = json.Unmarshal(data, out); err != nil {
				return err
			}
		}
		checks = append(checks, method+" "+path+" -> "+strconv.Itoa(want))
		return nil
	}
	var health struct {
		OK bool `json:"ok"`
	}
	if err := check("GET", "/api/health", "", 200, &health); err != nil {
		return checks, err
	}
	if !health.OK {
		return checks, errors.New("健康接口返回非ok")
	}
	if err := check("GET", "/", "", 200, nil); err != nil {
		return checks, err
	}
	var items struct {
		Items []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
			Done  bool   `json:"done"`
		} `json:"items"`
	}
	if err := check("GET", "/api/todos", "", 200, &items); err != nil {
		return checks, err
	}
	var created struct {
		Item struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
			Done  bool   `json:"done"`
		} `json:"item"`
	}
	if err := check("POST", "/api/todos", `{"title":"  完成多 Agent 后台演示  "}`, 201, &created); err != nil {
		return checks, err
	}
	if created.Item.ID <= 0 || created.Item.Title != "完成多 Agent 后台演示" || created.Item.Done {
		return checks, errors.New("新增任务内容与契约不符")
	}
	var updated struct {
		Item struct {
			Done bool `json:"done"`
		} `json:"item"`
	}
	if err := check("PATCH", "/api/todos/"+strconv.FormatInt(created.Item.ID, 10), `{"done":true}`, 200, &updated); err != nil {
		return checks, err
	}
	if !updated.Item.Done {
		return checks, errors.New("完成状态未保存")
	}
	if err := check("GET", "/api/todos", "", 200, &items); err != nil {
		return checks, err
	}
	found := false
	for _, v := range items.Items {
		if v.ID == created.Item.ID && v.Done {
			found = true
		}
	}
	if !found {
		return checks, errors.New("重新读取未找到完成状态")
	}
	for _, body := range []string{`{"title":"   "}`, `{"title":"` + strings.Repeat("测", 121) + `"}`} {
		if err := check("POST", "/api/todos", body, 400, nil); err != nil {
			return checks, err
		}
	}
	if err := check("PATCH", "/api/todos/999999999", `{"done":true}`, 404, nil); err != nil {
		return checks, err
	}
	return checks, nil
}
