package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func contentHash(value any) string {
	raw, _ := json.Marshal(value)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func captureTaskConfig(role Role, model Model, extensions []Extension, kind string, allowRemote bool) (*TaskConfig, error) {
	links, bases, err := expandSkillLinks(role.SkillIDs, extensions)
	if err != nil {
		return nil, err
	}
	role.SkillIDs = links
	role.SkillIDs = append([]string{}, role.SkillIDs...)
	role.KnowledgeIDs = uniqueIDs(append(append([]string{}, role.KnowledgeIDs...), bases...))
	c := &TaskConfig{
		LazySkills:    true,
		SchemaVersion: 1, CapturedAt: now(), Executor: "office-v1",
		Mode: "local", OutputKind: kind, AllowRemote: allowRemote,
		Role: role, RoleHash: contentHash(role), Extensions: []TaskExtension{},
	}
	if kind == "chat" {
		c.Executor = "chat-v1"
	}
	if kind == "code" {
		c.Executor = "code-v1"
	}
	if model.ID != "" {
		c.Model = &TaskModelConfig{ID: model.ID, Name: model.Name, Family: model.Family, BaseURL: model.BaseURL, ModelID: model.ModelID}
		if model.Secret != "" {
			c.Model.CredentialRef = model.ID
		}
		if kind != "pdf" && kind != "organize" {
			c.Mode = "model"
		}
	}
	seen := map[string]bool{}
	for _, sid := range role.SkillIDs {
		if seen[sid] {
			continue
		}
		seen[sid] = true
		found := false
		for _, extension := range extensions {
			if extension.ID != sid {
				continue
			}
			found = true
			if !extension.Enabled {
				// Disabled dependencies are excluded from new runs, but existing
				// tasks retain the content that was enabled at submission time.
				continue
			}
			if extension.Kind != "skill" && extension.Kind != "plugin" {
				return nil, fmt.Errorf("角色依赖 %s 不能作为技能使用", extension.Name)
			}
			c.Extensions = append(c.Extensions, TaskExtension{Extension: extension, ContentHash: contentHash(extension.Content)})
		}
		if !found {
			return nil, fmt.Errorf("所选技能 %s 已不存在，请重新选择", sid)
		}
	}
	return c, nil
}

func (c *TaskConfig) runtimeModel(secret string) Model {
	if c.Model == nil || c.Mode != "model" {
		return Model{}
	}
	m := c.Model
	return Model{ID: m.ID, Name: m.Name, Family: m.Family, BaseURL: m.BaseURL, ModelID: m.ModelID, Secret: secret}
}
