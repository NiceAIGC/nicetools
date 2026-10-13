package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"nicetools/backend/internal/config"
	"nicetools/backend/internal/model"
)

type Store struct {
	DB      *gorm.DB
	catalog map[string]model.Catalog
}

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
	models := []any{&model.Category{}, &model.Group{}, &model.User{}, &model.Tool{}, &model.GroupRule{}, &model.Session{}, &model.Audit{}}
	if err = db.AutoMigrate(models...); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return &Store{DB: db, catalog: map[string]model.Catalog{}}, nil
}

var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var username = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)

// initiallyPublic 决定「随版本默认开放」的工具；清单外的新工具默认对所有人关闭。
var initiallyPublic = map[string]bool{
	"photo-watermark": true, "alarm-clock": true, "llm-connectivity": true,
	"llm-cost": true, "prompt-cache-probe": true, "json-value-extractor": true, "text-delimiter": true,
	"doc-scan": true,
}

// EnsureCategory 按名称取或建分类，供种子同步与恢复默认共用。
func EnsureCategory(db *gorm.DB, name string, sort int) (model.Category, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Category{}, nil
	}
	var category model.Category
	err := db.Where("name = ?", name).First(&category).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		category = model.Category{Name: name, Sort: sort}
		if err := db.Create(&category).Error; err != nil {
			return category, err
		}
		return category, nil
	}
	return category, err
}

// migrateLegacyCategory 把旧的 tools.category 文本列迁到分类表后删除旧列。
func migrateLegacyCategory(tx *gorm.DB) error {
	var present int64
	if err := tx.Raw("SELECT COUNT(*) FROM pragma_table_info('tools') WHERE name = 'category'").Scan(&present).Error; err != nil {
		return err
	}
	if present == 0 {
		return nil
	}
	type legacyRow struct {
		ID       string
		Category string
	}
	var rows []legacyRow
	if err := tx.Raw("SELECT id, category FROM tools WHERE category_id IS NULL AND category != ''").Scan(&rows).Error; err != nil {
		return err
	}
	created := make(map[string]uint)
	for _, row := range rows {
		categoryID, ok := created[row.Category]
		if !ok {
			category, err := EnsureCategory(tx, row.Category, len(created))
			if err != nil {
				return err
			}
			categoryID = category.ID
			created[row.Category] = categoryID
		}
		if err := tx.Exec("UPDATE tools SET category_id = ? WHERE id = ?", categoryID, row.ID).Error; err != nil {
			return err
		}
	}
	return tx.Exec("ALTER TABLE tools DROP COLUMN category").Error
}

// Sync 用代码目录播种/清理工具；已存在工具的元数据与策略完全由后台拥有，不覆盖。
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
	defaults := make(map[string]model.Catalog, len(catalog))
	categorySort := make(map[string]int)
	for _, tool := range catalog {
		if !slug.MatchString(tool.ID) || strings.TrimSpace(tool.Name) == "" || seen[tool.ID] {
			return fmt.Errorf("invalid or duplicate catalog tool: %s", tool.ID)
		}
		if tool.Category != "" {
			if _, ok := categorySort[tool.Category]; !ok {
				categorySort[tool.Category] = len(categorySort)
			}
		}
		seen[tool.ID] = true
		ids = append(ids, tool.ID)
		defaults[tool.ID] = tool
	}
	s.catalog = defaults

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
		if err := migrateLegacyCategory(tx); err != nil {
			return err
		}
		for index, meta := range catalog {
			var tool model.Tool
			err := tx.First(&tool, "id = ?", meta.ID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				category, err := EnsureCategory(tx, meta.Category, categorySort[meta.Category])
				if err != nil {
					return err
				}
				tags, err := json.Marshal(meta.Tags)
				if err != nil {
					return err
				}
				public := initiallyPublic[meta.ID]
				tool = model.Tool{
					ID: meta.ID, Name: meta.Name, Description: meta.Description, Emoji: meta.Emoji,
					Tags: string(tags), Sort: index, Enabled: true,
					GuestVisible: public, GuestUse: public, MemberVisible: public, MemberUse: public,
				}
				if category.ID != 0 {
					tool.CategoryID = &category.ID
				}
				if err := tx.Create(&tool).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
		if err := tx.Where("id NOT IN ?", ids).Delete(&model.Tool{}).Error; err != nil {
			return err
		}
		return tx.Where("expires_at <= ?", time.Now()).Delete(&model.Session{}).Error
	})
}

// CatalogDefault 返回代码目录里的默认元数据，用于后台「恢复默认」。
func (s *Store) CatalogDefault(id string) (model.Catalog, bool) {
	meta, ok := s.catalog[id]
	return meta, ok
}

func (s *Store) Close() error {
	db, err := s.DB.DB()
	if err != nil {
		return err
	}
	return db.Close()
}
