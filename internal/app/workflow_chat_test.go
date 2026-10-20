package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkflowTextOutputPassesBetweenAgents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		prompt := payload.Messages[len(payload.Messages)-1].Content
		reply := "初稿内容：住宿标准为450元。"
		if strings.Contains(prompt, "审阅上游") {
			if !strings.Contains(prompt, "初稿内容") {
				t.Error("upstream text not forwarded")
			}
			reply = "定稿内容：住宿标准为450元，需附发票。"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "mock", "choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "text-model", Family: "openai", BaseURL: server.URL, ModelID: "mock"})
	f := WorkflowDefinition{ProjectID: "inbox", Name: "文字撰写与审阅", Enabled: true, Nodes: []WorkflowNode{
		{ID: "s", Type: "start", Label: "开始"},
		{ID: "write", Type: "agent", Label: "初稿", Kind: "chat", ModelID: "text-model", Prompt: "撰写初稿 {{input}}"},
		{ID: "review", Type: "agent", Label: "定稿", Kind: "chat", ModelID: "text-model", Prompt: "审阅上游 {{input}}"},
		{ID: "e", Type: "end", Label: "结束"},
	}, Edges: []WorkflowEdge{{ID: "1", Source: "s", Target: "write", SourceHandle: "out"}, {ID: "2", Source: "write", Target: "review", SourceHandle: "out"}, {ID: "3", Source: "review", Target: "e", SourceHandle: "out"}}}
	f = savedFlow(t, a, f)
	run := runFlow(t, a, f, "整理住宿规定", true)
	result := waitFlow(t, a, run.ID)
	if result.Status != "succeeded" || len(result.OutputIDs) != 0 {
		t.Fatalf("unexpected text workflow: %+v", result)
	}
	found := false
	for _, n := range result.Nodes {
		if n.NodeID == "review" && strings.Contains(n.Text, "定稿内容") {
			found = true
		}
	}
	if !found {
		t.Fatal("final text not retained in run")
	}
	f.ID = ""
	f.Nodes[1].Type = "tool"
	if w := request(t, a, "POST", "/api/workflows", f); w.Code != 400 {
		t.Fatal("local tool must not accept chat output", w.Code)
	}
}
