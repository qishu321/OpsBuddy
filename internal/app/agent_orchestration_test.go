package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"opsbuddy/internal/agentfixture"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type agentModelRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func agentMock(t *testing.T, fn func(agentModelRequest, *http.Request) (string, int)) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in agentModelRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		reply, status := fn(in, r)
		if status != 0 && status != 200 {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": in.Model + "-actual", "choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
	}))
	t.Cleanup(s.Close)
	return s
}
func agentReply(content any) string { raw, _ := json.Marshal(content); return string(raw) }
func reviewPass() string {
	return `{"status":"pass","summary":"核对提供材料通过，未声称运行代码测试","issues":[]}`
}
func registerAgentModel(t *testing.T, a *App, name, url, key string) {
	t.Helper()
	secret, err := seal(key)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Models = append(a.state.Models, Model{ID: name, Name: name, Family: "openai", BaseURL: url, ModelID: name + "-model", Secret: secret})
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
}
func dispatchTestAgents(t *testing.T, a *App, kind string, nodes []AgentNodeSpec, auto bool) AgentGroup {
	t.Helper()
	w := request(t, a, "POST", "/api/agent-groups", map[string]any{"projectId": "inbox", "prompt": "协作验收目标", "kind": kind, "modelId": "main", "allowRemote": true, "autoPlan": auto, "members": nodes})
	if w.Code != 201 {
		t.Fatalf("dispatch %d %s", w.Code, w.Body.String())
	}
	var g AgentGroup
	_ = json.Unmarshal(w.Body.Bytes(), &g)
	return g
}
func TestAgentDAGModelsCredentialsAndFrozenIsolation(t *testing.T) {
	var mu sync.Mutex
	seen := map[string][]string{}
	gate := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(gate) })
	started := make(chan string, 4)
	var active, peak atomic.Int32
	mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
		prompt := in.Messages[1].Content
		mu.Lock()
		seen[in.Model] = append(seen[in.Model], prompt)
		mu.Unlock()
		want := strings.TrimSuffix(in.Model, "-model") + "-credential-123456"
		if r.Header.Get("Authorization") != "Bearer "+want {
			t.Errorf("wrong credential for %s", in.Model)
		}
		if strings.Contains(prompt, "OTHER_PROJECT_HISTORY") || strings.Contains(prompt, "unrelated-session") || strings.Contains(prompt, "credential-123456") {
			t.Error("context leakage")
		}
		if in.Model == "left-model" || in.Model == "right-model" {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			started <- in.Model
			select {
			case <-gate:
			case <-r.Context().Done():
				return "", 499
			}
			return in.Model + " DELIVERED", 200
		}
		if strings.Contains(in.Messages[0].Content, "你是独立审阅者") {
			return reviewPass(), 200
		}
		if strings.Contains(prompt, "当前职责：JOIN") && (!strings.Contains(prompt, "left-model DELIVERED") || !strings.Contains(prompt, "right-model DELIVERED")) {
			t.Error("join missing actual dependency results")
		}
		return "JOIN_OR_FINAL", 200
	})
	a := newTestApp(t)
	for _, name := range []string{"main", "left", "right"} {
		registerAgentModel(t, a, name, mock.URL, name+"-credential-123456")
	}
	a.mu.Lock()
	a.state.Tasks = append(a.state.Tasks, Task{ID: "unrelated-session", ProjectID: "other", Status: "succeeded", Result: "OTHER_PROJECT_HISTORY"})
	_ = a.saveLocked()
	a.mu.Unlock()
	g := dispatchTestAgents(t, a, "chat", []AgentNodeSpec{{ID: "a", Role: "worker", Prompt: "LEFT_PRIVATE", ModelID: "left", Kind: "chat"}, {ID: "b", Role: "worker", Prompt: "RIGHT_PRIVATE", ModelID: "right", Kind: "chat"}, {ID: "c", Role: "worker", Prompt: "JOIN", DependsOn: []string{"a", "b"}, Kind: "chat"}, {ID: "r", Role: "reviewer", Prompt: "REVIEW", DependsOn: []string{"c"}}}, false)
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("branches not concurrent")
		}
	}
	mu.Lock()
	count := len(seen["main-model"])
	mu.Unlock()
	if count != 0 {
		t.Fatal("downstream dispatched before dependencies finished")
	}
	// Changes to live settings cannot affect queued member requests.
	a.mu.Lock()
	for i := range a.state.Models {
		a.state.Models[i].BaseURL = "http://127.0.0.1:1"
	}
	_ = a.saveLocked()
	a.mu.Unlock()
	once.Do(func() { close(gate) })
	done := waitGroup(t, a, g.ID)
	if done.Status != "succeeded" || peak.Load() != 2 {
		t.Fatalf("status=%s peak=%d error=%s", done.Status, peak.Load(), done.Error)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen["left-model"]) != 1 || len(seen["right-model"]) != 1 || strings.Contains(seen["left-model"][0], "RIGHT_PRIVATE") || strings.Contains(seen["right-model"][0], "LEFT_PRIVATE") {
		t.Fatal("models or member inputs mixed")
	}
	for _, task := range a.snapshot().Tasks {
		if task.AgentGroupID == g.ID && task.AgentMemberID != "" {
			if task.Config.ConversationContext != "" || len(task.Config.MemoryRefs) > 0 {
				t.Fatal("inherited unrelated history")
			}
			found := false
			for _, step := range task.Steps {
				if step.Name == "模型接口响应" && strings.Contains(step.Detail, "-actual") {
					found = true
				}
			}
			if !found {
				t.Fatal("missing actual model receipt")
			}
		}
	}
	raw, _ := json.Marshal(a.snapshot())
	if strings.Contains(string(raw), "credential-123456") {
		t.Fatal("credential persisted to public state")
	}
}

func TestAgentReviewReworkIsBoundedAndPreservesBranches(t *testing.T) {
	for _, endless := range []bool{false, true} {
		t.Run(fmt.Sprint(endless), func(t *testing.T) {
			var reviews atomic.Int32
			mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
				if strings.Contains(in.Messages[0].Content, "你是独立审阅者") {
					if reviews.Add(1) == 1 || endless {
						return `{"status":"rework","summary":"需修复 A","issues":[{"nodeId":"a","reason":"补足数字依据"}]}`, 200
					}
					return reviewPass(), 200
				}
				return "本轮实际交付", 200
			})
			a := newTestApp(t)
			registerAgentModel(t, a, "main", mock.URL, "")
			g := dispatchTestAgents(t, a, "md", []AgentNodeSpec{{ID: "a", Role: "worker", Prompt: "A", Kind: "md"}, {ID: "b", Role: "worker", Prompt: "B", Kind: "md"}, {ID: "c", Role: "worker", Prompt: "C", Kind: "md", DependsOn: []string{"a"}}, {ID: "r", Role: "reviewer", Prompt: "R", DependsOn: []string{"b", "c"}}}, false)
			done := waitGroup(t, a, g.ID)
			rounds := 2
			status := "succeeded"
			if endless {
				rounds = 3
				status = "blocked"
			}
			if done.Status != status || len(done.Members[0].TaskIDs) != rounds || len(done.Members[1].TaskIDs) != 1 || len(done.Members[2].TaskIDs) != rounds || len(done.Reviews) != rounds || done.ReworkRound != rounds-1 {
				t.Fatalf("unexpected rework %+v", done)
			}
			for _, task := range a.snapshot().Tasks {
				if task.AgentGroupID == g.ID && task.Kind == "md" && task.AgentMemberID != "" {
					if task.Status != "succeeded" || len(task.OutputIDs) != 1 {
						t.Fatal("old output lost")
					}
					asset, err := a.asset(task.OutputIDs[0])
					if err != nil {
						t.Fatal(err)
					}
					if _, err = a.filePath(asset); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestAgentNodeRetryAndFailedDependency(t *testing.T) {
	for _, recover := range []bool{true, false} {
		t.Run(fmt.Sprint(recover), func(t *testing.T) {
			var attempts, downstream atomic.Int32
			mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
				if strings.Contains(in.Messages[1].Content, "当前职责：FAIL_ONCE") {
					if attempts.Add(1) == 1 || !recover {
						return "", 503
					}
				}
				if strings.Contains(in.Messages[1].Content, "当前职责：DOWNSTREAM") {
					downstream.Add(1)
				}
				return "真实响应", 200
			})
			a := newTestApp(t)
			registerAgentModel(t, a, "main", mock.URL, "")
			g := dispatchTestAgents(t, a, "chat", []AgentNodeSpec{{ID: "a", Role: "worker", Prompt: "FAIL_ONCE", Kind: "chat"}, {ID: "b", Role: "worker", Prompt: "DOWNSTREAM", Kind: "chat", DependsOn: []string{"a"}}}, false)
			done := waitGroup(t, a, g.ID)
			if attempts.Load() != 2 || done.Members[0].Retries != 1 {
				t.Fatal("retry unbounded", done)
			}
			if recover {
				if done.Status != "succeeded" || downstream.Load() != 1 {
					t.Fatal(done)
				}
			} else {
				if done.Status != "failed" || downstream.Load() != 0 || done.Members[1].Status != "blocked" {
					t.Fatal(done)
				}
			}
		})
	}
}

func TestAgentAutoPlanUsesModelAndRejectsInvalidDAG(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprint(invalid), func(t *testing.T) {
			var calls atomic.Int32
			mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
				calls.Add(1)
				if strings.Contains(in.Messages[0].Content, "你是本项目任务的主协调者") {
					dep := []string{}
					if invalid {
						dep = []string{"review"}
					}
					return agentReply(AgentPlan{Summary: "真实模型规划", Members: []AgentNodeSpec{{ID: "work", Role: "worker", Prompt: "编写内容", Kind: "md", DependsOn: dep}, {ID: "review", Role: "reviewer", Prompt: "核对内容", DependsOn: []string{"work"}}}}), 200
				}
				if strings.Contains(in.Messages[0].Content, "你是独立审阅者") {
					return reviewPass(), 200
				}
				return "# 受控模型的真实文件\n\n数据来源明确。", 200
			})
			a := newTestApp(t)
			registerAgentModel(t, a, "main", mock.URL, "")
			g := dispatchTestAgents(t, a, "md", nil, true)
			done := waitGroup(t, a, g.ID)
			if invalid {
				if done.Status != "blocked" || calls.Load() != 1 || len(done.Members) != 0 {
					t.Fatal(done, calls.Load())
				}
			} else {
				if done.Status != "succeeded" || calls.Load() != 4 || done.Plan.Summary != "真实模型规划" || done.PlannerTaskID == "" {
					t.Fatal(done, calls.Load())
				}
			}
		})
	}
}

func TestAgentPlanValidationAndNestedDelegation(t *testing.T) {
	a := newTestApp(t)
	registerAgentModel(t, a, "main", "http://127.0.0.1:1", "")
	base := AgentNodeSpec{ID: "a", Role: "worker", Prompt: "x", Kind: "chat"}
	for _, change := range []func(*AgentNodeSpec){
		func(n *AgentNodeSpec) { n.DependsOn = []string{"missing"} }, func(n *AgentNodeSpec) { n.DependsOn = []string{"a"} },
		func(n *AgentNodeSpec) { n.ModelID = "unknown" }, func(n *AgentNodeSpec) { n.InputIDs = []string{"other-project-asset"} },
		func(n *AgentNodeSpec) { n.SkillIDs = []string{"unknown"} }, func(n *AgentNodeSpec) { n.Kind = "shell" },
		func(n *AgentNodeSpec) { n.Kind = "code"; n.OwnedPaths = []string{"../state.db"} },
	} {
		node := base
		change(&node)
		w := request(t, a, "POST", "/api/agent-groups", map[string]any{"projectId": "inbox", "prompt": "验证", "kind": "chat", "modelId": "main", "allowRemote": true, "members": []AgentNodeSpec{node}})
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	a.mu.Lock()
	a.state.Tasks = append(a.state.Tasks, Task{ID: "child", ProjectID: "inbox", AgentMemberID: "a", Status: "succeeded"})
	_ = a.saveLocked()
	a.mu.Unlock()
	w := request(t, a, "POST", "/api/agent-groups", map[string]any{"projectId": "inbox", "sourceTaskId": "child", "prompt": "再委派", "modelId": "main", "allowRemote": true, "members": []AgentNodeSpec{base}})
	if w.Code != 400 || len(a.snapshot().AgentGroups) != 0 {
		t.Fatal("invalid plan persisted")
	}
}

func TestAgentCancelPreservesOutputAndNeverDispatchesDownstream(t *testing.T) {
	entered := make(chan struct{}, 1)
	var late atomic.Int32
	mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
		if strings.Contains(in.Messages[1].Content, "当前职责：WAIT") {
			entered <- struct{}{}
			<-r.Context().Done()
			return "", 499
		}
		if strings.Contains(in.Messages[1].Content, "当前职责：LATE") {
			late.Add(1)
		}
		return "# 保留的成果", 200
	})
	a := newTestApp(t)
	registerAgentModel(t, a, "main", mock.URL, "")
	g := dispatchTestAgents(t, a, "md", []AgentNodeSpec{{ID: "a", Role: "worker", Prompt: "FIRST", Kind: "md"}, {ID: "b", Role: "worker", Prompt: "WAIT", Kind: "chat", DependsOn: []string{"a"}}, {ID: "c", Role: "worker", Prompt: "LATE", Kind: "chat", DependsOn: []string{"b"}}}, false)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("not started")
	}
	w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/cancel", nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	a.Close()
	done := waitGroup(t, a, g.ID)
	if done.Status != "cancelled" || done.Members[0].Status != "succeeded" || late.Load() != 0 || len(done.Members[2].TaskIDs) != 0 {
		t.Fatal(done)
	}
	for _, task := range a.snapshot().Tasks {
		if task.AgentGroupID == g.ID && task.AgentMemberID == "a" {
			if len(task.OutputIDs) != 1 {
				t.Fatal("success file missing")
			}
			asset, _ := a.asset(task.OutputIDs[0])
			if _, err := a.filePath(asset); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestAgentCandidateConflictsAndOwnership(t *testing.T) {
	a := newTestApp(t)
	assets := []Asset{}
	for i, content := range []string{"one", "two"} {
		asset, err := a.saveCodeProject(context.Background(), "inbox", "", "", CodeProject{Title: fmt.Sprint(i), Runtime: "go", Mode: "fragment", Files: []CodeFile{{Path: "main.go", Content: content}}})
		if err != nil {
			t.Fatal(err)
		}
		assets = append(assets, asset)
	}
	if _, err := a.agentCandidateFiles(assets); err == nil || !strings.Contains(err.Error(), "冲突") {
		t.Fatal("silent overwrite", err)
	}
	if _, err := a.startCodeRuntime(assets[0]); err == nil {
		t.Fatal("fragment was executable")
	}
	if err := validateCandidatePaths(CodeProject{Files: []CodeFile{{Path: "other.go"}}}, []string{"main.go"}); err == nil {
		t.Fatal("ownership bypass")
	}
	p := CodeProject{Runtime: "go", Mode: "server", Entry: "main.go", Files: []CodeFile{{Path: "MAIN.go", Content: "changed"}}}
	if err := mergeAgentCandidateFiles(&p, []CodeFile{{Path: "main.go", Content: "original"}}); err == nil {
		t.Fatal("final model silently replaced candidate")
	}
}

func TestAgentModelDoesNotPersistEchoedCredential(t *testing.T) {
	key := "private-model-key-012345"
	mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
		return "gateway incorrectly echoed " + key, 200
	})
	a := newTestApp(t)
	registerAgentModel(t, a, "main", mock.URL, key)
	g := dispatchTestAgents(t, a, "md", []AgentNodeSpec{{ID: "a", Role: "worker", Prompt: "生成文本", Kind: "md"}}, false)
	done := waitGroup(t, a, g.ID)
	if done.Status != "succeeded" {
		t.Fatal(done)
	}
	raw, _ := json.Marshal(a.snapshot())
	if strings.Contains(string(raw), key) {
		t.Fatal("secret in snapshot")
	}
	for _, asset := range a.snapshot().Assets {
		full, _ := a.asset(asset.ID)
		p, _ := a.filePath(full)
		text, err := readDocument(p, full.Kind)
		if err != nil || strings.Contains(text, key) {
			t.Fatal("secret in artifact", err)
		}
	}
}

// Response validation checks actual JSON values, not merely HTTP 200.
func TestAgentReviewRejectsFakePassAndForeignResponsibility(t *testing.T) {
	g := AgentGroup{Members: []AgentMember{{ID: "a"}, {ID: "r", Role: "reviewer", DependsOn: []string{"a"}}}}
	for _, raw := range []string{`{"status":"pass"}`, `{"status":"pass","summary":"ok","issues":[{"nodeId":"a","reason":"bad"}]}`, `{"status":"rework","summary":"fix","issues":[{"nodeId":"other-project","reason":"bad"}]}`, `everything passed`} {
		if _, err := parseAgentReview(raw, g.Members[1], g); err == nil {
			t.Fatal("invalid review accepted", raw)
		}
	}
}

func TestAgentFollowupInvalidatesRunningDownstream(t *testing.T) {
	var aCalls, bCalls atomic.Int32
	entered := make(chan struct{}, 1)
	mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
		prompt := in.Messages[1].Content
		if strings.Contains(in.Messages[0].Content, "你是独立审阅者") {
			return reviewPass(), 200
		}
		if strings.Contains(prompt, "当前职责：A") {
			return fmt.Sprintf("A_VERSION_%d", aCalls.Add(1)), 200
		}
		if strings.Contains(prompt, "当前职责：B") {
			if bCalls.Add(1) == 1 {
				entered <- struct{}{}
				<-r.Context().Done()
				return "", 499
			}
			if !strings.Contains(prompt, "A_VERSION_2") {
				t.Error("rerun used stale upstream")
			}
			return "B_UPDATED", 200
		}
		return "INDEPENDENT_OR_FINAL", 200
	})
	a := newTestApp(t)
	registerAgentModel(t, a, "main", mock.URL, "")
	g := dispatchTestAgents(t, a, "chat", []AgentNodeSpec{{ID: "a", Role: "worker", Prompt: "A", Kind: "chat"}, {ID: "b", Role: "worker", Prompt: "B", Kind: "chat", DependsOn: []string{"a"}}, {ID: "safe", Role: "worker", Prompt: "SAFE", Kind: "chat"}, {ID: "r", Role: "reviewer", Prompt: "R", DependsOn: []string{"b", "safe"}}}, false)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("downstream never started")
	}
	w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/members/a/instructions", map[string]string{"prompt": "A: add new requirement"})
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	done := waitGroup(t, a, g.ID)
	if done.Status != "succeeded" || len(done.Members[0].TaskIDs) != 2 || len(done.Members[1].TaskIDs) != 2 || len(done.Members[2].TaskIDs) != 1 {
		t.Fatal(done)
	}
	for _, task := range a.snapshot().Tasks {
		if task.ID == done.Members[1].TaskIDs[0] && task.Status != "cancelled" {
			t.Fatal("stale downstream survived")
		}
	}
}

func TestAgentRestartKeepsLegacyAndInterruptsPlanningWithoutReplay(t *testing.T) {
	a := newTestApp(t)
	a.mu.Lock()
	a.state.AgentGroups = append(a.state.AgentGroups, AgentGroup{ID: "legacy", RootTaskID: "old", ProjectID: "inbox", Status: "succeeded", Members: []AgentMember{{ID: "old-member", Status: "succeeded"}}}, AgentGroup{ID: "planning", RootTaskID: "root", ProjectID: "inbox", Status: "planning", SchemaVersion: 2, PlannerTaskID: "plan-task"})
	a.state.Tasks = append(a.state.Tasks, Task{ID: "root", ProjectID: "inbox", Status: "waiting", AgentGroupID: "planning"}, Task{ID: "plan-task", ProjectID: "inbox", Status: "queued", AgentGroupID: "planning", AgentMemberID: "__planner"})
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	a.mu.Unlock()
	a.Close()
	b, err := New(a.root, a.web)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	s := b.snapshot()
	if s.AgentGroups[0].Status != "succeeded" || s.AgentGroups[0].SchemaVersion != 0 || s.AgentGroups[1].Status != "interrupted" {
		t.Fatal(s.AgentGroups)
	}
	for _, task := range s.Tasks {
		if task.AgentGroupID == "planning" && task.Status != "interrupted" {
			t.Fatal(task)
		}
	}
	w := request(t, b, "DELETE", "/api/tasks/root", nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, task := range b.snapshot().Tasks {
		if task.AgentGroupID == "planning" {
			t.Fatal("planner task orphaned by conversation deletion")
		}
	}
}

func TestAgentActualOfficeAndGoSourceDelivery(t *testing.T) {
	fixture := &agentfixture.Server{}
	mock := httptest.NewServer(fixture)
	defer mock.Close()
	for _, kind := range []string{"docx", "code"} {
		t.Run(kind, func(t *testing.T) {
			a := newTestApp(t)
			registerAgentModel(t, a, "main", mock.URL+"/main", "")
			registerAgentModel(t, a, "front", mock.URL+"/front", "")
			registerAgentModel(t, a, "back", mock.URL+"/back", "")
			prompt := "OFFICE_COLLAB：预算120元数量3，分析并撰写报告"
			if kind == "code" {
				prompt = "CODE_COLLAB：前端与Go后端协作实现计算器并启动"
			}
			w := request(t, a, "POST", "/api/agent-groups", map[string]any{"projectId": "inbox", "prompt": prompt, "kind": kind, "modelId": "main", "allowRemote": true, "autoPlan": true})
			if w.Code != 201 {
				t.Fatal(w.Body.String())
			}
			var group AgentGroup
			_ = json.Unmarshal(w.Body.Bytes(), &group)
			deadline := time.Now().Add(100 * time.Second)
			var done AgentGroup
			for time.Now().Before(deadline) {
				for _, g := range a.snapshot().AgentGroups {
					if g.ID == group.ID {
						done = g
					}
				}
				if !activeAgentGroup(done.Status) && done.ID != "" {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if done.Status != "succeeded" {
				t.Fatalf("%s: %+v", kind, done)
			}
			var root Task
			for _, task := range a.snapshot().Tasks {
				if task.ID == done.RootTaskID {
					root = task
				}
			}
			if len(root.OutputIDs) != 1 {
				t.Fatal(root)
			}
			asset, err := a.asset(root.OutputIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			if kind == "docx" {
				path, err := a.filePath(asset)
				if err != nil {
					t.Fatal(err)
				}
				content, err := readDocument(path, "docx")
				if err != nil || !strings.Contains(content, "120") || !strings.Contains(content, "数量为3") {
					t.Fatal("office content missing", content, err)
				}
				if len(done.Members[0].TaskIDs) != 1 || len(done.Members[2].TaskIDs) != 1 || len(done.Reviews) != 1 {
					t.Fatal("office chain incomplete")
				}
			} else {
				p, err := a.loadCodeProject(asset)
				if err != nil || len(p.Files) != 3 {
					t.Fatal("source tree missing", err, p)
				}
				for _, file := range p.Files {
					if file.Path == "index.html" && file.Content != agentfixture.Frontend {
						t.Fatal("frontend silently changed")
					}
					if file.Path == "main.go" && file.Content != agentfixture.Backend {
						t.Fatal("backend silently changed")
					}
				}
				run := waitCodeRun(t, a, asset.ID, func(v CodeRunStatus) bool { return v.Status == "running" })
				resp, err := http.Get(run.URL + "api/add?a=7&b=5")
				if err != nil {
					t.Fatal(err)
				}
				var body struct {
					Sum    int    `json:"sum"`
					Source string `json:"source"`
				}
				err = json.NewDecoder(resp.Body).Decode(&body)
				resp.Body.Close()
				if err != nil || body.Sum != 12 || body.Source != "go-backend" {
					t.Fatal("Go API incorrect", body, err)
				}
				resp, err = http.Get(run.URL + "api/add?a=no&b=5")
				if err != nil {
					t.Fatal(err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 400 {
					t.Fatal("bad parameters accepted")
				}
				if !strings.Contains(run.Logs, "agent-go-backend-ready") {
					t.Fatal("startup log absent")
				}
				w = request(t, a, "POST", "/api/assets/"+asset.ID+"/runtime/stop", nil)
				if w.Code != 200 {
					t.Fatal(w.Body.String())
				}
			}
		})
	}
}
