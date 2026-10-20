package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureKnowledge(t *testing.T, a *App) (KnowledgeBase, Asset) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "费用管理规定.txt")
	if err := os.WriteFile(path, []byte("差旅报销标准：住宿每晚不超过 450 元。报销需要发票。\n研发办公设备采购额度为 8000 元。"), 0600); err != nil {
		t.Fatal(err)
	}
	assets, err := a.ImportLocalFiles("inbox", []string{path})
	if err != nil {
		t.Fatal(err)
	}
	w := request(t, a, "POST", "/api/knowledge", map[string]string{"name": "工作规范", "projectId": "inbox"})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var base KnowledgeBase
	_ = json.Unmarshal(w.Body.Bytes(), &base)
	w = request(t, a, "POST", "/api/knowledge/"+base.ID+"/documents", map[string]any{"assetIds": []string{assets[0].ID}})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	return base, assets[0]
}
func TestKnowledgeRetrievalAndFrozenCitations(t *testing.T) {
	a := newTestApp(t)
	base, asset := fixtureKnowledge(t, a)
	w := request(t, a, "POST", "/api/knowledge/search", map[string]any{"projectId": "inbox", "baseIds": []string{base.ID}, "query": "住宿报销标准"})
	var hits []KnowledgeHit
	_ = json.Unmarshal(w.Body.Bytes(), &hits)
	if w.Code != 200 || len(hits) == 0 || !strings.Contains(hits[0].Text, "450") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "roleId": "docx", "kind": "docx", "prompt": "住宿报销标准", "knowledgeIds": []string{base.ID}})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var task Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	task = waitTask(t, a, task.ID)
	if task.Status != "succeeded" || len(task.Config.KnowledgeRefs) == 0 {
		t.Fatal(task)
	}
	v, path, err := a.LocalAsset(task.OutputIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	text, err := readDocument(path, v.Kind)
	if err != nil || !strings.Contains(text, "450") || !strings.Contains(text, asset.Name) {
		t.Fatal(text, err)
	}
	w = request(t, a, "POST", "/api/knowledge/"+base.ID+"/documents", map[string]any{"assetIds": []string{asset.ID}})
	if w.Code != 200 || len(a.snapshot().KnowledgeDocuments) != 1 {
		t.Fatal("reindex duplicated document")
	}
	doc := a.snapshot().KnowledgeDocuments[0]
	w = request(t, a, "DELETE", "/api/knowledge/documents/"+doc.ID, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, _, err = a.LocalAsset(asset.ID); err != nil {
		t.Fatal("removing index removed original file")
	}
	for _, saved := range a.snapshot().Tasks {
		if saved.ID == task.ID && saved.Config.KnowledgeRefs[0].ContentHash != hits[0].ContentHash {
			t.Fatal("historical citation changed")
		}
	}
	base2, _ := fixtureKnowledge(t, a)
	root := a.root
	web := a.web
	a.Close()
	restored, err := New(root, web)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restored.mu.RLock()
	persisted, err := restored.searchKnowledgeLocked("inbox", []string{base2.ID}, "住宿", 5)
	restored.mu.RUnlock()
	if err != nil || len(persisted) == 0 {
		t.Fatal("index not persisted", err)
	}
	if len(restored.snapshot().KnowledgeDocuments[0].Chunks) != 0 {
		t.Fatal("public state contains full index")
	}
}
func TestKnowledgeScopeAndPDFPages(t *testing.T) {
	a := newTestApp(t)
	base, _ := fixtureKnowledge(t, a)
	w := request(t, a, "POST", "/api/knowledge/search", map[string]any{"projectId": "other", "baseIds": []string{base.ID}, "query": "住宿"})
	if w.Code != 400 {
		t.Fatal("cross-project search allowed")
	}
	w = request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "prompt": "生成文档", "knowledgeIds": []string{"missing"}})
	if w.Code != 400 {
		t.Fatal("missing knowledge base allowed")
	}
	chunks := chunkText("[第 1 页]\n第一条规定\n[第 2 页]\n第二条规定")
	if len(chunks) != 2 || chunks[1].Page != 2 {
		t.Fatal(chunks)
	}
}
func flowFixture() WorkflowDefinition {
	return WorkflowDefinition{ProjectID: "inbox", Name: "条件交付", Enabled: true, Nodes: []WorkflowNode{{ID: "s", Type: "start", Label: "开始"}, {ID: "c", Type: "condition", Label: "是否汇报", Field: "input", Operator: "contains", Value: "汇报"}, {ID: "a", Type: "agent", Label: "演示", RoleID: "pptx", Kind: "pptx", Prompt: "{{input}}"}, {ID: "b", Type: "agent", Label: "文档", RoleID: "docx", Kind: "docx", Prompt: "{{input}}"}, {ID: "m", Type: "merge", Label: "汇合"}, {ID: "e", Type: "end", Label: "交付"}}, Edges: []WorkflowEdge{{ID: "1", Source: "s", Target: "c", SourceHandle: "out"}, {ID: "2", Source: "c", Target: "a", SourceHandle: "true"}, {ID: "3", Source: "c", Target: "b", SourceHandle: "false"}, {ID: "4", Source: "a", Target: "m", SourceHandle: "out"}, {ID: "5", Source: "b", Target: "m", SourceHandle: "out"}, {ID: "6", Source: "m", Target: "e", SourceHandle: "out"}}}
}
func savedFlow(t *testing.T, a *App, f WorkflowDefinition) WorkflowDefinition {
	t.Helper()
	w := request(t, a, "POST", "/api/workflows", f)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var result WorkflowDefinition
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	return result
}
func runFlow(t *testing.T, a *App, f WorkflowDefinition, prompt string, allow bool) WorkflowRun {
	t.Helper()
	w := request(t, a, "POST", "/api/workflows/"+f.ID+"/runs", map[string]any{"prompt": prompt, "allowRemote": allow})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var run WorkflowRun
	_ = json.Unmarshal(w.Body.Bytes(), &run)
	return run
}
func waitFlow(t *testing.T, a *App, runID string) WorkflowRun {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		for _, run := range a.snapshot().WorkflowRuns {
			if run.ID == runID && run.Status != "running" && run.Status != "queued" {
				return run
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("workflow timed out")
	return WorkflowRun{}
}
func TestWorkflowConditionalExecutionAndPersistence(t *testing.T) {
	a := newTestApp(t)
	f := savedFlow(t, a, flowFixture())
	for _, tc := range []struct{ prompt, active, skipped, kind string }{{"制作项目汇报", "a", "b", "pptx"}, {"整理会议纪要", "b", "a", "docx"}} {
		run := runFlow(t, a, f, tc.prompt, false)
		result := waitFlow(t, a, run.ID)
		if result.Status != "succeeded" || len(result.OutputIDs) != 1 {
			t.Fatalf("%+v", result)
		}
		for _, n := range result.Nodes {
			if n.NodeID == tc.skipped && n.Status != "skipped" {
				t.Fatal("unchosen branch executed", n)
			}
			if n.NodeID == tc.active && n.TaskID == "" {
				t.Fatal("agent did not create real task")
			}
		}
		asset, _, err := a.LocalAsset(result.OutputIDs[0])
		if err != nil || asset.Kind != tc.kind {
			t.Fatal(asset, err)
		}
	}
	root, web := a.root, a.web
	a.Close()
	b, err := New(root, web)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if len(b.snapshot().WorkflowRuns) != 2 || b.snapshot().Workflows[0].Version != 1 {
		t.Fatal("workflow persistence failed")
	}
}
func TestWorkflowValidationAndOptimisticVersion(t *testing.T) {
	a := newTestApp(t)
	f := flowFixture()
	bad := f
	bad.Edges = append(append([]WorkflowEdge{}, f.Edges...), WorkflowEdge{ID: "cycle", Source: "m", Target: "c"})
	if validateWorkflow(bad) == nil {
		t.Fatal("cycle allowed")
	}
	bad = f
	bad.Edges = bad.Edges[:1]
	if validateWorkflow(bad) == nil {
		t.Fatal("unreachable nodes allowed")
	}
	f = savedFlow(t, a, f)
	old := f
	f.Name = "修改名称"
	f = savedFlow(t, a, f)
	w := request(t, a, "POST", "/api/workflows", old)
	if w.Code != 409 {
		t.Fatal("lost update accepted")
	}
	f.Enabled = false
	f = savedFlow(t, a, f)
	w = request(t, a, "POST", "/api/workflows/"+f.ID+"/runs", map[string]string{"prompt": "run"})
	if w.Code != 400 {
		t.Fatal("disabled workflow ran")
	}
}
func TestWorkflowKnowledgeHandoff(t *testing.T) {
	a := newTestApp(t)
	base, _ := fixtureKnowledge(t, a)
	f := WorkflowDefinition{ProjectID: "inbox", Name: "知识撰写", Enabled: true, Nodes: []WorkflowNode{{ID: "s", Type: "start", Label: "输入"}, {ID: "k", Type: "knowledge", Label: "知识检索", BaseIDs: []string{base.ID}, Query: "{{input}}"}, {ID: "a", Type: "agent", Label: "撰写", Kind: "docx", RoleID: "docx", Prompt: "{{input}}"}, {ID: "e", Type: "end", Label: "交付"}}, Edges: []WorkflowEdge{{ID: "1", Source: "s", Target: "k"}, {ID: "2", Source: "k", Target: "a"}, {ID: "3", Source: "a", Target: "e"}}}
	f = savedFlow(t, a, f)
	run := waitFlow(t, a, runFlow(t, a, f, "住宿报销标准", false).ID)
	if run.Status != "succeeded" {
		t.Fatal(run.Error)
	}
	v, path, err := a.LocalAsset(run.OutputIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	text, err := readDocument(path, v.Kind)
	if err != nil || !strings.Contains(text, "450") {
		t.Fatal("knowledge was not passed to agent", text, err)
	}
}
func TestParallelAgentsAndCancellation(t *testing.T) {
	a := newTestApp(t)
	var concurrent, maximum atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := concurrent.Add(1)
		defer concurrent.Add(-1)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		select {
		case <-time.After(120 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		text := `{"title":"并行成果","sections":[{"heading":"结果","body":"测试生成"}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": text}}}})
	}))
	defer server.Close()
	a.mu.Lock()
	a.state.Models = append(a.state.Models, Model{ID: "mock", Name: "测试模型", Family: "openai", BaseURL: server.URL, ModelID: "test"})
	_ = a.saveLocked()
	a.mu.Unlock()
	f := WorkflowDefinition{ProjectID: "inbox", Name: "并行协作", Enabled: true, Nodes: []WorkflowNode{{ID: "s", Type: "start", Label: "开始"}, {ID: "e", Type: "end", Label: "汇集"}}, Edges: []WorkflowEdge{}}
	for i := 0; i < 5; i++ {
		n := fmt.Sprint(i)
		f.Nodes = append(f.Nodes, WorkflowNode{ID: n, Type: "agent", Label: n, Kind: "docx", RoleID: "docx", ModelID: "mock", Prompt: "生成文档"})
		f.Edges = append(f.Edges, WorkflowEdge{ID: n + "in", Source: "s", Target: n}, WorkflowEdge{ID: n + "out", Source: n, Target: "e"})
	}
	f = savedFlow(t, a, f)
	w := request(t, a, "POST", "/api/workflows/"+f.ID+"/runs", map[string]string{"prompt": "test"})
	if w.Code != 400 {
		t.Fatal("model workflow ran without consent")
	}
	run := waitFlow(t, a, runFlow(t, a, f, "并行生成五份文档", true).ID)
	if run.Status != "succeeded" || len(run.OutputIDs) != 5 || maximum.Load() > 3 || maximum.Load() < 2 {
		t.Fatal(run.Status, len(run.OutputIDs), maximum.Load())
	}
	run = runFlow(t, a, f, "取消并行任务", true)
	deadline := time.Now().Add(2 * time.Second)
	for concurrent.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if concurrent.Load() == 0 {
		t.Fatal("no running child to cancel")
	}
	w = request(t, a, "POST", "/api/workflow-runs/"+run.ID+"/cancel", nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	a.Close()
	for _, task := range a.snapshot().Tasks {
		if task.WorkflowRunID == run.ID && (task.Status == "running" || task.Status == "queued") {
			t.Fatal("child left running after cancellation")
		}
	}
}
func TestPartialJoinPreservesSuccessfulBranch(t *testing.T) {
	a := newTestApp(t)
	f := WorkflowDefinition{ProjectID: "inbox", Name: "部分交付", Enabled: true, Nodes: []WorkflowNode{{ID: "s", Type: "start", Label: "开始"}, {ID: "good", Type: "agent", Label: "文档", Kind: "docx", RoleID: "docx", Prompt: "生成文档"}, {ID: "bad", Type: "tool", Label: "缺失 PDF 输入", Kind: "pdf", RoleID: "pdf"}, {ID: "m", Type: "merge", Label: "允许部分成功", JoinMode: "allow_partial"}, {ID: "e", Type: "end", Label: "结束"}}, Edges: []WorkflowEdge{{ID: "1", Source: "s", Target: "good"}, {ID: "2", Source: "s", Target: "bad"}, {ID: "3", Source: "good", Target: "m"}, {ID: "4", Source: "bad", Target: "m"}, {ID: "5", Source: "m", Target: "e"}}}
	f = savedFlow(t, a, f)
	run := waitFlow(t, a, runFlow(t, a, f, "部分交付", false).ID)
	if run.Status != "partially_succeeded" || len(run.OutputIDs) != 1 {
		t.Fatal(run.Status, run.Error, run.Nodes)
	}
}

func TestSchemaOneUpgradeKeepsExistingProjects(t *testing.T) {
	a := newTestApp(t)
	a.mu.Lock()
	a.state.Projects[0].Name = "升级前项目"
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.conn.ExecContext(t.Context(), "PRAGMA user_version=1"); err != nil {
		t.Fatal(err)
	}
	a.mu.Unlock()
	root, web := a.root, a.web
	a.Close()
	b, err := New(root, web)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var version int
	if err = b.store.conn.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil || version != 4 || b.snapshot().Projects[0].Name != "升级前项目" {
		t.Fatal(version, err)
	}
}
