package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func importFixture() ProjectImportBundle {
	return ProjectImportBundle{SchemaVersion: 1, ID: "fixture-completed-project", Project: Project{ID: "inbox", Name: "已完成协作演示", CreatedAt: now()}, EntryTaskID: "root", Tasks: []Task{
		{ID: "root", ProjectID: "inbox", AgentGroupID: "group", ConversationID: "root", Title: "主会话", Status: "succeeded", InputIDs: []string{"asset"}, OutputIDs: []string{"asset"}, CreatedAt: now(), UpdatedAt: now()},
		{ID: "work", ProjectID: "inbox", AgentGroupID: "group", AgentMemberID: "worker", ConversationID: "work", Title: "成员交付", Status: "succeeded", OutputIDs: []string{"asset"}, CreatedAt: now(), UpdatedAt: now()},
	}, AgentGroups: []AgentGroup{{ID: "group", RootTaskID: "root", ProjectID: "inbox", SchemaVersion: 2, PlanVersion: 1, Status: "succeeded", CreatedAt: now(), Plan: AgentPlan{Summary: "真实记录", Members: []AgentNodeSpec{{ID: "worker", Role: "worker", Name: "制作", Prompt: "任务", Kind: "code", InputIDs: []string{"asset"}}}}, Members: []AgentMember{{ID: "worker", Name: "制作", Role: "worker", Status: "succeeded", TaskIDs: []string{"work"}, ProcessedTaskID: "work", InputIDs: []string{"asset"}}}}}, Assets: []ProjectImportAsset{{Asset: Asset{ID: "asset", Kind: "code", ProjectID: "inbox", TaskID: "work", Name: "源码", CreatedAt: now()}, Code: CodeProject{Title: "代码交付", Runtime: "go", Mode: "command", Entry: "main.go", Files: []CodeFile{{Path: "main.go", Content: "package main\nfunc main() {}"}, {Path: "go.mod", Content: "module demo\ngo 1.22\n"}}}}}}
}
func importRaw(t *testing.T, b ProjectImportBundle) []byte {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestProjectImportRemapsHistoryAndIsIdempotentWithoutExecution(t *testing.T) {
	a := newTestApp(t)
	raw := importRaw(t, importFixture())
	result, err := a.ImportProjectBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	s := a.snapshot()
	if len(s.Projects) != 2 || len(s.Tasks) != 2 || len(s.AgentGroups) != 1 || len(s.Assets) != 1 || result.ProjectID == "inbox" || result.TaskID == "root" {
		t.Fatal("import overwrote source identities", result)
	}
	root := a.taskLocked(result.TaskID)
	g := s.AgentGroups[0]
	m := g.Members[0]
	if root.ProjectID != result.ProjectID || root.AgentGroupID != g.ID || g.RootTaskID != root.ID || root.InputIDs[0] != s.Assets[0].ID || m.TaskIDs[0] == "work" || m.ProcessedTaskID != m.TaskIDs[0] || g.Plan.Members[0].InputIDs[0] != s.Assets[0].ID {
		t.Fatal("broken references")
	}
	if !s.Assets[0].RequireRunConfirmation || len(a.codeRuns) != 0 || len(a.agentRuns) != 0 || g.AutoResume {
		t.Fatal("import launched work")
	}
	storedAsset, err := a.asset(s.Assets[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.loadCodeProject(storedAsset)
	if err != nil || p.Files[0].Content != "package main\nfunc main() {}" {
		t.Fatal("source not preserved", err)
	}
	again, err := a.ImportProjectBundle(raw)
	if err != nil || !again.AlreadyImported || again.ProjectID != result.ProjectID || again.TaskID != result.TaskID || len(a.snapshot().Tasks) != 2 {
		t.Fatal("duplicate import", again, err)
	}
}

func TestProjectImportRejectsUnsafeAndCrossProjectPackages(t *testing.T) {
	for _, mutate := range []func(*ProjectImportBundle){
		func(b *ProjectImportBundle) { b.Tasks[0].Status = "running" },
		func(b *ProjectImportBundle) { b.Tasks[0].InputIDs = []string{"foreign"} },
		func(b *ProjectImportBundle) { b.Tasks[1].ProjectID = "foreign" },
		func(b *ProjectImportBundle) { b.Assets[0].Code.Files[0].Path = "../outside.go" },
		func(b *ProjectImportBundle) { b.AgentGroups[0].Members[0].DependsOn = []string{"worker"} },
		func(b *ProjectImportBundle) { b.AgentGroups[0].Members[0].Pending = []string{"start command"} },
		func(b *ProjectImportBundle) { b.Tasks[0].ConversationID = "foreign" },
	} {
		a := newTestApp(t)
		b := importFixture()
		mutate(&b)
		if _, err := a.ImportProjectBundle(importRaw(t, b)); err == nil {
			t.Fatal("unsafe import accepted")
		}
		if len(a.snapshot().Projects) != 1 || len(a.snapshot().Assets) != 0 {
			t.Fatal("invalid import changed data")
		}
	}
	b := importRaw(t, importFixture())
	b = append(b, []byte(" trailing")...)
	if _, err := parseProjectImport(b); err == nil {
		t.Fatal("trailing data accepted")
	}
}

func TestProjectImportRollsBackFilesAndStateOnDatabaseFailure(t *testing.T) {
	a := newTestApp(t)
	_, err := a.store.conn.ExecContext(context.Background(), `CREATE TRIGGER reject_import BEFORE INSERT ON entities WHEN NEW.kind='projects' BEGIN SELECT RAISE(ABORT,'test failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ImportProjectBundle(importRaw(t, importFixture())); err == nil {
		t.Fatal("save should fail")
	}
	s := a.snapshot()
	files, err := os.ReadDir(filepath.Join(a.root, "files"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Projects) != 1 || len(s.Tasks) != 0 || len(s.AgentGroups) != 0 || len(s.Assets) != 0 || len(files) != 0 {
		t.Fatal("partial import escaped rollback")
	}
}

func TestProjectImportStartupQueueOnlyImportsOnce(t *testing.T) {
	root := t.TempDir()
	queue := filepath.Join(root, "imports")
	if err := os.Mkdir(queue, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(queue, "fixture.opsbuddy-project.json")
	if err := os.WriteFile(path, importRaw(t, importFixture()), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	s := a.snapshot()
	if s.StartupImport == nil || len(s.Projects) != 2 || s.StartupImport.TaskID == "" {
		t.Fatal("queue import was not surfaced")
	}
	projectID := s.StartupImport.ProjectID
	a.Close()
	b, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	s = b.snapshot()
	if s.StartupImport != nil || len(s.Projects) != 2 || len(s.Tasks) != 2 || s.Projects[1].ID != projectID {
		t.Fatal("queue replayed on restart")
	}
}

func TestProjectImportActualDemoRetainsRealReceipts(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "demos", "multi-agent-todo", "todo-demo.opsbuddy-project.json"))
	if os.IsNotExist(err) {
		t.Skip("local completed demo package unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t)
	result, err := a.ImportProjectBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	s := a.snapshot()
	if result.TaskID == "" || len(s.AgentGroups) != 1 || len(s.AgentGroups[0].Members) != 3 || len(s.Assets) != 3 || len(s.Tasks) != 6 {
		t.Fatal("real history incomplete")
	}
	receipts := 0
	for _, task := range s.Tasks {
		for _, step := range task.Steps {
			if step.Name == "模型接口响应" {
				receipts++
			}
		}
	}
	if receipts != 5 || len(s.Models) != 0 || len(a.codeRuns) != 0 {
		t.Fatal("receipt or credential mismatch", receipts)
	}
}
