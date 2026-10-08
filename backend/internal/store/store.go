package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"nicetools/backend/internal/config"
	"nicetools/backend/internal/model"
)

type Store struct{ DB *gorm.DB }

func Open(c config.Config) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(c.DBPath), 0700); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(c.DBPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000"), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// 单副本 SQLite；序列化写事务保证“最后管理员”检查和修改不可交错。
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err = db.AutoMigrate(&model.Group{}, &model.User{}, &model.Tool{}, &model.GroupRule{}, &model.Session{}, &model.Audit{}); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var username = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)
var initiallyPublic = map[string]bool{
	"photo-watermark": true, "alarm-clock": true, "llm-connectivity": true,
	"llm-cost": true, "prompt-cache-probe": true, "json-value-extractor": true, "text-delimiter": true,
}

func (s *Store) Sync(path, adminUser, adminPass string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var catalog []model.Catalog
	if err = json.Unmarshal(data, &catalog); err != nil || len(catalog) == 0 {
		return errors.New("invalid or empty tool catalog")
	}
	ids := make([]string, 0, len(catalog))
	seen := make(map[string]bool, len(catalog))
	for _, tool := range catalog {
		if !slug.MatchString(tool.ID) || tool.Name == "" || seen[tool.ID] {
			return fmt.Errorf("invalid or duplicate catalog tool: %s", tool.ID)
		}
		seen[tool.ID] = true
		ids = append(ids, tool.ID)
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.User{}).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if !username.MatchString(adminUser) || len(adminPass) < 12 || len(adminPass) > 72 {
				return errors.New("bootstrap requires ADMIN_USERNAME (3..32 letters/digits/_.-) and ADMIN_PASSWORD (12..72 bytes)")
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			user := model.User{Username: adminUser, DisplayName: adminUser, PasswordHash: string(hash), Role: "admin"}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.Audit{Actor: adminUser, Action: "admin.bootstrap", Target: adminUser}).Error; err != nil {
				return err
			}
		}
		for _, meta := range catalog {
			var tool model.Tool
			err := tx.First(&tool, "id = ?", meta.ID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				public := initiallyPublic[meta.ID]
				tool = model.Tool{ID: meta.ID, Enabled: true, GuestVisible: public, GuestUse: public, MemberVisible: public, MemberUse: public}
			} else if err != nil {
				return err
			}
			tags, err := json.Marshal(meta.Tags)
			if err != nil {
				return err
			}
			tool.Name, tool.Description, tool.Emoji, tool.Category, tool.Tags = meta.Name, meta.Description, meta.Emoji, meta.Category, string(tags)
			if err := tx.Save(&tool).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id NOT IN ?", ids).Delete(&model.Tool{}).Error; err != nil {
			return err
		}
		return tx.Where("expires_at <= ?", time.Now()).Delete(&model.Session{}).Error
	})
}

func (s *Store) Close() error {
	db, err := s.DB.DB()
	if err != nil {
		return err
	}
	return db.Close()
}
