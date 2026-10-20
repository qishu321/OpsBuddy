package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type WorkflowNode struct {
	JoinMode string   `json:"joinMode,omitempty"`
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Label    string   `json:"label"`
	X        float64  `json:"x"`
	Y        float64  `json:"y"`
	Prompt   string   `json:"prompt,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	RoleID   string   `json:"roleId,omitempty"`
	SkillIDs []string `json:"skillIds,omitempty"`
	ModelID  string   `json:"modelId,omitempty"`
	BaseIDs  []string `json:"baseIds,omitempty"`
	Query    string   `json:"query,omitempty"`
	Operator string   `json:"operator,omitempty"`
	Value    string   `json:"value,omitempty"`
	Field    string   `json:"field,omitempty"`
}
type WorkflowEdge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"sourceHandle,omitempty"`
}
type WorkflowDefinition struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"projectId"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     int            `json:"version"`
	Enabled     bool           `json:"enabled"`
	Nodes       []WorkflowNode `json:"nodes"`
	Edges       []WorkflowEdge `json:"edges"`
	UpdatedAt   string         `json:"updatedAt"`
}
type WorkflowNodeRun struct {
	ID            string         `json:"id"`
	NodeID        string         `json:"nodeId"`
	Label         string         `json:"label"`
	Status        string         `json:"status"`
	TaskID        string         `json:"taskId,omitempty"`
	Text          string         `json:"text,omitempty"`
	AssetIDs      []string       `json:"assetIds"`
	KnowledgeRefs []KnowledgeHit `json:"knowledgeRefs,omitempty"`
	Branch        string         `json:"branch,omitempty"`
	Error         string         `json:"error,omitempty"`
	StartedAt     string         `json:"startedAt,omitempty"`
	FinishedAt    string         `json:"finishedAt,omitempty"`
}
type WorkflowRun struct {
	ID                 string                    `json:"id"`
	WorkflowID         string                    `json:"workflowId"`
	ProjectID          string                    `json:"projectId"`
	Status             string                    `json:"status"`
	Prompt             string                    `json:"prompt"`
	InputIDs           []string                  `json:"inputIds"`
	AllowRemote        bool                      `json:"allowRemote"`
	Definition         WorkflowDefinition        `json:"definition"`
	Configs            map[string]*TaskConfig    `json:"configs"`
	KnowledgeSnapshots map[string][]KnowledgeHit `json:"knowledgeSnapshots"`
	Nodes              []WorkflowNodeRun         `json:"nodes"`
	OutputIDs          []string                  `json:"outputIds"`
	Error              string                    `json:"error,omitempty"`
	CreatedAt          string                    `json:"createdAt"`
	FinishedAt         string                    `json:"finishedAt,omitempty"`
}

func validateWorkflow(f WorkflowDefinition) error {
	if strings.TrimSpace(f.Name) == "" || len(f.Name) > 160 {
		return errors.New("请填写工作流名称")
	}
	if len(f.Nodes) < 2 || len(f.Nodes) > 40 || len(f.Edges) > 120 {
		return errors.New("流程需包含 2–40 个节点，最多 120 条连线")
	}
	nodes := map[string]WorkflowNode{}
	incoming := map[string][]WorkflowEdge{}
	outgoing := map[string][]WorkflowEdge{}
	start, end := "", ""
	for _, n := range f.Nodes {
		if n.ID == "" || len(n.ID) > 80 {
			return errors.New("节点 ID 无效")
		}
		if _, ok := nodes[n.ID]; ok {
			return errors.New("节点 ID 重复")
		}
		nodes[n.ID] = n
		switch n.Type {
		case "start":
			if start != "" {
				return errors.New("只允许一个开始节点")
			}
			start = n.ID
		case "end":
			if end != "" {
				return errors.New("只允许一个结束节点")
			}
			end = n.ID
		case "agent", "tool":
			if !(n.Type == "agent" && (n.Kind == "chat" || n.Kind == "md" || n.Kind == "html" || n.Kind == "code")) && n.Kind != "docx" && n.Kind != "pptx" && n.Kind != "xlsx" && n.Kind != "pdf" && n.Kind != "organize" {
				return errors.New("办公节点必须选择支持的交付格式")
			}
			if len(n.Prompt) > 20000 {
				return errors.New("节点提示词过长")
			}
		case "knowledge":
			if len(n.BaseIDs) == 0 {
				return errors.New("知识检索节点需选择知识库")
			}
		case "condition":
			if n.Operator != "contains" && n.Operator != "not_contains" && n.Operator != "has_files" {
				return errors.New("条件节点请选择包含、不包含或有文件")
			}
			if n.Operator != "has_files" && n.Value == "" {
				return errors.New("条件节点需填写匹配内容")
			}
			if n.Field != "input" && n.Field != "context" {
				return errors.New("条件节点请选择输入或上游内容")
			}
		case "merge":
			if n.JoinMode != "" && n.JoinMode != "all_success" && n.JoinMode != "allow_partial" {
				return errors.New("不支持的汇合策略")
			}
		default:
			return errors.New("未知节点类型")
		}
	}
	if start == "" || end == "" {
		return errors.New("流程必须有开始与结束节点")
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	for _, e := range f.Edges {
		source, sok := nodes[e.Source]
		_, tok := nodes[e.Target]
		if !sok || !tok || e.Source == e.Target {
			return errors.New("连线包含不存在节点或自连接")
		}
		if e.ID == "" || ids[e.ID] {
			return errors.New("连线 ID 缺失或重复")
		}
		ids[e.ID] = true
		key := e.Source + "/" + e.Target + "/" + e.SourceHandle
		if seen[key] {
			return errors.New("同一出口和目标不能重复连线")
		}
		seen[key] = true
		if source.Type == "condition" && e.SourceHandle != "true" && e.SourceHandle != "false" {
			return errors.New("条件节点必须从 TRUE/FALSE 出口连接")
		}
		if source.Type != "condition" && e.SourceHandle != "" && e.SourceHandle != "out" {
			return errors.New("连线引用了不存在的节点出口")
		}
		incoming[e.Target] = append(incoming[e.Target], e)
		outgoing[e.Source] = append(outgoing[e.Source], e)
	}
	if len(incoming[start]) > 0 || len(outgoing[end]) > 0 {
		return errors.New("开始不能有输入连线，结束不能有输出连线")
	}
	dependencies := map[string][]string{}
	for node := range nodes {
		dependencies[node] = []string{}
		for _, e := range incoming[node] {
			dependencies[node] = append(dependencies[node], e.Source)
		}
		dependencies[node] = uniqueIDs(dependencies[node])
	}
	if err := validateDAG(dependencies); err != nil {
		return err
	}
	var walk func(string, map[string][]WorkflowEdge, bool, map[string]bool)
	walk = func(n string, edges map[string][]WorkflowEdge, reverse bool, visited map[string]bool) {
		if visited[n] {
			return
		}
		visited[n] = true
		for _, e := range edges[n] {
			next := e.Target
			if reverse {
				next = e.Source
			}
			walk(next, edges, reverse, visited)
		}
	}
	reachable := map[string]bool{}
	walk(start, outgoing, false, reachable)
	toEnd := map[string]bool{}
	walk(end, incoming, true, toEnd)
	for n := range nodes {
		if !reachable[n] || !toEnd[n] {
			return errors.New("所有节点都必须连接开始并能到达结束，请补齐连线")
		}
	}
	return nil
}
func (a *App) saveWorkflow(w http.ResponseWriter, r *http.Request) {
	var f WorkflowDefinition
	if !decode(w, r, &f) {
		return
	}
	if err := validateWorkflow(f); err != nil {
		problem(w, 400, err)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	found := false
	for _, p := range a.state.Projects {
		if p.ID == f.ProjectID {
			found = true
		}
	}
	if !found {
		problem(w, 400, errors.New("项目不存在"))
		return
	}
	index := -1
	for i, old := range a.state.Workflows {
		if old.ID == f.ID {
			if old.ProjectID != f.ProjectID {
				problem(w, 400, errors.New("不能跨项目修改流程"))
				return
			}
			if f.Version != old.Version {
				problem(w, 409, errors.New("流程已被其他编辑修改，请重新打开"))
				return
			}
			index = i
			f.Version = old.Version + 1
		}
	}
	if index < 0 {
		f.ID = id()
		f.Version = 1
	}
	f.UpdatedAt = now()
	if index < 0 {
		a.state.Workflows = append(a.state.Workflows, f)
	} else {
		a.state.Workflows[index] = f
	}
	if err := a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 200, f)
}
func (a *App) startWorkflow(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Prompt      string   `json:"prompt"`
		InputIDs    []string `json:"inputIds"`
		AllowRemote bool     `json:"allowRemote"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > 20000 || len(in.InputIDs) > 16 {
		problem(w, 400, errors.New("请输入任务要求（最多 20000 字节），附件最多 16 个"))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		problem(w, 503, errors.New("应用正在关闭"))
		return
	}
	var f WorkflowDefinition
	for _, v := range a.state.Workflows {
		if v.ID == r.PathValue("id") {
			f = v
		}
	}
	if f.ID == "" || !f.Enabled {
		problem(w, 400, errors.New("流程不存在或未启用"))
		return
	}
	if err := validateWorkflow(f); err != nil {
		problem(w, 400, err)
		return
	}
	for _, assetID := range in.InputIDs {
		found := false
		for _, v := range a.state.Assets {
			if v.ID == assetID && v.ProjectID == f.ProjectID {
				found = true
			}
		}
		if !found {
			problem(w, 400, errors.New("附件不存在或不属于流程项目"))
			return
		}
	}
	// Clone the definition so later canvas edits never alter this run.
	raw, _ := json.Marshal(f)
	_ = json.Unmarshal(raw, &f)
	run := WorkflowRun{ID: id(), WorkflowID: f.ID, ProjectID: f.ProjectID, Status: "queued", Prompt: in.Prompt, InputIDs: in.InputIDs, AllowRemote: in.AllowRemote, Definition: f, Configs: map[string]*TaskConfig{}, KnowledgeSnapshots: map[string][]KnowledgeHit{}, Nodes: []WorkflowNodeRun{}, OutputIDs: []string{}, CreatedAt: now()}
	secrets := map[string]string{}
	for _, n := range f.Nodes {
		run.Nodes = append(run.Nodes, WorkflowNodeRun{ID: id(), NodeID: n.ID, Label: n.Label, Status: "pending", AssetIDs: []string{}})
		if n.Type == "knowledge" {
			query := strings.ReplaceAll(n.Query, "{{input}}", in.Prompt)
			if query == "" {
				query = in.Prompt
			}
			hits, err := a.searchKnowledgeLocked(f.ProjectID, n.BaseIDs, query, 6)
			if err != nil {
				problem(w, 400, err)
				return
			}
			run.KnowledgeSnapshots[n.ID] = hits
		}
		if n.Type != "agent" && n.Type != "tool" {
			continue
		}
		role := Role{}
		roleID := n.RoleID
		if roleID == "" {
			roleID = "auto"
		}
		for _, v := range a.state.Roles {
			if v.ID == roleID {
				role = v
			}
		}
		if role.ID == "" {
			problem(w, 400, errors.New("流程引用的角色已不存在"))
			return
		}
		var model Model
		if n.Type == "agent" && n.ModelID != "" {
			for _, v := range a.state.Models {
				if v.ID == n.ModelID {
					model = v
				}
			}
			if model.ID == "" {
				problem(w, 400, errors.New("流程引用的模型已不存在"))
				return
			}
			if !in.AllowRemote {
				problem(w, 400, errors.New("请确认允许流程内容发送到节点配置的模型"))
				return
			}
		}
		if n.Type == "agent" && (n.Kind == "chat" || n.Kind == "md" || n.Kind == "html" || n.Kind == "code") && model.ID == "" {
			problem(w, 400, errors.New("文字输出 Agent 需要选择模型"))
			return
		}
		role = migratedRoleGuidance(role)
		role.SkillIDs = uniqueIDs(append(append([]string{}, role.SkillIDs...), n.SkillIDs...))
		if model.ID != "" && len(role.SkillIDs) == 0 {
			role.SkillIDs = builtinSkillsForTask(a.state.Extensions, n.Kind, in.Prompt+"\n"+n.Prompt)
		}
		config, err := captureTaskConfig(role, model, a.state.Extensions, n.Kind, in.AllowRemote)
		if err != nil {
			problem(w, 400, err)
			return
		}
		for _, project := range a.state.Projects {
			if project.ID == f.ProjectID {
				config.ProjectInstructions = project.Description
				break
			}
		}
		bases := uniqueIDs(append(append([]string{}, config.Role.KnowledgeIDs...), n.BaseIDs...))
		if len(bases) > 16 {
			problem(w, 400, errors.New("节点引用知识库过多"))
			return
		}
		hits, err := a.searchKnowledgeLocked(f.ProjectID, bases, in.Prompt+"\n"+n.Prompt, 6)
		if err != nil {
			problem(w, 400, err)
			return
		}
		config.KnowledgeIDs = bases
		config.KnowledgeRefs = hits
		config.MemoryRefs = a.memoryRefsLocked(f.ProjectID, in.Prompt+"\n"+n.Prompt, n.Kind, nil)
		run.Configs[n.ID] = config
		secrets[n.ID] = model.Secret
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	a.cancels[run.ID] = cancel
	a.state.WorkflowRuns = append(a.state.WorkflowRuns, run)
	if err := a.saveLocked(); err != nil {
		delete(a.cancels, run.ID)
		cancel()
		problem(w, 500, err)
		return
	}
	a.workers.Add(1)
	go func() { defer a.workers.Done(); defer cancel(); a.executeWorkflow(ctx, run, secrets) }()
	respond(w, 201, run)
}
func (a *App) updateWorkflowRun(runID string, fn func(*WorkflowRun)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.state.WorkflowRuns {
		if a.state.WorkflowRuns[i].ID == runID {
			fn(&a.state.WorkflowRuns[i])
			return a.saveLocked()
		}
	}
	return errors.New("运行记录不存在")
}
func (a *App) cancelWorkflow(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	a.mu.Lock()
	cancel, ok := a.cancels[runID]
	a.mu.Unlock()
	if !ok {
		problem(w, 409, errors.New("流程已结束"))
		return
	}
	cancel()
	if err := a.updateWorkflowRun(runID, func(run *WorkflowRun) { run.Status = "cancelled" }); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func uniqueIDs(ids []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range ids {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
func (a *App) executeWorkflow(ctx context.Context, run WorkflowRun, secrets map[string]string) {
	defer func() { a.mu.Lock(); delete(a.cancels, run.ID); a.mu.Unlock() }()
	if err := a.updateWorkflowRun(run.ID, func(r *WorkflowRun) {
		if r.Status == "queued" {
			r.Status = "running"
		}
	}); err != nil {
		return
	}
	states := map[string]WorkflowNodeRun{}
	nodes := map[string]WorkflowNode{}
	incoming := map[string][]WorkflowEdge{}
	for _, n := range run.Nodes {
		states[n.NodeID] = n
	}
	for _, n := range run.Definition.Nodes {
		nodes[n.ID] = n
	}
	for _, e := range run.Definition.Edges {
		incoming[e.Target] = append(incoming[e.Target], e)
	}
	done := make(chan WorkflowNodeRun, len(nodes))
	running := 0
	finished := 0
	persist := func(result WorkflowNodeRun) {
		states[result.NodeID] = result
		_ = a.updateWorkflowRun(run.ID, func(r *WorkflowRun) {
			for i := range r.Nodes {
				if r.Nodes[i].NodeID == result.NodeID {
					r.Nodes[i] = result
				}
			}
		})
	}
	for finished < len(nodes) {
		progress := false
		for _, node := range run.Definition.Nodes {
			state := states[node.ID]
			if state.Status != "pending" {
				continue
			}
			if ctx.Err() != nil {
				state.Status = "cancelled"
				state.FinishedAt = now()
				persist(state)
				finished++
				progress = true
				continue
			}
			ready := true
			active := []WorkflowNodeRun{}
			failed := false
			for _, e := range incoming[node.ID] {
				source := states[e.Source]
				if source.Status == "pending" || source.Status == "running" {
					ready = false
					break
				}
				if source.Status == "skipped" {
					continue
				}
				if nodes[e.Source].Type == "condition" && source.Branch != e.SourceHandle {
					continue
				}
				if source.Status == "failed" || source.Status == "cancelled" || source.Status == "interrupted" {
					failed = true
				}
				active = append(active, source)
			}
			if !ready {
				continue
			}
			if failed && node.Type == "merge" && node.JoinMode == "allow_partial" {
				valid := []WorkflowNodeRun{}
				for _, up := range active {
					if up.Status == "succeeded" {
						valid = append(valid, up)
					}
				}
				active = valid
				failed = len(active) == 0
			}
			if failed {
				state.Status = "failed"
				state.Error = "上游节点未成功，停止该分支"
				state.FinishedAt = now()
				persist(state)
				finished++
				progress = true
				continue
			}
			if node.Type != "start" && len(active) == 0 {
				state.Status = "skipped"
				state.FinishedAt = now()
				persist(state)
				finished++
				progress = true
				continue
			}
			state.Status = "running"
			state.StartedAt = now()
			persist(state)
			running++
			progress = true
			go func(n WorkflowNode, s WorkflowNodeRun, upstream []WorkflowNodeRun) {
				result := a.executeWorkflowNode(ctx, run, n, s, upstream, secrets[n.ID])
				done <- result
			}(node, state, active)
		}
		if running > 0 {
			result := <-done
			running--
			finished++
			persist(result)
		} else if !progress {
			_ = a.updateWorkflowRun(run.ID, func(r *WorkflowRun) { r.Status = "failed"; r.Error = "流程无法继续调度"; r.FinishedAt = now() })
			return
		}
	}
	outputs := []string{}
	errorsCount := 0
	for _, n := range run.Definition.Nodes {
		s := states[n.ID]
		if s.Status == "failed" {
			errorsCount++
		}
		if n.Type == "end" && s.Status == "succeeded" {
			outputs = append(outputs, s.AssetIDs...)
		}
	}
	status := "succeeded"
	message := ""
	if ctx.Err() != nil {
		status = "cancelled"
		message = "流程已停止，已生成文件和运行记录保留"
	} else if errorsCount > 0 {
		status = "failed"
		message = "部分节点失败，请查看运行记录"
		if len(outputs) > 0 {
			status = "partially_succeeded"
			message = "部分分支失败；已按汇合策略交付成功分支的成果"
		}
	}
	_ = a.updateWorkflowRun(run.ID, func(r *WorkflowRun) {
		r.Status = status
		r.OutputIDs = uniqueIDs(outputs)
		r.Error = message
		r.FinishedAt = now()
	})
}
func (a *App) executeWorkflowNode(ctx context.Context, run WorkflowRun, n WorkflowNode, s WorkflowNodeRun, upstream []WorkflowNodeRun, secret string) (result WorkflowNodeRun) {
	result = s
	defer func() {
		if recover() != nil {
			result.Status = "failed"
			result.Error = "节点执行异常"
		}
		result.FinishedAt = now()
		if ctx.Err() != nil && result.Status != "succeeded" {
			result.Status = "cancelled"
		}
	}()
	ids := []string{}
	var contextText strings.Builder
	refs := []KnowledgeHit{}
	for _, p := range upstream {
		ids = append(ids, p.AssetIDs...)
		if p.Text != "" {
			contextText.WriteString(p.Text + "\n")
		}
		refs = append(refs, p.KnowledgeRefs...)
	}
	ids = uniqueIDs(ids)
	if len([]rune(contextText.String())) > 30000 {
		result.Status = "failed"
		result.Error = "上游内容超过 30000 字符，请缩小检索范围"
		return
	}
	switch n.Type {
	case "start":
		result.Text = run.Prompt
		result.AssetIDs = run.InputIDs
	case "knowledge":
		hits := run.KnowledgeSnapshots[n.ID]
		result.Text = knowledgeContext(hits)
		result.KnowledgeRefs = hits
		result.AssetIDs = ids
	case "condition":
		value := run.Prompt
		if n.Field == "context" {
			value = contextText.String()
		}
		match := strings.Contains(strings.ToLower(value), strings.ToLower(n.Value))
		if n.Operator == "not_contains" {
			match = !match
		}
		if n.Operator == "has_files" {
			match = len(ids) > 0
		}
		result.Branch = "false"
		if match {
			result.Branch = "true"
		}
		result.Text = contextText.String()
		result.AssetIDs = ids
		result.KnowledgeRefs = refs
	case "merge", "end":
		result.Text = contextText.String()
		result.AssetIDs = ids
		result.KnowledgeRefs = refs
	case "agent", "tool":
		config := run.Configs[n.ID]
		prompt := n.Prompt
		if strings.TrimSpace(prompt) == "" {
			prompt = run.Prompt
		}
		prompt = strings.ReplaceAll(prompt, "{{input}}", run.Prompt)
		prompt = strings.ReplaceAll(prompt, "{{context}}", contextText.String())
		if len(refs) == 0 && !strings.Contains(n.Prompt, "{{context}}") && strings.TrimSpace(contextText.String()) != "" && strings.TrimSpace(contextText.String()) != strings.TrimSpace(run.Prompt) {
			prompt += "\n上游内容：\n" + contextText.String()
		}
		if len(refs) > 0 {
			copyConfig := *config
			copyConfig.KnowledgeRefs = append(append([]KnowledgeHit{}, config.KnowledgeRefs...), refs...)
			config = &copyConfig
		}
		inputs := []Asset{}
		for _, assetID := range ids {
			v, err := a.asset(assetID)
			if err != nil {
				result.Status = "failed"
				result.Error = err.Error()
				return
			}
			inputs = append(inputs, v)
		}
		if len(inputs) > 16 {
			result.Status = "failed"
			result.Error = "上游附件超过 16 个，请拆分流程"
			return
		}
		modelName := "本地整理模式"
		if config.Mode == "model" {
			modelName = config.Model.Name
		}
		task := Task{ID: id(), WorkflowRunID: run.ID, WorkflowNodeID: n.ID, ProjectID: run.ProjectID, Title: n.Label, Prompt: prompt, Kind: n.Kind, RoleID: config.Role.ID, ModelName: modelName, Status: "queued", InputIDs: ids, OutputIDs: []string{}, Steps: []Step{}, Config: config, CreatedAt: now(), UpdatedAt: now()}
		if config.Model != nil {
			task.ModelID = config.Model.ID
		}
		a.mu.Lock()
		if a.closing {
			a.mu.Unlock()
			result.Status = "cancelled"
			return
		}
		a.state.Tasks = append(a.state.Tasks, task)
		childContext, cancel := context.WithCancel(ctx)
		a.cancels[task.ID] = cancel
		err := a.saveLocked()
		if err != nil {
			delete(a.cancels, task.ID)
			cancel()
			a.mu.Unlock()
			result.Status = "failed"
			result.Error = err.Error()
			return
		}
		a.mu.Unlock()
		defer cancel()
		result.TaskID = task.ID
		_ = a.updateWorkflowRun(run.ID, func(r *WorkflowRun) {
			for i := range r.Nodes {
				if r.Nodes[i].NodeID == n.ID {
					r.Nodes[i].TaskID = task.ID
				}
			}
		})
		a.runTask(childContext, task, inputs, secret)
		a.mu.RLock()
		for _, v := range a.state.Tasks {
			if v.ID == task.ID {
				task = v
			}
		}
		a.mu.RUnlock()
		if task.Status == "queued" || task.Status == "running" {
			_ = a.updateTask(task.ID, func(t *Task) { t.Status = "cancelled" })
			task.Status = "cancelled"
		}
		result.Status = task.Status
		result.Error = task.Error
		result.Text = task.Result
		result.AssetIDs = task.OutputIDs
		result.KnowledgeRefs = config.KnowledgeRefs
		return
	}
	result.Status = "succeeded"
	return
}
