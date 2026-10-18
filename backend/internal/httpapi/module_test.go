package httpapi

import (
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nicetools/backend/internal/module"
	"nicetools/backend/internal/service"
)

// futureRow 是 fakeModule 的私有表，遵循 tool_<id>_ 前缀约定。
type futureRow struct {
	ID   uint `gorm:"primaryKey"`
	Note string
}

func (futureRow) TableName() string { return "tool_future_tool_rows" }

// futureModule 挂在测试目录里的 future-tool 上（默认对游客和会员都关闭）。
type futureModule struct{}

func (futureModule) ID() string { return "future-tool" }
func (futureModule) Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&futureRow{})
}
func (futureModule) Register(read, write *gin.RouterGroup, d module.Deps) {
	read.GET("/rows", func(c *gin.Context) {
		var count int64
		if err := d.DB.Model(&futureRow{}).Count(&count).Error; err != nil {
			module.Fail(c, err)
			return
		}
		c.JSON(200, gin.H{"count": count, "authed": module.CurrentUser(c) != nil})
	})
	write.POST("/rows", func(c *gin.Context) {
		if err := d.DB.Create(&futureRow{Note: "x"}).Error; err != nil {
			module.Fail(c, err)
			return
		}
		c.JSON(201, gin.H{"ok": true})
	})
}

// 模块默认继承平台访问策略：未开放时游客 404，开放后可用且能区分游客/登录。
func TestModuleInheritsToolPolicy(t *testing.T) {
	f := setup(t, futureModule{})
	if !f.store.DB.Migrator().HasTable(&futureRow{}) {
		t.Fatal("module table not migrated")
	}
	guest := f.client()
	expect(t, guest.request("GET", "/api/session", nil, false), 200)
	expect(t, guest.request("GET", "/api/tools/future-tool/rows", nil, false), 404)

	admin := f.admin()
	policy := service.PolicyInput{Enabled: true, GuestVisible: true, GuestUse: true, MemberVisible: true, MemberUse: true}
	expect(t, admin.request("PUT", "/api/admin/tools/future-tool", policy, true), 200)

	res := guest.request("GET", "/api/tools/future-tool/rows", nil, false)
	expect(t, res, 200)
	body := decode[struct {
		Count  int  `json:"count"`
		Authed bool `json:"authed"`
	}](t, res)
	if body.Authed || body.Count != 0 {
		t.Fatalf("guest rows: %+v", body)
	}

	member := f.client()
	member.login("admin", adminPassword)
	res = member.request("GET", "/api/tools/future-tool/rows", nil, false)
	expect(t, res, 200)
	if decode[struct {
		Authed bool `json:"authed"`
	}](t, res).Authed != true {
		t.Fatal("member should be authenticated")
	}
	// 平台 /api/tools/:id/access 与模块路由共存，互不影响。
	expect(t, guest.request("GET", "/api/tools/future-tool/access", nil, false), 200)
}

// 模块写路由必须过 CSRF；通过后模块内写入的数据可回读。
func TestModuleWriteRequiresCSRF(t *testing.T) {
	f := setup(t, futureModule{})
	admin := f.admin()
	policy := service.PolicyInput{Enabled: true, GuestVisible: true, GuestUse: true, MemberVisible: true, MemberUse: true}
	expect(t, admin.request("PUT", "/api/admin/tools/future-tool", policy, true), 200)

	c := f.client()
	expect(t, c.request("GET", "/api/session", nil, false), 200)
	expect(t, c.request("POST", "/api/tools/future-tool/rows", map[string]string{}, false), 403)
	expect(t, c.request("POST", "/api/tools/future-tool/rows", map[string]string{}, true), 201)
	res := c.request("GET", "/api/tools/future-tool/rows", nil, false)
	expect(t, res, 200)
	if decode[struct {
		Count int `json:"count"`
	}](t, res).Count != 1 {
		t.Fatal("row not persisted")
	}
}

// 非法或重复的模块 id 在装配期即失败，不留到运行时。
func TestModuleValidation(t *testing.T) {
	f := setup(t)
	bad := New(f.api.S, f.config, f.store.DB, []module.Module{badModule{id: "Bad_ID"}})
	if bad.Err() == nil {
		t.Fatal("invalid module id accepted")
	}
	dup := New(f.api.S, f.config, f.store.DB, []module.Module{futureModule{}, futureModule{}})
	if dup.Err() == nil {
		t.Fatal("duplicate module id accepted")
	}
}

type badModule struct{ id string }

func (m badModule) ID() string                                           { return m.id }
func (m badModule) Migrate(db *gorm.DB) error                            { return nil }
func (m badModule) Register(read, write *gin.RouterGroup, d module.Deps) {}
