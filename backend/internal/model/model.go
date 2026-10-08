package model

import "time"

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

type Tool struct {
	ID                                                        string `gorm:"primaryKey"`
	Name, Description, Emoji, Category, Tags                  string
	Enabled, GuestVisible, GuestUse, MemberVisible, MemberUse bool
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

type Catalog struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Emoji       string   `json:"emoji"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
}
