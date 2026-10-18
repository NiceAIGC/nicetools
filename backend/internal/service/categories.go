package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"nicetools/backend/internal/model"
	"nicetools/backend/internal/store"
)

type CategoryInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Service) Categories() ([]model.Category, error) {
	categories := []model.Category{}
	err := s.S.DB.Order("sort ASC, id ASC").Find(&categories).Error
	return categories, err
}

func (s *Service) SaveCategory(actorID uint, id uint, input CategoryInput) (model.Category, error) {
	var out model.Category
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || !Bounded(input.Name, 32) || !Bounded(input.Description, 200) {
		return out, Problem(400, "分类名须为 1–32 字，说明不超过 200 字")
	}
	err := s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		action := "category.create"
		if id == 0 {
			next, err := nextCategorySort(tx)
			if err != nil {
				return err
			}
			out = model.Category{Name: input.Name, Description: input.Description, Sort: next}
			if err := tx.Create(&out).Error; err != nil {
				return DBError(err)
			}
		} else {
			if err := tx.First(&out, id).Error; err != nil {
				return DBError(err)
			}
			out.Name, out.Description = input.Name, input.Description
			if err := tx.Save(&out).Error; err != nil {
				return DBError(err)
			}
			action = "category.update"
		}
		return Audit(tx, actor.Username, action, out.Name)
	})
	return out, err
}

// nextCategorySort 新分类排在末尾，顺序统一由「上移/下移」维护。
func nextCategorySort(tx *gorm.DB) (int, error) {
	var largest int
	if err := tx.Model(&model.Category{}).Select("COALESCE(MAX(sort), 0)").Scan(&largest).Error; err != nil {
		return 0, err
	}
	return largest + 10, nil
}

// DeleteCategory 删除分类；原属该分类的工具改为「未分类」，不删除工具。
func (s *Service) DeleteCategory(actorID uint, id uint) error {
	return s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		var category model.Category
		if err := tx.First(&category, id).Error; err != nil {
			return DBError(err)
		}
		if err := tx.Model(&model.Tool{}).Where("category_id = ?", id).Update("category_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Delete(&category).Error; err != nil {
			return err
		}
		return Audit(tx, actor.Username, "category.delete", category.Name)
	})
}

// ReorderCategories 按给定顺序重排分类，排序值整体重编号，避免同值无法交换。
func (s *Service) ReorderCategories(actorID uint, ids []uint) error {
	return s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		categories := []model.Category{}
		if err := tx.Order("sort ASC, id ASC").Find(&categories).Error; err != nil {
			return err
		}
		if len(ids) != len(categories) {
			return Problem(400, "分类顺序不完整")
		}
		position := make(map[uint]int, len(ids))
		for index, id := range ids {
			if _, ok := position[id]; ok {
				return Problem(400, "分类顺序包含重复项")
			}
			position[id] = index
		}
		for _, category := range categories {
			index, ok := position[category.ID]
			if !ok {
				return Problem(400, "分类顺序包含未知分类")
			}
			if category.Sort == index*10 {
				continue
			}
			if err := tx.Model(&category).Update("sort", index*10).Error; err != nil {
				return err
			}
		}
		return Audit(tx, actor.Username, "category.reorder", fmt.Sprintf("%d 个分类", len(categories)))
	})
}

type ToolMetaInput struct {
	Name        string   `json:"name"`
	Emoji       string   `json:"emoji"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	CategoryID  *uint    `json:"category_id"`
	Sort        int      `json:"sort"`
}

func validateMeta(input *ToolMetaInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Emoji = strings.TrimSpace(input.Emoji)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || !Bounded(input.Name, 60) {
		return Problem(400, "工具名称须为 1–60 字")
	}
	if input.Emoji == "" || utf8.RuneCountInString(input.Emoji) > 8 {
		return Problem(400, "Logo/图标须为 1–8 个字符")
	}
	if !Bounded(input.Description, 300) {
		return Problem(400, "工具说明不超过 300 字")
	}
	if len(input.Tags) > 12 {
		return Problem(400, "标签最多 12 个")
	}
	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || !Bounded(tag, 24) {
			return Problem(400, "每个标签须为 1–24 字")
		}
	}
	if input.Sort < 0 || input.Sort > 9999 {
		return Problem(400, "排序值为 0–9999")
	}
	return nil
}

func (s *Service) SaveToolMeta(actorID uint, id string, input ToolMetaInput) (ToolPolicy, error) {
	if err := validateMeta(&input); err != nil {
		return ToolPolicy{}, err
	}
	tags := make([]string, 0, len(input.Tags))
	seen := make(map[string]bool, len(input.Tags))
	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	encoded, err := json.Marshal(tags)
	if err != nil {
		return ToolPolicy{}, err
	}
	err = s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		var tool model.Tool
		if err := tx.First(&tool, "id = ?", id).Error; err != nil {
			return DBError(err)
		}
		if input.CategoryID != nil {
			var count int64
			if err := tx.Model(&model.Category{}).Where("id = ?", *input.CategoryID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return Problem(400, "分类不存在")
			}
		}
		tool.Name, tool.Emoji, tool.Description = input.Name, input.Emoji, input.Description
		tool.Tags, tool.CategoryID, tool.Sort = string(encoded), input.CategoryID, input.Sort
		if err := tx.Save(&tool).Error; err != nil {
			return err
		}
		return Audit(tx, actor.Username, "tool.meta", tool.ID)
	})
	if err != nil {
		return ToolPolicy{}, err
	}
	return s.policyByID(id)
}

// ResetToolMeta 把展示信息恢复为代码目录里的默认值，不动访问策略。
func (s *Service) ResetToolMeta(actorID uint, id string) (ToolPolicy, error) {
	meta, ok := s.S.CatalogDefault(id)
	if !ok {
		return ToolPolicy{}, Problem(404, "代码目录中没有该工具的默认信息")
	}
	tags, err := json.Marshal(meta.Tags)
	if err != nil {
		return ToolPolicy{}, err
	}
	err = s.S.DB.Transaction(func(tx *gorm.DB) error {
		actor, err := administrator(tx, actorID)
		if err != nil {
			return err
		}
		var tool model.Tool
		if err := tx.First(&tool, "id = ?", id).Error; err != nil {
			return DBError(err)
		}
		next, err := nextCategorySort(tx)
		if err != nil {
			return err
		}
		category, err := store.EnsureCategory(tx, meta.Category, next)
		if err != nil {
			return err
		}
		tool.Name, tool.Emoji, tool.Description = meta.Name, meta.Emoji, meta.Description
		tool.Tags, tool.Sort = string(tags), 0
		if category.ID != 0 {
			tool.CategoryID = &category.ID
		} else {
			tool.CategoryID = nil
		}
		if err := tx.Save(&tool).Error; err != nil {
			return err
		}
		return Audit(tx, actor.Username, "tool.meta_reset", tool.ID)
	})
	if err != nil {
		return ToolPolicy{}, err
	}
	return s.policyByID(id)
}
