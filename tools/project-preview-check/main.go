// Runs one explicit code-project fixture in an isolated data directory for browser QA.
// No desktop server or model connection is started. Press Enter to stop the preview.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"opsbuddy/internal/app"
	"os"
	"time"
)

func main() {
	if len(os.Args) == 4 && os.Args[1] == "--open-source" {
		core, err := app.New(os.Args[2], "")
		if err != nil {
			panic(err)
		}
		defer core.Close()
		folder, err := core.CodeSourceDirectory(os.Args[3])
		if err != nil {
			panic(err)
		}
		fmt.Println(folder)
		return
	}
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: project-preview-check fixture.json")
		return
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	var project app.CodeProject
	if err = json.Unmarshal(raw, &project); err != nil {
		panic(err)
	}
	root, err := os.MkdirTemp(".cache", "preview-check-")
	if err != nil {
		panic(err)
	}
	core, err := app.New(root, "")
	if err != nil {
		panic(err)
	}
	defer core.Close()
	call := func(path string, value any) []byte {
		data, _ := json.Marshal(value)
		r := httptest.NewRequest("POST", "http://127.0.0.1/api"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-OpsBuddy-Request", "1")
		w := httptest.NewRecorder()
		core.Handler().ServeHTTP(w, r)
		if w.Code >= 400 {
			panic(w.Body.String())
		}
		return w.Body.Bytes()
	}
	data := call("/code-projects", map[string]any{"projectId": "inbox", "project": project})
	var asset app.Asset
	_ = json.Unmarshal(data, &asset)
	_ = call("/assets/"+asset.ID+"/runtime/start", nil)
	for i := 0; i < 1000; i++ {
		r := httptest.NewRequest("GET", "http://127.0.0.1/api/assets/"+asset.ID+"/runtime", nil)
		w := httptest.NewRecorder()
		core.Handler().ServeHTTP(w, r)
		var state app.CodeRunStatus
		_ = json.Unmarshal(w.Body.Bytes(), &state)
		if state.Status == "failed" {
			panic(state.Error + "\n" + state.Logs)
		}
		if state.Status == "running" {
			out, _ := json.Marshal(map[string]string{"url": state.URL, "title": project.Title, "data": root})
			fmt.Println(string(out))
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	_ = call("/assets/"+asset.ID+"/runtime/stop", nil)
}
