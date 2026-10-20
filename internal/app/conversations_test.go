package app

import (
	"path/filepath"
	"testing"
)

func TestDeleteConversationPersistsAndPreservesFiles(t *testing.T) {
	a := newTestApp(t)
	a.mu.Lock()
	a.state.Tasks = append(a.state.Tasks,
		Task{ID: "conversation-root", ConversationID: "conversation-root", ProjectID: "inbox", Status: "succeeded"},
		Task{ID: "followup", ParentID: "conversation-root", ConversationID: "conversation-root", ProjectID: "inbox", Status: "succeeded"},
		Task{ID: "independent", ConversationID: "independent", ProjectID: "inbox", Status: "succeeded"},
		Task{ID: "member", ConversationID: "member", AgentMemberID: "member-id", ProjectID: "inbox", Status: "succeeded"},
	)
	a.state.Assets = append(a.state.Assets, Asset{ID: "keep-file", TaskID: "conversation-root", ProjectID: "inbox", Name: "保留成果.docx"})
	a.state.AgentGroups = append(a.state.AgentGroups, AgentGroup{ID: "group", RootTaskID: "followup", Status: "succeeded", Members: []AgentMember{{ID: "member-id", TaskIDs: []string{"member"}}}})
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	a.mu.Unlock()
	w := request(t, a, "DELETE", "/api/tasks/followup", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	state := a.snapshot()
	if len(state.Tasks) != 1 || state.Tasks[0].ID != "independent" || len(state.AgentGroups) != 0 || len(state.Assets) != 1 {
		t.Fatal("conversation deletion crossed file/other-session boundaries")
	}
	root := a.root
	a.Close()
	reopened, err := New(root, filepath.Join(root, "web"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.snapshot().Tasks) != 1 || len(reopened.snapshot().Assets) != 1 {
		t.Fatal("deletion did not persist")
	}
}

func TestDeleteConversationRejectsActiveAndRollsBackFailure(t *testing.T) {
	a := newTestApp(t)
	a.mu.Lock()
	a.state.Tasks = append(a.state.Tasks, Task{ID: "busy", Status: "waiting", ConversationID: "busy", ProjectID: "inbox"})
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	a.mu.Unlock()
	if w := request(t, a, "DELETE", "/api/tasks/busy", nil); w.Code != 409 {
		t.Fatal(w.Code)
	}
	a.mu.Lock()
	a.state.Tasks[0].Status = "succeeded"
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	_, err := a.store.conn.ExecContext(t.Context(), `CREATE TRIGGER deny_task_delete BEFORE DELETE ON entities WHEN OLD.kind='tasks' BEGIN SELECT RAISE(FAIL, 'injected failure'); END`)
	a.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if w := request(t, a, "DELETE", "/api/tasks/busy", nil); w.Code != 500 {
		t.Fatal(w.Code)
	}
	if len(a.snapshot().Tasks) != 1 {
		t.Fatal("failed delete removed session from memory")
	}
	if w := request(t, a, "DELETE", "/api/tasks/missing", nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
