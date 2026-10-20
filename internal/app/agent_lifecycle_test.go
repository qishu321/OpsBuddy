package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func waitAgentCondition(t *testing.T, a *App, groupID string, fn func(AgentGroup) bool) AgentGroup {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, g := range a.snapshot().AgentGroups {
			if g.ID == groupID && fn(g) {
				return g
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("agent condition timed out", a.snapshot().AgentGroups)
	return AgentGroup{}
}

func TestAgentPauseEditResumeKeepsIndependentSuccessAndRejectsStalePlan(t *testing.T) {
	var safeCalls, workCalls atomic.Int32
	entered := make(chan struct{}, 1)
	mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
		p := in.Messages[1].Content
		if strings.Contains(in.Messages[0].Content, "你是独立审阅者") {
			return reviewPass(), 200
		}
		if strings.Contains(p, "当前职责：SAFE") {
			safeCalls.Add(1)
			return "SAFE_RESULT", 200
		}
		if strings.Contains(p, "当前职责：WORK") {
			if workCalls.Add(1) == 1 {
				entered <- struct{}{}
				<-r.Context().Done()
				return "", 499
			}
			if !strings.Contains(p, "UPDATED") {
				t.Error("new plan was not used")
			}
			return "UPDATED_RESULT", 200
		}
		return "FINAL", 200
	})
	a := newTestApp(t)
	registerAgentModel(t, a, "main", mock.URL, "")
	g := dispatchTestAgents(t, a, "chat", []AgentNodeSpec{{ID: "safe", Role: "worker", Prompt: "SAFE", Kind: "chat"}, {ID: "work", Role: "worker", Prompt: "WORK", Kind: "chat", DependsOn: []string{"safe"}}, {ID: "review", Role: "reviewer", Prompt: "CHECK", DependsOn: []string{"work"}}}, false)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("work did not start")
	}
	// A live graph cannot be changed while requests are in flight.
	if w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/plan", map[string]any{"expectedVersion": 1, "members": g.Plan.Members}); w.Code != 409 {
		t.Fatal("live mutation accepted", w.Body.String())
	}
	if w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/pause", nil); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	paused := waitAgentCondition(t, a, g.ID, func(g AgentGroup) bool { return g.Status == "paused" && !g.Settling })
	// Restore the JSON checkpoint before editing: omitted empty slices must not
	// make unchanged branches look modified after a restart.
	a.Close()
	restored, restoreErr := New(a.root, a.web)
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
	a = restored
	defer a.Close()
	if paused.Members[2].TaskIDs != nil && len(paused.Members[2].TaskIDs) > 0 {
		t.Fatal("downstream dispatched during pause")
	}
	specs := paused.Plan.Members
	specs[1].Prompt = "WORK UPDATED"
	// Invalid DAG and cross-project inputs never alter the checkpoint.
	bad := append([]AgentNodeSpec{}, specs...)
	bad[0].DependsOn = []string{"work"}
	if w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/plan", map[string]any{"expectedVersion": 1, "members": bad}); w.Code != 400 {
		t.Fatal("cycle accepted", w.Body.String())
	}
	bad = append([]AgentNodeSpec{}, specs...)
	bad[1].InputIDs = []string{"other-project-material"}
	if w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/plan", map[string]any{"expectedVersion": 1, "members": bad}); w.Code != 400 {
		t.Fatal("out of scope input accepted", w.Body.String())
	}
	w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/plan", map[string]any{"expectedVersion": 1, "members": specs})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/plan", map[string]any{"expectedVersion": 1, "members": specs}); w.Code != 409 {
		t.Fatal("stale edit accepted")
	}
	if w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/resume", nil); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	done := waitGroup(t, a, g.ID)
	if done.Status != "succeeded" || done.PlanVersion != 2 || len(done.PlanHistory) != 1 || safeCalls.Load() != 1 || workCalls.Load() != 2 || len(done.Members[1].TaskIDs) != 2 || len(done.Members[2].TaskIDs) != 1 {
		t.Fatalf("invalid resume: %+v safe=%d work=%d", done, safeCalls.Load(), workCalls.Load())
	}
}

func TestAgentRestartRecoveryOptInAndPendingInstructions(t *testing.T) {
	for _, auto := range []bool{false, true} {
		t.Run(map[bool]string{false: "manual", true: "automatic"}[auto], func(t *testing.T) {
			var calls atomic.Int32
			entered := make(chan struct{}, 1)
			mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
				if strings.Contains(in.Messages[0].Content, "你是独立审阅者") {
					return reviewPass(), 200
				}
				p := in.Messages[1].Content
				if strings.Contains(p, "当前职责：WORK") && calls.Add(1) == 1 {
					entered <- struct{}{}
					<-r.Context().Done()
					return "", 499
				}
				return "ACTUAL_RESULT", 200
			})
			a := newTestApp(t)
			registerAgentModel(t, a, "main", mock.URL, "")
			g := dispatchTestAgents(t, a, "chat", []AgentNodeSpec{{ID: "w", Role: "worker", Prompt: "WORK", Kind: "chat"}, {ID: "r", Role: "reviewer", Prompt: "CHECK", DependsOn: []string{"w"}}}, false)
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("worker not started")
			}
			request(t, a, "POST", "/api/agent-groups/"+g.ID+"/members/w/instructions", map[string]string{"prompt": "FOLLOWUP_MUST_SURVIVE"})
			request(t, a, "POST", "/api/agent-groups/"+g.ID+"/recovery", map[string]bool{"autoResume": auto})
			a.Close()
			b, err := New(a.root, a.web)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			if !auto {
				time.Sleep(100 * time.Millisecond)
				if calls.Load() != 1 || b.snapshot().AgentGroups[0].Status != "interrupted" {
					t.Fatal("unexpected automatic replay")
				}
				if w := request(t, b, "POST", "/api/agent-groups/"+g.ID+"/resume", nil); w.Code != 202 {
					t.Fatal(w.Body.String())
				}
			}
			done := waitGroup(t, b, g.ID)
			if done.Status != "succeeded" || len(done.Members[0].TaskIDs) != 3 || done.ResumeCount != 1 {
				t.Fatal(done)
			}
			found := false
			for _, task := range b.snapshot().Tasks {
				if task.ID == done.Members[0].TaskIDs[2] && strings.Contains(task.Prompt, "FOLLOWUP_MUST_SURVIVE") {
					found = true
				}
			}
			if !found {
				t.Fatal("pending instruction was discarded")
			}
		})
	}
}

func TestAgentResumeRejectsChangedModelAndStoppedGroup(t *testing.T) {
	entered := make(chan struct{}, 1)
	mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
		entered <- struct{}{}
		<-r.Context().Done()
		return "", 499
	})
	a := newTestApp(t)
	registerAgentModel(t, a, "main", mock.URL, "")
	g := dispatchTestAgents(t, a, "chat", []AgentNodeSpec{{ID: "w", Role: "worker", Prompt: "WORK", Kind: "chat"}}, false)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker not started")
	}
	request(t, a, "POST", "/api/agent-groups/"+g.ID+"/pause", nil)
	waitAgentCondition(t, a, g.ID, func(g AgentGroup) bool { return !g.Settling })
	a.mu.Lock()
	a.state.Models[0].BaseURL = "http://127.0.0.1:1"
	_ = a.saveLocked()
	a.mu.Unlock()
	w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/resume", nil)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "变更") {
		t.Fatal(w.Body.String())
	}
	if w = request(t, a, "POST", "/api/agent-groups/"+g.ID+"/cancel", nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = request(t, a, "POST", "/api/agent-groups/"+g.ID+"/resume", nil); w.Code != 409 {
		t.Fatal("cancelled group resumed")
	}
	raw, _ := json.Marshal(a.snapshot())
	if strings.Contains(string(raw), "credential-123456") {
		t.Fatal("credential leak")
	}
}

func TestAgentRecoveryAtPlanningAndAggregationBoundaries(t *testing.T) {
	for _, stage := range []string{"planning", "aggregating"} {
		t.Run(stage, func(t *testing.T) {
			var plans, workers, finals atomic.Int32
			entered := make(chan struct{}, 1)
			mock := agentMock(t, func(in agentModelRequest, r *http.Request) (string, int) {
				sys, p := in.Messages[0].Content, in.Messages[1].Content
				if strings.Contains(sys, "你是本项目任务的主协调者") {
					if plans.Add(1) == 1 && stage == "planning" {
						entered <- struct{}{}
						<-r.Context().Done()
						return "", 499
					}
					return agentReply(AgentPlan{Summary: "实际计划", Members: []AgentNodeSpec{{ID: "w", Role: "worker", Prompt: "WORK", Kind: "chat"}, {ID: "r", Role: "reviewer", Prompt: "REVIEW", DependsOn: []string{"w"}}}}), 200
				}
				if strings.Contains(sys, "你是独立审阅者") {
					return reviewPass(), 200
				}
				if strings.Contains(p, "请综合子任务成果") {
					if finals.Add(1) == 1 && stage == "aggregating" {
						entered <- struct{}{}
						<-r.Context().Done()
						return "", 499
					}
					return "FINAL", 200
				}
				workers.Add(1)
				return "DELIVERED", 200
			})
			a := newTestApp(t)
			registerAgentModel(t, a, "main", mock.URL, "")
			g := dispatchTestAgents(t, a, "chat", nil, true)
			select {
			case <-entered:
			case <-time.After(4 * time.Second):
				t.Fatal("stage not reached", stage)
			}
			request(t, a, "POST", "/api/agent-groups/"+g.ID+"/pause", nil)
			waitAgentCondition(t, a, g.ID, func(g AgentGroup) bool { return !g.Settling })
			if w := request(t, a, "POST", "/api/agent-groups/"+g.ID+"/resume", nil); w.Code != 202 {
				t.Fatal(w.Body.String())
			}
			done := waitGroup(t, a, g.ID)
			if done.Status != "succeeded" || workers.Load() != 1 {
				t.Fatal(done, workers.Load())
			}
		})
	}
}
