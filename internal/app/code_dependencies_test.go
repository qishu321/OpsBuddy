package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIncompleteVueLoadingTemplateRejected(t *testing.T) {
	p := nodeCodeFixture()
	p.Files = append(p.Files, CodeFile{Path: "index.html", Content: `<div id="app"><p class="loading">正在加载…</p></div><script>Vue.createApp({setup(){return {}}}).mount('#app')</script>`})
	if validateCodeUI(p) == nil {
		t.Fatal("loading-only Vue page accepted")
	}
	p.Files[1].Content = strings.ReplaceAll(p.Files[1].Content, "setup()", `template:'<button>开始</button>',setup()`)
	if err := validateCodeUI(p); err != nil {
		t.Fatal(err)
	}
}

func TestCodeDirectoriesAndExplicitArchiveExport(t *testing.T) {
	a := newTestApp(t)
	p := nodeCodeFixture()
	p.Files = append(p.Files, CodeFile{Path: "web/index.html", Content: `<script src="https://unpkg.com/vue@3/dist/vue.global.prod.js"></script>`})
	asset, err := a.saveCodeProject(context.Background(), "inbox", "", "", p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(asset.Name, ".zip") || !strings.HasSuffix(asset.Path, "/opsbuddy-project.json") {
		t.Fatal("project is still an archive", asset)
	}
	folder, err := a.CodeSourceDirectory(asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(folder, "web", "index.html"))
	if err != nil || !strings.Contains(string(raw), "../__opsbuddy_runtime__/vue.global.prod.js") {
		t.Fatal("CDN was not localized", err, string(raw))
	}
	if _, err = os.Stat(filepath.Join(folder, runtimeAssetDir, "vue.global.prod.js")); err != nil {
		t.Fatal("offline runtime not in source directory", err)
	}
	if err = os.WriteFile(filepath.Join(folder, "web", "index.html"), []byte("edited outside app"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := a.loadCodeProject(asset)
	if err != nil || loaded.Files[1].Content != "edited outside app" {
		t.Fatal("disk edits ignored", err)
	}
	w := request(t, a, "GET", "/api/assets/"+asset.ID+"/file", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), ".zip") {
		t.Fatal("explicit archive export failed", w.Code)
	}
	z, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = z.Open("web/index.html"); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyCodeArchiveLoadsOfflineAndKeepsOriginal(t *testing.T) {
	a := newTestApp(t)
	p := nodeCodeFixture()
	p.Files[0].Content += `\n// https://cdn.jsdelivr.net/npm/vue@3.4.27/dist/vue.global.prod.js`
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	f, _ := writer.Create("opsbuddy-project.json")
	raw, _ := json.Marshal(p)
	_, _ = f.Write(raw)
	_ = writer.Close()
	legacy := Asset{ID: "legacy-code", Kind: "code", ProjectID: "inbox", Name: "旧项目.zip", Path: "files/legacy-code.zip"}
	if err := os.WriteFile(filepath.Join(a.root, "files", "legacy-code.zip"), buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.addAsset(legacy); err != nil {
		t.Fatal(err)
	}
	loaded, err := a.loadCodeProject(legacy)
	if err != nil || strings.Contains(loaded.Files[0].Content, "jsdelivr.net") {
		t.Fatal("legacy CDN not repaired", err)
	}
	folder, err := a.CodeSourceDirectory(legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(folder, "server.mjs")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(a.root, "files", "legacy-code.zip")); err != nil {
		t.Fatal("legacy archive deleted")
	}
	current, _ := a.asset(legacy.ID)
	if !strings.HasSuffix(current.Path, ".json") {
		t.Fatal("legacy source directory not persisted")
	}
}

func TestPreviewLocalVueRuntimeAndHealth(t *testing.T) {
	a := newTestApp(t)
	p := nodeCodeFixture()
	asset, err := a.saveCodeProject(context.Background(), "inbox", "", "", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.startCodeRuntime(asset); err != nil {
		t.Fatal(err)
	}
	run := waitCodeRun(t, a, asset.ID, func(v CodeRunStatus) bool { return v.Status == "running" })
	for _, name := range []string{"vue.global.prod.js", "vue.esm-browser.prod.js"} {
		response, err := http.Get(run.URL + runtimeAssetDir + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != 200 || !bytes.Contains(raw, []byte("vue v3.5.44")) {
			t.Fatal("local Vue production build unavailable", name, response.StatusCode)
		}
	}
	response, err := http.Get(run.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if !bytes.Contains(raw, []byte("opsbuddy-preview-health")) || !strings.Contains(response.Header.Get("Content-Security-Policy"), "'unsafe-eval'") {
		t.Fatal("page diagnostics or local compiler unavailable")
	}
	if !strings.Contains(response.Header.Get("Content-Security-Policy"), "frame-src 'none'") {
		t.Fatal("preview boundary changed")
	}
}

func TestNewCodeIntentDoesNotAttachPreviousProject(t *testing.T) {
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "test-model", Family: "openai"})
	a.state.Assets = append(a.state.Assets, Asset{ID: "old-code", ProjectID: "inbox", Kind: "code"})
	a.state.Tasks = append(a.state.Tasks, Task{ID: "old-task", ProjectID: "inbox", Kind: "code", Status: "succeeded", OutputIDs: []string{"old-code"}})
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, tt := range []struct {
		prompt string
		want   int
	}{{"帮我写个贪吃蛇，分别用vue和golang实现", 0}, {"修改按钮的颜色", 1}} {
		task, _, _, err := a.prepareTaskLocked(TaskRequest{ProjectID: "inbox", ParentID: "old-task", ModelID: "test-model", RoleID: "auto", Kind: "auto", AllowRemote: true, Prompt: tt.prompt})
		if err != nil || len(task.InputIDs) != tt.want {
			t.Fatal(tt.prompt, task.InputIDs, err)
		}
	}
}
