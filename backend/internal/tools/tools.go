// Package tools 汇总全部工具后端模块。
// 新增带后端的工具：在 internal/tools/<id>/ 实现 module.Module，然后在 All 里加一行。
package tools

import "nicetools/backend/internal/module"

// All 返回全部工具后端模块，启动时装配。
func All() []module.Module {
	return []module.Module{}
}
