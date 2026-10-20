package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCodeIntentRouting(t *testing.T) {
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "code-test", Family: "openai"})
	for _, tt := range []struct{ prompt, kind string }{
		{"帮我写个2048，并本地启动", "html"},
		{"帮我做一个贪吃蛇", "html"},
		{"使用 Go 开发前后端项目并启动", "code"},
		{"帮我写一个 Python 脚本并运行", "code"},
		{"写一份 API 接口文档", "docx"},
		{"上海今天天气怎么样？", "chat"},
		{"Python 是如何运行的？", "chat"},
		{"解释 2048 游戏的规则", "chat"},
	} {
		a.mu.Lock()
		task, _, _, err := a.prepareTaskLocked(TaskRequest{ProjectID: "inbox", RoleID: "auto", Kind: "auto", ModelID: "code-test", AllowRemote: true, Prompt: tt.prompt})
		a.mu.Unlock()
		if err != nil || task.Kind != tt.kind {
			t.Errorf("%s: kind=%s err=%v", tt.prompt, task.Kind, err)
		}
	}
}

func nodeCodeFixture() CodeProject {
	return CodeProject{Title: "前后端测试", Runtime: "node", Mode: "server", Entry: "server.mjs", Files: []CodeFile{{Path: "server.mjs", Content: `import http from 'node:http';
console.log('server fixture started');
http.createServer((req,res)=>{
 if(req.url==='/api'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({value:42,auth:req.headers.authorization||'',cookie:req.headers.cookie||''}));return;}
 if(req.url==='/redirect'){res.writeHead(302,{Location:'https://example.com/'});res.end();return;}
 res.setHeader('Content-Type','text/html; charset=utf-8');res.end('<!doctype html><html><body><h1>Full stack preview</h1><button id="counter">0</button><script>fetch("./api").then(r=>r.json()).then(d=>{document.querySelector("#counter").textContent=d.value;parent.postMessage({opsbuddyBackend:d.value},"*")});document.querySelector("#counter").onclick=e=>e.target.textContent=Number(e.target.textContent)+1;</script></body></html>');
}).listen(Number(process.env.PORT),'127.0.0.1');`}}}
}
func waitCodeRun(t *testing.T, a *App, assetID string, done func(CodeRunStatus) bool) CodeRunStatus {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		a.codeMu.Lock()
		run := a.codeRuns[assetID]
		a.codeMu.Unlock()
		if run != nil {
			v := run.snapshot()
			if done(v) {
				return v
			}
			if v.Status == "failed" {
				t.Fatalf("code failed: %s\n%s", v.Error, v.Logs)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("runtime timeout")
	return CodeRunStatus{}
}

func TestCodeRuntimeLifecycleAndIsolation(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node not installed")
	}
	a := newTestApp(t)
	p := nodeCodeFixture()
	asset, err := a.saveCodeProject(context.Background(), "inbox", "", "", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.startCodeRuntime(asset); err != nil {
		t.Fatal(err)
	}
	run := waitCodeRun(t, a, asset.ID, func(v CodeRunStatus) bool { return v.Status == "running" })
	if !strings.HasPrefix(run.URL, "http://127.0.0.1:") {
		t.Fatal(run.URL)
	}
	resp, err := http.Get(run.URL)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(raw), "Full stack preview") || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-src 'none'") {
		t.Fatal("preview missing or unrestricted")
	}
	req, _ := http.NewRequest("GET", run.URL+"api", nil)
	req.Header.Set("Authorization", "secret-test")
	req.Header.Set("Cookie", "secret-cookie")
	req.Header.Set("Origin", "null")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(raw), `"value":42`) || strings.Contains(string(raw), "secret") || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("API proxy: %s", raw)
	}
	base := run.URL[:strings.LastIndex(strings.TrimSuffix(run.URL, "/"), "/")]
	for _, url := range []string{base + "/api/state", base + "/unknown", run.URL + "../api/state"} {
		resp, err = http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Fatalf("unauthorized route exposed: %s", url)
		}
	}
	resp, err = http.Get(run.URL + "redirect")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatal("external redirect allowed")
	}
	w := request(t, a, "POST", "/api/assets/"+asset.ID+"/runtime/stop", nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	a.codeMu.Lock()
	runtime := a.codeRuns[asset.ID]
	a.codeMu.Unlock()
	select {
	case <-runtime.done:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not stop")
	}
	if _, err = http.Get(run.URL); err == nil {
		t.Fatal("preview listener survived stop")
	}
	if _, err = a.startCodeRuntime(asset); err != nil {
		t.Fatal(err)
	}
	waitCodeRun(t, a, asset.ID, func(v CodeRunStatus) bool { return v.Status == "running" })
	a.Close()
	if v := a.codeRuns[asset.ID].snapshot(); v.Status != "stopped" {
		t.Fatal(v)
	}
	if _, err = a.startCodeRuntime(asset); err == nil {
		t.Fatal("closed application started code")
	}
}

func TestCodeCommandsAndFailures(t *testing.T) {
	for _, runtime := range []string{"node", "python", "go"} {
		t.Run(runtime, func(t *testing.T) {
			bin := runtime
			if bin == "node" {
				bin = "node"
			}
			if _, err := exec.LookPath(bin); err != nil {
				t.Skip("runtime not installed")
			}
			p := CodeProject{Title: "命令行测试", Runtime: runtime, Mode: "command"}
			switch runtime {
			case "node":
				p.Entry = "main.mjs"
				p.Files = []CodeFile{{Path: p.Entry, Content: `console.log('runtime-result-42');`}}
			case "python":
				p.Entry = "main.py"
				p.Files = []CodeFile{{Path: p.Entry, Content: `print('runtime-result-42')`}}
			case "go":
				p.Entry = "main.go"
				p.Files = []CodeFile{{Path: "go.mod", Content: "module sample\n\ngo 1.22\n"}, {Path: p.Entry, Content: "package main\nimport \"fmt\"\nfunc main(){fmt.Println(\"runtime-result-42\")}\n"}}
			}
			a := newTestApp(t)
			asset, err := a.saveCodeProject(context.Background(), "inbox", "", "", p)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = a.startCodeRuntime(asset); err != nil {
				t.Fatal(err)
			}
			v := waitCodeRun(t, a, asset.ID, func(v CodeRunStatus) bool { return v.Status == "exited" })
			if v.ExitCode == nil || *v.ExitCode != 0 || !strings.Contains(v.Logs, "runtime-result-42") {
				t.Fatalf("%+v", v)
			}
		})
	}
	if _, err := exec.LookPath("node"); err != nil {
		return
	}
	a := newTestApp(t)
	p := nodeCodeFixture()
	p.Mode = "command"
	p.Files[0].Content = "throw new Error('failure-fixture');"
	asset, _ := a.saveCodeProject(context.Background(), "inbox", "", "", p)
	_, _ = a.startCodeRuntime(asset)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.codeMu.Lock()
		r := a.codeRuns[asset.ID]
		a.codeMu.Unlock()
		v := r.snapshot()
		if v.Status == "failed" {
			if !strings.Contains(v.Logs, "failure-fixture") || v.ExitCode == nil || *v.ExitCode == 0 {
				t.Fatal(v)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("failure was not reported")
}

func TestCodeProjectValidationAndLegacyHTML(t *testing.T) {
	for _, bad := range []string{"../outside.py", "C:/bad.py", "folder\\bad.py", "/bad.py", "CON.py", "a/../b.py", "SERVER.MJS", "opsbuddy-project.json"} {
		p := nodeCodeFixture()
		p.Files = append(p.Files, CodeFile{Path: bad, Content: "bad"})
		if validateCodeProject(&p) == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	a := newTestApp(t)
	a.state.Tasks = append(a.state.Tasks, Task{ID: "legacy-html", ProjectID: "inbox", Status: "succeeded", Kind: "chat", Title: "2048", Result: "下面是代码：\n```html\n<!doctype html><html><head></head><body>2048</body></html>\n```"})
	for i := 0; i < 2; i++ {
		w := request(t, a, "POST", "/api/tasks/legacy-html/html", nil)
		if w.Code != 201 && w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if len(a.state.Tasks[0].OutputIDs) != 1 {
		t.Fatal("legacy extraction duplicated output")
	}
	if w := request(t, a, "POST", "/api/code-projects", map[string]any{"projectId": "missing", "project": nodeCodeFixture()}); w.Code != 400 {
		t.Fatal("missing project accepted")
	}
}

func TestModelCodeProjectAndContinuation(t *testing.T) {
	p := nodeCodeFixture()
	raw, _ := json.Marshal(p)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if !strings.Contains(payload.Messages[0].Content, "可运行") {
			t.Error("wrong system prompt")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}})
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "code-model", Family: "openai", BaseURL: server.URL, ModelID: "mock"})
	parent := ""
	for _, prompt := range []string{"帮我开发一个 Node.js 后端项目并在本地启动", "修改按钮的颜色"} {
		w := request(t, a, "POST", "/api/tasks", TaskRequest{ProjectID: "inbox", RoleID: "auto", Kind: "auto", ModelID: "code-model", AllowRemote: true, Prompt: prompt, ParentID: parent})
		if w.Code != 201 {
			t.Fatal(w.Body.String())
		}
		var task Task
		_ = json.Unmarshal(w.Body.Bytes(), &task)
		task = waitTask(t, a, task.ID)
		if task.Status != "succeeded" || task.Kind != "code" || len(task.OutputIDs) != 1 {
			t.Fatalf("%+v", task)
		}
		asset, _ := a.asset(task.OutputIDs[0])
		project, err := a.loadCodeProject(asset)
		if err != nil || project.Entry != p.Entry {
			t.Fatal(err)
		}
		if parent != "" && asset.ParentID == "" {
			t.Fatal("code revision did not preserve previous version")
		}
		if parent == "" {
			if _, err := exec.LookPath("node"); err == nil {
				waitCodeRun(t, a, asset.ID, func(v CodeRunStatus) bool { return v.Status == "running" })
			}
		}
		parent = task.ID
	}
}
