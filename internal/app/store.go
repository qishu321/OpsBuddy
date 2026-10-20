package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type App struct {
	mu               sync.RWMutex
	state            State
	root, web        string
	cancels          map[string]context.CancelFunc
	agentRuns        map[string]bool
	slots            chan struct{}
	store            *stateStore
	persisted        []byte
	workers          sync.WaitGroup
	closeOnce        sync.Once
	closing          bool
	startupImport    *ProjectImportResult
	schedulerCancel  context.CancelFunc
	weatherFetch     func(context.Context, string) (string, error) // test seam; production uses fixed weather endpoints
	codeMu           sync.Mutex
	extractMu        sync.Mutex
	codeRuns         map[string]*codeRuntime
	codeWorkers      sync.WaitGroup
	activityWatchers map[chan Activity]struct{}
	lastActivity     Activity
}

func New(root, web string) (*App, error) {
	abs, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Join(abs, "files"), 0700); e != nil {
		return nil, e
	}
	store, state, e := openStore(abs)
	if e != nil {
		return nil, e
	}
	a := &App{root: abs, web: web, cancels: map[string]context.CancelFunc{}, slots: make(chan struct{}, 3), state: state, store: store, codeRuns: map[string]*codeRuntime{}}
	a.persisted, _ = json.Marshal(state)
	if e = ensureBuiltinSkills(&a.state); e != nil {
		store.close()
		return nil, e
	}
	migrateRoleSkills(&a.state)
	for i := range a.state.ScheduleRuns {
		run := &a.state.ScheduleRuns[i]
		if runningSchedule(run.Status) {
			run.Status = "interrupted"
			run.Error = "应用退出中断本次执行；不会自动重放。"
			run.FinishedAt = now()
			notified := false
			for _, notification := range a.state.Notifications {
				if notification.RunID == run.ID {
					notified = true
				}
			}
			if !notified {
				a.state.Notifications = append(a.state.Notifications, Notification{ID: id(), RunID: run.ID, TaskID: run.TaskID, Title: run.Definition.Name + " · 上次执行中断", Body: run.Error, CreatedAt: now()})
			}
		}
	}
	for i := range a.state.AgentGroups {
		group := &a.state.AgentGroups[i]
		if activeAgentGroup(group.Status) {
			group.InterruptedStage = group.Status
			group.Status = "interrupted"
			group.Error = "桌面程序退出导致协作中断；已交付结果保留，不自动重复模型请求。"
			for j := range group.Members {
				if activeTask(group.Members[j].Status) {
					group.Members[j].Status = "interrupted"
				}
			}
		}
	}
	for i := range a.state.Tasks {
		t := &a.state.Tasks[i]
		if activeTask(t.Status) {
			t.Status = "interrupted"
			t.Error = "应用重启中断了任务。可以重试；已完成成果仍然保留。"
			t.UpdatedAt = now()
		}
	}
	for i := range a.state.WorkflowRuns {
		run := &a.state.WorkflowRuns[i]
		if run.Status == "running" || run.Status == "queued" {
			run.Status = "interrupted"
			run.Error = "应用退出导致流程中断，可查看已交付文件后创建新运行。"
			for j := range run.Nodes {
				if run.Nodes[j].Status == "running" || run.Nodes[j].Status == "pending" {
					run.Nodes[j].Status = "interrupted"
				}
			}
		}
	}
	if e = a.saveLocked(); e != nil {
		store.close()
		return nil, e
	}
	a.importPendingProjects()
	a.startScheduler()
	a.resumeOptedInAgents()
	return a, nil
}
func (a *App) saveLocked() error {
	b, e := json.Marshal(a.state)
	if e != nil {
		return e
	}
	var before State
	if e = json.Unmarshal(a.persisted, &before); e != nil {
		return e
	}
	if e = a.store.save(before, a.state); e != nil {
		// Failed writes must not remain visible or leak into a later successful save.
		a.state = before
		return e
	}
	a.persisted = b
	a.publishActivityLocked()
	return nil
}
func (a *App) snapshot() State {
	a.mu.RLock()
	defer a.mu.RUnlock()
	b, _ := json.Marshal(a.state)
	var s State
	_ = json.Unmarshal(b, &s)
	s.StartupImport = a.startupImport
	for i := range s.AgentGroups {
		s.AgentGroups[i].Settling = a.agentSettlingLocked(&s.AgentGroups[i])
	}
	for i := range s.Models {
		s.Models[i].HasKey = s.Models[i].Secret != ""
		s.Models[i].Secret = ""
	}
	for i := range s.Assets {
		s.Assets[i].Path = ""
		if s.Assets[i].Kind == "code" {
			s.Assets[i].Name = strings.TrimSuffix(s.Assets[i].Name, ".zip")
		}
	}
	for i := range s.KnowledgeDocuments {
		s.KnowledgeDocuments[i].Chunks = nil
	}
	return s
}
func (a *App) asset(assetID string) (Asset, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, v := range a.state.Assets {
		if v.ID == assetID {
			return v, nil
		}
	}
	return Asset{}, errors.New("文件不存在")
}
func (a *App) filePath(v Asset) (string, error) {
	p := filepath.Join(a.root, filepath.FromSlash(v.Path))
	base := filepath.Join(a.root, "files")
	base, e := filepath.EvalSymlinks(base)
	if e != nil {
		return "", e
	}
	actual, e := filepath.EvalSymlinks(p)
	if e != nil {
		return "", e
	}
	rel, e := filepath.Rel(base, actual)
	if e != nil || rel == ".." || filepath.IsAbs(rel) || (len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator)) {
		return "", errors.New("文件超出工作空间")
	}
	return actual, nil
}
func (a *App) addAsset(v Asset) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Assets = append(a.state.Assets, v)
	if e := a.saveLocked(); e != nil {
		return e
	}
	return nil
}
func (a *App) updateTask(taskID string, fn func(*Task)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	found := false
	for i := range a.state.Tasks {
		if a.state.Tasks[i].ID == taskID {
			found = true
			fn(&a.state.Tasks[i])
			a.state.Tasks[i].UpdatedAt = now()
			break
		}
	}
	if !found {
		return errors.New("任务不存在")
	}
	if e := a.saveLocked(); e != nil {
		fmt.Fprintln(os.Stderr, "任务保存失败:", e)
		return e
	}
	return nil
}
func (a *App) step(taskID, name, status, detail string) {
	a.updateTask(taskID, func(t *Task) { t.Steps = append(t.Steps, Step{Name: name, Status: status, Detail: detail, At: now()}) })
}
func (a *App) Close() {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.closing = true
		for i := range a.state.AgentGroups {
			if activeAgentGroup(a.state.AgentGroups[i].Status) {
				a.suspendAgentsLocked(&a.state.AgentGroups[i], "interrupted")
			}
		}
		_ = a.saveLocked()
		if a.schedulerCancel != nil {
			a.schedulerCancel()
		}
		for _, cancel := range a.cancels {
			cancel()
		}
		a.mu.Unlock()
		a.stopCodeRuntimes()
		a.workers.Wait()
		// A worker already generating code can finish after the first stop pass.
		a.stopCodeRuntimes()
		a.mu.Lock()
		defer a.mu.Unlock()
		for ch := range a.activityWatchers {
			delete(a.activityWatchers, ch)
			close(ch)
		}
		a.store.close()
	})
}
