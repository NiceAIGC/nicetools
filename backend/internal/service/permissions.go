package service

import (
	"encoding/json"
	"sort"

	"gorm.io/gorm"
	"nicetools/backend/internal/model"
)

// UncategorizedName 是没有指定分类时的展示名。
const UncategorizedName = "未分类"

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
	CategoryID  *uint    `json:"category_id"`
	Tags        []string `json:"tags"`
	Sort        int      `json:"sort"`
	PolicyInput
}

func categoryName(byID map[uint]model.Category, id *uint) string {
	if id != nil {
		if category, ok := byID[*id]; ok && category.Name != "" {
			return category.Name
		}
	}
	return UncategorizedName
}

func toolAccess(tool model.Tool, category string, canUse bool) ToolAccess {
	tags := []string{}
	_ = json.Unmarshal([]byte(tool.Tags), &tags)
	return ToolAccess{ID: tool.ID, Name: tool.Name, Description: tool.Description, Emoji: tool.Emoji, Category: category, Tags: tags, CanUse: canUse}
}

func policy(tool model.Tool, category string, rules []model.GroupRule) ToolPolicy {
	access := toolAccess(tool, category, false)
	out := ToolPolicy{
		ID: access.ID, Name: access.Name, Description: access.Description, Emoji: access.Emoji,
		Category: access.Category, CategoryID: tool.CategoryID, Tags: access.Tags, Sort: tool.Sort,
		PolicyInput: PolicyInput{Enabled: tool.Enabled, GuestVisible: tool.GuestVisible, GuestUse: tool.GuestUse, MemberVisible: tool.MemberVisible, MemberUse: tool.MemberUse, GroupRules: []RuleInput{}},
	}
	for _, rule := range rules {
		if rule.ToolID == tool.ID {
			out.GroupRules = append(out.GroupRules, RuleInput{GroupID: rule.GroupID, Visible: rule.Visible, Use: rule.Use})
		}
	}
	return out
}

// loadTools 返回按「分类顺序 → 工具顺序」排好的工具，以及分类 id → 分类的映射。
func (s *Service) loadTools() ([]model.Tool, map[uint]model.Category, error) {
	categories := []model.Category{}
	if err := s.S.DB.Order("sort ASC, id ASC").Find(&categories).Error; err != nil {
		return nil, nil, err
	}
	byID := make(map[uint]model.Category, len(categories))
	rank := make(map[uint]int, len(categories))
	for index, category := range categories {
		byID[category.ID] = category
		rank[category.ID] = index
	}
	tools := []model.Tool{}
	if err := s.S.DB.Order("sort ASC, id ASC").Find(&tools).Error; err != nil {
		return nil, nil, err
	}
	rankOf := func(id *uint) int {
		if id != nil {
			if position, ok := rank[*id]; ok {
				return position
			}
		}
		return len(categories)
	}
	sort.SliceStable(tools, func(i, j int) bool {
		left, right := rankOf(tools[i].CategoryID), rankOf(tools[j].CategoryID)
		if left != right {
			return left < right
		}
		if tools[i].Sort != tools[j].Sort {
			return tools[i].Sort < tools[j].Sort
		}
		return tools[i].ID < tools[j].ID
	})
	return tools, byID, nil
}

func (s *Service) Policies() ([]ToolPolicy, error) {
	tools, byID, err := s.loadTools()
	if err != nil {
		return nil, err
	}
	rules := []model.GroupRule{}
	if err := s.S.DB.Order("group_id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	out := make([]ToolPolicy, 0, len(tools))
	for _, tool := range tools {
		out = append(out, policy(tool, categoryName(byID, tool.CategoryID), rules))
	}
	return out, nil
}

func (s *Service) SavePolicy(actorID uint, id string, input PolicyInput) (ToolPolicy, error) {
	var out ToolPolicy
	if (input.GuestUse && !input.GuestVisible) || (input.MemberUse && !input.MemberVisible) || len(input.GroupRules) > 1000 {
		return out, Problem(400, "可用须同时可见，或用户组规则过多")
	}
	seen := make(map[uint]bool, len(input.GroupRules))
	ids := make([]uint, 0, len(input.GroupRules))
	rules := make([]model.GroupRule, 0, len(input.GroupRules))
	for _, rule := range input.GroupRules {
		if seen[rule.GroupID] || rule.GroupID == 0 {
			return out, Problem(400, "用户组规则重复或无效")
		}
		seen[rule.GroupID] = true
		ids = append(ids, rule.GroupID)
		if rule.Use != nil && *rule.Use && (rule.Visible == nil || !*rule.Visible) {
			return out, Problem(400, "用户组允许使用时，必须同时允许可见")
		}
		if rule.Visible != nil || rule.Use != nil {
			rules = append(rules, model.GroupRule{ToolID: id, GroupID: rule.GroupID, Visible: rule.Visible, Use: rule.Use})
		}
	}
	err := s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		var tool model.Tool
		if err := tx.First(&tool, "id = ?", id).Error; err != nil {
			return DBError(err)
		}
		if len(seen) > 0 {
			var count int64
			if err := tx.Model(&model.Group{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(seen)) {
				return Problem(400, "用户组不存在")
			}
		}
		tool.Enabled, tool.GuestVisible, tool.GuestUse, tool.MemberVisible, tool.MemberUse =
			input.Enabled, input.GuestVisible, input.GuestUse, input.MemberVisible, input.MemberUse
		if err := tx.Save(&tool).Error; err != nil {
			return err
		}
		if err := tx.Where("tool_id = ?", id).Delete(&model.GroupRule{}).Error; err != nil {
			return err
		}
		if len(rules) > 0 {
			if err := tx.Create(&rules).Error; err != nil {
				return err
			}
		}
		if err := Audit(tx, actor.Username, "tool.policy", id); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	return s.policyByID(id)
}

// policyByID 重新读取一次，保证返回值带正确的分类名。
func (s *Service) policyByID(id string) (ToolPolicy, error) {
	policies, err := s.Policies()
	if err != nil {
		return ToolPolicy{}, err
	}
	for _, item := range policies {
		if item.ID == id {
			return item, nil
		}
	}
	return ToolPolicy{}, Problem(404, "记录不存在")
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
	tools, byID, err := s.loadTools()
	if err != nil {
		return nil, err
	}
	rules := []model.GroupRule{}
	if user != nil {
		if err := s.S.DB.Find(&rules).Error; err != nil {
			return nil, err
		}
	}
	out := make([]ToolAccess, 0, len(tools))
	for _, tool := range tools {
		visible, usable := accessFor(tool, user, rules)
		if visible {
			out = append(out, toolAccess(tool, categoryName(byID, tool.CategoryID), usable))
		}
	}
	return out, nil
}

func (s *Service) Access(id string, user *model.User) (ToolAccess, error) {
	var tool model.Tool
	if err := s.S.DB.First(&tool, "id = ?", id).Error; err != nil {
		return ToolAccess{}, DBError(err)
	}
	rules := []model.GroupRule{}
	if user != nil {
		if err := s.S.DB.Where("tool_id = ?", id).Find(&rules).Error; err != nil {
			return ToolAccess{}, err
		}
	}
	visible, usable := accessFor(tool, user, rules)
	if !visible {
		return ToolAccess{}, Problem(404, "工具不存在")
	}
	if !usable {
		return ToolAccess{}, Problem(403, "没有使用权限或工具已停用")
	}
	_, byID, err := s.loadTools()
	if err != nil {
		return ToolAccess{}, err
	}
	return toolAccess(tool, categoryName(byID, tool.CategoryID), true), nil
}
