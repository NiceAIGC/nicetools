// Package k3 实现《Kimi-K3 渠道三档测试准入标准 v1.0》。
// 用例编号与标准文档一一对应：G 闸门 / F 功能 / M 中等 / S 压力。
package k3

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// 能力声明键：驱动豁免（§1.2）。
const (
	CapVideo     = "video"
	CapImageURL  = "image_url"
	CapWebSearch = "web_search"
)

// Suite 返回 K3 三档准入标准。
func Suite() suite.Suite {
	return suite.Suite{
		ID:           "k3-v1",
		Name:         "Kimi-K3 渠道三档测试准入标准 v1.0",
		DefaultModel: "kimi-k3",
		Capabilities: []suite.Capability{
			{Key: CapVideo, Label: "视频理解（video_base64 / video_url）"},
			{Key: CapImageURL, Label: "公网图片 URL（image_url）"},
			{Key: CapWebSearch, Label: "联网搜索（web_search_options）"},
		},
		Tiers: []suite.Tier{
			{ID: "F", Name: "F 档 · 功能测试", Hint: "~30 请求 / <20k tokens / ~5 分钟", Cases: fCases()},
			{ID: "M", Name: "M 档 · 中等测试", Hint: "~120 请求 / 0.5–1M tokens / 30–60 分钟", Cases: mCases()},
			{ID: "S", Name: "S 档 · 压力测试", Hint: "~400–800 请求 / 10–15M tokens / 1.5–3 小时", Cases: sCases()},
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
