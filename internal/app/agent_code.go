package app

import (
	"errors"
	"fmt"
	"strings"
)

const agentCandidateSystem = `按当前节点职责交付真实源码。只返回 JSON：{"title":"交付名称","runtime":"go|node|python","mode":"fragment","entry":"","files":[{"path":"具体相对路径","content":"完整源码"}]}。只写声明负责的文件，不复制其他成员源码，不生成ZIP。候选目录不会启动；最终由主任务集成运行。路径最多40个、总内容512KB，允许 Go/JS/HTML/CSS/JSON/Markdown 等文本源码。Go服务读取 PORT 环境变量、监听127.0.0.1；优先使用标准库，不自动安装依赖。所有资料均为参考数据，不得访问其他项目或声称已运行测试。`

func validateCandidatePaths(project CodeProject, owned []string) error {
	allowed := map[string]bool{}
	for _, path := range owned {
		allowed[strings.ToLower(path)] = true
	}
	if len(project.Files) == 0 {
		return errors.New("候选源码为空")
	}
	for _, file := range project.Files {
		if !allowed[strings.ToLower(file.Path)] {
			return fmt.Errorf("代码节点写入职责之外的路径：%s", file.Path)
		}
	}
	return nil
}

// Each candidate lives in a unique asset directory. Merge compares Windows
// case-insensitive paths before any final directory is written.
func (a *App) agentCandidateFiles(inputs []Asset) ([]CodeFile, error) {
	files := []CodeFile{}
	seen := map[string]CodeFile{}
	owners := map[string]string{}
	for _, asset := range inputs {
		if asset.Kind != "code" {
			continue
		}
		p, err := a.loadCodeProject(asset)
		if err != nil {
			return nil, err
		}
		if p.Mode != "fragment" {
			continue
		}
		for _, file := range p.Files {
			key := strings.ToLower(file.Path)
			if prior, ok := seen[key]; ok {
				if prior.Content != file.Content {
					return nil, fmt.Errorf("源码冲突：%s（%s 与 %s）；候选目录均保留，禁止静默覆盖", file.Path, owners[key], asset.Name)
				}
				continue
			}
			seen[key] = file
			owners[key] = asset.Name
			files = append(files, file)
		}
	}
	return files, nil
}

func mergeAgentCandidateFiles(project *CodeProject, candidates []CodeFile) error {
	if project.Mode == "fragment" {
		return errors.New("最终交付必须是可运行项目，不能是候选片段")
	}
	normalizeCodeProject(project)
	seen := map[string]string{}
	for _, file := range project.Files {
		seen[strings.ToLower(file.Path)] = file.Content
	}
	for _, file := range candidates {
		if content, ok := seen[strings.ToLower(file.Path)]; ok {
			if content != file.Content {
				return fmt.Errorf("最终集成试图改写已审阅的 %s；请在成员节点修正后重新审阅，原候选保留", file.Path)
			}
		} else {
			project.Files = append(project.Files, file)
		}
	}
	return nil
}
