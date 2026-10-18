// Package validate 持有跨包共用的标识符校验规则，全库只此一份。
package validate

import "regexp"

// Slug 是工具 id 的合法形式，同时是路由段、表名前缀与前端目录名。
var Slug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Username 是账号名的合法形式。
var Username = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)
