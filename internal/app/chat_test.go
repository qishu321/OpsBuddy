package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConversationReplyAndContinuation(t *testing.T) {
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if strings.Contains(payload.Messages[0].Content, "OpsBuddy") || strings.Contains(payload.Messages[0].Content, "deepseek-flash") {
			t.Error("client identity overrides model identity")
		}
		requests <- payload.Messages[len(payload.Messages)-1].Content
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "provider-model-from-response", "choices": []any{map[string]any{"message": map[string]string{"content": "客户端配置的是 deepseek-flash。\n可以协助处理办公任务。"}}}})
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "chat-model", Family: "openai", BaseURL: server.URL, ModelID: "deepseek-flash"})
	create := func(prompt, parent, kind string) Task {
		t.Helper()
		w := request(t, a, "POST", "/api/tasks", TaskRequest{ProjectID: "inbox", RoleID: "auto", Kind: kind, Prompt: prompt, ParentID: parent, ModelID: "chat-model", AllowRemote: true})
		if w.Code != 201 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var queued Task
		_ = json.Unmarshal(w.Body.Bytes(), &queued)
		return waitTask(t, a, queued.ID)
	}
	old := request(t, a, "POST", "/api/tasks", TaskRequest{ProjectID: "inbox", Kind: "docx", Prompt: "制作旧会话文档"})
	var oldTask Task
	_ = json.Unmarshal(old.Body.Bytes(), &oldTask)
	oldTask = waitTask(t, a, oldTask.ID)
	if oldTask.Status != "succeeded" || len(oldTask.OutputIDs) != 1 {
		t.Fatal("office fixture failed")
	}
	// A stale Word selection must not turn an identity question into a document.
	first := create("你好，你是什么大模型啊？", oldTask.ID, "docx")
	if first.Status != "succeeded" || first.Kind != "chat" || len(first.OutputIDs) != 0 || !strings.Contains(first.Result, "deepseek-flash") || first.Config.Executor != "chat-v1" {
		t.Fatalf("unexpected chat result: %+v", first)
	}
	receiptFound := false
	for _, step := range first.Steps {
		if step.Name == "模型接口响应" && strings.Contains(step.Detail, "provider-model-from-response") && strings.Contains(step.Detail, "HTTP 200") {
			receiptFound = true
		}
	}
	if !receiptFound {
		t.Fatal("missing actual provider response model receipt")
	}
	if len(first.InputIDs) != 0 {
		t.Fatal("identity question inherited old Word file")
	}
	<-requests
	second := create("具体能做哪些事情？", first.ID, "auto")
	if second.Status != "succeeded" || second.Kind != "chat" || second.ConversationID != first.ConversationID || len(second.OutputIDs) != 0 {
		t.Fatalf("unexpected continuation: %+v", second)
	}
	if prompt := <-requests; !strings.Contains(prompt, first.Result) {
		t.Fatal("previous assistant reply missing from context")
	}
	if !requestsArtifact("制作一份 Word 文档说明你是谁") || conversationalQuestion("制作一份 Word 文档说明你是谁") {
		t.Fatal("explicit document request must retain office route")
	}
}

func TestChatProtocolsAndEmptyReply(t *testing.T) {
	for _, family := range []string{"openai", "anthropic", "gemini"} {
		t.Run(family, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				_ = json.NewDecoder(r.Body).Decode(&payload)
				if family == "gemini" && payload["generationConfig"].(map[string]any)["responseMimeType"] != "text/plain" {
					t.Error("conversation incorrectly forced JSON")
				}
				switch family {
				case "openai":
					_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"**正常回复**"}}]}`))
				case "anthropic":
					_, _ = w.Write([]byte(`{"content":[{"text":"**正常回复**"}]}`))
				case "gemini":
					_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"**正常回复**"}]}}]}`))
				}
			}))
			defer server.Close()
			text, err := callModelText(context.Background(), Model{Family: family, BaseURL: server.URL, ModelID: "mock"}, "system", "prompt", nil, false)
			if err != nil || text != "**正常回复**" {
				t.Fatalf("%q %v", text, err)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer server.Close()
	_, err := callModelText(context.Background(), Model{Family: "openai", BaseURL: server.URL}, "", "", nil, false)
	if err == nil || !strings.Contains(err.Error(), "未返回回复正文") {
		t.Fatalf("empty response silently accepted: %v", err)
	}
}
