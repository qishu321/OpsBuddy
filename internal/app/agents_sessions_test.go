package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitGroup(t *testing.T, a *App, groupID string) AgentGroup {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		for _, g := range a.snapshot().AgentGroups {
			if g.ID == groupID && !activeAgentGroup(g.Status) {
				return g
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("agent group did not finish")
	return AgentGroup{}
}
func TestAgentGroupParallelReviewFollowupAndIsolation(t *testing.T) {
	var active, maxActive atomic.Int32
	var mu sync.Mutex
	payloads := []string{}
	gate := make(chan struct{})
	var release sync.Once
	defer release.Do(func() { close(gate) })
	started := make(chan struct{}, 6)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		payloads = append(payloads, string(raw))
		mu.Unlock()
		n := active.Add(1)
		defer active.Add(-1)
		for current := maxActive.Load(); n > current; current = maxActive.Load() {
			if maxActive.CompareAndSwap(current, n) {
				break
			}
		}
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		}
		if strings.Contains(string(raw), "你是独立审阅者") {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"status\":\"pass\",\"summary\":\"受控资料审阅通过\",\"issues\":[]}"}}]}`)
		} else {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"协作结果\",\"summary\":\"已完成本成员工作\",\"sections\":[{\"heading\":\"材料\",\"body\":\"受控模型实际交付\"}]}"}}]}`)
		}
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "team-model", Name: "受控模型", Family: "openai", BaseURL: server.URL, ModelID: "test"})
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	w := request(t, a, "POST", "/api/agent-groups", map[string]any{"projectId": "inbox", "prompt": "完成项目汇报", "kind": "docx", "modelId": "team-model", "allowRemote": true, "members": []map[string]string{{"role": "explorer", "prompt": "ONLY_EXPLORER_SECRET"}, {"role": "worker", "prompt": "ONLY_WORKER_SECRET"}, {"role": "reviewer", "prompt": "CHECK_DELIVERABLES"}}})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var group AgentGroup
	_ = json.Unmarshal(w.Body.Bytes(), &group)
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(4 * time.Second):
			t.Fatal("independent members did not run in parallel")
		}
	}
	w = request(t, a, "POST", "/api/agent-groups/"+group.ID+"/members/"+group.Members[0].ID+"/instructions", map[string]string{"prompt": "FOLLOWUP_ONLY_EXPLORER"})
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	release.Do(func() { close(gate) })
	done := waitGroup(t, a, group.ID)
	if done.Status != "succeeded" || len(done.Members[0].TaskIDs) != 2 {
		t.Fatal(done)
	}
	if maxActive.Load() < 2 || maxActive.Load() > 3 {
		t.Fatal("unexpected concurrency", maxActive.Load())
	}
	snapshot := a.snapshot()
	root := Task{}
	for _, task := range snapshot.Tasks {
		if task.ID == done.RootTaskID {
			root = task
		}
	}
	if root.Status != "succeeded" || len(root.OutputIDs) != 1 {
		t.Fatal(root)
	}
	initialExplorer, initialWorker := "", ""
	mu.Lock()
	defer mu.Unlock()
	for _, raw := range payloads {
		if strings.Contains(raw, "请综合子任务成果") {
			continue
		}
		if strings.Contains(raw, "当前职责：ONLY_EXPLORER_SECRET") && !strings.Contains(raw, "FOLLOWUP_ONLY_EXPLORER") {
			initialExplorer = raw
		}
		if strings.Contains(raw, "当前职责：ONLY_WORKER_SECRET") {
			initialWorker = raw
		}
	}
	if initialExplorer == "" || initialWorker == "" || strings.Contains(initialExplorer, "ONLY_WORKER_SECRET") || strings.Contains(initialWorker, "ONLY_EXPLORER_SECRET") {
		t.Fatal("member contexts mixed")
	}
	reviewer := done.Members[2]
	for _, task := range snapshot.Tasks {
		if task.ID == reviewer.TaskIDs[0] && (len(task.InputIDs) < 1 || !strings.Contains(task.Prompt, "依赖交付")) {
			t.Fatal("reviewer ran without upstream artifacts")
		}
	}
	a.Close()
	reopened, err := New(a.root, a.web)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.snapshot().AgentGroups) != 1 {
		t.Fatal("agent group not persisted")
	}
}
func TestAgentGroupCancelQueuedAndRunning(t *testing.T) {
	entered := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		r.Body.Close()
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "cancel-model", Family: "openai", BaseURL: server.URL, ModelID: "test"})
	_ = a.saveLocked()
	members := []map[string]string{}
	for i := 0; i < 6; i++ {
		members = append(members, map[string]string{"role": "worker", "prompt": fmt.Sprintf("任务 %d", i)})
	}
	w := request(t, a, "POST", "/api/agent-groups", map[string]any{"projectId": "inbox", "prompt": "取消测试", "kind": "docx", "modelId": "cancel-model", "allowRemote": true, "members": members})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var g AgentGroup
	_ = json.Unmarshal(w.Body.Bytes(), &g)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("no running model")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		count := 0
		for _, task := range a.snapshot().Tasks {
			if task.AgentGroupID == g.ID {
				count++
			}
		}
		if count == 7 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	queued := 0
	for _, task := range a.snapshot().Tasks {
		if task.AgentGroupID == g.ID && task.Status == "queued" {
			queued++
		}
	}
	if queued == 0 {
		t.Fatal("no queued child was tested")
	}
	w = request(t, a, "POST", "/api/agent-groups/"+g.ID+"/cancel", nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	done := waitGroup(t, a, g.ID)
	if done.Status != "cancelled" {
		t.Fatal(done.Status)
	}
	a.Close()
	for _, task := range a.snapshot().Tasks {
		if task.AgentGroupID == g.ID && activeTask(task.Status) {
			t.Fatal("active child survived group stop", task.ID, task.Status)
		}
	}
	if w = request(t, a, "POST", "/api/agent-groups/"+g.ID+"/members/"+g.Members[0].ID+"/instructions", map[string]string{"prompt": "不应恢复"}); w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func TestAgentGroupRejectsCrossProjectAndRemoteConsent(t *testing.T) {
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "m", Family: "openai"})
	_ = a.saveLocked()
	for _, body := range []map[string]any{{"projectId": "missing", "prompt": "x"}, {"projectId": "inbox", "prompt": "x", "modelId": "m"}, {"projectId": "inbox", "prompt": "x", "sourceTaskId": "missing"}} {
		body["members"] = []map[string]string{{"role": "worker", "prompt": "任务"}}
		w := request(t, a, "POST", "/api/agent-groups", body)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if len(a.snapshot().AgentGroups) != 0 {
		t.Fatal("invalid group persisted")
	}
}

func TestFolderReferenceRejectsOversizeBeforeImport(t *testing.T) {
	a := newTestApp(t)
	folder := t.TempDir()
	for i := 0; i < 17; i++ {
		if err := os.WriteFile(filepath.Join(folder, fmt.Sprintf("%d.txt", i)), []byte("material"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.ImportLocalFolder("inbox", folder); err == nil {
		t.Fatal("oversize folder accepted")
	}
	if len(a.snapshot().Assets) != 0 {
		t.Fatal("partial folder imported")
	}
	_ = os.Remove(filepath.Join(folder, "16.txt"))
	assets, err := a.ImportLocalFolder("inbox", folder)
	if err != nil || len(assets) != 16 {
		t.Fatal(len(assets), err)
	}
	original, _ := os.ReadFile(filepath.Join(folder, "0.txt"))
	if string(original) != "material" {
		t.Fatal("original changed")
	}
}

func TestImageModelPayloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.png")
	file, _ := os.Create(path)
	_ = png.Encode(file, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	file.Close()
	img, err := loadModelImage(path)
	if err != nil || img.MIME != "image/png" {
		t.Fatal(img.MIME, err)
	}
	for _, family := range []string{"openai", "anthropic", "gemini"} {
		t.Run(family, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				body := string(raw)
				if !strings.Contains(body, img.Data) || !strings.Contains(body, "image/png") {
					t.Error("image bytes missing")
				}
				switch family {
				case "openai":
					if !strings.Contains(body, "image_url") {
						t.Error("wrong OpenAI schema")
					}
					fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"sections\":[{\"body\":\"视觉测试\"}]}"}}]}`)
				case "anthropic":
					if !strings.Contains(body, "media_type") {
						t.Error("wrong Anthropic schema")
					}
					fmt.Fprint(w, `{"content":[{"text":"{\"sections\":[{\"body\":\"视觉测试\"}]}"}]}`)
				case "gemini":
					if !strings.Contains(body, "inlineData") {
						t.Error("wrong Gemini schema")
					}
					fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"{\"sections\":[{\"body\":\"视觉测试\"}]}"}]}}]}`)
				}
			}))
			defer server.Close()
			if _, err := generateWithImages(t.Context(), Model{ID: "m", Family: family, BaseURL: server.URL, ModelID: "test"}, "system", "读取图片", []modelImage{img}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestDOCXRevisionPreservesTablesAndOtherParts(t *testing.T) {
	folder := t.TempDir()
	source := filepath.Join(folder, "source.docx")
	destination := filepath.Join(folder, "revision.docx")
	table := `<w:tbl><w:tr><w:tc><w:p><w:r><w:t>TABLE_KEEP_00450</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	body := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>第一部分保持</w:t></w:r></w:p><w:p><w:r><w:t>第二部分很长</w:t></w:r><w:r><w:t>继续很长</w:t></w:r></w:p>` + table + `<w:p><w:r><w:t>第三部分保持</w:t></w:r></w:p></w:body></w:document>`
	f, _ := os.Create(source)
	z := zip.NewWriter(f)
	for name, content := range map[string]string{"word/document.xml": body, "word/styles.xml": "STYLE_KEEP"} {
		w, _ := z.Create(name)
		_, _ = io.WriteString(w, content)
	}
	_ = z.Close()
	_ = f.Close()
	err := reviseDocxFile(destination, source, Deliverable{Rows: [][]any{{"p1", "压缩后的第二部分 <保留>"}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := readDocxBody(destination)
	if !bytes.Contains(raw, []byte(table)) || !bytes.Contains(raw, []byte("第一部分保持")) || !bytes.Contains(raw, []byte("第三部分保持")) || bytes.Contains(raw, []byte("继续很长")) || !bytes.Contains(raw, []byte("&lt;保留&gt;")) {
		t.Fatal(string(raw))
	}
	original, _ := readDocxBody(source)
	if string(original) != body {
		t.Fatal("original overwritten")
	}
	archive, _ := zip.OpenReader(destination)
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name == "word/styles.xml" {
			r, _ := entry.Open()
			v, _ := io.ReadAll(r)
			r.Close()
			if string(v) != "STYLE_KEEP" {
				t.Fatal("styles changed")
			}
		}
	}
}
func TestConversationContinuesWithFrozenHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"继续\",\"sections\":[{\"body\":\"保留历史\"}]}"}}]}`)
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "m", Family: "openai", BaseURL: server.URL, ModelID: "test"})
	_ = a.saveLocked()
	first := request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "kind": "docx", "prompt": "第一轮需求"})
	var root Task
	_ = json.Unmarshal(first.Body.Bytes(), &root)
	root = waitTask(t, a, root.ID)
	next := request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "kind": "pptx", "prompt": "改成演示", "parentId": root.ID, "modelId": "m", "allowRemote": true})
	if next.Code != 201 {
		t.Fatal(next.Body.String())
	}
	var child Task
	_ = json.Unmarshal(next.Body.Bytes(), &child)
	if child.ConversationID != root.ConversationID || !strings.Contains(child.Config.ConversationContext, "第一轮需求") || len(child.InputIDs) != 1 {
		t.Fatal(child)
	}
	if task := waitTask(t, a, child.ID); task.Status != "succeeded" {
		t.Fatal(task.Error)
	}
	wrong := request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "kind": "docx", "prompt": "无效续写", "parentId": "missing", "modelId": "m", "allowRemote": true})
	if wrong.Code != 400 {
		t.Fatal(wrong.Code)
	}
}

func TestSkillSelectionOnlyLoadsChosenBodies(t *testing.T) {
	var selection, generation atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		if strings.Contains(body, "技能选择器") {
			selection.Add(1)
			if strings.Contains(body, "BODY_SELECTED") || strings.Contains(body, "BODY_UNUSED") {
				t.Error("skill body leaked into catalog")
			}
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"rows\":[[\"chosen\"]]}"}}]}`)
			return
		}
		generation.Add(1)
		if !strings.Contains(body, "BODY_SELECTED") || strings.Contains(body, "BODY_UNUSED") {
			t.Error("incorrect skill bodies in generation")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"sections\":[{\"body\":\"结果\"}]}"}}]}`)
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "m", Family: "openai", BaseURL: server.URL, ModelID: "test"})
	a.state.Extensions = []Extension{{ID: "chosen", Name: "文档规则", Description: "文档编辑", Kind: "skill", Enabled: true, Content: "BODY_SELECTED"}, {ID: "unused", Name: "表格规则", Description: "表格分析", Kind: "skill", Enabled: true, Content: "BODY_UNUSED"}}
	_ = a.saveLocked()
	w := request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "prompt": "生成文档", "kind": "docx", "modelId": "m", "allowRemote": true, "skillIds": []string{"chosen", "unused"}})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var task Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	task = waitTask(t, a, task.ID)
	if task.Status != "succeeded" || selection.Load() != 1 || generation.Load() != 1 || len(task.UsedSkillIDs) != 1 || task.UsedSkillIDs[0] != "chosen" {
		t.Fatal(task.Error, task.UsedSkillIDs)
	}
}
func TestSchemaTwoUpgradeRetainsConversations(t *testing.T) {
	a := newTestApp(t)
	a.state.Tasks = append(a.state.Tasks, Task{ID: "old-conversation", ProjectID: "inbox", Prompt: "历史内容", Status: "succeeded"})
	_ = a.saveLocked()
	if _, err := a.store.conn.ExecContext(t.Context(), "PRAGMA user_version=2"); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b, err := New(a.root, a.web)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var version int
	_ = b.store.conn.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version)
	if version != 4 || len(b.snapshot().Tasks) != 1 || b.snapshot().AgentGroups == nil {
		t.Fatal(version, b.snapshot().Tasks)
	}
}

func TestAgentGroupPartialFailureRetainsSuccessfulFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "当前职责：FAIL_THIS_MEMBER") && !strings.Contains(string(raw), "请综合子任务成果") {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"summary\":\"汇总成功分支\",\"sections\":[{\"body\":\"交付内容\"}]}"}}]}`)
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "m", Family: "openai", BaseURL: server.URL, ModelID: "test"})
	_ = a.saveLocked()
	w := request(t, a, "POST", "/api/agent-groups", map[string]any{"projectId": "inbox", "prompt": "部分失败测试", "kind": "docx", "modelId": "m", "allowRemote": true, "members": []map[string]string{{"role": "worker", "prompt": "成功任务"}, {"role": "worker", "prompt": "FAIL_THIS_MEMBER"}}})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var group AgentGroup
	_ = json.Unmarshal(w.Body.Bytes(), &group)
	done := waitGroup(t, a, group.ID)
	if done.Status != "partially_succeeded" {
		t.Fatal(done.Status, done.Error)
	}
	for _, task := range a.snapshot().Tasks {
		if task.ID == done.RootTaskID && (len(task.OutputIDs) != 1 || !strings.Contains(task.Result, "未通过完整验收")) {
			t.Fatal(task)
		}
	}
}
