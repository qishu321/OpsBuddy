package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"
)

type DirectoryGrant struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	CreatedAt string `json:"createdAt"`
}
type Schedule struct {
	TaskRequest
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Mode            string           `json:"mode"`
	Frequency       string           `json:"frequency"`
	Timezone        string           `json:"timezone"`
	StartDate       string           `json:"startDate"`
	Clock           string           `json:"clock"`
	Weekdays        []int            `json:"weekdays"`
	MissedPolicy    string           `json:"missedPolicy"`
	DirectoryID     string           `json:"directoryId,omitempty"`
	Status          string           `json:"status"`
	NextAt          string           `json:"nextAt,omitempty"`
	AuthorizedModel *TaskModelConfig `json:"authorizedModel,omitempty"`
	Version         int              `json:"version"`
	CreatedAt       string           `json:"createdAt"`
	UpdatedAt       string           `json:"updatedAt"`
	ResumedAt       string           `json:"resumedAt,omitempty"`
}
type ScheduleRun struct {
	ID          string   `json:"id"`
	ScheduleID  string   `json:"scheduleId"`
	TaskID      string   `json:"taskId,omitempty"`
	ScheduledAt string   `json:"scheduledAt"`
	Trigger     string   `json:"trigger"`
	Status      string   `json:"status"`
	Error       string   `json:"error,omitempty"`
	CreatedAt   string   `json:"createdAt"`
	FinishedAt  string   `json:"finishedAt,omitempty"`
	Definition  Schedule `json:"definition"`
}
type Notification struct {
	ID        string `json:"id"`
	TaskID    string `json:"taskId,omitempty"`
	RunID     string `json:"runId"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	ReadAt    string `json:"readAt,omitempty"`
}

func nextScheduleTime(s Schedule, after time.Time) (time.Time, error) {
	if s.Timezone == "" || s.Timezone == "Local" {
		return time.Time{}, errors.New("请选择明确的时区，例如 Asia/Shanghai")
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.Time{}, errors.New("请输入有效 IANA 时区，例如 Asia/Shanghai")
	}
	clock, err := time.Parse("15:04", s.Clock)
	if err != nil {
		return time.Time{}, errors.New("执行时间格式应为 HH:mm")
	}
	start, err := time.ParseInLocation("2006-01-02", s.StartDate, loc)
	if err != nil || start.Year() < 2000 || start.Year() > 2100 {
		return time.Time{}, errors.New("请选择有效开始日期（2000–2100）")
	}
	if s.Frequency != "once" && s.Frequency != "daily" && s.Frequency != "weekly" {
		return time.Time{}, errors.New("请选择一次、每天或每周")
	}
	if s.Frequency == "weekly" {
		if len(s.Weekdays) == 0 || len(s.Weekdays) > 7 {
			return time.Time{}, errors.New("每周至少选择一天")
		}
		seen := map[int]bool{}
		for _, day := range s.Weekdays {
			if day < 0 || day > 6 || seen[day] {
				return time.Time{}, errors.New("星期设置无效")
			}
			seen[day] = true
		}
	}
	local := after.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	if start.After(day) || s.Frequency == "once" {
		day = start
	}
	for i := 0; i < 370; i++ {
		date := day.AddDate(0, 0, i)
		if s.Frequency == "once" && i > 0 {
			break
		}
		if s.Frequency == "weekly" {
			match := false
			for _, w := range s.Weekdays {
				if w == int(date.Weekday()) {
					match = true
				}
			}
			if !match {
				continue
			}
		}
		candidate := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
		// Skip nonexistent DST wall times; an ambiguous day has one chosen occurrence.
		if candidate.Day() != date.Day() || candidate.Hour() != clock.Hour() || candidate.Minute() != clock.Minute() {
			if s.Frequency == "once" {
				return time.Time{}, errors.New("此时区的该时间不存在，请避开夏令时跳时")
			}
			continue
		}
		if candidate.After(after) {
			return candidate.UTC(), nil
		}
	}
	return time.Time{}, nil
}
func (a *App) RegisterDirectory(projectID, path string) (DirectoryGrant, error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return DirectoryGrant{}, err
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return DirectoryGrant{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return DirectoryGrant{}, errors.New("请选择存在的文件夹")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	valid := false
	for _, p := range a.state.Projects {
		if p.ID == projectID {
			valid = true
		}
	}
	if !valid || a.closing {
		return DirectoryGrant{}, errors.New("项目不可用")
	}
	for _, g := range a.state.Directories {
		if g.ProjectID == projectID && strings.EqualFold(g.Path, resolved) {
			return g, nil
		}
	}
	g := DirectoryGrant{ID: id(), ProjectID: projectID, Name: filepath.Base(resolved), Path: resolved, CreatedAt: now()}
	a.state.Directories = append(a.state.Directories, g)
	if err = a.saveLocked(); err != nil {
		return DirectoryGrant{}, err
	}
	return g, nil
}
func (a *App) scheduleLocked(scheduleID string) *Schedule {
	for i := range a.state.Schedules {
		if a.state.Schedules[i].ID == scheduleID {
			return &a.state.Schedules[i]
		}
	}
	return nil
}
func (a *App) scheduleRunLocked(runID string) *ScheduleRun {
	for i := range a.state.ScheduleRuns {
		if a.state.ScheduleRuns[i].ID == runID {
			return &a.state.ScheduleRuns[i]
		}
	}
	return nil
}
func runningSchedule(status string) bool {
	return status == "queued" || status == "preparing" || status == "running"
}
func (a *App) saveSchedule(w http.ResponseWriter, r *http.Request) {
	var in Schedule
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.Name == "" || len([]rune(in.Name)) > 80 || in.Mode != "task" && in.Mode != "reminder" || in.MissedPolicy != "skip" && in.MissedPolicy != "catch_up_once" {
		problem(w, 400, errors.New("请填写名称、执行方式和错过执行策略"))
		return
	}
	next, err := nextScheduleTime(in, time.Now().UTC())
	if err != nil {
		problem(w, 400, err)
		return
	}
	if next.IsZero() {
		problem(w, 400, errors.New("执行时间必须在未来"))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if in.ParentID != "" {
		problem(w, 400, errors.New("定时任务每次创建独立会话，不能续接旧会话"))
		return
	}
	if in.Mode == "reminder" {
		in.ModelID = ""
		in.AllowRemote = false
		in.Kind = "docx"
		in.InputIDs = nil
		in.SkillIDs = nil
		in.KnowledgeIDs = nil
		in.DirectoryID = ""
	}
	task, _, _, err := a.prepareTaskLocked(in.TaskRequest)
	if err != nil {
		problem(w, 400, err)
		return
	}
	if in.DirectoryID != "" {
		found := false
		for _, g := range a.state.Directories {
			if g.ID == in.DirectoryID && g.ProjectID == in.ProjectID {
				found = true
			}
		}
		if !found {
			problem(w, 400, errors.New("请用桌面文件夹选择器授权当前项目的工作目录"))
			return
		}
	}
	in.AuthorizedModel = task.Config.Model
	in.NextAt = next.Format(time.RFC3339)
	in.Status = "active"
	in.ResumedAt = ""
	in.UpdatedAt = now()
	if in.ID != "" {
		for i, old := range a.state.Schedules {
			if old.ID != in.ID {
				continue
			}
			if old.Version != in.Version {
				problem(w, 409, errors.New("计划已更新，请刷新后编辑"))
				return
			}
			in.Version++
			in.CreatedAt = old.CreatedAt
			a.state.Schedules[i] = in
			if err = a.saveLocked(); err != nil {
				problem(w, 500, err)
				return
			}
			respond(w, 200, in)
			return
		}
		problem(w, 404, errors.New("计划已删除"))
		return
	}
	if len(a.state.Schedules) >= 100 {
		problem(w, 400, errors.New("本版最多保存 100 个计划"))
		return
	}
	in.ID = id()
	in.Version = 1
	in.CreatedAt = in.UpdatedAt
	a.state.Schedules = append(a.state.Schedules, in)
	if err = a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 201, in)
}

func (a *App) previewSchedule(w http.ResponseWriter, r *http.Request) {
	var in Schedule
	if !decode(w, r, &in) {
		return
	}
	times := []string{}
	at := time.Now().UTC()
	for i := 0; i < 5; i++ {
		next, err := nextScheduleTime(in, at)
		if err != nil {
			problem(w, 400, err)
			return
		}
		if next.IsZero() {
			break
		}
		times = append(times, next.Format(time.RFC3339))
		at = next
	}
	respond(w, 200, map[string]any{"times": times})
}
func (a *App) scheduleAction(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	plan := a.scheduleLocked(r.PathValue("id"))
	if plan == nil {
		problem(w, 404, errors.New("计划不存在"))
		return
	}
	switch r.PathValue("action") {
	case "pause":
		plan.Status = "paused"
	case "resume":
		if plan.NextAt == "" {
			problem(w, 409, errors.New("一次性计划已完成，请编辑新的时间"))
			return
		}
		plan.Status = "active"
		plan.ResumedAt = now()
	case "run":
		if a.closing {
			problem(w, 503, errors.New("应用正在关闭"))
			return
		}
		for _, run := range a.state.ScheduleRuns {
			if run.ScheduleID == plan.ID && runningSchedule(run.Status) {
				problem(w, 409, errors.New("此计划已有执行中的任务"))
				return
			}
		}
		run, err := a.claimScheduleLocked(*plan, time.Now().UTC().Format(time.RFC3339Nano), "manual")
		if err != nil {
			problem(w, 500, err)
			return
		}
		respond(w, 201, run)
		return
	default:
		problem(w, 404, errors.New("未知计划操作"))
		return
	}
	plan.UpdatedAt = now()
	plan.Version++
	if err := a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 200, plan)
}
func (a *App) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, p := range a.state.Schedules {
		if p.ID != r.PathValue("id") {
			continue
		}
		for _, run := range a.state.ScheduleRuns {
			if run.ScheduleID == p.ID {
				if cancel := a.cancels[run.ID]; cancel != nil {
					cancel()
				}
			}
		}
		a.state.Schedules = append(a.state.Schedules[:i], a.state.Schedules[i+1:]...)
		if err := a.saveLocked(); err != nil {
			problem(w, 500, err)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	problem(w, 404, errors.New("计划不存在"))
}
func (a *App) cancelScheduleRun(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	run := a.scheduleRunLocked(r.PathValue("id"))
	if run == nil {
		problem(w, 404, errors.New("执行记录不存在"))
		return
	}
	cancel := a.cancels[run.ID]
	if cancel == nil || !runningSchedule(run.Status) {
		problem(w, 409, errors.New("本次执行已结束"))
		return
	}
	cancel()
	run.Status = "cancelled"
	run.FinishedAt = now()
	if task := a.taskLocked(run.TaskID); task != nil && activeTask(task.Status) {
		task.Status = "cancelled"
		task.UpdatedAt = now()
	}
	if err := a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func (a *App) readNotification(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	found := false
	for i := range a.state.Notifications {
		n := &a.state.Notifications[i]
		if r.PathValue("id") == "all" || n.ID == r.PathValue("id") {
			if n.ReadAt == "" {
				n.ReadAt = now()
			}
			found = true
		}
	}
	if !found && r.PathValue("id") != "all" {
		problem(w, 404, errors.New("通知不存在"))
		return
	}
	if err := a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func (a *App) advanceScheduleLocked(plan *Schedule, at time.Time) {
	next, err := nextScheduleTime(*plan, at)
	if err != nil || next.IsZero() {
		plan.NextAt = ""
		plan.Status = "completed"
	} else {
		plan.NextAt = next.Format(time.RFC3339)
	}
	plan.UpdatedAt = now()
}
func (a *App) claimScheduleLocked(plan Schedule, scheduledAt, trigger string) (ScheduleRun, error) {
	if a.closing {
		return ScheduleRun{}, errors.New("应用正在关闭")
	}
	if trigger != "manual" {
		for _, r := range a.state.ScheduleRuns {
			if r.ScheduleID == plan.ID && r.ScheduledAt == scheduledAt && r.Trigger != "manual" {
				return ScheduleRun{}, errors.New("本次触发已记录")
			}
		}
	}
	run := ScheduleRun{ID: id(), ScheduleID: plan.ID, TaskID: id(), ScheduledAt: scheduledAt, Trigger: trigger, Status: "queued", CreatedAt: now(), Definition: plan}
	task := Task{ID: run.TaskID, ConversationID: run.TaskID, ScheduleID: plan.ID, ScheduleRunID: run.ID, ProjectID: plan.ProjectID, Title: "定时 · " + plan.Name, Prompt: plan.Prompt, Kind: plan.Kind, RoleID: plan.RoleID, ModelID: plan.ModelID, ModelName: "等待执行", Status: "waiting", InputIDs: []string{}, OutputIDs: []string{}, Steps: []Step{}, CreatedAt: run.CreatedAt, UpdatedAt: run.CreatedAt}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	a.cancels[run.ID] = cancel
	a.cancels[task.ID] = cancel
	a.state.Tasks = append(a.state.Tasks, task)
	a.state.ScheduleRuns = append(a.state.ScheduleRuns, run)
	if err := a.saveLocked(); err != nil {
		delete(a.cancels, run.ID)
		delete(a.cancels, task.ID)
		cancel()
		return ScheduleRun{}, err
	}
	a.workers.Add(1)
	go func() { defer a.workers.Done(); defer cancel(); a.executeSchedule(ctx, run) }()
	return run, nil
}
func (a *App) processSchedules(at time.Time, startup bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return
	}
	// Iterate IDs: failed saves can replace the whole state.
	ids := []string{}
	for _, p := range a.state.Schedules {
		ids = append(ids, p.ID)
	}
	for _, planID := range ids {
		p := a.scheduleLocked(planID)
		if p == nil || p.Status != "active" || p.NextAt == "" {
			continue
		}
		due, err := time.Parse(time.RFC3339, p.NextAt)
		if err != nil || due.After(at) {
			continue
		}
		plan := *p
		alreadyRecorded := false
		for _, prior := range a.state.ScheduleRuns {
			if prior.ScheduleID == plan.ID && prior.ScheduledAt == plan.NextAt && prior.Trigger != "manual" {
				alreadyRecorded = true
				break
			}
		}
		if alreadyRecorded {
			a.advanceScheduleLocked(p, at)
			if a.saveLocked() != nil {
				return
			}
			continue
		}
		busy := false
		for _, r := range a.state.ScheduleRuns {
			if r.ScheduleID == p.ID && runningSchedule(r.Status) {
				busy = true
			}
		}
		resumed, _ := time.Parse(time.RFC3339, p.ResumedAt)
		missed := startup || !resumed.IsZero() && !due.After(resumed) || at.Sub(due) > time.Minute
		if busy || missed && p.MissedPolicy == "skip" {
			status := "missed"
			message := "应用未在触发时间运行，已按计划跳过；重复计划不逐条补跑。"
			if busy {
				status = "skipped_overlap"
				message = "上次执行尚未结束，本次已跳过，避免重叠执行。"
			}
			run := ScheduleRun{ID: id(), ScheduleID: p.ID, ScheduledAt: p.NextAt, Trigger: "timer", Status: status, Error: message, CreatedAt: now(), FinishedAt: now(), Definition: plan}
			a.state.ScheduleRuns = append(a.state.ScheduleRuns, run)
			a.state.Notifications = append(a.state.Notifications, Notification{ID: id(), RunID: run.ID, Title: plan.Name + " · 已跳过", Body: message, CreatedAt: now()})
			a.advanceScheduleLocked(p, at)
			if a.saveLocked() != nil {
				return
			}
			continue
		}
		trigger := "timer"
		if missed {
			trigger = "catch_up"
		}
		a.advanceScheduleLocked(p, at)
		if _, err := a.claimScheduleLocked(plan, plan.NextAt, trigger); err != nil {
			return
		}
	}
}
func (a *App) startScheduler() {
	ctx, cancel := context.WithCancel(context.Background())
	a.schedulerCancel = cancel
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		a.processSchedules(time.Now().UTC(), true)
		lastTick := time.Now().UTC()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				at := time.Now().UTC()
				a.processSchedules(at, at.Sub(lastTick) > 3*time.Second)
				lastTick = at
			}
		}
	}()
}
func (a *App) executeSchedule(ctx context.Context, run ScheduleRun) {
	defer func() { a.mu.Lock(); delete(a.cancels, run.ID); delete(a.cancels, run.TaskID); a.mu.Unlock() }()
	finish := func(status, message string) {
		a.mu.Lock()
		defer a.mu.Unlock()
		record := a.scheduleRunLocked(run.ID)
		if record == nil {
			return
		}
		if ctx.Err() != nil || record.Status == "cancelled" {
			status = "cancelled"
			message = "本次执行已停止，已交付文件保留。"
		}
		record.Status = status
		record.Error = message
		record.FinishedAt = now()
		if task := a.taskLocked(run.TaskID); task != nil && activeTask(task.Status) {
			task.Status = status
			task.Error = message
			task.Result = message
			task.UpdatedAt = now()
		}
		title := run.Definition.Name + " · " + status
		body := message
		if status == "succeeded" {
			title = run.Definition.Name + " · 已完成"
			body = "点击查看本次独立会话和成果。"
			if run.Definition.Mode == "reminder" {
				title = run.Definition.Name
				body = run.Definition.Prompt
			}
		}
		a.state.Notifications = append(a.state.Notifications, Notification{ID: id(), RunID: run.ID, TaskID: run.TaskID, Title: title, Body: body, CreatedAt: now()})
		_ = a.saveLocked()
	}
	if ctx.Err() != nil {
		finish("cancelled", "")
		return
	}
	plan := run.Definition
	if plan.Mode == "reminder" {
		_ = a.updateTask(run.TaskID, func(t *Task) {
			if ctx.Err() != nil || t.Status == "cancelled" {
				return
			}
			t.Status = "succeeded"
			t.Result = plan.Prompt
			t.ModelID = ""
			t.ModelName = "本机定时提醒"
			t.Steps = append(t.Steps, Step{Name: "到时提醒", Status: "completed", Detail: "已在应用内提醒中心交付，未向外部渠道发送", At: now()})
		})
		finish("succeeded", "")
		return
	}
	var directory *DirectoryGrant
	a.mu.Lock()
	record := a.scheduleRunLocked(run.ID)
	if record != nil {
		record.Status = "preparing"
	}
	if plan.DirectoryID != "" {
		for _, g := range a.state.Directories {
			if g.ID == plan.DirectoryID && g.ProjectID == plan.ProjectID {
				copy := g
				directory = &copy
			}
		}
	}
	err := a.saveLocked()
	a.mu.Unlock()
	if err != nil {
		finish("failed", err.Error())
		return
	}
	if plan.DirectoryID != "" {
		if directory == nil {
			finish("failed", "工作目录授权已失效")
			return
		}
		resolved, err := filepath.EvalSymlinks(directory.Path)
		if err != nil || !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(directory.Path)) {
			finish("failed", "工作目录已变化或不可读取，请重新授权")
			return
		}
		assets, err := a.ImportLocalFolder(plan.ProjectID, directory.Path)
		if err != nil && !errors.Is(err, ErrEmptyFolder) {
			finish("failed", err.Error())
			return
		}
		for _, v := range assets {
			plan.InputIDs = append(plan.InputIDs, v.ID)
		}
	}
	if ctx.Err() != nil {
		finish("cancelled", "")
		return
	}
	a.mu.Lock()
	task, inputs, secret, err := a.prepareTaskLocked(plan.TaskRequest)
	if err == nil && plan.ModelID != "" {
		m := task.Config.Model
		authorized := plan.AuthorizedModel
		if m == nil || authorized == nil || m.ID != authorized.ID || m.Family != authorized.Family || m.BaseURL != authorized.BaseURL || m.ModelID != authorized.ModelID {
			err = errors.New("模型服务配置已变更，请编辑计划并重新确认发送授权")
		}
	}
	if err != nil {
		a.mu.Unlock()
		finish("failed", err.Error())
		return
	}
	task.ID = run.TaskID
	task.ConversationID = run.TaskID
	task.ScheduleID = run.ScheduleID
	task.ScheduleRunID = run.ID
	task.Title = "定时 · " + plan.Name
	task.CreatedAt = run.CreatedAt
	if existing := a.taskLocked(task.ID); existing != nil {
		*existing = task
	}
	if record := a.scheduleRunLocked(run.ID); record != nil {
		record.Status = "running"
	}
	err = a.saveLocked()
	a.mu.Unlock()
	if err != nil {
		finish("failed", err.Error())
		return
	}
	a.runTask(ctx, task, inputs, secret)
	a.mu.RLock()
	status := "failed"
	message := "任务未交付"
	if done := a.taskLocked(task.ID); done != nil {
		status = done.Status
		message = done.Error
	}
	a.mu.RUnlock()
	finish(status, message)
}
