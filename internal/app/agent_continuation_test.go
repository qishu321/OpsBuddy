package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"opsbuddy/internal/agentfixture"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAgentPlanRejectsTrailingJSONAndMarksPlannerFailed(t *testing.T) {
	for _, suffix := range []string{` {"members":[]}`, ` trailing text`} {
		t.Run(suffix, func(t *testing.T) {
			var calls atomic.Int32
			mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
				calls.Add(1)
				if strings.Contains(in.Messages[0].Content, "你是本项目任务的主协调者") {
					return agentReply(AgentPlan{Summary: "计划", Members: []AgentNodeSpec{{ID: "a", Role: "worker", Prompt: "执行", Kind: "chat"}, {ID: "r", Role: "reviewer", Prompt: "审阅", DependsOn: []string{"a"}}}}) + suffix, 200
				}
				return reviewPass(), 200
			})
			a := newTestApp(t)
			registerAgentModel(t, a, "main", mock.URL, "")
			group := dispatchTestAgents(t, a, "chat", nil, true)
			done := waitGroup(t, a, group.ID)
			if done.Status != "blocked" || calls.Load() != 1 || len(done.Members) != 0 {
				t.Fatalf("invalid plan dispatched work: status=%s calls=%d", done.Status, calls.Load())
			}
			for _, task := range a.snapshot().Tasks {
				if task.ID == done.PlannerTaskID && (task.Status != "failed" || task.Error == "") {
					t.Fatal("planner falsely succeeded", task.Status)
				}
			}
		})
	}
}

// A referenced candidate from an earlier run is input material, not a current
// delivery. It must not conflict with the replacement produced and reviewed now.
func TestAgentCodeReferenceDoesNotOverrideCurrentDelivery(t *testing.T) {
	fixture := &agentfixture.Server{}
	var fronts, backs, reviews atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var in agentModelRequest
		_ = json.Unmarshal(raw, &in)
		system, prompt := in.Messages[0].Content, in.Messages[1].Content
		reply := ""
		if strings.Contains(system, "本节点负责路径") {
			if strings.Contains(prompt, "当前职责：FRONTEND_FIXTURE") {
				version := fronts.Add(1)
				reply = agentReply(CodeProject{Title: "前端", Runtime: "go", Mode: "fragment", Files: []CodeFile{{Path: "index.html", Content: agentfixture.Frontend + fmt.Sprintf("<!-- FRONT_REV_%d -->", version)}}})
			} else {
				backs.Add(1)
			}
		}
		if strings.Contains(system, "你是独立审阅者") {
			if reviews.Add(1) == 1 {
				if !strings.Contains(prompt, "FRONT_REV_1") || !strings.Contains(prompt, "strconv.Atoi") {
					t.Error("review missing actual candidate source")
				}
				reply = `{"status":"rework","summary":"前端需要更新","issues":[{"nodeId":"front","reason":"生成第二版页面"}]}`
			} else {
				if !strings.Contains(prompt, "FRONT_REV_2") || strings.Contains(prompt, "FRONT_REV_1") {
					t.Error("review used stale candidate revision")
				}
				reply = reviewPass()
			}
		}
		if reply != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		fixture.ServeHTTP(w, r)
	}))
	defer mock.Close()
	a := newTestApp(t)
	for _, name := range []string{"main", "front", "back"} {
		registerAgentModel(t, a, name, mock.URL+"/"+name, "")
	}
	old, err := a.saveCodeProject(context.Background(), "inbox", "old-task", "", CodeProject{Title: "旧版参考", Runtime: "go", Mode: "fragment", Files: []CodeFile{{Path: "main.go", Content: "package main // OLD_REFERENCE"}}})
	if err != nil {
		t.Fatal(err)
	}
	w := request(t, a, "POST", "/api/agent-groups", map[string]any{"projectId": "inbox", "prompt": "CODE_COLLAB：参考旧版，重新交付前端与Go后端计算器", "kind": "code", "modelId": "main", "allowRemote": true, "autoPlan": true, "inputIds": []string{old.ID}})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var group AgentGroup
	_ = json.Unmarshal(w.Body.Bytes(), &group)
	done := waitGroup(t, a, group.ID)
	if done.Status != "succeeded" {
		t.Fatalf("reference polluted integration: %s %s", done.Status, done.Error)
	}
	if fronts.Load() != 2 || backs.Load() != 1 || reviews.Load() != 2 || done.ReworkRound != 1 {
		t.Fatalf("unexpected reruns: front=%d back=%d review=%d", fronts.Load(), backs.Load(), reviews.Load())
	}
	for _, task := range a.snapshot().Tasks {
		if task.ID == done.RootTaskID {
			asset, err := a.asset(task.OutputIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			project, err := a.loadCodeProject(asset)
			if err != nil || len(project.Files) != 3 {
				t.Fatal(project, err)
			}
			for _, file := range project.Files {
				if file.Path == "index.html" && file.Content != agentfixture.Frontend+"<!-- FRONT_REV_2 -->" {
					t.Fatal("integration used stale frontend")
				}
				if file.Path == "main.go" && file.Content != agentfixture.Backend {
					t.Fatal("current reviewed source changed")
				}
			}
		}
	}
	previous, err := a.loadCodeProject(old)
	if err != nil || !strings.Contains(previous.Files[0].Content, "OLD_REFERENCE") {
		t.Fatal("original reference overwritten", err)
	}
}

func TestAgentRoutesIndependentProtocolEndpoints(t *testing.T) {
	a := newTestApp(t)
	var mainCalls, anthropicCalls, geminiCalls atomic.Int32
	main := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
		mainCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer main-secret-123456" {
			t.Error("wrong main credential")
		}
		if !strings.Contains(in.Messages[1].Content, "ANTHROPIC_DELIVERY") || !strings.Contains(in.Messages[1].Content, "GEMINI_DELIVERY") {
			t.Error("review or final missing real upstream delivery")
		}
		if strings.Contains(in.Messages[0].Content, "你是独立审阅者") {
			return reviewPass(), 200
		}
		return "已汇总两个协议端点的实际结果", 200
	})
	registerAgentModel(t, a, "main", main.URL, "main-secret-123456")
	for _, family := range []string{"anthropic", "gemini"} {
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			if strings.Contains(string(raw), "DELIVERY") || strings.Contains(string(raw), "secret-123456") {
				t.Error("independent member received sibling delivery or secret")
			}
			if family == "anthropic" {
				anthropicCalls.Add(1)
				if r.URL.Path != "/messages" || r.Header.Get("x-api-key") != "anthropic-secret-123456" || !strings.Contains(string(raw), `"model":"anthropic-model"`) {
					t.Error("anthropic routing mismatch")
				}
				fmt.Fprint(w, `{"model":"anthropic-actual","content":[{"type":"text","text":"ANTHROPIC_DELIVERY"}]}`)
			} else {
				geminiCalls.Add(1)
				if r.URL.Path != "/models/gemini-model:generateContent" || r.Header.Get("x-goog-api-key") != "gemini-secret-123456" || !strings.Contains(string(raw), `"systemInstruction"`) {
					t.Error("gemini routing mismatch")
				}
				fmt.Fprint(w, `{"modelVersion":"gemini-actual","candidates":[{"content":{"parts":[{"text":"GEMINI_DELIVERY"}]}}]}`)
			}
		}))
		t.Cleanup(mock.Close)
		registerAgentModel(t, a, family, mock.URL, family+"-secret-123456")
		a.mu.Lock()
		for i := range a.state.Models {
			if a.state.Models[i].ID == family {
				a.state.Models[i].Family = family
			}
		}
		a.mu.Unlock()
	}
	g := dispatchTestAgents(t, a, "chat", []AgentNodeSpec{{ID: "a", Role: "worker", Prompt: "独立A", ModelID: "anthropic", Kind: "chat"}, {ID: "b", Role: "worker", Prompt: "独立B", ModelID: "gemini", Kind: "chat"}, {ID: "r", Role: "reviewer", Prompt: "按真实结果审阅", DependsOn: []string{"a", "b"}}}, false)
	done := waitGroup(t, a, g.ID)
	if done.Status != "succeeded" || mainCalls.Load() != 2 || anthropicCalls.Load() != 1 || geminiCalls.Load() != 1 {
		t.Fatalf("protocol route failed: %s, calls=%d/%d/%d", done.Status, mainCalls.Load(), anthropicCalls.Load(), geminiCalls.Load())
	}
}
