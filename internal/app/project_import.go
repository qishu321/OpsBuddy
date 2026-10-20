package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// A portable, data-only code collaboration. No model credentials, schedules,
// filesystem source paths, or commands are accepted as executable actions.
type ProjectImportBundle struct {
	SchemaVersion int                  `json:"schemaVersion"`
	ID            string               `json:"id"`
	Project       Project              `json:"project"`
	EntryTaskID   string               `json:"entryTaskId"`
	Tasks         []Task               `json:"tasks"`
	AgentGroups   []AgentGroup         `json:"agentGroups"`
	Assets        []ProjectImportAsset `json:"assets"`
}
type ProjectImportAsset struct {
	Asset Asset       `json:"asset"`
	Code  CodeProject `json:"code"`
}
type ProjectImportResult struct {
	BundleID        string `json:"bundleId"`
	ProjectID       string `json:"projectId"`
	TaskID          string `json:"taskId"`
	AlreadyImported bool   `json:"alreadyImported"`
}

func importFinished(status string) bool {
	switch status {
	case "succeeded", "failed", "cancelled", "interrupted", "blocked", "partially_succeeded":
		return true
	}
	return false
}
func parseProjectImport(raw []byte) (ProjectImportBundle, error) {
	var bundle ProjectImportBundle
	if len(raw) > 8<<20 {
		return bundle, errors.New("项目导入包最多8MB")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&bundle); err != nil {
		return bundle, errors.New("项目包格式无效")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return bundle, errors.New("项目包包含多余内容")
	}
	if bundle.SchemaVersion != 1 || strings.TrimSpace(bundle.ID) == "" || len(bundle.ID) > 160 || bundle.Project.ID == "" || strings.TrimSpace(bundle.Project.Name) == "" || len(bundle.Project.Name) > 160 || len(bundle.Project.Description) > 8000 {
		return bundle, errors.New("项目包版本、名称或标识无效")
	}
	if len(bundle.Tasks) < 1 || len(bundle.Tasks) > 80 || len(bundle.AgentGroups) > 8 || len(bundle.Assets) > 32 {
		return bundle, errors.New("导入包任务或文件数量超限")
	}
	tasks := map[string]Task{}
	assets := map[string]bool{}
	groups := map[string]AgentGroup{}
	for _, t := range bundle.Tasks {
		if t.ID == "" || tasks[t.ID].ID != "" || t.ProjectID != bundle.Project.ID || !importFinished(t.Status) {
			return bundle, errors.New("仅支持当前项目的已结束任务，不能导入待执行操作")
		}
		if t.Config != nil && (len(t.Config.MemoryRefs) > 0 || len(t.Config.KnowledgeRefs) > 0 || len(t.Config.KnowledgeIDs) > 0) {
			return bundle, errors.New("代码协作导入暂不迁移外部记忆或知识库")
		}
		if t.ScheduleID != "" || t.ScheduleRunID != "" || t.WorkflowRunID != "" || t.WorkflowNodeID != "" {
			return bundle, errors.New("代码协作导入不包含定时任务或其他工作流")
		}
		tasks[t.ID] = t
	}
	if tasks[bundle.EntryTaskID].ID == "" || tasks[bundle.EntryTaskID].AgentMemberID != "" {
		return bundle, errors.New("主会话入口不存在")
	}
	for i := range bundle.Assets {
		p := &bundle.Assets[i]
		if p.Asset.ID == "" || assets[p.Asset.ID] || p.Asset.ProjectID != bundle.Project.ID || p.Asset.Kind != "code" {
			return bundle, errors.New("仅支持当前项目的独立代码成果")
		}
		if err := validateCodeProject(&p.Code); err != nil {
			return bundle, err
		}
		if err := validateCodeUI(p.Code); err != nil {
			return bundle, err
		}
		assets[p.Asset.ID] = true
	}
	for _, g := range bundle.AgentGroups {
		if g.ID == "" || groups[g.ID].ID != "" || g.ProjectID != bundle.Project.ID || !importFinished(g.Status) || g.SchemaVersion < 2 || len(g.Members) < 1 || len(g.Members) > 6 {
			return bundle, errors.New("协作记录必须完整且已结束")
		}
		if tasks[g.RootTaskID].AgentGroupID != g.ID || tasks[g.RootTaskID].AgentMemberID != "" {
			return bundle, errors.New("协作主会话引用无效")
		}
		deps := map[string][]string{}
		for _, m := range g.Members {
			if m.ID == "" || !importFinished(m.Status) || len(m.Pending) > 0 {
				return bundle, errors.New("成员包含未结束或待执行指令")
			}
			if _, ok := deps[m.ID]; ok {
				return bundle, errors.New("成员ID重复")
			}
			deps[m.ID] = m.DependsOn
			for _, tid := range m.TaskIDs {
				if tasks[tid].AgentGroupID != g.ID || tasks[tid].AgentMemberID != m.ID {
					return bundle, errors.New("成员实际任务引用无效")
				}
			}
		}
		if err := validateDAG(deps); err != nil {
			return bundle, err
		}
		if g.PlannerTaskID != "" && tasks[g.PlannerTaskID].AgentGroupID != g.ID {
			return bundle, errors.New("规划任务引用无效")
		}
		groups[g.ID] = g
	}
	checkAssets := func(ids []string) error {
		for _, aid := range ids {
			if !assets[aid] {
				return errors.New("文件引用超出本次导入项目")
			}
		}
		return nil
	}
	for _, t := range bundle.Tasks {
		if (t.ParentID != "" && tasks[t.ParentID].ID == "") || (t.ConversationID != "" && tasks[t.ConversationID].ID == "") || (t.AgentGroupID != "" && groups[t.AgentGroupID].ID == "") {
			return bundle, errors.New("会话或协作引用跨越项目边界")
		}
		if err := checkAssets(append(append([]string{}, t.InputIDs...), t.OutputIDs...)); err != nil {
			return bundle, err
		}
		if t.Config != nil {
			if err := checkAssets(t.Config.AgentCandidateIDs); err != nil {
				return bundle, err
			}
		}
	}
	for _, p := range bundle.Assets {
		if p.Asset.TaskID != "" && tasks[p.Asset.TaskID].ID == "" {
			return bundle, errors.New("成果任务不存在")
		}
		if p.Asset.ParentID != "" && !assets[p.Asset.ParentID] {
			return bundle, errors.New("成果历史引用不存在")
		}
	}
	for _, g := range bundle.AgentGroups {
		if g.SourceTaskID != "" && tasks[g.SourceTaskID].ID == "" {
			return bundle, errors.New("协作来源会话不存在")
		}
		if err := checkAssets(g.OriginalInputIDs); err != nil {
			return bundle, err
		}
		for _, m := range g.Members {
			if err := checkAssets(m.InputIDs); err != nil {
				return bundle, err
			}
			if m.ProcessedTaskID != "" && tasks[m.ProcessedTaskID].AgentGroupID != g.ID {
				return bundle, errors.New("成员轮次引用无效")
			}
		}
		for _, r := range g.Reviews {
			if tasks[r.TaskID].AgentGroupID != g.ID {
				return bundle, errors.New("审阅引用无效")
			}
		}
		for _, p := range append([]AgentPlan{g.Plan}, func() []AgentPlan {
			ps := []AgentPlan{}
			for _, v := range g.PlanHistory {
				ps = append(ps, v.Plan)
			}
			return ps
		}()...) {
			for _, n := range p.Members {
				if err := checkAssets(n.InputIDs); err != nil {
					return bundle, err
				}
			}
		}
	}
	return bundle, nil
}

func (a *App) ImportProjectBundle(raw []byte) (ProjectImportResult, error) {
	b, err := parseProjectImport(raw)
	if err != nil {
		return ProjectImportResult{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return ProjectImportResult{}, errors.New("应用正在关闭")
	}
	key := contentHash(b)
	for _, p := range a.state.Projects {
		if p.ImportKey == key {
			result := ProjectImportResult{BundleID: b.ID, ProjectID: p.ID, AlreadyImported: true}
			for _, t := range a.state.Tasks {
				if t.ProjectID == p.ID && t.AgentMemberID == "" && t.AgentGroupID != "" {
					result.TaskID = t.ID
					break
				}
			}
			return result, nil
		}
	}
	// Allocate new identities even if source IDs happen to exist locally.
	projectID := id()
	taskIDs := map[string]string{"": ""}
	assetIDs := map[string]string{"": ""}
	groupIDs := map[string]string{"": ""}
	for _, t := range b.Tasks {
		taskIDs[t.ID] = id()
	}
	for _, p := range b.Assets {
		assetIDs[p.Asset.ID] = id()
	}
	for _, g := range b.AgentGroups {
		groupIDs[g.ID] = id()
	}
	mapAssets := func(ids []string) []string {
		mapped := make([]string, 0, len(ids))
		for _, old := range ids {
			mapped = append(mapped, assetIDs[old])
		}
		return mapped
	}
	remapConfig := func(c *TaskConfig) *TaskConfig {
		if c == nil {
			return nil
		}
		copy := cloneConfig(c)
		copy.AgentCandidateIDs = mapAssets(copy.AgentCandidateIDs)
		return copy
	}
	remapPlan := func(p AgentPlan) AgentPlan {
		for i := range p.Members {
			p.Members[i].InputIDs = mapAssets(p.Members[i].InputIDs)
		}
		return p
	}
	rootActual, err := filepath.EvalSymlinks(a.root)
	if err != nil {
		return ProjectImportResult{}, err
	}
	base, err := filepath.EvalSymlinks(filepath.Join(a.root, "files"))
	if err != nil {
		return ProjectImportResult{}, err
	}
	if !strings.EqualFold(filepath.Dir(base), rootActual) {
		return ProjectImportResult{}, errors.New("导入文件目录必须位于当前工作区")
	}
	created := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, folder := range created {
				if filepath.Dir(folder) == base {
					_ = os.RemoveAll(folder)
				}
			}
		}
	}()
	newAssets := []Asset{}
	for _, item := range b.Assets {
		asset := item.Asset
		asset.ID = assetIDs[asset.ID]
		asset.ProjectID = projectID
		asset.TaskID = taskIDs[asset.TaskID]
		asset.ParentID = assetIDs[asset.ParentID]
		asset.RequireRunConfirmation = true
		folder := filepath.Join(base, asset.ID+"-project")
		if err = os.Mkdir(folder, 0700); err != nil {
			return ProjectImportResult{}, err
		}
		created = append(created, folder)
		for _, f := range append(append([]CodeFile{}, item.Code.Files...), bundledProjectFiles(item.Code)...) {
			target := filepath.Join(folder, filepath.FromSlash(f.Path))
			if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return ProjectImportResult{}, err
			}
			if err = os.WriteFile(target, []byte(f.Content), 0600); err != nil {
				return ProjectImportResult{}, err
			}
		}
		manifest, _ := json.Marshal(item.Code)
		if err = os.WriteFile(filepath.Join(folder, "opsbuddy-project.json"), manifest, 0600); err != nil {
			return ProjectImportResult{}, err
		}
		asset.Path = "files/" + asset.ID + "-project/opsbuddy-project.json"
		asset.Size = int64(len(manifest))
		asset.Name = strings.TrimSuffix(safeName(item.Code.Title), ".zip")
		asset.Validation = "导入的历史交付；查看源码不会自动执行。手动启动后显示当前运行结果。"
		newAssets = append(newAssets, asset)
	}
	p := b.Project
	p.ID = projectID
	p.ImportKey = key
	newTasks := []Task{}
	for _, t := range b.Tasks {
		t.ID = taskIDs[t.ID]
		t.ProjectID = projectID
		t.ParentID = taskIDs[t.ParentID]
		t.ConversationID = taskIDs[t.ConversationID]
		t.AgentGroupID = groupIDs[t.AgentGroupID]
		t.InputIDs = mapAssets(t.InputIDs)
		t.OutputIDs = mapAssets(t.OutputIDs)
		t.Config = remapConfig(t.Config)
		newTasks = append(newTasks, t)
	}
	newGroups := []AgentGroup{}
	for _, g := range b.AgentGroups {
		g.ID = groupIDs[g.ID]
		g.ProjectID = projectID
		g.RootTaskID = taskIDs[g.RootTaskID]
		g.SourceTaskID = taskIDs[g.SourceTaskID]
		g.PlannerTaskID = taskIDs[g.PlannerTaskID]
		g.AutoResume = false
		g.Settling = false
		g.OriginalInputIDs = mapAssets(g.OriginalInputIDs)
		g.Plan = remapPlan(g.Plan)
		for i := range g.Members {
			m := &g.Members[i]
			m.TaskIDs = func() []string {
				ids := []string{}
				for _, tid := range m.TaskIDs {
					ids = append(ids, taskIDs[tid])
				}
				return ids
			}()
			m.InputIDs = mapAssets(m.InputIDs)
			m.ProcessedTaskID = taskIDs[m.ProcessedTaskID]
			m.Config = remapConfig(m.Config)
			m.Pending = nil
		}
		for i := range g.Reviews {
			g.Reviews[i].TaskID = taskIDs[g.Reviews[i].TaskID]
		}
		for i := range g.PlanHistory {
			g.PlanHistory[i].Plan = remapPlan(g.PlanHistory[i].Plan)
		}
		newGroups = append(newGroups, g)
	}
	a.state.Projects = append(a.state.Projects, p)
	a.state.Tasks = append(a.state.Tasks, newTasks...)
	a.state.Assets = append(a.state.Assets, newAssets...)
	a.state.AgentGroups = append(a.state.AgentGroups, newGroups...)
	if err = a.saveLocked(); err != nil {
		return ProjectImportResult{}, err
	}
	committed = true
	return ProjectImportResult{BundleID: b.ID, ProjectID: projectID, TaskID: taskIDs[b.EntryTaskID]}, nil
}

// Only runs inside the owning application, after opening its exclusive store.
// External tools stage a data-only package; they never edit the live database.
func (a *App) importPendingProjects() {
	queue := filepath.Join(a.root, "imports")
	info, err := os.Lstat(queue)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	entries, err := os.ReadDir(queue)
	if err != nil {
		return
	}
	count := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".opsbuddy-project.json") || entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			continue
		}
		count++
		if count > 8 {
			break
		}
		path := filepath.Join(queue, entry.Name())
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
		f.Close()
		if err != nil {
			continue
		}
		result, err := a.ImportProjectBundle(raw)
		receipt := map[string]any{"at": now(), "result": result, "ok": err == nil}
		if err != nil {
			receipt["error"] = err.Error()
		} else if !result.AlreadyImported {
			a.startupImport = &result
		}
		data, _ := json.MarshalIndent(receipt, "", "  ")
		_ = os.WriteFile(path+".receipt.json", data, 0600)
	}
}

func (b ProjectImportBundle) Summary() string {
	return fmt.Sprintf("%s · %d 个任务 · %d 组协作 · %d 份代码", b.Project.Name, len(b.Tasks), len(b.AgentGroups), len(b.Assets))
}
