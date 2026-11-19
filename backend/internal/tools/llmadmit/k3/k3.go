// Package k3 实现 Kimi-K3 渠道准入标准。
// 用例按格式协议、性能基准、缓存分组；G/F/M 编号保留标准来源语义。
package k3

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// 能力声明键：驱动豁免（§1.2）。
const (
	CapVideo       = "video"
	CapImageURL    = "image_url"
	CapWebSearch   = "web_search"
	CapOutput600K  = "max_output_600k"
	CapOutput1024K = "max_output_1024k"
	CapContext1M   = "context_1m"
)

// Group 给用例打分组标签（gate/format/cache/latency），驱动预设过滤。
func group(cases []suite.Case, name string) []suite.Case {
	for i := range cases {
		if cases[i].Group != "" {
			continue
		}
		if cases[i].Level == suite.LevelGate {
			cases[i].Group = suite.GroupGate
			continue
		}
		cases[i].Group = name
	}
	return cases
}

// Suite 返回 K3 准入标准：用例平铺，预设决定本次跑哪一组。
func Suite() suite.Suite {
	cases := make([]suite.Case, 0, 128)
	cases = append(cases, group(fCases(), suite.GroupFormat)...)
	cases = append(cases, group(mCases(), suite.GroupFormat)...)
	cases = append(cases, bLoadCases()...)
	// 执行顺序：闸门 → 格式 → 缓存 → 性能基准；基准最耗时放最后。
	rank := func(c suite.Case) int {
		switch c.Group {
		case suite.GroupGate:
			return 0
		case suite.GroupFormat:
			return 1
		case suite.GroupCache:
			return 2
		case suite.GroupLatency:
			return 3
		}
		return 4
	}
	sort.SliceStable(cases, func(i, j int) bool { return rank(cases[i]) < rank(cases[j]) })
	return suite.Suite{
		ID:           "k3-v1",
		Name:         "Kimi-K3 渠道准入标准 v1.3",
		DefaultModel: "kimi-k3",
		Capabilities: []suite.Capability{
			{Key: CapVideo, Label: "视频理解（video_base64 / video_url）"},
			{Key: CapImageURL, Label: "公网图片 URL（image_url）"},
			{Key: CapWebSearch, Label: "联网搜索（web_search_options）"},
			{Key: CapOutput600K, Label: "最大输出 600K tokens"},
			{Key: CapOutput1024K, Label: "最大输出 1024K tokens（600K 通过后测试）"},
			{Key: CapContext1M, Label: "1M 上下文（1024K）"},
		},
		Cases: cases,
		Presets: []suite.Preset{
			{Key: suite.PresetAll, Label: "全部测试", Hint: "闸门 → 格式 → 缓存 → 性能基准，约 30–70 分钟"},
			{Key: suite.PresetFormat, Label: "只测格式与协议", Hint: "功能、协议矩阵、边界与鉴权，约 15–30 分钟"},
			{Key: suite.PresetCache, Label: "只测缓存", Hint: "缓存写入、多轮/后缀覆盖、5m TTL 曲线、1h 三协议格式，约 10–15 分钟"},
			{Key: suite.PresetLatency, Label: "只测延迟与吞吐", Hint: "8 档语料基准：TTFT/TPOT/Tokens/s/缓存，约 15–25 分钟"},
			{Key: suite.PresetVideo, Label: "只测视频相关", Hint: "闸门 + 视频红蓝对照、base64/URL 通道与可选体积上限"},
		},
	}
}

// ---- 用例编写辅助 ----

// chat 是带重试的常规请求（§8.1：429/5xx 最多重试 2 次）。
func chat(c *suite.CaseCtx, body map[string]any) (suite.CallResult, int) {
	return suite.CallWithRetry(c, func() suite.CallResult {
		return c.Client.Chat(c, c.Target, body, suite.TimeoutNormal)
	})
}

// chatTimeout 用于大输入/大输出场景的超时请求。
func chatTimeout(c *suite.CaseCtx, body map[string]any, timeout time.Duration) (suite.CallResult, int) {
	return suite.CallWithRetry(c, func() suite.CallResult {
		return c.Client.Chat(c, c.Target, body, timeout)
	})
}

// ev 把一次请求构造为证据条目；请求体中 base64 媒体截断标注。
func ev(label string, body map[string]any, r suite.CallResult, retries int) suite.Evidence {
	payload, _ := json.Marshal(truncateMedia(body))
	return suite.Evidence{Label: label, Request: string(payload), Result: r, Retries: retries}
}

// truncateMedia 把请求体里的 data: URI 媒体内容替换为长度标注，防止证据膨胀。
func truncateMedia(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = truncateMedia(item)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = truncateMedia(item)
		}
		return out
	case string:
		if len(t) > 512 && len(t) > 20 && t[:5] == "data:" {
			return fmt.Sprintf("%s…[base64 %d bytes]", t[:64], len(t))
		}
		return t
	default:
		return v
	}
}

func pass(summary string, metrics map[string]any, evidence ...suite.Evidence) suite.Outcome {
	return suite.Outcome{Status: suite.StatusPassed, Summary: summary, Metrics: metrics, Evidence: evidence}
}

func fail(summary string, defects []suite.Defect, evidence ...suite.Evidence) suite.Outcome {
	return suite.Outcome{Status: suite.StatusFailed, Summary: summary, Defects: defects, Evidence: evidence}
}

func nonce() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// userMsg 构造单条 user 文本消息体。
func userMsg(text string) []any {
	return []any{map[string]any{"role": "user", "content": text}}
}

// fillPrompt 生成约 targetTokens 个 token 的输入文本。
// 通过一次校准请求测得每块 token 数，再按倍数重复；prefix 防缓存污染（§6）。
func fillPrompt(c *suite.CaseCtx, targetTokens int) (string, int, error) {
	const block = "人工智能系统的准入测试需要覆盖功能、性能、稳定性与安全性多个维度，" +
		"每个维度都需要可重复、可量化的判据，并且所有结论都必须有原始证据支撑。"
	probe, _ := chat(c, map[string]any{
		"messages": userMsg("a"), "max_tokens": 32,
	})
	if probe.HTTPStatus != 200 || probe.Usage == nil {
		return "", 0, fmt.Errorf("校准请求失败: HTTP %d %s", probe.HTTPStatus, probe.ErrMsg)
	}
	base := probe.Usage.PromptTokens
	probe2, _ := chat(c, map[string]any{
		"messages": userMsg(strings.Repeat(block, 10)), "max_tokens": 32,
	})
	if probe2.HTTPStatus != 200 || probe2.Usage == nil {
		return "", 0, fmt.Errorf("校准请求失败: HTTP %d %s", probe2.HTTPStatus, probe2.ErrMsg)
	}
	perBlock := (probe2.Usage.PromptTokens - base) / 10
	if perBlock <= 0 {
		return "", 0, fmt.Errorf("token 校准异常: %d", perBlock)
	}
	repeats := max(targetTokens/perBlock, 1)
	return fmt.Sprintf("[%s] %s", nonce(), strings.Repeat(block, repeats)), repeats * perBlock, nil
}
