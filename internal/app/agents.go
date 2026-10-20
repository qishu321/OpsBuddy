package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type AgentMember struct {
	ID              string      `json:"id"`
	Role            string      `json:"role"`
	Name            string      `json:"name"`
	Prompt          string      `json:"prompt"`
	DependsOn       []string    `json:"dependsOn,omitempty"`
	InputIDs        []string    `json:"inputIds,omitempty"`
	Acceptance      string      `json:"acceptance,omitempty"`
	Status          string      `json:"status"`
	Error           string      `json:"error,omitempty"`
	TaskIDs         []string    `json:"taskIds"`
	Pending         []string    `json:"pending"`
	Instructions    []string    `json:"instructions"`
	Config          *TaskConfig `json:"configSnapshot"`
	ProcessedTaskID string      `json:"processedTaskId,omitempty"`
	Retries         int         `json:"retries"`
	CurrentPrompt   string      `json:"currentPrompt,omitempty"`
}
type AgentGroup struct {
	ID               string              `json:"id"`
	ProjectID        string              `json:"projectId"`
	RootTaskID       string              `json:"rootTaskId"`
	SourceTaskID     string              `json:"sourceTaskId,omitempty"`
	Prompt           string              `json:"prompt"`
	Status           string              `json:"status"`
	Members          []AgentMember       `json:"members"`
	CreatedAt        string              `json:"createdAt"`
	FinishedAt       string              `json:"finishedAt,omitempty"`
	Error            string              `json:"error,omitempty"`
	SchemaVersion    int                 `json:"schemaVersion,omitempty"`
	PlanVersion      int                 `json:"planVersion,omitempty"`
	Plan             AgentPlan           `json:"plan"`
	PlannerTaskID    string              `json:"plannerTaskId,omitempty"`
	ModelCatalog     []TaskModelConfig   `json:"modelCatalog,omitempty"`
	Limits           AgentLimits         `json:"limits"`
	ReworkRound      int                 `json:"reworkRound"`
	Reviews          []AgentReview       `json:"reviews"`
	OriginalInputIDs []string            `json:"originalInputIds,omitempty"`
	AutoResume       bool                `json:"autoResume"`
	ResumeCount      int                 `json:"resumeCount"`
	InterruptedStage string              `json:"interruptedStage,omitempty"`
	PlanHistory      []AgentPlanRevision `json:"planHistory,omitempty"`
	Settling         bool                `json:"settling,omitempty"` // public runtime state; never a recovery checkpoint
}

var agentInstructions = map[string]string{
	"planner":    "分析当前目标、输入与约束，给出可验证的工作方案；不得委派或访问其他会话。",
	"explorer":   "只探查本次提供的文件和知识片段，梳理事实与缺失项。没有访问未引用目录或互联网的能力。",
	"researcher": "依据提供的资料研究任务，标注依据、假设与待核实项。没有联网搜索工具，不得编造来源。",
	"worker":     "根据自己的任务要求和明确提供的材料制作交付内容，保留关键事实和数字。",
	"reviewer":   "按验收条件核对已提供的上游成果，指出具体责任节点与问题，不得声称完成未做的验证。",
}

func activeTask(status string) bool {
	return status == "queued" || status == "running" || status == "waiting"
}
func activeAgentGroup(status string) bool {
	return status == "planning" || status == "running" || status == "aggregating"
}
func (a *App) groupLocked(groupID string) *AgentGroup {
	for i := range a.state.AgentGroups {
		if a.state.AgentGroups[i].ID == groupID {
			return &a.state.AgentGroups[i]
		}
	}
	return nil
}
func (a *App) taskLocked(taskID string) *Task {
	for i := range a.state.Tasks {
		if a.state.Tasks[i].ID == taskID {
			return &a.state.Tasks[i]
		}
	}
	return nil
}
func cloneConfig(config *TaskConfig) *TaskConfig {
	raw, _ := json.Marshal(config)
	var copy TaskConfig
	_ = json.Unmarshal(raw, &copy)
	return &copy
}

func (a *App) dispatchAgents(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TaskRequest
		SourceTaskID string          `json:"sourceTaskId"`
		AutoPlan     bool            `json:"autoPlan"`
		AutoResume   bool            `json:"autoResume"`
		Members      []AgentNodeSpec `json:"members"`
	}
	if !decode(w, r, &in) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if in.ParentID != "" {
		problem(w, 400, errors.New("协作请通过主会话入口创建，不能嵌套委派"))
		return
	}
	if in.SourceTaskID != "" {
		source := a.taskLocked(in.SourceTaskID)
		if source == nil || source.ProjectID != in.ProjectID || activeTask(source.Status) || source.AgentMemberID != "" {
			problem(w, 400, errors.New("请选择当前项目中已结束的主会话；子 Agent 不可再次委派"))
			return
		}
		if len(in.InputIDs) == 0 {
			in.InputIDs = append([]string{}, source.OutputIDs...)
		}
	}
	root, _, rootSecret, err := a.prepareTaskLocked(in.TaskRequest)
	if err != nil {
		problem(w, 400, err)
		return
	}
	if root.Config.Model == nil {
		problem(w, 400, errors.New("协作需要选择已配置模型，不使用固定本地模板模拟 Agent"))
		return
	}
	if root.Kind == "pdf" || root.Kind == "organize" {
		problem(w, 400, errors.New("协作汇总请选择会话、Markdown、代码或办公成果格式"))
		return
	}
	root.Config.ConversationContext = ""
	group := AgentGroup{ID: id(), ProjectID: in.ProjectID, RootTaskID: root.ID, SourceTaskID: in.SourceTaskID, Prompt: root.Prompt, Status: "running", CreatedAt: now(), SchemaVersion: 2, PlanVersion: 1, Members: []AgentMember{}, Reviews: []AgentReview{}, Limits: AgentLimits{6, 3, 1, 1, 2}}
	group.OriginalInputIDs = append([]string{}, root.InputIDs...)
	group.AutoResume = in.AutoResume
	secrets := map[string]string{root.ModelID: rootSecret}
	for _, model := range a.state.Models {
		if model.Family != "openai" && model.Family != "anthropic" && model.Family != "gemini" {
			continue
		}
		if model.BaseURL == "" || model.ModelID == "" {
			continue
		}
		config := TaskModelConfig{ID: model.ID, Name: model.Name, Family: model.Family, BaseURL: model.BaseURL, ModelID: model.ModelID}
		if model.Secret != "" {
			config.CredentialRef = model.ID
		}
		group.ModelCatalog = append(group.ModelCatalog, config)
		secrets[model.ID] = model.Secret
	}
	if !in.AutoPlan {
		group.Members, group.Plan.Members, err = buildAgentMembers(root, in.Members, group.ModelCatalog, false)
		if err != nil {
			problem(w, 400, err)
			return
		}
		group.Plan.Summary = "用户编辑的执行计划"
	} else {
		group.Status = "planning"
	}
	root.AgentGroupID, root.Status, root.Title = group.ID, "waiting", "协作 · "+root.Title
	if source := a.taskLocked(in.SourceTaskID); source != nil {
		root.ParentID, root.ConversationID = source.ID, source.ConversationID
		if root.ConversationID == "" {
			root.ConversationID = source.ID
		}
	}
	root.Steps = append(root.Steps, Step{Name: "协作计划", Status: "running", Detail: "校验依赖、独立模型与输入快照；仅上游成功后派发", At: now()})
	a.state.Tasks = append(a.state.Tasks, root)
	a.state.AgentGroups = append(a.state.AgentGroups, group)
	if err = a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	a.startAgentRunLocked(group.ID, root.ID, secrets)
	respond(w, 201, group)
}

func (a *App) instructAgent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Prompt string `json:"prompt"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.Prompt == "" || len(in.Prompt) > 12000 {
		problem(w, 400, errors.New("请输入追加要求，最多 12000 字节"))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	group := a.groupLocked(r.PathValue("id"))
	if group == nil {
		problem(w, 404, errors.New("协作任务不存在"))
		return
	}
	if group.Status != "running" {
		problem(w, 409, errors.New("本组尚未完成规划或已进入汇总/结束，请在主会话继续提出要求"))
		return
	}
	for i := range group.Members {
		member := &group.Members[i]
		if member.ID != r.PathValue("member") {
			continue
		}
		if len(member.Instructions) >= 8 {
			problem(w, 400, errors.New("每个子任务最多追加 7 轮要求"))
			return
		}
		member.Pending = append(member.Pending, in.Prompt)
		member.Instructions = append(member.Instructions, in.Prompt)
		member.Error = ""
		if !activeTask(member.Status) {
			member.Status = "pending"
		}
		a.invalidateAgentNodesLocked(group, map[string]string{member.ID: in.Prompt}, false)
		if err := a.saveLocked(); err != nil {
			problem(w, 500, err)
			return
		}
		respond(w, 202, map[string]bool{"queued": true})
		return
	}
	problem(w, 404, errors.New("子 Agent 不存在"))
}

// Invalidation never deletes tasks or assets. Each rerun is a new isolated task.
func (a *App) invalidateAgentNodesLocked(group *AgentGroup, reasons map[string]string, includeTargets bool) {
	affected := map[string]bool{}
	for target := range reasons {
		affected[target] = true
	}
	for pass := 0; pass < len(group.Members); pass++ {
		for _, member := range group.Members {
			for _, parent := range member.DependsOn {
				if affected[parent] {
					affected[member.ID] = true
				}
			}
		}
	}
	for i := range group.Members {
		m := &group.Members[i]
		if !affected[m.ID] {
			continue
		}
		_, target := reasons[m.ID]
		if target && !includeTargets {
			continue
		}
		if len(m.TaskIDs) > 0 {
			latest := m.TaskIDs[len(m.TaskIDs)-1]
			if task := a.taskLocked(latest); task != nil && activeTask(task.Status) {
				if cancel := a.cancels[latest]; cancel != nil {
					cancel()
				}
				task.Status, task.Error = "cancelled", "上游要求变更，本轮失效；文件保留，等待新一轮依赖"
				task.UpdatedAt = now()
			}
			m.ProcessedTaskID = latest
		}
		prompt := m.Prompt
		if len(m.Instructions) > 1 {
			prompt += "\n用户追加要求：\n" + strings.Join(m.Instructions[1:], "\n")
		}
		if reason := reasons[m.ID]; reason != "" {
			prompt += "\n审阅返工要求：" + reason
		}
		m.Pending, m.Status, m.Error = []string{prompt}, "pending", ""
	}
}

func (a *App) stopAgents(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	group := a.groupLocked(r.PathValue("id"))
	if group == nil {
		problem(w, 404, errors.New("协作任务不存在"))
		return
	}
	if !activeAgentGroup(group.Status) && group.Status != "paused" && group.Status != "interrupted" {
		problem(w, 409, errors.New("协作任务已结束"))
		return
	}
	if cancel := a.cancels[group.ID]; cancel != nil {
		cancel()
	}
	group.Status = "running" // explicit stop also closes paused recovery checkpoints
	a.finishAgentGroupLocked(group, "cancelled", "协作已停止，已生成文件保留。")
	if err := a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

func (a *App) finishAgentGroupLocked(group *AgentGroup, status, message string) {
	if group.Status == "paused" || group.Status == "interrupted" {
		return // A cancelled old request must not erase a resumable checkpoint.
	}
	if group.Status == "cancelled" {
		status = "cancelled"
	}
	group.Status, group.Error, group.FinishedAt = status, message, now()
	for i := range a.state.Tasks {
		task := &a.state.Tasks[i]
		if task.AgentGroupID == group.ID && activeTask(task.Status) {
			if cancel := a.cancels[task.ID]; cancel != nil {
				cancel()
			}
			task.Status, task.Error, task.UpdatedAt = status, message, now()
		}
	}
	for i := range group.Members {
		m := &group.Members[i]
		if m.Status == "pending" || activeTask(m.Status) {
			m.Status = status
			m.Error = message
		}
		m.Pending = nil
	}
}
func (a *App) finishAgentGroup(groupID, status, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if group := a.groupLocked(groupID); group != nil {
		a.finishAgentGroupLocked(group, status, message)
		_ = a.saveLocked()
	}
}

func (a *App) agentInputsLocked(projectID string, ids []string) ([]Asset, error) {
	ids = uniqueIDs(ids)
	if len(ids) > 16 {
		return nil, errors.New("节点引用文件超过 16 个，请拆分协作")
	}
	inputs := []Asset{}
	for _, assetID := range ids {
		found := false
		for _, v := range a.state.Assets {
			if v.ID == assetID && v.ProjectID == projectID {
				inputs = append(inputs, v)
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("节点材料不存在或跨越项目边界")
		}
	}
	return inputs, nil
}

func (a *App) newAgentTask(group *AgentGroup, member *AgentMember) (Task, []Asset, error) {
	ids := append([]string{}, member.InputIDs...)
	prompt := "主任务目标：" + group.Prompt + "\n当前职责：" + member.Pending[0] + "\n验收条件：" + member.Acceptance
	if len(member.TaskIDs) > 0 {
		if previous := a.taskLocked(member.TaskIDs[len(member.TaskIDs)-1]); previous != nil && previous.Status == "succeeded" {
			ids = append(ids, previous.OutputIDs...)
			prompt += "\n本成员上一轮结果（参考材料）：" + previous.Result
		}
	}
	deps := map[string][]string{}
	for _, other := range group.Members {
		deps[other.ID] = other.DependsOn
	}
	selected := map[string]bool{}
	for _, parent := range member.DependsOn {
		selected[parent] = true
	}
	if member.Role == "reviewer" {
		selected = agentAncestors(member.ID, deps)
	}
	for _, other := range group.Members {
		if !selected[other.ID] {
			continue
		}
		if len(other.TaskIDs) == 0 || other.Status != "succeeded" || len(other.Pending) > 0 {
			return Task{}, nil, errors.New("依赖尚未实际成功")
		}
		t := a.taskLocked(other.TaskIDs[len(other.TaskIDs)-1])
		if t == nil || t.ProjectID != group.ProjectID || t.Status != "succeeded" {
			return Task{}, nil, errors.New("依赖任务不存在或未完成")
		}
		ids = append(ids, t.OutputIDs...)
		prompt += fmt.Sprintf("\n依赖交付（参考数据）：节点 %s / %s；任务 %s\n验收：%s\n结果：%s\n", other.ID, other.Name, t.ID, other.Acceptance, t.Result)
	}
	if len(prompt) > 60000 {
		return Task{}, nil, errors.New("节点上下文超过 60000 字节，请减少材料")
	}
	ids = uniqueIDs(ids)
	inputs, err := a.agentInputsLocked(group.ProjectID, ids)
	if err != nil {
		return Task{}, nil, err
	}
	task := Task{ID: id(), ProjectID: group.ProjectID, AgentGroupID: group.ID, AgentMemberID: member.ID, Title: member.Name, Prompt: prompt, Kind: member.Config.OutputKind, RoleID: member.Config.Role.ID, Status: "queued", InputIDs: ids, OutputIDs: []string{}, Steps: []Step{}, Config: cloneConfig(member.Config), CreatedAt: now(), UpdatedAt: now()}
	task.ModelID, task.ModelName = task.Config.Model.ID, task.Config.Model.Name
	task.ConversationID = task.ID
	if len(member.TaskIDs) > 0 {
		task.ConversationID = member.TaskIDs[0]
	}
	return task, inputs, nil
}
