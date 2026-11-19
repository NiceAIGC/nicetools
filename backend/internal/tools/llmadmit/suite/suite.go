// Package suite 定义准入测试套件的抽象：用例、分组预设、判据结果与执行上下文。
// 一个新标准（模型/渠道）= 一个实现 Suite 的包，在 llmadmit 注册表加一行。
package suite

import (
	"context"
	"time"
)

// Level 是用例级别（P0–P3），决定失败对裁决的影响；历史库中的
// required/record/optional 分别对应 P1/P2/P3，展示层做了兼容。
type Level string

const (
	LevelP0       Level = "P0" // 否决项
	LevelRequired Level = "P1" // 必过项
	LevelRecord   Level = "P2" // 记录项：产出数据，不影响通过
	LevelOptional Level = "P3" // 可选项：声明支持才必须过
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
	// MediaURL 返回测试媒体（红/蓝视频等）的可公开拉取地址（带任务令牌）；
	// 未配置时返回空串，URL 类用例应降级为记录。
	MediaURL func(name string) string
	// CheckControl 供长耗时用例在内部循环检查暂停/终止；
	// 返回非 nil 时用例应立即收尾返回该错误。
	CheckControl func() error
	// Progress 可选：长用例在内部循环汇报子进度。
	Progress func(done, total int, step string)
	// OnRequest 可选：每次真正发起上游 HTTP 请求时递增请求计数。
	OnRequest func()
}

// CaseDoc 是用例说明：为什么测、怎么测、预期是什么、规范出处。
// 前端展示与详细报告共用同一份文字，判据文字必须与 Run 的逻辑一致。
type CaseDoc struct {
	Purpose string `json:"purpose"` // 为什么这样测试
	Method  string `json:"method"`  // 怎么测试（请求构造）
	Expect  string `json:"expect"`  // 预期结果/通过判据
	Ref     string `json:"ref"`     // 规范出处，如 §4.1
}

// 用例分组：驱动「只测延迟 / 只测缓存 / 只测格式参数 / 全测」快捷开关。
const (
	GroupGate    = "gate"    // 前置闸门：任何预设都必须先跑
	GroupFormat  = "format"  // 功能与协议格式
	GroupCache   = "cache"   // 缓存命中、TTL、覆盖率
	GroupLatency = "latency" // 性能基准（分位数/吞吐）
)

// Group 是测试预设：空字符串表示全测。
type Group string

const (
	PresetAll     Group = ""        // 全测：格式 + 缓存 + 延迟
	PresetFormat  Group = "format"  // 只测格式与协议
	PresetCache   Group = "cache"   // 只测缓存
	PresetLatency Group = "latency" // 只测延迟与吞吐
	PresetVideo   Group = "video"   // 只测视频相关能力（闸门恒包含）
)

// ParseGroup 校验并归一化预设取值。
func ParseGroup(raw string) (Group, bool) {
	switch Group(raw) {
	case PresetAll, PresetFormat, PresetCache, PresetLatency, PresetVideo:
		return Group(raw), true
	default:
		return PresetAll, false
	}
}

// Includes 判断某分组的用例是否属于该预设；全测包含一切，闸门恒包含。
func (g Group) Includes(group string) bool {
	if g == PresetAll || group == GroupGate {
		return true
	}
	return group == string(g)
}

// Case 是一个准入用例。Run 只在能力豁免判断之后被调用。
type Case struct {
	ID    string `json:"id"`   // F-01 / M-38 / B-4K
	Name  string `json:"name"` // 用例名
	Level Level  `json:"level"`
	// Group 是用例分组（gate/format/cache/latency），驱动预设过滤。
	Group string `json:"group"`
	// Capability 非空时，未声明支持该能力 → 豁免。
	Capability string `json:"capability,omitempty"`
	// DefaultSkip 是前端任务创建时的默认不勾选标记（可选/昂贵用例），
	// 不影响直接调用 API 或管理员永久配置。
	DefaultSkip bool                     `json:"default_skip,omitempty"`
	Doc         CaseDoc                  `json:"doc"`
	Run         func(c *CaseCtx) Outcome `json:"-"`
}

// Capability 是可在测试前声明的能力，驱动用例豁免（§1.2）。
type Capability struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Preset 是前端可选的测试范围快捷开关。
type Preset struct {
	Key   Group  `json:"key"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// Suite 是一套准入标准：用例平铺，由 Group + Preset 决定本次跑哪一组。
type Suite struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	DefaultModel string       `json:"default_model"`
	Capabilities []Capability `json:"capabilities"`
	Cases        []Case       `json:"cases"`
	Presets      []Preset     `json:"presets"`
}

// Select 返回预设命中的用例（保持声明顺序）；全测预设返回全部。
func (s Suite) Select(preset Group) []Case {
	out := make([]Case, 0, len(s.Cases))
	for _, c := range s.Cases {
		// 视频预设不是 group，而是 capability=video 的横切范围。
		if preset == PresetVideo {
			if c.Group == GroupGate || c.Capability == "video" {
				out = append(out, c)
			}
			continue
		}
		if preset.Includes(c.Group) {
			out = append(out, c)
		}
	}
	return out
}

// SelectIDs 返回显式选择的用例，保持声明顺序；前置闸门始终补入。
// 返回 false 表示存在套件中不存在的 ID。
func (s Suite) SelectIDs(ids []string) ([]Case, bool) {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	known := make(map[string]bool, len(s.Cases))
	out := make([]Case, 0, len(ids)+2)
	for _, c := range s.Cases {
		known[c.ID] = true
		if wanted[c.ID] || c.Group == GroupGate {
			out = append(out, c)
		}
	}
	for id := range wanted {
		if !known[id] {
			return nil, false
		}
	}
	return out, true
}

// 供长请求使用的超时分级（§8.3）。
const (
	TimeoutNormal  = 180 * time.Second
	TimeoutLarge   = 600 * time.Second
	TimeoutOutput  = 1800 * time.Second
	TimeoutConnect = 30 * time.Second
)
