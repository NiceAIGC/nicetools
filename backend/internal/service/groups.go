package service

import (
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

func (s *Service) SaveGroup(actorID uint, id uint, input GroupInput) (model.Group, error) {
	var out model.Group
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || !bounded(input.Name, 64) || !bounded(input.Description, 500) {
		return out, problem(400, "组名须为 1–64 字，说明不超过 500 字")
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

// DeleteGroup 删除分组：同时清掉成员归属与该组的工具覆盖规则。
func (s *Service) DeleteGroup(actorID uint, id uint) error {
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
