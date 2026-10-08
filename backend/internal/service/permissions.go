package service

import (
	"encoding/json"
	"strings"

	"gorm.io/gorm"
	"nicetools/backend/internal/model"
)

type GroupInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Service) Groups() ([]model.Group, error) {
	groups := []model.Group{}
	err := s.S.DB.Order("id ASC").Find(&groups).Error
	return groups, err
}
func (s *Service) SaveGroup(actorID, id uint, input GroupInput) (model.Group, error) {
	var out model.Group
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || !bounded(input.Name, 64) || !bounded(input.Description, 500) {
		return out, problem(400, "组名须为 1–64 字，说明最多 500 字")
	}
	err := s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		action := "group.create"
		if id == 0 {
			out = model.Group{Name: input.Name, Description: input.Description}
			if err := tx.Create(&out).Error; err != nil {
				return dbError(err)
			}
		} else {
			if err := tx.First(&out, id).Error; err != nil {
				return dbError(err)
			}
			out.Name, out.Description = input.Name, input.Description
			if err := tx.Save(&out).Error; err != nil {
				return dbError(err)
			}
			action = "group.update"
		}
		return audit(tx, actor.Username, action, out.Name)
	})
	return out, err
}
func (s *Service) DeleteGroup(actorID, id uint) error {
	return s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		var group model.Group
		if err := tx.First(&group, id).Error; err != nil {
			return dbError(err)
		}
		if err := tx.Where("group_id = ?", id).Delete(&model.GroupRule{}).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM user_groups WHERE group_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&group).Error; err != nil {
			return err
		}
		return audit(tx, actor.Username, "group.delete", group.Name)
	})
}

type ToolAccess struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Emoji       string   `json:"emoji"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	CanUse      bool     `json:"can_use"`
}

type RuleInput struct {
	GroupID uint  `json:"group_id"`
	Visible *bool `json:"visible"`
	Use     *bool `json:"use"`
}

type PolicyInput struct {
	Enabled       bool        `json:"enabled"`
	GuestVisible  bool        `json:"guest_visible"`
	GuestUse      bool        `json:"guest_use"`
	MemberVisible bool        `json:"member_visible"`
	MemberUse     bool        `json:"member_use"`
	GroupRules    []RuleInput `json:"group_rules"`
}

type ToolPolicy struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Emoji       string   `json:"emoji"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	PolicyInput
}

func toolAccess(tool model.Tool, canUse bool) ToolAccess {
	tags := []string{}
	_ = json.Unmarshal([]byte(tool.Tags), &tags)
	return ToolAccess{ID: tool.ID, Name: tool.Name, Description: tool.Description, Emoji: tool.Emoji, Category: tool.Category, Tags: tags, CanUse: canUse}
}
func policy(tool model.Tool, rules []model.GroupRule) ToolPolicy {
	access := toolAccess(tool, false)
	out := ToolPolicy{ID: access.ID, Name: access.Name, Description: access.Description, Emoji: access.Emoji, Category: access.Category, Tags: access.Tags, PolicyInput: PolicyInput{Enabled: tool.Enabled, GuestVisible: tool.GuestVisible, GuestUse: tool.GuestUse, MemberVisible: tool.MemberVisible, MemberUse: tool.MemberUse, GroupRules: []RuleInput{}}}
	for _, rule := range rules {
		if rule.ToolID == tool.ID {
			out.GroupRules = append(out.GroupRules, RuleInput{GroupID: rule.GroupID, Visible: rule.Visible, Use: rule.Use})
		}
	}
	return out
}
func (s *Service) Policies() ([]ToolPolicy, error) {
	tools := []model.Tool{}
	rules := []model.GroupRule{}
	if err := s.S.DB.Order("id ASC").Find(&tools).Error; err != nil {
		return nil, err
	}
	if err := s.S.DB.Order("group_id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	out := make([]ToolPolicy, 0, len(tools))
	for _, tool := range tools {
		out = append(out, policy(tool, rules))
	}
	return out, nil
}
func (s *Service) SavePolicy(actorID uint, id string, input PolicyInput) (ToolPolicy, error) {
	var out ToolPolicy
	if (input.GuestUse && !input.GuestVisible) || (input.MemberUse && !input.MemberVisible) || len(input.GroupRules) > 1000 {
		return out, problem(400, "可用须同时可见，或用户组规则过多")
	}
	ids := make([]uint, 0, len(input.GroupRules))
	seen := make(map[uint]bool, len(input.GroupRules))
	for _, rule := range input.GroupRules {
		if seen[rule.GroupID] || rule.GroupID == 0 {
			return out, problem(400, "用户组规则重复或无效")
		}
		seen[rule.GroupID] = true
		ids = append(ids, rule.GroupID)
		if rule.Use != nil && *rule.Use && (rule.Visible == nil || !*rule.Visible) {
			return out, problem(400, "用户组允许使用时，必须同时允许可见")
		}
	}
	err := s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		var tool model.Tool
		if err := tx.First(&tool, "id = ?", id).Error; err != nil {
			return dbError(err)
		}
		if len(ids) > 0 {
			var count int64
			if err := tx.Model(&model.Group{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return problem(400, "用户组不存在")
			}
		}
		tool.Enabled, tool.GuestVisible, tool.GuestUse, tool.MemberVisible, tool.MemberUse = input.Enabled, input.GuestVisible, input.GuestUse, input.MemberVisible, input.MemberUse
		if err := tx.Save(&tool).Error; err != nil {
			return err
		}
		if err := tx.Where("tool_id = ?", id).Delete(&model.GroupRule{}).Error; err != nil {
			return err
		}
		rules := []model.GroupRule{}
		for _, rule := range input.GroupRules {
			if rule.Visible != nil || rule.Use != nil {
				rules = append(rules, model.GroupRule{ToolID: id, GroupID: rule.GroupID, Visible: rule.Visible, Use: rule.Use})
			}
		}
		if len(rules) > 0 {
			if err := tx.Create(&rules).Error; err != nil {
				return err
			}
		}
		if err := audit(tx, actor.Username, "tool.policy", id); err != nil {
			return err
		}
		out = policy(tool, rules)
		return nil
	})
	return out, err
}

// 每个旗标分别决策：显式拒绝 > 显式允许 > 会员默认；可用最终与可见相交。
func accessFor(tool model.Tool, user *model.User, rules []model.GroupRule) (visible, usable bool) {
	if user != nil && user.Role == "admin" && !user.Disabled {
		return true, tool.Enabled
	}
	if !tool.Enabled || (user != nil && user.Disabled) {
		return false, false
	}
	if user == nil {
		return tool.GuestVisible, tool.GuestVisible && tool.GuestUse
	}
	visibleAllowed, useAllowed := tool.MemberVisible, tool.MemberUse
	visibleDenied, useDenied := false, false
	for _, rule := range rules {
		if rule.ToolID != tool.ID {
			continue
		}
		for _, group := range user.Groups {
			if group.ID == rule.GroupID {
				if rule.Visible != nil {
					if *rule.Visible {
						visibleAllowed = true
					} else {
						visibleDenied = true
					}
				}
				if rule.Use != nil {
					if *rule.Use {
						useAllowed = true
					} else {
						useDenied = true
					}
				}
				break
			}
		}
	}
	visible = visibleAllowed && !visibleDenied
	return visible, visible && useAllowed && !useDenied
}
func (s *Service) Tools(user *model.User) ([]ToolAccess, error) {
	tools := []model.Tool{}
	rules := []model.GroupRule{}
	if err := s.S.DB.Order("id ASC").Find(&tools).Error; err != nil {
		return nil, err
	}
	if user != nil {
		if err := s.S.DB.Find(&rules).Error; err != nil {
			return nil, err
		}
	}
	out := make([]ToolAccess, 0, len(tools))
	for _, tool := range tools {
		visible, usable := accessFor(tool, user, rules)
		if visible {
			out = append(out, toolAccess(tool, usable))
		}
	}
	return out, nil
}
func (s *Service) Access(id string, user *model.User) (ToolAccess, error) {
	var tool model.Tool
	if err := s.S.DB.First(&tool, "id = ?", id).Error; err != nil {
		return ToolAccess{}, dbError(err)
	}
	rules := []model.GroupRule{}
	if user != nil {
		if err := s.S.DB.Where("tool_id = ?", id).Find(&rules).Error; err != nil {
			return ToolAccess{}, err
		}
	}
	visible, usable := accessFor(tool, user, rules)
	if !visible {
		return ToolAccess{}, problem(404, "工具不存在")
	}
	if !usable {
		return ToolAccess{}, problem(403, "没有使用权限或工具已停用")
	}
	return toolAccess(tool, true), nil
}
