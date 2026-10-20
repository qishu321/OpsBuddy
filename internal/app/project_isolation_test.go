package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProjectConversationContextIsolation(t *testing.T) {
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- string(body)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "isolated-answer"}}}})
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Projects = append(a.state.Projects, Project{ID: "project-b", Name: "B", Description: "B_PROJECT_GUIDANCE"})
	a.state.Projects[0].Description = "A_PRIVATE_GUIDANCE"
	a.state.Models = append(a.state.Models, Model{ID: "isolation-model", Family: "openai", BaseURL: server.URL, ModelID: "mock"})
	a.state.Tasks = append(a.state.Tasks, Task{ID: "a-history", ProjectID: "inbox", ConversationID: "a-history", Status: "succeeded", Prompt: "A_PRIVATE_PROMPT", Result: "A_PRIVATE_ANSWER", Config: &TaskConfig{ConversationContext: "A_PRIVATE_EARLIER"}})
	a.state.Assets = append(a.state.Assets, Asset{ID: "a-file", ProjectID: "inbox", Name: "A_PRIVATE_FILE.txt"})
	a.state.KnowledgeBases = append(a.state.KnowledgeBases, KnowledgeBase{ID: "a-kb", ProjectID: "inbox", Name: "A_PRIVATE_KNOWLEDGE"})
	a.state.Memories = append(a.state.Memories,
		Memory{ID: "a-memory", ProjectID: "inbox", Scope: "project", Kind: "preference", Title: "style", Content: "A_PRIVATE_MEMORY", Enabled: true},
		Memory{ID: "b-memory", ProjectID: "project-b", Scope: "project", Kind: "preference", Title: "style", Content: "B_PROJECT_MEMORY", Enabled: true},
	)
	base := TaskRequest{ProjectID: "project-b", Kind: "chat", RoleID: "auto", ModelID: "isolation-model", AllowRemote: true, Prompt: "B_FRESH_QUESTION"}
	for name, modify := range map[string]func(*TaskRequest){
		"history":   func(in *TaskRequest) { in.ParentID = "a-history" },
		"file":      func(in *TaskRequest) { in.InputIDs = []string{"a-file"} },
		"knowledge": func(in *TaskRequest) { in.KnowledgeIDs = []string{"a-kb"} },
	} {
		t.Run(name, func(t *testing.T) {
			in := base
			modify(&in)
			if w := request(t, a, "POST", "/api/tasks", in); w.Code != 400 {
				t.Fatalf("cross-project %s accepted: %d %s", name, w.Code, w.Body.String())
			}
		})
	}
	run := func(in TaskRequest) Task {
		t.Helper()
		w := request(t, a, "POST", "/api/tasks", in)
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		var submitted Task
		_ = json.Unmarshal(w.Body.Bytes(), &submitted)
		done := waitTask(t, a, submitted.ID)
		if done.Status != "succeeded" {
			t.Fatal(done.Error)
		}
		return done
	}
	b := run(base)
	if b.Config.ConversationContext != "" || b.ConversationID == "a-history" {
		t.Fatal("fresh B conversation inherited A")
	}
	checkB := func() {
		t.Helper()
		payload := <-requests
		if strings.Contains(payload, "A_PRIVATE") || !strings.Contains(payload, "B_PROJECT_GUIDANCE") || !strings.Contains(payload, "B_PROJECT_MEMORY") {
			t.Fatal("B model request crossed project boundary or lost its own context")
		}
	}
	checkB()
	base.ParentID = b.ID
	base.Prompt = "B_FOLLOWUP"
	followup := run(base)
	if followup.ConversationID != b.ConversationID || !strings.Contains(followup.Config.ConversationContext, "B_FRESH_QUESTION") {
		t.Fatal("B followup lost its own conversation")
	}
	checkB()
	base.ProjectID, base.ParentID, base.Prompt = "inbox", "a-history", "A_FOLLOWUP"
	run(base)
	payload := <-requests
	if !strings.Contains(payload, "A_PRIVATE_EARLIER") || !strings.Contains(payload, "A_PRIVATE_ANSWER") || strings.Contains(payload, "B_PROJECT") || strings.Contains(payload, "B_FRESH") {
		t.Fatal("A continuation did not retain only A context")
	}
}

func TestDeleteConversationCannotCrossProjectEvenWithLegacyIDCollision(t *testing.T) {
	a := newTestApp(t)
	a.state.Tasks = append(a.state.Tasks,
		Task{ID: "root-a", ProjectID: "inbox", ConversationID: "same-legacy-id", Status: "succeeded"},
		Task{ID: "root-b", ProjectID: "project-b", ConversationID: "same-legacy-id", Status: "succeeded"},
		Task{ID: "member-b", ProjectID: "project-b", Status: "succeeded"},
	)
	// A stale legacy member reference must not remove another project's task either.
	a.state.AgentGroups = append(a.state.AgentGroups, AgentGroup{ID: "group-a", ProjectID: "inbox", RootTaskID: "root-a", Status: "succeeded", Members: []AgentMember{{TaskIDs: []string{"member-b"}}}})
	w := request(t, a, "DELETE", "/api/tasks/root-a", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	state := a.snapshot()
	if len(state.Tasks) != 2 || state.Tasks[0].ID != "root-b" || state.Tasks[1].ID != "member-b" {
		t.Fatal("deleting A affected B")
	}
}
