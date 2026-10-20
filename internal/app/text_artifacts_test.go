package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestModelTextFilesAndEditing(t *testing.T) {
	markdown := "# 需求\n\n|事项|状态|\n|---|---|\n|交付|完成|\n\n```js\nconsole.log('中文');\n```"
	page := "<!doctype html><html><head><title>计算器</title></head><body><button onclick=\"this.textContent=2+3\">计算</button></body></html>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		text := markdown
		if strings.Contains(in.Messages[0].Content, "单文件 HTML") {
			text = page
		}
		if strings.Contains(in.Messages[len(in.Messages)-1].Content, "保留中文") && !strings.Contains(in.Messages[len(in.Messages)-1].Content, "console.log('中文')") {
			t.Error("previous source missing")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": text}}}})
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "text-model", Family: "openai", BaseURL: server.URL, ModelID: "mock"})
	create := func(prompt, parent string) Task {
		w := request(t, a, "POST", "/api/tasks", TaskRequest{ProjectID: "inbox", Kind: "auto", RoleID: "auto", ModelID: "text-model", AllowRemote: true, Prompt: prompt, ParentID: parent})
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		var queued Task
		_ = json.Unmarshal(w.Body.Bytes(), &queued)
		return waitTask(t, a, queued.ID)
	}
	first := create("生成 md 格式需求文档", "")
	if first.Status != "succeeded" || first.Kind != "md" || len(first.OutputIDs) != 1 {
		t.Fatalf("%+v", first)
	}
	second := create("修改措辞，保留中文", first.ID)
	if second.Status != "succeeded" || second.Kind != "md" {
		t.Fatalf("%+v", second)
	}
	w := request(t, a, "GET", "/api/assets/"+first.OutputIDs[0]+"/text", nil)
	var text struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &text)
	if text.Content != markdown {
		t.Fatal("Markdown source changed")
	}
	w = request(t, a, "POST", "/api/assets/"+first.OutputIDs[0]+"/versions", map[string]string{"content": markdown + "\n新版本"})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var version Asset
	_ = json.Unmarshal(w.Body.Bytes(), &version)
	if version.ParentID != first.OutputIDs[0] || version.Kind != "md" {
		t.Fatal("version ancestry missing")
	}
	_, path, err := a.LocalAsset(first.OutputIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(path)
	if string(original) != markdown {
		t.Fatal("original overwritten")
	}
	web := create("做一个HTML计算器网页", "")
	if web.Status != "succeeded" || web.Kind != "html" {
		t.Fatalf("%+v", web)
	}
	_, path, err = a.LocalAsset(web.OutputIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != artifactBody(page, "html") {
		t.Fatal("HTML changed on disk")
	}
	if a.snapshot().Tasks[0].Config.Mode != "model" {
		t.Fatal("text task used local fallback")
	}
}

func TestTextArtifactsLimitsAndNoModel(t *testing.T) {
	a := newTestApp(t)
	for _, kind := range []string{"md", "html"} {
		w := request(t, a, "POST", "/api/tasks", TaskRequest{ProjectID: "inbox", Kind: kind, Prompt: "生成文件"})
		if w.Code != 400 {
			t.Fatal("missing model accepted", kind, w.Code)
		}
	}
	for _, tc := range []struct{ kind, body string }{{"html", "<p>fragment</p>"}, {"md", " "}, {"md", string([]byte{0xff})}, {"md", strings.Repeat("x", maxTextArtifact+1)}, {"docx", "text"}} {
		if validateTextArtifact(tc.kind, tc.body) == nil {
			t.Fatal("invalid file accepted", tc.kind)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.saveTextArtifact(ctx, "inbox", "", "", "cancel", "md", "# body"); err == nil {
		t.Fatal("cancelled write accepted")
	}
	code := "```js\nconsole.log('preserve');\n```"
	if artifactBody(code, "md") != code {
		t.Fatal("Markdown code fence removed")
	}
	if inferKind("生成MD文档") != "md" || inferKind("做一个网页") != "html" {
		t.Fatal("text format routing failed")
	}
}
