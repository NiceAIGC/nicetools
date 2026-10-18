package model

import "time"

// Category 是工具分类：名称、说明、顺序都由管理员在后台维护。
type Category struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"uniqueIndex;not null" json:"name"`
	Description string    `json:"description"`
	Sort        int       `json:"sort"`
	CreatedAt   time.Time `json:"created_at"`
}

type Group struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"uniqueIndex;not null" json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"-"`
	Role         string    `gorm:"not null;check:role IN ('admin','user')" json:"role"`
	Disabled     bool      `json:"disabled"`
	Groups       []Group   `gorm:"many2many:user_groups;constraint:OnDelete:CASCADE;" json:"groups"`
	CreatedAt    time.Time `json:"created_at"`
}

// Tool 的展示信息（名称/图标/说明/分类/标签/顺序）由后台维护，
// 代码目录只负责种子默认值；访问策略同样是后台数据。
type Tool struct {
	ID            string `gorm:"primaryKey"`
	Name          string
	Description   string
	Emoji         string
	Tags          string
	CategoryID    *uint     `gorm:"index"`
	Category      *Category `gorm:"foreignKey:CategoryID;constraint:OnDelete:SET NULL" json:"-"`
	Sort          int
	Enabled       bool
	GuestVisible  bool
	GuestUse      bool
	MemberVisible bool
	MemberUse     bool
}

type GroupRule struct {
	ToolID       string `gorm:"primaryKey"`
	GroupID      uint   `gorm:"primaryKey"`
	Visible, Use *bool
	Tool         Tool  `gorm:"foreignKey:ToolID;constraint:OnDelete:CASCADE" json:"-"`
	Group        Group `gorm:"foreignKey:GroupID;constraint:OnDelete:CASCADE" json:"-"`
}

type Session struct {
	TokenHash string `gorm:"primaryKey"`
	UserID    *uint  `gorm:"index"`
	CSRFHash  string
	ExpiresAt time.Time `gorm:"index"`
	CreatedAt time.Time
	User      *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

type Audit struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	CreatedAt time.Time `json:"created_at"`
}

// Catalog 是代码目录里携带的默认元数据，用来播种新工具与「恢复默认」。
type Catalog struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Emoji       string   `json:"emoji"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	// Backend 声明该工具带后端模块；启动时校验模块与目录双向对齐。
	Backend bool `json:"backend"`
}
