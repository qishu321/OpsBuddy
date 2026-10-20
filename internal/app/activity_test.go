package app

import "testing"

func TestActivityEventsCoalesceAndContainOnlyStatus(t *testing.T) {
	a := newTestApp(t)
	ch, cancel := a.SubscribeActivity()
	defer cancel()
	if view := <-ch; view.Phase != "idle" {
		t.Fatal(view)
	}
	a.mu.Lock()
	a.state.Tasks = append(a.state.Tasks, Task{ID: "private-id", ProjectID: "inbox", Prompt: "private prompt", Status: "queued"})
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	// Do not consume intermediate events: the one-slot channel must keep only the latest.
	a.state.Tasks[0].Status = "waiting"
	a.publishActivityLocked()
	a.state.Tasks[0].Status = "failed"
	a.publishActivityLocked()
	a.mu.Unlock()
	view := <-ch
	if view.Phase != "error" || view.Count != 0 || len(ch) != 0 {
		t.Fatal(view)
	}
	a.mu.Lock()
	a.publishActivityLocked()
	a.mu.Unlock()
	select {
	case <-ch:
		t.Fatal("unchanged state emitted an event")
	default:
	}
	cancel()
	cancel()
	if _, ok := <-ch; ok {
		t.Fatal("subscription not closed")
	}
	if len(a.activityWatchers) != 0 {
		t.Fatal("subscription retained")
	}
}

func TestActivityStartupDoesNotReplayHistoricalCompletions(t *testing.T) {
	a := newTestApp(t)
	a.state.Tasks = append(a.state.Tasks, Task{ID: "old", Status: "succeeded", UpdatedAt: "2026-01-01"})
	ch, cancel := a.SubscribeActivity()
	defer cancel()
	if view := <-ch; view.Phase != "idle" {
		t.Fatal("replayed old completion", view)
	}
	a.mu.Lock()
	a.publishActivityLocked()
	a.mu.Unlock()
	select {
	case <-ch:
		t.Fatal("settings-only save replayed completion")
	default:
	}
	a.mu.Lock()
	a.state.Tasks = append(a.state.Tasks, Task{ID: "new", Status: "running"})
	a.publishActivityLocked()
	a.mu.Unlock()
	if view := <-ch; view.Phase != "working" || view.Count != 1 {
		t.Fatal(view)
	}
}

func TestActivityTracksWorkStageWithoutTaskContent(t *testing.T) {
	a := newTestApp(t)
	ch, cancel := a.SubscribeActivity()
	defer cancel()
	<-ch
	a.mu.Lock()
	a.state.Tasks = append(a.state.Tasks, Task{ID: "stage", Status: "running", Kind: "code", Prompt: "private", Steps: []Step{{Name: "读取输入材料", Status: "running"}}})
	a.publishActivityLocked()
	a.mu.Unlock()
	if view := <-ch; view.Phase != "working" || view.Stage != "reading" {
		t.Fatal(view)
	}
	a.mu.Lock()
	a.state.Tasks[0].Steps = append(a.state.Tasks[0].Steps, Step{Name: "组织内容", Status: "running"})
	a.publishActivityLocked()
	a.mu.Unlock()
	if view := <-ch; view.Stage != "coding" {
		t.Fatal("work stage change was lost", view)
	}
	a.mu.Lock()
	a.state.Tasks[0].Steps = append(a.state.Tasks[0].Steps, Step{Name: "启动项目预览", Status: "running"})
	a.publishActivityLocked()
	a.mu.Unlock()
	if view := <-ch; view.Stage != "running" {
		t.Fatal(view)
	}
}
