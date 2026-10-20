// Package module 定义工具后端模块的装配契约。
//
// 一个带后端的工具 = internal/tools/<id>/ 下一个实现 Module 的包，
// 在 internal/tools 注册表中加一行即完成装配。模块之间禁止互相 import，
// 共享概念抽到 service 或新建领域包，绝不跨工具读表。
package module

import (
	"errors"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nicetools/backend/internal/config"
	"nicetools/backend/internal/model"
	"nicetools/backend/internal/service"
)

// Module 是一个工具的后端：表迁移与路由都内聚在模块内部。
type Module interface {
	// ID 返回工具 id，必须等于 src/tools/catalog.json 中的 id。
	ID() string
	// Migrate 建表/迁移本工具的表，启动时先于路由装配调用；
	// 表名一律 tool_<id>_ 前缀（id 中的 - 转 _）。
	Migrate(db *gorm.DB) error
	// Register 注册路由，统一挂在 /api/tools/<id>/ 下。
	// read 已带「工具可见且可用」策略判定；write 在其上再加 CSRF 校验，
	// 所有 POST/PUT/DELETE 必须挂 write。
	// pub 不带任何鉴权，供必须被外部系统（如被测网关）直接拉取的资源使用；
	// 模块必须自行用一次性/任务级 token 保护 pub 端点，严禁放任何敏感数据。
	// 不得注册名为 access 的子路径（与平台 /api/tools/:id/access 冲突）。
	Register(read, write, pub *gin.RouterGroup, d Deps)
}

// Deps 是平台提供给模块的共享能力；模块不得另开数据库连接。
type Deps struct {
	DB      *gorm.DB
	Service *service.Service
	Config  config.Config
}

// CurrentUser 返回当前登录用户；游客请求返回 nil。
func CurrentUser(c *gin.Context) *model.User {
	value, ok := c.Get("user")
	if !ok {
		return nil
	}
	user, _ := value.(*model.User)
	return user
}

// Fail 是模块 handler 的唯一错误出口：service.Error 按其状态码渲染，
// 其余错误记日志并返回 500。模块不得自行 c.JSON 写错误状态。
func Fail(c *gin.Context, err error) {
	var problem *service.Error
	if errors.As(err, &problem) {
		c.AbortWithStatusJSON(problem.Status, gin.H{"error": problem.Message})
		return
	}
	log.Printf("request %s %s: %v", c.Request.Method, c.FullPath(), err)
	c.AbortWithStatusJSON(500, gin.H{"error": "服务暂时不可用"})
}

// VerifyCatalog 校验模块与代码目录的双向对齐：
// 模块必须对应目录里 backend=true 的工具；目录里 backend=true 的工具必须有模块。
func VerifyCatalog(modules []Module, entries map[string]model.Catalog) error {
	seen := make(map[string]bool, len(modules))
	for _, m := range modules {
		entry, ok := entries[m.ID()]
		if !ok {
			return fmt.Errorf("module %q missing from tool catalog", m.ID())
		}
		if !entry.Backend {
			return fmt.Errorf("catalog tool %q must set \"backend\": true", m.ID())
		}
		seen[m.ID()] = true
	}
	for id, entry := range entries {
		if entry.Backend && !seen[id] {
			return fmt.Errorf("catalog tool %q declares backend but has no module", id)
		}
	}
	return nil
}
