package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWeatherRoutingAndModelContext(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Messages[len(req.Messages)-1].Content, "22.1°C") {
			t.Error("real weather tool result missing from selected model request")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "上海当前阴，22.1°C。来源：Open-Meteo。"}}}})
	}))
	defer server.Close()
	a := newTestApp(t)
	a.state.Models = append(a.state.Models, Model{ID: "wx-model", Family: "openai", BaseURL: server.URL, ModelID: "selected-model"})
	a.state.Tasks = append(a.state.Tasks, Task{ID: "old-html", ProjectID: "inbox", Kind: "html", Status: "succeeded", OutputIDs: []string{"missing-old-output"}})
	a.mu.Lock()
	prepared, inputs, _, err := a.prepareTaskLocked(TaskRequest{ProjectID: "inbox", Kind: "auto", RoleID: "auto", ModelID: "wx-model", AllowRemote: true, Prompt: "上海今天天气", ParentID: "old-html"})
	a.mu.Unlock()
	if err != nil || prepared.Kind != "chat" || len(inputs) != 0 {
		t.Fatalf("weather inherited old HTML or attachments: %+v %v", prepared, err)
	}
	a.mu.Lock()
	prepared, _, _, err = a.prepareTaskLocked(TaskRequest{ProjectID: "inbox", Kind: "auto", RoleID: "auto", ModelID: "wx-model", AllowRemote: true, Prompt: "生成上海天气网页"})
	a.mu.Unlock()
	if err != nil || prepared.Kind != "html" {
		t.Fatalf("explicit weather page intent lost: %+v %v", prepared, err)
	}
	a.weatherFetch = func(ctx context.Context, city string) (string, error) {
		if city != "Shanghai" {
			t.Errorf("wrong city %s", city)
		}
		return "上海：阴，22.1°C。来源：https://open-meteo.com/，数据时间：" + time.Now().Format(time.RFC3339), nil
	}
	w := request(t, a, "POST", "/api/tasks", TaskRequest{ProjectID: "inbox", Kind: "html", RoleID: "auto", ModelID: "wx-model", AllowRemote: true, Prompt: "{{上海天气}}"})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var queued Task
	_ = json.Unmarshal(w.Body.Bytes(), &queued)
	task := waitTask(t, a, queued.ID)
	if task.Status != "succeeded" || task.Kind != "chat" || len(task.OutputIDs) != 0 || !strings.Contains(task.Result, "22.1°C") {
		t.Fatalf("weather became fake file: %+v", task)
	}
	if calls != 1 {
		t.Fatalf("expected selected model response, calls=%d", calls)
	}
	a.weatherFetch = func(context.Context, string) (string, error) { return "", errors.New("offline") }
	w = request(t, a, "POST", "/api/tasks", TaskRequest{ProjectID: "inbox", Kind: "auto", RoleID: "auto", ModelID: "wx-model", AllowRemote: true, Prompt: "上海今天天气", ParentID: task.ID})
	_ = json.Unmarshal(w.Body.Bytes(), &queued)
	task = waitTask(t, a, queued.ID)
	if task.Status != "failed" || len(task.OutputIDs) != 0 || calls != 1 || !strings.Contains(task.Error, "未生成模拟天气") {
		t.Fatalf("offline generated fake success: %+v", task)
	}
}

func TestWeatherProviderValidation(t *testing.T) {
	mode := "valid"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("model credentials sent to weather provider")
		}
		if r.URL.Path == "/geo" {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"name": "上海", "country": "中国", "latitude": 31.22, "longitude": 121.46}}})
			return
		}
		if mode == "http" {
			w.WriteHeader(503)
			return
		}
		if mode == "invalid" {
			_, _ = w.Write([]byte("not json"))
			return
		}
		stamp := time.Now().Unix()
		if mode == "stale" {
			stamp -= 86400
		}
		var temp any = 22.1
		if mode == "null" {
			temp = nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"timezone": "Asia/Shanghai", "utc_offset_seconds": 28800, "current": map[string]any{"time": stamp, "temperature_2m": temp, "weather_code": 3}, "daily": map[string]any{"time": []int64{stamp}, "temperature_2m_max": []float64{24}, "temperature_2m_min": []float64{19}, "weather_code": []int{3}, "precipitation_probability_max": []int{20}}})
	}))
	defer server.Close()
	data, err := fetchWeatherFrom(context.Background(), server.Client(), server.URL+"/geo", server.URL+"/forecast", "Shanghai")
	if err != nil || !strings.Contains(data, "22.1°C") || !strings.Contains(data, "Asia/Shanghai") || !strings.Contains(data, "Open-Meteo") {
		t.Fatalf("%s %v", data, err)
	}
	for _, mode = range []string{"http", "invalid", "stale", "null"} {
		if _, err = fetchWeatherFrom(context.Background(), server.Client(), server.URL+"/geo", server.URL+"/forecast", "Shanghai"); err == nil {
			t.Errorf("accepted %s weather", mode)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = fetchWeatherFrom(ctx, server.Client(), server.URL+"/geo", server.URL+"/forecast", "Shanghai"); err == nil {
		t.Error("ignored cancellation")
	}
}

func TestHTMLExtractsPageWithoutCommentary(t *testing.T) {
	page := "<html><head><title>测试</title></head><body><button>计算</button></body></html>"
	wrapped := "这是可运行的页面：\n```html\n<!doctype html>\n" + page + "\n```\n### 使用说明\n点击按钮。"
	got := artifactBody(wrapped, "html")
	if got != "<!doctype html>\n"+page {
		t.Fatalf("prose leaked into HTML: %s", got)
	}
	if err := validateTextArtifact("html", got); err != nil {
		t.Fatal(err)
	}
	if err := validateTextArtifact("html", artifactBody("没有页面源码", "html")); err == nil {
		t.Error("accepted prose as HTML")
	}
}

func TestLiveWeatherOptIn(t *testing.T) {
	if os.Getenv("OPSBUDDY_LIVE_WEATHER") != "1" {
		t.Skip("explicit network verification only")
	}
	data, err := fetchWeather(context.Background(), "Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(data)
}

// Explicitly requested end-to-end check: creates one real conversation in the
// provided desktop data directory and uses its existing single model connection.
// Never runs in the ordinary suite or reads/logs a plaintext credential.
func TestLiveWeatherTaskOptIn(t *testing.T) {
	root := os.Getenv("OPSBUDDY_LIVE_TASK_ROOT")
	if root == "" {
		t.Skip("explicit model-backed task verification only")
	}
	a, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if len(a.state.Models) != 1 {
		t.Fatal("expected one unambiguous configured model")
	}
	model := a.state.Models[0]
	useMemory := false
	w := request(t, a, "POST", "/api/tasks", TaskRequest{ProjectID: "inbox", Kind: "auto", RoleID: "auto", ModelID: model.ID, AllowRemote: true, UseMemory: &useMemory, Prompt: "上海今天天气怎么样？请根据刚查询的数据简短回答当前天气和今日温度范围，附数据时间和来源。"})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var queued Task
	if err := json.Unmarshal(w.Body.Bytes(), &queued); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		for _, task := range a.snapshot().Tasks {
			if task.ID != queued.ID || activeTask(task.Status) {
				continue
			}
			if task.Status != "succeeded" || task.Kind != "chat" || len(task.OutputIDs) != 0 {
				t.Fatalf("live weather task failed: %s %s", task.Status, task.Error)
			}
			t.Logf("Model: %s; task: %s; result:\n%s", model.ModelID, task.ID, task.Result)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("live weather task timed out")
}
