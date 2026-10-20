package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBuiltinSkillCatalogAndUpgrade(t *testing.T) {
	catalog, err := builtinSkillCatalog()
	if err != nil || len(catalog) != 12 {
		t.Fatalf("catalog %d: %v", len(catalog), err)
	}
	seen := map[string]bool{}
	for _, x := range catalog {
		m, err := parseSkillManifest(x.Content)
		if err != nil || m.Name == "" || !x.Enabled || !x.Builtin || x.Category == "" || seen[x.ID] || len(x.Content) > 10000 {
			t.Fatalf("bad built-in %+v: %v", x, err)
		}
		seen[x.ID] = true
		if strings.Contains(skillBody(x.Content), "display_name:") {
			t.Fatal("frontmatter leaked into body")
		}
	}
	a := newTestApp(t)
	if len(a.state.Extensions) != 12 {
		t.Fatal("builtins not installed on existing state")
	}
	a.state.Extensions[0].Enabled = false
	a.state.Extensions[0].Content = "old bundled content"
	a.state.Extensions = append(a.state.Extensions, Extension{ID: "user-skill", Name: "用户技能", Kind: "skill", Content: "user-instructions", Enabled: true})
	a.state.Tasks = append(a.state.Tasks, Task{ID: "old-snapshot", ProjectID: "inbox", Status: "succeeded", Config: &TaskConfig{Extensions: []TaskExtension{{Extension: a.state.Extensions[0], ContentHash: "old-hash"}}}})
	if err = a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	a.Close()
	reopened, err := New(a.root, a.web)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.state.Extensions) != 13 || reopened.state.Extensions[0].Enabled || reopened.state.Extensions[0].Content == "old bundled content" {
		t.Fatal("bundle update lost disable preference or duplicated skills")
	}
	if reopened.state.Extensions[12].Content != "user-instructions" || reopened.state.Tasks[0].Config.Extensions[0].Content != "old bundled content" {
		t.Fatal("user content or historical snapshot was overwritten")
	}
}

func TestBuiltinSkillSelectionAndControl(t *testing.T) {
	a := newTestApp(t)
	for _, tt := range []struct{ kind, prompt, want string }{
		{"docx", "整理项目方案", "office-writing"}, {"pptx", "制作演示", "presentations"},
		{"xlsx", "整理数据", "spreadsheets"}, {"md", "写说明", "markdown-writing"},
		{"html", "写2048", "web-development"}, {"code", "开发服务", "backend-development"},
		{"chat", "根据截图解释报错", "screenshot-analysis"}, {"chat", "请检查代码", "code-review"},
		{"chat", "会议纪要", "meeting-notes"}, {"chat", "你好", ""},
	} {
		ids := builtinSkillsForTask(a.state.Extensions, tt.kind, tt.prompt)
		if tt.want == "" && len(ids) > 0 {
			t.Fatalf("unrelated skill in greeting: %v", ids)
		}
		if tt.want != "" && !strings.Contains(strings.Join(ids, ","), "builtin-skill-"+tt.want) {
			t.Errorf("%s: %v", tt.want, ids)
		}
	}
	x, _ := func() (Extension, bool) {
		for _, x := range a.state.Extensions {
			if x.ID == "builtin-skill-web-development" {
				return x, true
			}
		}
		return Extension{}, false
	}()
	if w := request(t, a, "DELETE", "/api/extensions/"+x.ID, nil); w.Code != 400 {
		t.Fatal("builtin deletion allowed")
	}
	x.Enabled = false
	if w := request(t, a, "POST", "/api/extensions", x); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if ids := builtinSkillsForTask(a.state.Extensions, "html", "写2048"); len(ids) != 0 {
		t.Fatal("disabled builtin selected")
	}
	x.Content = "modified"
	if w := request(t, a, "POST", "/api/extensions", x); w.Code != 400 {
		t.Fatal("builtin body editing allowed")
	}
	x.ID = ""
	x.Builtin = true
	x.Enabled = true
	w := request(t, a, "POST", "/api/extensions", x)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var clone Extension
	_ = json.Unmarshal(w.Body.Bytes(), &clone)
	if clone.Builtin || clone.ID == "" || clone.Content != "modified" {
		t.Fatal("custom copy cannot be edited or spoofed built-in origin")
	}
}

func TestBuiltinSkillsReachOnlyRelevantGeneration(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		system := payload.Messages[0].Content
		if !strings.Contains(system, "报告宜先给结论和依据") {
			t.Error("selected builtin instructions missing")
		}
		if strings.Contains(system, "服务读取 PORT 环境变量") || strings.Contains(system, "技能选择器") {
			t.Error("unrelated skill or extra selector injected")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"title":"测试报告","sections":[{"heading":"结论","body":"已提供的测试内容"}]}`}}}})
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "builtin-model", Family: "openai", BaseURL: server.URL, ModelID: "mock"})
	w := request(t, a, "POST", "/api/tasks", TaskRequest{ProjectID: "inbox", Kind: "docx", RoleID: "auto", ModelID: "builtin-model", AllowRemote: true, Prompt: "请整理一份 Word 报告"})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var task Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	task = waitTask(t, a, task.ID)
	if task.Status != "succeeded" || len(task.UsedSkillIDs) != 1 || task.UsedSkillIDs[0] != "builtin-skill-office-writing" || calls.Load() != 1 {
		t.Fatalf("%+v calls=%d", task, calls.Load())
	}
	if task.Config.Extensions[0].ContentHash != contentHash(task.Config.Extensions[0].Content) {
		t.Fatal("builtin snapshot hash missing")
	}
	a.mu.Lock()
	a.state.Roles = append(a.state.Roles, Role{ID: "explicit", SkillIDs: []string{"builtin-skill-code-review"}})
	explicit, _, _, err := a.prepareTaskLocked(TaskRequest{ProjectID: "inbox", RoleID: "explicit", Kind: "docx", ModelID: "builtin-model", AllowRemote: true, Prompt: "写Word"})
	a.mu.Unlock()
	if err != nil || len(explicit.Config.Extensions) != 1 || explicit.Config.Extensions[0].ID != "builtin-skill-code-review" {
		t.Fatal("explicit skill did not take precedence", err)
	}
}
