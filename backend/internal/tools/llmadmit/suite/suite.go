// Package suite 定义准入测试套件的抽象：档位、用例、判据结果与执行上下文。
// 一个新标准（模型/渠道）= 一个实现 Suite 的包，在 llmadmit 注册表加一行。
package suite

import (
	"context"
	"time"
)

// Level 是用例级别，决定失败时对当档 verdict 的影响。
type Level string

const (
	LevelP0       Level = "P0"       // 否决项：失败即当档不通过并终止
	LevelRequired Level = "required" // 必过项：失败即当档不通过
	LevelRecord   Level = "record"   // 记录项：产出数据，不影响通过
	LevelOptional Level = "optional" // 可选项：声明支持才必须过
	// LevelGate 是前置闸门（§3）：失败 → 本次测试无效，不产出 P0/P1 结论。
	LevelGate Level = "gate"
)

// 用例状态。
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusExempt  = "exempt"  // 能力声明豁免，不计入失败
	StatusSkipped = "skipped" // 前置失败等原因未执行
)

// Defect 是一条缺陷记录（§1.3 分级）。
type Defect struct {
	Level       string `json:"level"`       // P0/P1/P2
	Expect      string `json:"expect"`      // 期望值
	Actual      string `json:"actual"`      // 实测值
	Attribution string `json:"attribution"` // upstream / gateway / client
}

// Evidence 是一次请求的证据条目：请求构造 + 完整调用留档（§7.2）。
type Evidence struct {
	Label   string     `json:"label"`   // 子请求说明，如 "生僻词串"
	Request string     `json:"request"` // 请求体 JSON（图片/视频 base64 已截断标注）
	Result  CallResult `json:"result"`
	Retries int        `json:"retries,omitempty"` // 重试次数（§8.1）
}

// Outcome 是用例执行结论。
type Outcome struct {
	Status   string         `json:"status"`
	Summary  string         `json:"summary"`
	Metrics  map[string]any `json:"metrics,omitempty"`
	Defects  []Defect       `json:"defects,omitempty"`
	Evidence []Evidence     `json:"evidence,omitempty"`
}

// Target 是测试目标。
type Target struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"-"`
	Model   string `json:"model"`
}

// CaseCtx 是用例执行上下文。
type CaseCtx struct {
	context.Context
	Target Target
	Client *Client
	// Declared 返回能力是否被声明支持（未声明 → 对应用例豁免）。
	Declared func(capability string) bool
	// CheckControl 供长耗时用例在内部循环检查暂停/终止；
	// 返回非 nil 时用例应立即收尾返回该错误。
	CheckControl func() error
}

// Case 是一个准入用例。Run 只在 Declared 豁免判断之后被调用。
type Case struct {
	ID    string `json:"id"`   // F-01 / M-38 / S-T1
	Name  string `json:"name"` // 用例名
	Level Level  `json:"level"`
	// Capability 非空时，未声明支持该能力 → 豁免。
	Capability string                   `json:"capability,omitempty"`
	Run        func(c *CaseCtx) Outcome `json:"-"`
}

// Tier 是一个档位。
type Tier struct {
	ID    string `json:"id"` // F / M / S
	Name  string `json:"name"`
	Cases []Case `json:"-"`
	// 给前端的档位说明（请求量级/token 预算/耗时）。
	Hint string `json:"hint"`
}

// Capability 是可在测试前声明的能力，驱动用例豁免（§1.2）。
type Capability struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Suite 是一套准入标准。
type Suite struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	DefaultModel string       `json:"default_model"`
	Capabilities []Capability `json:"capabilities"`
	Tiers        []Tier       `json:"tiers"`
}

// 供长请求使用的超时分级（§8.3）。
const (
	TimeoutNormal  = 180 * time.Second
	TimeoutLarge   = 600 * time.Second
	TimeoutOutput  = 1800 * time.Second
	TimeoutConnect = 30 * time.Second
)
