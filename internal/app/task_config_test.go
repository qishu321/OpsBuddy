package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestQueuedTaskUsesFrozenConfigurationAfterEditsAndDeletion(t *testing.T) {
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "技能选择器") {
			if strings.Contains(string(raw), "original-skill-instructions") || strings.Contains(string(raw), "must-not-be-copied") {
				t.Error("skill body leaked into selection catalog")
			}
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"技能选择\",\"rows\":[[\"selected-skill\"]]}"}}]}`)
			return
		}
		requests <- string(raw)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"快照验证\",\"sections\":[{\"heading\":\"验证\",\"body\":\"冻结配置生成内容\"}]}"}}]}`)
	}))
	defer server.Close()
	a := newTestApp(t)
	role := Role{ID: "custom-role", Name: "原角色", Kind: "docx", Instructions: "original-role-instructions", SkillIDs: []string{"selected-skill"}}
	model := Model{ID: "selected-model", Name: "原模型", Family: "openai", BaseURL: server.URL, ModelID: "original-model"}
	extension := Extension{ID: "selected-skill", Name: "原技能", Kind: "skill", Enabled: true, Version: "1.0", Content: "original-skill-instructions"}
	a.state.Roles = append(a.state.Roles, role)
	a.state.Models = append(a.state.Models, model)
	a.state.Extensions = []Extension{extension, {ID: "unrelated", Kind: "skill", Enabled: true, Content: "must-not-be-copied"}}
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < cap(a.slots); i++ {
		a.slots <- struct{}{}
	}
	w := request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "prompt": "生成 Word", "kind": "docx", "roleId": role.ID, "modelId": model.ID, "allowRemote": true})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var submitted Task
	if err := json.Unmarshal(w.Body.Bytes(), &submitted); err != nil {
		t.Fatal(err)
	}
	if submitted.Config == nil || len(submitted.Config.Extensions) != 1 || submitted.Config.Mode != "model" || submitted.Config.Extensions[0].ContentHash != contentHash(extension.Content) {
		t.Fatal("incomplete configuration snapshot")
	}
	role.Instructions = "changed-role"
	extension.Content = "changed-skill"
	model.ModelID = "changed-model"
	for _, tc := range []struct {
		path string
		body any
	}{{"/api/roles", role}, {"/api/extensions", extension}, {"/api/models", model}} {
		if w = request(t, a, "POST", tc.path, tc.body); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	for _, path := range []string{"/api/models/selected-model", "/api/extensions/selected-skill"} {
		if w = request(t, a, "DELETE", path, nil); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	<-a.slots
	finished := waitTask(t, a, submitted.ID)
	if finished.Status != "succeeded" || !reflect.DeepEqual(submitted.Config, finished.Config) {
		t.Fatalf("configuration changed during execution: %s %s", finished.Status, finished.Error)
	}
	select {
	case body := <-requests:
		for _, required := range []string{"original-model", "original-role-instructions", "original-skill-instructions"} {
			if !strings.Contains(body, required) {
				t.Fatal("model did not receive frozen configuration", required)
			}
		}
		if strings.Contains(body, "changed-") || strings.Contains(body, "must-not-be-copied") {
			t.Fatal("unselected or updated content leaked into request")
		}
	default:
		t.Fatal("model was not called")
	}
	view := a.snapshot()
	view.Tasks[0].Config.Role.SkillIDs[0] = "mutated-copy"
	view.Tasks[0].Config.Extensions[0].Content = "mutated-copy"
	if !reflect.DeepEqual(a.snapshot().Tasks[0].Config, submitted.Config) {
		t.Fatal("snapshot aliases live state")
	}
	if w = request(t, a, "POST", "/api/tasks/"+submitted.ID+"/archive", nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	a.Close()
	reopened, err := New(a.root, a.web)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(reopened.snapshot().Tasks[0].Config, submitted.Config) || !reopened.snapshot().Tasks[0].Archived {
		t.Fatal("frozen configuration did not survive restart")
	}
}

func TestConfigSnapshotContainsNoCredentialAndTracksEffectiveMode(t *testing.T) {
	role := Role{ID: "role", SkillIDs: []string{"disabled", "enabled", "enabled"}}
	model := Model{ID: "model", Secret: "must-never-be-copied", Name: "模型", Family: "openai", ModelID: "test"}
	exts := []Extension{{ID: "disabled", Kind: "skill", Content: "disabled-content"}, {ID: "enabled", Kind: "plugin", Enabled: true, Content: `{"name":"package","version":"1","skills":[]}`, Version: "1"}}
	for _, kind := range []string{"docx", "pdf", "organize"} {
		config, err := captureTaskConfig(role, model, exts, kind, true)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(config)
		if strings.Contains(string(raw), model.Secret) || strings.Contains(string(raw), "disabled-content") || len(config.Extensions) != 1 || config.Model.CredentialRef != model.ID {
			t.Fatal("invalid secret/dependency filtering")
		}
		if (config.Mode == "model") != (kind == "docx") || (config.runtimeModel(model.Secret).ID != "") != (kind == "docx") {
			t.Fatal("snapshot misrepresents local execution")
		}
		role.SkillIDs[0] = "mutated"
		if config.Role.SkillIDs[0] != "disabled" {
			t.Fatal("role aliases caller data")
		}
		role.SkillIDs[0] = "disabled"
	}
}

func TestTaskRejectsMissingRoleAndSkill(t *testing.T) {
	a := newTestApp(t)
	a.state.Roles = append(a.state.Roles, Role{ID: "invalid-role", SkillIDs: []string{"deleted-skill"}})
	if err := a.saveLocked(); err != nil {
		t.Fatal(err)
	}
	for _, roleID := range []string{"deleted-role", "invalid-role"} {
		w := request(t, a, "POST", "/api/tasks", map[string]string{"projectId": "inbox", "prompt": "test", "roleId": roleID, "kind": "docx"})
		if w.Code != 400 || len(a.snapshot().Tasks) != 0 {
			t.Fatal("invalid configuration was silently accepted", w.Code)
		}
	}
}

func TestCloseWaitsForQueuedWorkersAndReleasesDatabase(t *testing.T) {
	a := newTestApp(t)
	for i := 0; i < cap(a.slots); i++ {
		a.slots <- struct{}{}
	}
	w := request(t, a, "POST", "/api/tasks", map[string]string{"projectId": "inbox", "prompt": "queued", "kind": "docx"})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	a.Close()
	if len(a.cancels) != 0 {
		t.Fatal("workers still running after close")
	}
	reopened, err := New(a.root, a.web)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.state.Tasks[0].Status != "interrupted" || reopened.state.Tasks[0].Config == nil || len(reopened.state.Assets) != 0 {
		t.Fatal("queued shutdown recovery failed")
	}
}
