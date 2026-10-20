// Packages an already completed, real collaboration. Does not invoke a model,
// open a desktop window, or edit any running application's database.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"opsbuddy/internal/app"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	runRoot := flag.String("run", "demos/multi-agent-todo/runs/20261020-103853", "completed run directory")
	out := flag.String("out", "demos/multi-agent-todo/todo-demo.opsbuddy-project.json", "portable package path")
	flag.Parse()
	raw, err := os.ReadFile(filepath.Join(*runRoot, "execution.json"))
	if err != nil {
		return err
	}
	var state app.State
	if err = json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if len(state.AgentGroups) != 1 || state.AgentGroups[0].Status != "succeeded" {
		return fmt.Errorf("需要真实成功的单组演示记录")
	}
	g := state.AgentGroups[0]
	var project app.Project
	for _, p := range state.Projects {
		if p.ID == g.ProjectID {
			project = p
		}
	}
	if project.ID == "" {
		return fmt.Errorf("项目记录缺失")
	}
	project.Name = "多 Agent Demo · 办公待办"
	project.Description = "真实 DeepSeek 协作：前端与 Go 后端并行开发、独立审阅、主任务集成；9 项接口和8项页面检查已通过。"
	b := app.ProjectImportBundle{SchemaVersion: 1, ID: "todo-demo-" + filepath.Base(*runRoot), Project: project, EntryTaskID: g.RootTaskID, AgentGroups: []app.AgentGroup{g}}
	for _, t := range state.Tasks {
		if t.ProjectID == g.ProjectID {
			if t.ID == g.RootTaskID {
				t.Title = "办公待办看板 · 多 Agent 演示"
			}
			b.Tasks = append(b.Tasks, t)
		}
	}
	for _, asset := range state.Assets {
		if asset.ProjectID != g.ProjectID {
			continue
		}
		if asset.Kind != "code" || !regexp.MustCompile(`^[a-f0-9]{24}$`).MatchString(asset.ID) {
			return fmt.Errorf("演示代码文件标识不受支持")
		}
		raw, err = os.ReadFile(filepath.Join(*runRoot, ".opsbuddy", "files", asset.ID+"-project", "opsbuddy-project.json"))
		if err != nil {
			return err
		}
		var code app.CodeProject
		if err = json.Unmarshal(raw, &code); err != nil {
			return err
		}
		asset.Path = ""
		b.Assets = append(b.Assets, app.ProjectImportAsset{Asset: asset, Code: code})
	}
	raw, err = json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(*out, raw, 0600); err != nil {
		return err
	}
	fmt.Println(b.Summary())
	fmt.Println("项目包：", *out)
	return nil
}
