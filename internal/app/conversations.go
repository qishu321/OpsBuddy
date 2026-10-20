package app

import (
	"errors"
	"net/http"
)

func (a *App) deleteConversation(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	task := a.taskLocked(r.PathValue("id"))
	if task == nil {
		problem(w, 404, errors.New("会话不存在"))
		return
	}
	if task.AgentMemberID != "" {
		problem(w, 409, errors.New("请从主会话删除完整的协作任务"))
		return
	}
	conversationID := task.ConversationID
	if conversationID == "" {
		conversationID = task.ID
	}
	removed := map[string]bool{}
	for _, item := range a.state.Tasks {
		if item.ProjectID == task.ProjectID && (item.ID == conversationID || item.ConversationID == conversationID) {
			removed[item.ID] = true
		}
	}
	groups := map[string]bool{}
	for _, group := range a.state.AgentGroups {
		if (group.ProjectID == "" || group.ProjectID == task.ProjectID) && removed[group.RootTaskID] {
			if activeAgentGroup(group.Status) {
				problem(w, 409, errors.New("请先停止正在执行的协作任务，再删除会话"))
				return
			}
			groups[group.ID] = true
			if planner := a.taskLocked(group.PlannerTaskID); planner != nil && planner.ProjectID == task.ProjectID {
				removed[planner.ID] = true
			}
			for _, member := range group.Members {
				for _, id := range member.TaskIDs {
					if memberTask := a.taskLocked(id); memberTask != nil && memberTask.ProjectID == task.ProjectID {
						removed[id] = true
					}
				}
			}
		}
	}
	for _, item := range a.state.Tasks {
		if removed[item.ID] && activeTask(item.Status) {
			problem(w, 409, errors.New("请先停止正在执行的任务，再删除会话"))
			return
		}
	}
	kept := []Task{}
	for _, item := range a.state.Tasks {
		if !removed[item.ID] {
			kept = append(kept, item)
		}
	}
	keptGroups := []AgentGroup{}
	for _, group := range a.state.AgentGroups {
		if !groups[group.ID] {
			keptGroups = append(keptGroups, group)
		}
	}
	a.state.Tasks = kept
	a.state.AgentGroups = keptGroups
	// Files remain independently available in the project's file list.
	if err := a.saveLocked(); err != nil {
		problem(w, 500, err)
		return
	}
	respond(w, 200, map[string]any{"ok": true, "deletedTasks": len(removed), "filesPreserved": true})
}
