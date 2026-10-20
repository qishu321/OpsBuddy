package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/xuri/excelize/v2"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makePDF(t *testing.T, path string) {
	t.Helper()
	stream := "BT /F1 18 Tf 50 750 Td (OpsBuddy test document) Tj ET"
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>", fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream), "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objects)+1, xref)
	if e := os.WriteFile(path, b.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
}

func TestPDFOperations(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.pdf")
	second := filepath.Join(dir, "second.pdf")
	makePDF(t, first)
	makePDF(t, second)
	text, e := pdfText(first)
	if e != nil || !strings.Contains(text, "OpsBuddy") {
		t.Fatal(text, e)
	}
	merged := filepath.Join(dir, "merged.pdf")
	if e = pdfProcess(context.Background(), []string{first, second}, merged, "merge", ""); e != nil {
		t.Fatal(e)
	}
	count, e := pdfapi.PageCountFile(context.Background(), merged)
	if e != nil || count != 2 {
		t.Fatal(count, e)
	}
	trimmed := filepath.Join(dir, "trimmed.pdf")
	if e = pdfProcess(context.Background(), []string{merged}, trimmed, "split", "2"); e != nil {
		t.Fatal(e)
	}
	count, e = pdfapi.PageCountFile(context.Background(), trimmed)
	if e != nil || count != 1 {
		t.Fatal(count, e)
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	a, e := New(t.TempDir(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(a.Close)
	return a
}
func request(t *testing.T, a *App, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(payload)
	r := httptest.NewRequest(method, "http://127.0.0.1:47821"+path, bytes.NewReader(b))
	r.Header.Set("X-OpsBuddy-Request", "1")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}
func waitTask(t *testing.T, a *App, taskID string) Task {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, task := range a.snapshot().Tasks {
			if task.ID == taskID && task.Status != "queued" && task.Status != "running" {
				return task
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("task timed out")
	return Task{}
}
func TestOfficeRoundTrip(t *testing.T) {
	d := Deliverable{Title: "季度报告 & <概要>", Sections: []Section{{Heading: "项目进展", Body: "中文内容\n第二段与特殊字符 <>&"}}}
	for _, kind := range []string{"docx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test."+kind)
			var e error
			if kind == "docx" {
				e = docxFile(path, d)
			} else {
				e = pptxFile(path, d)
			}
			if e != nil {
				t.Fatal(e)
			}
			z, e := zip.OpenReader(path)
			if e != nil {
				t.Fatal(e)
			}
			defer z.Close()
			for _, f := range z.File {
				r, e := f.Open()
				if e != nil {
					t.Fatal(e)
				}
				dec := xml.NewDecoder(r)
				for {
					_, e = dec.Token()
					if e == io.EOF {
						break
					}
					if e != nil {
						t.Fatalf("%s: %v", f.Name, e)
					}
				}
				r.Close()
			}
			text, e := readDocument(path, kind)
			if e != nil || !strings.Contains(text, "中文内容") || !strings.Contains(text, "<>&") {
				t.Fatalf("text=%s error=%v", text, e)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "data.xlsx")
	e := xlsxFile(path, Deliverable{Headers: []string{"编号", "金额", "备注"}, Rows: [][]any{{"00123", float64(23), "=HYPERLINK(\"unsafe\")"}}})
	if e != nil {
		t.Fatal(e)
	}
	f, e := excelize.OpenFile(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	v, _ := f.GetCellValue("Sheet1", "A2")
	if v != "00123" {
		t.Fatal("leading zero lost", v)
	}
	formula, _ := f.GetCellFormula("Sheet1", "C2")
	if formula != "" {
		t.Fatal("input became executable formula")
	}
}
func TestTaskPersistenceAndDownload(t *testing.T) {
	a := newTestApp(t)
	w := request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "prompt": "会议纪要\n张三下周完成方案。", "kind": "docx"})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var task Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	task = waitTask(t, a, task.ID)
	if task.Status != "succeeded" || len(task.OutputIDs) != 1 {
		t.Fatalf("%+v", task)
	}
	v, _ := a.asset(task.OutputIDs[0])
	if _, e := a.filePath(v); e != nil {
		t.Fatalf("asset path failure root=%s path=%s: %v", a.root, v.Path, e)
	}
	w = request(t, a, "GET", "/api/assets/"+task.OutputIDs[0]+"/file", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal(w.Code, w.Body.String())
	}
	a.Close()
	loaded, e := New(a.root, a.web)
	if e != nil {
		t.Fatal(e)
	}
	defer loaded.Close()
	if len(loaded.snapshot().Tasks) != 1 || loaded.snapshot().Tasks[0].Status != "succeeded" {
		t.Fatal("persistence failed")
	}
	w = request(t, loaded, "GET", "/api/state", nil)
	if strings.Contains(w.Body.String(), "files/") {
		t.Fatal("internal paths exposed")
	}
}
func TestRequestAndProjectBoundaries(t *testing.T) {
	a := newTestApp(t)
	for _, tc := range []struct{ host, origin, header string }{{"evil.example", "", "1"}, {"127.0.0.1:47821", "https://evil.example", "1"}, {"127.0.0.1:47821", "", ""}} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/api/projects", strings.NewReader(`{"name":"x"}`))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-OpsBuddy-Request", tc.header)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("expected rejection", tc, w.Code)
		}
	}
	w := request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "prompt": "test", "inputIds": []string{"unknown"}})
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	_ = os.WriteFile(outside, []byte("private"), 0600)
	relative, _ := filepath.Rel(a.root, outside)
	if _, e := a.filePath(Asset{Path: relative}); e == nil {
		t.Fatal("path escape allowed")
	}
}
func TestUploadFilenameAndClassification(t *testing.T) {
	a := newTestApp(t)
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	_ = mw.WriteField("projectId", "inbox")
	part, _ := mw.CreateFormFile("files", "../../evil.txt")
	_, _ = part.Write([]byte("hello"))
	_ = mw.Close()
	r := httptest.NewRequest("POST", "http://127.0.0.1:47821/api/assets", &b)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("X-OpsBuddy-Request", "1")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var assets []Asset
	_ = json.Unmarshal(w.Body.Bytes(), &assets)
	if assets[0].Name != "evil.txt" {
		t.Fatal(assets)
	}
	w = request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "prompt": "整理文件", "kind": "organize", "inputIds": []string{assets[0].ID}})
	var task Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	task = waitTask(t, a, task.ID)
	if task.Status != "succeeded" {
		t.Fatal(task.Error)
	}
	v, _ := a.asset(task.OutputIDs[0])
	path, _ := a.filePath(v)
	z, e := zip.OpenReader(path)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	if len(z.File) != 1 || z.File[0].Name != "文档/01_evil.txt" {
		t.Fatal("unexpected archive")
	}
}
func TestModelFamilies(t *testing.T) {
	for _, family := range []string{"openai", "anthropic", "gemini"} {
		t.Run(family, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasPrefix(r.URL.Path, "/v1/") {
					t.Error(r.URL.Path)
				}
				text := `{"title":"测试","sections":[{"heading":"结论","body":"真实模型内容"}]}`
				var response any
				switch family {
				case "openai":
					response = map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": text}}}}
				case "anthropic":
					response = map[string]any{"content": []any{map[string]string{"type": "text", "text": text}}}
				case "gemini":
					response = map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]string{"text": text}}}}}}
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			d, e := generate(context.Background(), Model{Family: family, BaseURL: server.URL + "/v1", ModelID: "test"}, "system", "prompt")
			if e != nil || d.Title != "测试" {
				t.Fatal(d, e)
			}
		})
	}
}
func TestModelErrorsAndConsent(t *testing.T) {
	a := newTestApp(t)
	w := request(t, a, "POST", "/api/models", map[string]string{"name": "local", "family": "openai", "baseUrl": "http://127.0.0.1:9999/v1", "modelId": "test"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var m Model
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	w = request(t, a, "POST", "/api/tasks", map[string]any{"projectId": "inbox", "prompt": "write", "modelId": m.ID})
	if w.Code != 400 {
		t.Fatal("consent was not required")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"not JSON"}}]}`)
	}))
	defer server.Close()
	_, e := generate(context.Background(), Model{Family: "openai", BaseURL: server.URL, ModelID: "test"}, "", "test")
	if e == nil {
		t.Fatal("malformed model result accepted")
	}
}
func TestRecoveryAndLocalTable(t *testing.T) {
	a := newTestApp(t)
	a.mu.Lock()
	a.state.Tasks = append(a.state.Tasks, Task{ID: "unfinished", Status: "running"})
	_ = a.saveLocked()
	a.mu.Unlock()
	a.Close()
	restored, e := New(a.root, a.web)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	if restored.snapshot().Tasks[0].Status != "interrupted" {
		t.Fatal("recovery state invalid")
	}
	d, e := a.localTable(Task{Prompt: "编号,金额\n00123,10\n00124,20"}, nil)
	if e != nil || d.Rows[0][0] != "00123" || d.Rows[0][1] != float64(10) {
		t.Fatal(d, e)
	}
}

func TestMCPEventStreamWithoutClosing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[]}}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, _, e := rpc(ctx, server.URL, "", map[string]any{"method": "tools/list"})
	if e != nil || result["result"] == nil {
		t.Fatal(result, e)
	}
}
