package app

import (
	"fmt"
	"strings"
)

// Keep role records for historical IDs and saved automation; surface reusable
// custom instructions in the existing Skills UI instead of a second library.
func migrateRoleSkills(state *State) {
	for i := range state.Roles {
		role := &state.Roles[i]
		if role.Builtin || role.MigratedSkillID != "" {
			continue
		}
		role.MigratedSkillID = "migrated-role-" + role.ID
		exists := false
		for _, x := range state.Extensions {
			if x.ID == role.MigratedSkillID {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		body := strings.TrimSpace(role.Instructions)
		if body == "" {
			body = "依据当前用户要求，结合所引用的技能与材料完成任务。"
		}
		state.Extensions = append(state.Extensions, Extension{
			ID: role.MigratedSkillID, Name: role.Name, Kind: "skill", Description: role.Description,
			Content: "# " + role.Name + "\n\n" + body, Enabled: true, Version: "1.0.0", Status: "原自定义配置 · 已迁移", Category: "自定义", SourceRoleID: role.ID,
			Dependencies: append([]string{}, role.SkillIDs...), KnowledgeIDs: append([]string{}, role.KnowledgeIDs...),
		})
	}
}

func migratedRoleGuidance(role Role) Role {
	if role.MigratedSkillID != "" {
		role.Instructions = ""
		role.SkillIDs = []string{role.MigratedSkillID}
		role.KnowledgeIDs = nil
	}
	return role
}

func expandSkillLinks(ids []string, extensions []Extension) ([]string, []string, error) {
	byID := map[string]Extension{}
	for _, x := range extensions {
		byID[x.ID] = x
	}
	seen, visiting := map[string]bool{}, map[string]bool{}
	out, bases := []string{}, []string{}
	var visit func(string) error
	visit = func(sid string) error {
		if visiting[sid] {
			return fmt.Errorf("技能关联出现循环：%s", sid)
		}
		if seen[sid] {
			return nil
		}
		x, ok := byID[sid]
		if !ok {
			return fmt.Errorf("所选技能 %s 已不存在，请重新选择", sid)
		}
		seen[sid] = true
		out = append(out, sid)
		if len(out) > 16 {
			return fmt.Errorf("关联技能超过 16 项，请减少引用")
		}
		if !x.Enabled {
			return nil
		}
		if x.SourceRoleID == "" {
			return nil
		}
		visiting[sid] = true
		for _, child := range x.Dependencies {
			if err := visit(child); err != nil {
				return err
			}
		}
		delete(visiting, sid)
		bases = append(bases, x.KnowledgeIDs...)
		return nil
	}
	for _, sid := range ids {
		if err := visit(sid); err != nil {
			return nil, nil, err
		}
	}
	return out, uniqueIDs(bases), nil
}
