package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLegacyRoleMigrationPreservesGuidanceAndReferences(t *testing.T) {
	a := newTestApp(t)
	role := Role{ID: "legacy-custom", Name: "原自定义工作方式", Kind: "docx", Instructions: "original-custom-guidance", SkillIDs: []string{"support-skill"}, KnowledgeIDs: []string{"source-kb"}}
	a.state.Roles = append(a.state.Roles, role)
	a.state.Extensions = append(a.state.Extensions, Extension{ID: "support-skill", Name: "关联技能", Kind: "skill", Enabled: true, Content: "support-guidance"})
	a.state.KnowledgeBases = append(a.state.KnowledgeBases, KnowledgeBase{ID: "source-kb", ProjectID: "inbox", Name: "原资料库"})
	a.state.Tasks = append(a.state.Tasks, Task{ID: "historical-task", ProjectID: "inbox", Status: "succeeded", Config: &TaskConfig{Role: role}})
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b, err := New(a.root, a.web)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var migrated Extension
	for _, x := range b.state.Extensions {
		if x.SourceRoleID == role.ID {
			migrated = x
		}
	}
	if migrated.ID == "" || !strings.Contains(migrated.Content, role.Instructions) || len(migrated.Dependencies) != 1 || len(migrated.KnowledgeIDs) != 1 {
		t.Fatal("migration lost data")
	}
	if b.state.Tasks[0].Config.Role.Instructions != role.Instructions || b.state.Tasks[0].Config.Role.MigratedSkillID != "" {
		t.Fatal("historical snapshot changed")
	}
	before := len(b.state.Extensions)
	migrateRoleSkills(&b.state)
	if len(b.state.Extensions) != before {
		t.Fatal("migration duplicated skills")
	}
	b.state.Models = append(b.state.Models, Model{ID: "migration-model", Family: "openai"})
	prepare := func() (Task, error) {
		b.mu.Lock()
		defer b.mu.Unlock()
		task, _, _, err := b.prepareTaskLocked(TaskRequest{ProjectID: "inbox", Kind: "docx", RoleID: role.ID, ModelID: "migration-model", AllowRemote: true, Prompt: "按原配置写报告"})
		return task, err
	}
	prepared, err := prepare()
	if err != nil || len(prepared.Config.Extensions) != 2 || len(prepared.Config.KnowledgeIDs) != 1 || prepared.Config.Role.Instructions != "" {
		t.Fatalf("old saved role did not resolve to Skills: %v", err)
	}
	migrated.Content = "edited-migrated-guidance"
	w := request(t, b, "POST", "/api/extensions", migrated)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	prepared, err = prepare()
	if err != nil || prepared.Config.Extensions[0].Content != migrated.Content {
		t.Fatal("migration edits did not affect future legacy references", err)
	}
	migrated.Enabled = false
	if w = request(t, b, "POST", "/api/extensions", migrated); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	prepared, err = prepare()
	if err != nil || len(prepared.Config.Extensions) != 0 || len(prepared.Config.KnowledgeIDs) != 0 {
		t.Fatal("disabled migrated skill still loaded linked context", err)
	}
	if w = request(t, b, "DELETE", "/api/extensions/"+migrated.ID, nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	migrateRoleSkills(&b.state)
	for _, x := range b.state.Extensions {
		if x.ID == migrated.ID {
			t.Fatal("deleted migrated skill resurrected")
		}
	}
	if _, err = prepare(); err == nil {
		t.Fatal("missing legacy guidance silently ignored")
	}
}

func TestMigratedSkillAndProjectContextGeneration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if !strings.Contains(payload.Messages[0].Content, "custom-migrated-body") || !strings.Contains(payload.Messages[1].Content, "project-context-before-edit") {
			t.Error("migrated guidance or project context missing")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"title":"结果","sections":[{"heading":"正文","body":"真实测试结果"}]}`}}}})
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Roles = append(a.state.Roles, Role{ID: "migrated-test", Name: "原写作配置", Kind: "docx", Instructions: "custom-migrated-body"})
	migrateRoleSkills(&a.state)
	a.state.Models = append(a.state.Models, Model{ID: "context-model", Family: "openai", BaseURL: server.URL, ModelID: "mock"})
	w := request(t, a, "PUT", "/api/projects/inbox", Project{Name: "我的工作空间", Description: "project-context-before-edit"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for i := 0; i < cap(a.slots); i++ {
		a.slots <- struct{}{}
	}
	w = request(t, a, "POST", "/api/tasks", TaskRequest{ProjectID: "inbox", Kind: "docx", RoleID: "migrated-test", ModelID: "context-model", AllowRemote: true, Prompt: "写文档"})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var submitted Task
	_ = json.Unmarshal(w.Body.Bytes(), &submitted)
	w = request(t, a, "PUT", "/api/projects/inbox", Project{Name: "我的工作空间", Description: "project-context-after-edit"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	<-a.slots
	finished := waitTask(t, a, submitted.ID)
	if finished.Status != "succeeded" || len(finished.UsedSkillIDs) != 1 || finished.Config.ProjectInstructions != "project-context-before-edit" {
		t.Fatalf("%+v", finished)
	}
	if finished.Config.Role.Instructions != "" {
		t.Fatal("guidance duplicated in role and Skill")
	}
	if w = request(t, a, "PUT", "/api/projects/missing", Project{Name: "test"}); w.Code != 404 {
		t.Fatal("missing project updated")
	}
}

func TestWorkflowSkillConfigurationWithoutRolePicker(t *testing.T) {
	a := newTestApp(t)
	a.state.Extensions = append(a.state.Extensions, Extension{ID: "workflow-guidance", Name: "自定义工作方式", Kind: "skill", Enabled: true, Content: "workflow-guidance-body", SourceRoleID: "original-role"})
	a.state.Projects[0].Description = "workflow-project-context"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "工作流文本结果"}}}})
	}))
	defer server.Close()
	a.state.Models = append(a.state.Models, Model{ID: "flow-skill-model", Family: "openai", BaseURL: server.URL, ModelID: "mock"})
	f := WorkflowDefinition{Name: "技能节点", ProjectID: "inbox", Enabled: true, Nodes: []WorkflowNode{{ID: "start", Type: "start"}, {ID: "writer", Type: "agent", Kind: "chat", RoleID: "auto", ModelID: "flow-skill-model", SkillIDs: []string{"workflow-guidance"}, Prompt: "写结果"}, {ID: "end", Type: "end"}}, Edges: []WorkflowEdge{{ID: "e1", Source: "start", Target: "writer", SourceHandle: "out"}, {ID: "e2", Source: "writer", Target: "end", SourceHandle: "out"}}}
	w := request(t, a, "POST", "/api/workflows", f)
	if w.Code != 200 && w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &f)
	w = request(t, a, "POST", "/api/workflows/"+f.ID+"/runs", map[string]any{"prompt": "test", "allowRemote": true})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var run WorkflowRun
	_ = json.Unmarshal(w.Body.Bytes(), &run)
	config := run.Configs["writer"]
	if config == nil || len(config.Extensions) != 1 || config.Extensions[0].ID != "workflow-guidance" || config.ProjectInstructions != "workflow-project-context" {
		t.Fatal("workflow Skill binding not captured")
	}
}
