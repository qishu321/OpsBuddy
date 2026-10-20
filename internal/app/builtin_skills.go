package app

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"go.yaml.in/yaml/v3"
)

//go:embed builtin_skills/*/SKILL.md
var bundledSkills embed.FS

type skillManifest struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Metadata    struct {
		DisplayName string `yaml:"display_name"`
		Category    string `yaml:"category"`
		Kinds       string `yaml:"kinds"`
		Keywords    string `yaml:"keywords"`
	} `yaml:"metadata"`
}

func parseSkillManifest(content string) (skillManifest, error) {
	var m skillManifest
	parts := strings.SplitN(strings.ReplaceAll(content, "\r\n", "\n"), "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		return m, fmt.Errorf("SKILL.md 缺少 YAML frontmatter")
	}
	if err := yaml.Unmarshal([]byte(parts[1]), &m); err != nil {
		return m, err
	}
	if m.Name == "" || m.Description == "" || strings.TrimSpace(parts[2]) == "" {
		return m, fmt.Errorf("SKILL.md 需要 name、description 和技能正文")
	}
	return m, nil
}

func builtinSkillCatalog() ([]Extension, error) {
	paths, err := fs.Glob(bundledSkills, "builtin_skills/*/SKILL.md")
	if err != nil {
		return nil, err
	}
	out := []Extension{}
	for _, path := range paths {
		raw, err := bundledSkills.ReadFile(path)
		if err != nil {
			return nil, err
		}
		m, err := parseSkillManifest(string(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		name := m.Metadata.DisplayName
		if name == "" {
			name = m.Name
		}
		out = append(out, Extension{ID: "builtin-skill-" + m.Name, Name: name, Kind: "skill", Description: m.Description, Content: string(raw), Enabled: true, Version: "1.0.0", Status: "内置 · 按需使用", Builtin: true, Category: m.Metadata.Category})
	}
	return out, nil
}

func ensureBuiltinSkills(state *State) error {
	catalog, err := builtinSkillCatalog()
	if err != nil {
		return err
	}
	for _, built := range catalog {
		found := false
		for i, old := range state.Extensions {
			if old.ID == built.ID {
				built.Enabled = old.Enabled
				state.Extensions[i] = built
				found = true
				break
			}
		}
		if !found {
			state.Extensions = append(state.Extensions, built)
		}
	}
	return nil
}

// Route bundled guidance locally, so a normal task needs no extra model request.
// Explicit user/role skill choices take precedence over automatic suggestions.
func builtinSkillsForTask(extensions []Extension, kind, prompt string) []string {
	ids := []string{}
	lower := strings.ToLower(prompt)
	for _, x := range extensions {
		if !x.Builtin || !x.Enabled || x.Kind != "skill" {
			continue
		}
		m, err := parseSkillManifest(x.Content)
		if err != nil {
			continue
		}
		match := strings.Contains(lower, "$"+m.Name)
		for _, k := range strings.Split(m.Metadata.Kinds, ",") {
			if k != "" && k == kind {
				match = true
			}
		}
		for _, word := range strings.Split(m.Metadata.Keywords, ",") {
			if word != "" && strings.Contains(lower, strings.ToLower(strings.TrimSpace(word))) {
				match = true
			}
		}
		if match {
			ids = append(ids, x.ID)
		}
	}
	if len(ids) > 4 {
		ids = ids[:4]
	}
	return ids
}

func skillBody(content string) string {
	parts := strings.SplitN(strings.ReplaceAll(content, "\r\n", "\n"), "---\n", 3)
	if len(parts) == 3 && parts[0] == "" {
		return strings.TrimSpace(parts[2])
	}
	return content
}
