package app

import "strings"

// Activity deliberately contains no prompts, paths, project names or model data.
// The desktop companion is an event consumer, never a polling client of /state.
type Activity struct {
	Phase    string
	Count    int
	Stage    string
	revision string
}

func (a *App) activityLocked() Activity {
	view := Activity{Phase: "idle"}
	latest := ""
	terminal := ""
	waiting := 0
	activeAt := ""
	for _, task := range a.state.Tasks {
		switch task.Status {
		case "queued", "running":
			view.Count++
			if task.UpdatedAt >= activeAt {
				activeAt = task.UpdatedAt
				view.Stage = taskActivityStage(task)
			}
		case "waiting":
			waiting++
		case "succeeded", "failed", "cancelled":
			if task.UpdatedAt >= latest {
				latest = task.UpdatedAt
				terminal = task.Status
				view.revision = task.ID + ":" + task.Status
			}
		}
	}
	if view.Count == 0 {
		for _, run := range a.state.WorkflowRuns {
			if run.Status == "queued" || run.Status == "running" {
				view.Count++
			}
		}
	}
	if view.Count > 0 {
		view.Phase = "working"
	} else if waiting > 0 {
		view.Phase, view.Count = "waiting", waiting
	} else if terminal == "failed" {
		view.Phase = "error"
	} else if terminal == "succeeded" {
		view.Phase = "done"
	}
	return view
}

// Only application-defined categories reach the pet; task titles and step details stay private.
func taskActivityStage(task Task) string {
	if task.Status == "queued" {
		return "queued"
	}
	name := ""
	if len(task.Steps) > 0 {
		name = task.Steps[len(task.Steps)-1].Name
	}
	switch {
	case strings.Contains(name, "启动") || strings.Contains(name, "运行") || strings.Contains(name, "编译"):
		return "running"
	case strings.Contains(name, "派发") || strings.Contains(name, "汇总"):
		return "parallel"
	case strings.Contains(name, "检索") || strings.Contains(name, "查询"):
		return "searching"
	case strings.Contains(name, "读取"):
		return "reading"
	case strings.Contains(name, "保存") || strings.Contains(name, "生成"):
		return "writing"
	case task.Kind == "code" || task.Kind == "html":
		return "coding"
	default:
		return "thinking"
	}
}

func (a *App) publishActivityLocked() {
	if len(a.activityWatchers) == 0 {
		return
	}
	view := a.activityLocked()
	if view == a.lastActivity {
		return
	}
	a.lastActivity = view
	for ch := range a.activityWatchers {
		select {
		case <-ch:
		default:
		}
		ch <- view // One latest-state slot; no blocked producers or growing queue.
	}
}

// SubscribeActivity returns an initial view, followed only by actual changes.
func (a *App) SubscribeActivity() (<-chan Activity, func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ch := make(chan Activity, 1)
	if a.activityWatchers == nil {
		a.activityWatchers = map[chan Activity]struct{}{}
	}
	view := a.activityLocked()
	if len(a.activityWatchers) == 0 {
		a.lastActivity = view
	}
	a.activityWatchers[ch] = struct{}{}
	if view.Phase == "done" || view.Phase == "error" {
		view.Phase = "idle"
	}
	ch <- view
	return ch, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if _, ok := a.activityWatchers[ch]; ok {
			delete(a.activityWatchers, ch)
			close(ch)
		}
	}
}
