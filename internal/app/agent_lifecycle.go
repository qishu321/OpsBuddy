package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type AgentPlanRevision struct {
	Version int       `json:"version"`
	At      string    `json:"at"`
	Plan    AgentPlan `json:"plan"`
}

// One coordinator per group. Paused requests must release their slots before a
// new coordinator starts, even if a provider takes time to observe cancellation.
func (a *App) startAgentRunLocked(groupID, rootID string, secrets map[string]string) {
	if a.agentRuns == nil {
		a.agentRuns = map[string]bool{}
	}
	a.agentRuns[groupID] = true
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	a.cancels[groupID], a.cancels[rootID] = cancel, cancel
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		defer cancel()
		defer func() { a.mu.Lock(); delete(a.agentRuns, groupID); a.mu.Unlock() }()
		a.executeAgents(ctx, groupID, secrets)
	}()
}

func (a *App) agentSettlingLocked(g *AgentGroup) bool {
	if a.agentRuns[g.ID] {
		return true
	}
	for _, t := range a.state.Tasks {
		if t.AgentGroupID == g.ID && a.cancels[t.ID] != nil {
			return true
		}
	}
	return false
}

func (a *App) suspendAgentsLocked(g *AgentGroup, status string) {
	g.InterruptedStage = g.Status
	g.Status, g.Error, g.FinishedAt = status, "已暂停派发，已完成的成果保留。未完成请求将在继续时重新执行。", ""
	if status == "interrupted" {
		g.Error = "桌面程序退出，协作中断。已完成成果保留。"
	}
	if cancel := a.cancels[g.ID]; cancel != nil {
		cancel()
	}
	for i := range a.state.Tasks {
		t := &a.state.Tasks[i]
		if t.AgentGroupID == g.ID && activeTask(t.Status) {
			if cancel := a.cancels[t.ID]; cancel != nil {
				cancel()
			}
			t.Status, t.Error, t.UpdatedAt = "cancelled", g.Error, now()
			if status == "interrupted" {
				t.Status = "interrupted"
			}
		}
	}
	// Pending follow-up instructions are checkpointed, not discarded.
}

func (a *App) pauseAgents(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	g := a.groupLocked(r.PathValue("id"))
	if g == nil {
		problem(w, 404, errors.New("协作不存在"))
		return
	}
	if !activeAgentGroup(g.Status) {
		problem(w, 409, errors.New("当前协作不在执行中"))
		return
	}
	a.suspendAgentsLocked(g, "paused")
	if err := a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 202, map[string]bool{"paused": true})
}

// Credentials are resolved afresh, but destinations and model IDs must still
// match the submitted snapshot. Editing a connection cannot redirect recovery.
func (a *App) agentRecoverySecretsLocked(g *AgentGroup, root Task) (map[string]string, error) {
	needed := []*TaskModelConfig{root.Config.Model}
	for _, m := range g.Members {
		if m.Config == nil {
			return nil, errors.New("历史成员缺少配置快照，请新建协作")
		}
		needed = append(needed, m.Config.Model)
	}
	if len(g.Members) == 0 {
		for i := range g.ModelCatalog {
			needed = append(needed, &g.ModelCatalog[i])
		}
	}
	secrets := map[string]string{}
	for _, snap := range needed {
		if snap == nil {
			return nil, errors.New("缺少模型快照")
		}
		found := false
		for _, model := range a.state.Models {
			if model.ID != snap.ID {
				continue
			}
			if model.BaseURL != snap.BaseURL || model.Family != snap.Family || model.ModelID != snap.ModelID {
				return nil, fmt.Errorf("模型连接 %s 已变更，请恢复原连接或新建协作", snap.Name)
			}
			secrets[snap.ID], found = model.Secret, true
		}
		if !found {
			return nil, fmt.Errorf("模型连接 %s 已删除，请重新配置", snap.Name)
		}
	}
	return secrets, nil
}

func (a *App) agentOriginalInputsLocked(g *AgentGroup, root Task) []string {
	if g.OriginalInputIDs != nil {
		return append([]string{}, g.OriginalInputIDs...)
	}
	// Compatibility for 0.16 groups whose root inputs may already contain output
	// candidates from aggregation. Those outputs are never editable input scope.
	ids := []string{}
	for _, aid := range root.InputIDs {
		for _, asset := range a.state.Assets {
			if asset.ID == aid {
				task := a.taskLocked(asset.TaskID)
				if task == nil || task.AgentGroupID != g.ID {
					ids = append(ids, aid)
				}
			}
		}
	}
	return ids
}

func (a *App) resumeAgentsLocked(g *AgentGroup) error {
	if a.closing {
		return errors.New("应用正在关闭")
	}
	if g.Status != "paused" && g.Status != "interrupted" {
		return errors.New("仅暂停或中断的协作可以继续")
	}
	if a.agentSettlingLocked(g) {
		return errors.New("上一轮请求正在收尾，请稍后继续")
	}
	root := a.taskLocked(g.RootTaskID)
	if root == nil || root.Config == nil || root.ProjectID != g.ProjectID || g.SchemaVersion < 2 {
		return errors.New("历史协作缺少恢复快照，请新建协作")
	}
	secrets, err := a.agentRecoverySecretsLocked(g, *root)
	if err != nil {
		return err
	}
	original := a.agentOriginalInputsLocked(g, *root)
	if _, err := a.agentInputsLocked(g.ProjectID, original); err != nil {
		return err
	}
	for _, m := range g.Members {
		if _, err := a.agentInputsLocked(g.ProjectID, m.InputIDs); err != nil {
			return err
		}
		if len(m.TaskIDs) > 0 && m.Status == "succeeded" {
			t := a.taskLocked(m.TaskIDs[len(m.TaskIDs)-1])
			if t == nil || t.Status != "succeeded" || t.ProjectID != g.ProjectID {
				return errors.New("已完成成员的任务记录缺失，无法安全恢复")
			}
			if _, err := a.agentInputsLocked(g.ProjectID, t.OutputIDs); err != nil {
				return err
			}
		}
	}
	// A crash after final completion but before group completion needs no replay.
	if g.InterruptedStage == "aggregating" && root.Status == "succeeded" {
		g.Status, g.Error, g.FinishedAt = "succeeded", "", now()
		return a.saveLocked()
	}
	g.OriginalInputIDs = original
	root.InputIDs, root.Prompt = append([]string{}, original...), g.Prompt
	root.Status, root.Error, root.UpdatedAt = "waiting", "", now()
	root.Config.AgentCandidateIDs = nil
	g.Status, g.Error, g.FinishedAt = "running", "", ""
	g.ResumeCount++
	if len(g.Members) == 0 {
		g.Status = "planning"
		if t := a.taskLocked(g.PlannerTaskID); t == nil || t.Status != "succeeded" {
			g.PlannerTaskID = ""
		}
	}
	for i := range g.Members {
		m := &g.Members[i]
		if m.Status == "pending" {
			if len(m.Pending) == 0 {
				m.Pending = []string{m.Prompt}
			}
			continue
		}
		if len(m.TaskIDs) > 0 {
			t := a.taskLocked(m.TaskIDs[len(m.TaskIDs)-1])
			// Leave successful, unprocessed review results for the coordinator to
			// apply exactly once. Do not bypass a rework/blocked decision on restart.
			if t != nil && t.Status == "succeeded" {
				m.Status = "succeeded"
				if len(m.Pending) > 0 {
					m.Status = "pending"
				}
				continue
			}
		}
		if m.Status == "succeeded" || m.Status == "blocked" || m.Status == "failed" {
			continue
		}
		prompt := m.CurrentPrompt
		if prompt == "" {
			prompt = m.Prompt
		}
		if len(m.TaskIDs) > 0 {
			m.Pending = append([]string{prompt}, m.Pending...)
		} else if len(m.Pending) == 0 {
			m.Pending = []string{prompt}
		}
		if len(m.TaskIDs) > 0 {
			m.ProcessedTaskID = m.TaskIDs[len(m.TaskIDs)-1]
		}
		m.Status, m.Error = "pending", ""
	}
	if err := a.saveLocked(); err != nil {
		return err
	}
	a.startAgentRunLocked(g.ID, g.RootTaskID, secrets)
	return nil
}

func (a *App) resumeAgents(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	g := a.groupLocked(r.PathValue("id"))
	if g == nil {
		problem(w, 404, errors.New("协作不存在"))
		return
	}
	if err := a.resumeAgentsLocked(g); err != nil {
		problem(w, 409, err)
		return
	}
	respond(w, 202, g)
}

func (a *App) resumeOptedInAgents() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.state.AgentGroups {
		g := &a.state.AgentGroups[i]
		if g.Status != "interrupted" || !g.AutoResume {
			continue
		}
		if err := a.resumeAgentsLocked(g); err != nil {
			g.Error = "自动恢复未启动：" + err.Error()
			_ = a.saveLocked()
		}
	}
}

func (a *App) agentRecoveryOption(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AutoResume bool `json:"autoResume"`
	}
	if !decode(w, r, &in) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	g := a.groupLocked(r.PathValue("id"))
	if g == nil {
		problem(w, 404, errors.New("协作不存在"))
		return
	}
	g.AutoResume = in.AutoResume
	if err := a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 200, map[string]bool{"autoResume": in.AutoResume})
}

func (a *App) reviseAgentPlan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ExpectedVersion int             `json:"expectedVersion"`
		Members         []AgentNodeSpec `json:"members"`
	}
	if !decode(w, r, &in) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	g := a.groupLocked(r.PathValue("id"))
	if g == nil {
		problem(w, 404, errors.New("协作不存在"))
		return
	}
	if (g.Status != "paused" && g.Status != "interrupted") || a.agentSettlingLocked(g) {
		problem(w, 409, errors.New("请先暂停协作并等待请求收尾，再调整计划"))
		return
	}
	if in.ExpectedVersion != g.PlanVersion {
		problem(w, 409, errors.New("计划版本已更新，请刷新后重新编辑"))
		return
	}
	if len(g.PlanHistory) >= 20 {
		problem(w, 400, errors.New("计划调整已达 20 次，请新建协作"))
		return
	}
	root := a.taskLocked(g.RootTaskID)
	if root == nil || root.Config == nil || g.SchemaVersion < 2 {
		problem(w, 400, errors.New("历史协作缺少完整快照，请新建协作"))
		return
	}
	copyRoot := *root
	copyRoot.InputIDs = a.agentOriginalInputsLocked(g, copyRoot)
	requireReview := false
	for _, m := range g.Members {
		if m.Role == "reviewer" {
			requireReview = true
		}
	}
	// Round-trip to isolate validator normalisation from persisted snapshots.
	raw, _ := json.Marshal(in.Members)
	var specs []AgentNodeSpec
	_ = json.Unmarshal(raw, &specs)
	members, specs, err := buildAgentMembers(copyRoot, specs, g.ModelCatalog, requireReview)
	if err != nil {
		problem(w, 400, err)
		return
	}
	old := map[string]AgentMember{}
	oldSpec := map[string]AgentNodeSpec{}
	for _, m := range g.Members {
		old[m.ID] = m
	}
	for _, s := range g.Plan.Members {
		oldSpec[s.ID] = s
	}
	affected := map[string]bool{}
	for _, s := range specs {
		if contentHash(oldSpec[s.ID]) != contentHash(s) {
			affected[s.ID] = true
		}
	}
	// Propagate along BOTH graphs: removing a dependency also invalidates the old
	// downstream result. Independent completed branches remain valid.
	for _, s := range g.Plan.Members {
		found := false
		for _, n := range specs {
			if n.ID == s.ID {
				found = true
			}
		}
		if !found {
			affected[s.ID] = true
		}
	}
	for pass := 0; pass < 6; pass++ {
		for _, s := range append(append([]AgentNodeSpec{}, g.Plan.Members...), specs...) {
			for _, d := range s.DependsOn {
				if affected[d] {
					affected[s.ID] = true
				}
			}
		}
	}
	for i := range members {
		m := &members[i]
		if prior, ok := old[m.ID]; ok {
			if !affected[m.ID] {
				*m = prior
				continue
			}
			m.TaskIDs, m.Retries = prior.TaskIDs, prior.Retries
			if len(m.TaskIDs) > 0 {
				m.ProcessedTaskID = m.TaskIDs[len(m.TaskIDs)-1]
			}
			m.Instructions = append(prior.Instructions, "计划调整："+m.Prompt)
		}
	}
	g.PlanHistory = append(g.PlanHistory, AgentPlanRevision{Version: g.PlanVersion, At: now(), Plan: g.Plan})
	g.PlanVersion++
	g.Plan = AgentPlan{Summary: "已调整分工；受影响分支将在继续后重跑，历史文件保留。", Members: specs}
	g.Members, g.Error = members, ""
	g.OriginalInputIDs = copyRoot.InputIDs
	if err := a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 200, g)
}
