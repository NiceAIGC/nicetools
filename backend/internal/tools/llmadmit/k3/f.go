package k3

import (
	"fmt"
	"strings"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// 词表指纹探测串（§2.1）：生僻词串增量 +7、emoji 串增量 +10（相对 "a"）。
const (
	fpRare  = "photosynthesis mitochondria endoplasmic"
	fpEmoji = "😀😃😄😁😆"
)

// fCases 返回前置闸门 + F 档全部用例。
func fCases() []suite.Case {
	return []suite.Case{
		{ID: "G1", Name: "模型元信息", Level: suite.LevelGate, Run: g1Models},
		{ID: "G2", Name: "探活 ×2", Level: suite.LevelGate, Run: g2Alive},
		{ID: "F-01", Name: "真伪指纹", Level: suite.LevelP0, Run: f01Fingerprint},
		{ID: "F-02", Name: "身份与注入", Level: suite.LevelP0, Run: f02Identity},
		{ID: "F-03", Name: "非流式结构", Level: suite.LevelRequired, Run: f03Structure},
		{ID: "F-04", Name: "流式 SSE", Level: suite.LevelRequired, Run: f04Stream},
		{ID: "F-05", Name: "usage 字段", Level: suite.LevelRequired, Run: f05Usage},
		{ID: "F-06", Name: "输出控制 ×2", Level: suite.LevelRequired, Run: f06OutputControl},
		{ID: "F-07", Name: "思考默认", Level: suite.LevelRequired, Run: f07ThinkingDefault},
		{ID: "F-08", Name: "采样契约 ×4", Level: suite.LevelRecord, Run: f08Sampling},
		{ID: "F-09", Name: "思考开关", Level: suite.LevelRecord, Run: f09ThinkingOff},
		{ID: "F-10", Name: "图片识别", Level: suite.LevelP0, Run: f10Image},
		{ID: "F-11", Name: "鉴权 ×2", Level: suite.LevelRequired, Run: f11Auth},
		{ID: "F-12", Name: "TTFT 观测", Level: suite.LevelRecord, Run: f12TTFT},
	}
}

// G1：GET /v1/models → 200 且目标模型在列。
func g1Models(c *suite.CaseCtx) suite.Outcome {
	status, ids, err := c.Client.Models(c, c.Target)
	if err != nil || status != 200 {
		return fail(fmt.Sprintf("GET /models: HTTP %d %v", status, err), []suite.Defect{
			{Level: "P0", Expect: "200", Actual: fmt.Sprintf("%d %v", status, err), Attribution: "gateway"},
		})
	}
	for _, id := range ids {
		if id == c.Target.Model {
			return pass("目标模型在列", map[string]any{"models": ids})
		}
	}
	return fail("目标模型不在 /models 列表", []suite.Defect{
		{Level: "P0", Expect: c.Target.Model, Actual: strings.Join(ids, ","), Attribution: "gateway"},
	})
}

// G2：探活 ×2，均须 200；quota 错误 → 中止（G3 合并判定）。
func g2Alive(c *suite.CaseCtx) suite.Outcome {
	var evidence []suite.Evidence
	for i := range 2 {
		r, retries := chat(c, map[string]any{"messages": userMsg("a"), "max_tokens": 32})
		evidence = append(evidence, ev(fmt.Sprintf("探活 %d", i+1), map[string]any{"max_tokens": 32}, r, retries))
		if r.HTTPStatus != 200 {
			hint := "核对地址与 key 配对"
			body := r.Raw
			switch {
			case r.HTTPStatus == 401:
				hint = "401：核对地址/key 配对"
			case r.HTTPStatus == 502:
				hint = "502 且 models 通：后端宕机"
			case strings.Contains(body, "quota") || strings.Contains(body, "余额"):
				hint = "余额/配额不足：充值后重跑"
			case strings.Contains(body, "<html"):
				hint = "返回 HTML：地址可能是文档站"
			}
			return fail("探活失败："+hint, []suite.Defect{
				{Level: "P0", Expect: "200", Actual: fmt.Sprintf("HTTP %d", r.HTTPStatus), Attribution: "gateway"},
			}, evidence...)
		}
	}
	return pass("两次探活均 200", nil, evidence...)
}

// F-01：prompt_tokens 增量指纹；偏离时 12 轮采样复核，绝对值稳定方算通过。
func f01Fingerprint(c *suite.CaseCtx) suite.Outcome {
	probe := func(text string) (int, suite.CallResult, int) {
		r, retries := chat(c, map[string]any{"messages": userMsg(text), "max_tokens": 32})
		if r.HTTPStatus != 200 || r.Usage == nil {
			return -1, r, retries
		}
		return r.Usage.PromptTokens, r, retries
	}
	var evidence []suite.Evidence
	base, r, rt := probe("a")
	evidence = append(evidence, ev("基线 \"a\"", map[string]any{"prompt": "a"}, r, rt))
	if base < 0 {
		return fail("基线探测失败", []suite.Defect{{Level: "P0", Expect: "200", Actual: fmt.Sprintf("HTTP %d", r.HTTPStatus), Attribution: "gateway"}}, evidence...)
	}
	deltas := map[string][]int{"rare": {}, "emoji": {}}
	stable := true
	for round := range 12 {
		for _, item := range []struct {
			key, text string
		}{{"rare", fpRare}, {"emoji", fpEmoji}} {
			tokens, r2, rt2 := probe(item.text)
			if round == 0 {
				evidence = append(evidence, ev(item.key, map[string]any{"prompt": item.text}, r2, rt2))
			}
			if tokens < 0 {
				return fail(fmt.Sprintf("%s 探测失败 HTTP %d", item.key, r2.HTTPStatus), nil, evidence...)
			}
			d := tokens - base
			deltas[item.key] = append(deltas[item.key], d)
			if d != deltas[item.key][0] {
				stable = false
			}
		}
		if round == 0 && deltas["rare"][0] == 7 && deltas["emoji"][0] == 10 {
			return pass(fmt.Sprintf("增量 +%d/+%d 与官方一致", deltas["rare"][0], deltas["emoji"][0]),
				map[string]any{"rare_delta": deltas["rare"][0], "emoji_delta": deltas["emoji"][0]}, evidence...)
		}
	}
	metrics := map[string]any{"rare_delta": deltas["rare"][0], "emoji_delta": deltas["emoji"][0]}
	if stable {
		return pass(fmt.Sprintf("增量 +%d/+%d（与官方 +7/+10 不同但 12 轮稳定）", deltas["rare"][0], deltas["emoji"][0]), metrics, evidence...)
	}
	return fail(fmt.Sprintf("增量不稳定: rare=%v emoji=%v", deltas["rare"], deltas["emoji"]), []suite.Defect{
		{Level: "P0", Expect: "+7/+10 或稳定值", Actual: fmt.Sprintf("rare=%v emoji=%v", deltas["rare"], deltas["emoji"]), Attribution: "upstream"},
	}, evidence...)
}

// F-02：身份自报 Kimi/Moonshot；"你是谁" prompt_tokens ≈8，≥50 判注入。
func f02Identity(c *suite.CaseCtx) suite.Outcome {
	r1, rt1 := chat(c, map[string]any{"messages": userMsg("你是谁？"), "max_tokens": 64})
	r2, rt2 := chat(c, map[string]any{"messages": userMsg("你是谁"), "max_tokens": 32})
	evidence := []suite.Evidence{
		ev("你是谁？", map[string]any{"max_tokens": 64}, r1, rt1),
		ev("你是谁", map[string]any{"max_tokens": 32}, r2, rt2),
	}
	var defects []suite.Defect
	lower := strings.ToLower(r1.Content)
	identityOK := strings.Contains(lower, "kimi") || strings.Contains(lower, "moonshot") || strings.Contains(r1.Content, "月之暗面")
	if !identityOK {
		defects = append(defects, suite.Defect{Level: "P0", Expect: "自报 Kimi/Moonshot", Actual: truncate(r1.Content, 120), Attribution: "upstream"})
	}
	promptTokens := -1
	if r2.HTTPStatus == 200 && r2.Usage != nil {
		promptTokens = r2.Usage.PromptTokens
		if promptTokens >= 50 {
			defects = append(defects, suite.Defect{Level: "P0", Expect: "prompt_tokens ≈8", Actual: fmt.Sprintf("%d（判定注入隐藏 system prompt）", promptTokens), Attribution: "gateway"})
		}
	}
	metrics := map[string]any{"prompt_tokens": promptTokens, "identity_ok": identityOK}
	if len(defects) > 0 {
		return fail("身份/注入检查未通过", defects, evidence...)
	}
	return pass(fmt.Sprintf("身份正确，prompt_tokens=%d", promptTokens), metrics, evidence...)
}

// F-03：非流式标准结构。
func f03Structure(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("用一句话介绍你自己"), "max_tokens": 64})
	evidence := []suite.Evidence{ev("非流式", map[string]any{"max_tokens": 64}, r, retries)}
	if r.HTTPStatus != 200 {
		return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), []suite.Defect{{Level: "P1", Expect: "200", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "gateway"}}, evidence...)
	}
	if !strings.Contains(r.Raw, `"chat.completion"`) || r.Content == "" || r.FinishReason == "" {
		return fail("结构不符合 chat.completion 标准", []suite.Defect{
			{Level: "P1", Expect: "object=chat.completion + content 非空 + finish_reason", Actual: truncate(r.Raw, 200), Attribution: "gateway"},
		}, evidence...)
	}
	return pass("结构标准", map[string]any{"finish_reason": r.FinishReason}, evidence...)
}

// F-04：流式 SSE：首帧 data:，分片 ≥2，[DONE] 结束。
func f04Stream(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("数到 5"), "max_tokens": 64, "stream": true})
	evidence := []suite.Evidence{ev("流式", map[string]any{"stream": true}, r, retries)}
	if r.HTTPStatus != 200 {
		return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), nil, evidence...)
	}
	done := strings.Contains(r.Raw, "[DONE]")
	if r.Chunks >= 2 && done && strings.HasPrefix(strings.TrimSpace(r.Raw), "data:") {
		return pass(fmt.Sprintf("分片 %d，[DONE] 收尾", r.Chunks), map[string]any{"chunks": r.Chunks, "ttft_ms": r.TTFTMs}, evidence...)
	}
	return fail(fmt.Sprintf("分片=%d DONE=%v", r.Chunks, done), []suite.Defect{
		{Level: "P1", Expect: "分片≥2 且 [DONE] 收尾", Actual: fmt.Sprintf("chunks=%d done=%v", r.Chunks, done), Attribution: "gateway"},
	}, evidence...)
}

// F-05：usage 字段（非流式 + 流式 include_usage）。
func f05Usage(c *suite.CaseCtx) suite.Outcome {
	r1, rt1 := chat(c, map[string]any{"messages": userMsg("a"), "max_tokens": 32})
	r2, rt2 := chat(c, map[string]any{"messages": userMsg("a"), "max_tokens": 32, "stream": true, "stream_options": map[string]any{"include_usage": true}})
	evidence := []suite.Evidence{
		ev("非流式", map[string]any{}, r1, rt1),
		ev("流式", map[string]any{"stream_options": "include_usage"}, r2, rt2),
	}
	ok := r1.Usage != nil && r1.Usage.PromptTokens > 0 && r1.Usage.CompletionTokens > 0 && r1.Usage.TotalTokens > 0 &&
		r2.Usage != nil && r2.Usage.PromptTokens > 0 && r2.Usage.TotalTokens > 0
	metrics := map[string]any{
		"nonstream_usage": r1.Usage != nil, "stream_usage": r2.Usage != nil,
		"cached_field": r1.Usage != nil && strings.Contains(r1.Raw, "prompt_tokens_details"),
	}
	if ok {
		return pass("usage 三字段齐全（含流式末包）", metrics, evidence...)
	}
	return fail("usage 字段缺失", []suite.Defect{
		{Level: "P1", Expect: "prompt/completion/total 齐全", Actual: fmt.Sprintf("nonstream=%v stream=%v", r1.Usage != nil, r2.Usage != nil), Attribution: "gateway"},
	}, evidence...)
}

// F-06：max_tokens=32 与 max_completion_tokens=32 均应 finish_reason=length（§1.4-3 下限 32）。
func f06OutputControl(c *suite.CaseCtx) suite.Outcome {
	r1, rt1 := chat(c, map[string]any{"messages": userMsg("写一首长诗"), "max_tokens": 32})
	r2, rt2 := chat(c, map[string]any{"messages": userMsg("写一首长诗"), "max_completion_tokens": 32})
	evidence := []suite.Evidence{
		ev("max_tokens", map[string]any{"max_tokens": 32}, r1, rt1),
		ev("max_completion_tokens", map[string]any{"max_completion_tokens": 32}, r2, rt2),
	}
	var defects []suite.Defect
	if r1.HTTPStatus == 200 && r1.FinishReason != "length" {
		defects = append(defects, suite.Defect{Level: "P0", Expect: "max_tokens → finish=length", Actual: r1.FinishReason, Attribution: "upstream"})
	}
	if r2.HTTPStatus == 200 && r2.FinishReason != "length" {
		defects = append(defects, suite.Defect{Level: "P0", Expect: "max_completion_tokens → finish=length", Actual: r2.FinishReason, Attribution: "upstream"})
	}
	if r1.HTTPStatus != 200 || r2.HTTPStatus != 200 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "HTTP 200", Actual: fmt.Sprintf("%d/%d", r1.HTTPStatus, r2.HTTPStatus), Attribution: "gateway"})
	}
	if len(defects) > 0 {
		return fail("输出控制失效", defects, evidence...)
	}
	return pass("两种参数均 finish=length", nil, evidence...)
}

// F-07：默认思考：reasoning_tokens > 0 或 reasoning_content 非空。
func f07ThinkingDefault(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("9.11 和 9.9 哪个大？"), "max_tokens": 256})
	evidence := []suite.Evidence{ev("默认思考", map[string]any{}, r, retries)}
	reasoning := r.Reasoning != "" || (r.Usage != nil && r.Usage.ReasoningTokens > 0)
	if r.HTTPStatus == 200 && reasoning {
		return pass("默认有思考", map[string]any{"reasoning_tokens": usageReasoning(r)}, evidence...)
	}
	return fail("默认无思考", []suite.Defect{
		{Level: "P1", Expect: "reasoning_tokens>0 或 reasoning_content 非空", Actual: fmt.Sprintf("HTTP %d reasoning=%v", r.HTTPStatus, reasoning), Attribution: "upstream"},
	}, evidence...)
}

// F-08：采样契约：temperature=1.0→200；0.6 / top_p=0.8 / n=2 → 400；放行记 P2。
func f08Sampling(c *suite.CaseCtx) suite.Outcome {
	probes := []struct {
		label string
		body  map[string]any
		want  int
	}{
		{"temperature=1.0", map[string]any{"messages": userMsg("a"), "max_tokens": 32, "temperature": 1.0}, 200},
		{"temperature=0.6", map[string]any{"messages": userMsg("a"), "max_tokens": 32, "temperature": 0.6}, 400},
		{"top_p=0.8", map[string]any{"messages": userMsg("a"), "max_tokens": 32, "top_p": 0.8}, 400},
		{"n=2", map[string]any{"messages": userMsg("a"), "max_tokens": 32, "n": 2}, 400},
	}
	var evidence []suite.Evidence
	var defects []suite.Defect
	for _, p := range probes {
		r, retries := chat(c, p.body)
		evidence = append(evidence, ev(p.label, p.body, r, retries))
		if r.HTTPStatus != p.want {
			defects = append(defects, suite.Defect{Level: "P2", Expect: fmt.Sprintf("%s → %d", p.label, p.want), Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "gateway"})
		}
	}
	if len(defects) > 0 {
		return fail(fmt.Sprintf("%d 项采样参数与官方不一致", len(defects)), defects, evidence...)
	}
	return pass("采样契约与官方一致", nil, evidence...)
}

// F-09：思考开关：官方不可关；能关记 P1。
func f09ThinkingOff(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{"messages": userMsg("1+1=?"), "max_tokens": 64, "thinking": map[string]any{"type": "disabled"}}
	r, retries := chat(c, body)
	evidence := []suite.Evidence{ev("thinking.disabled", body, r, retries)}
	if r.HTTPStatus != 200 {
		return pass(fmt.Sprintf("HTTP %d（官方不可关，拒绝属正常）", r.HTTPStatus), nil, evidence...)
	}
	if r.Reasoning == "" && (r.Usage == nil || r.Usage.ReasoningTokens == 0) {
		return fail("思考被关闭（官方应不可关）", []suite.Defect{
			{Level: "P1", Expect: "思考不可关", Actual: "reasoning 为空", Attribution: "upstream"},
		}, evidence...)
	}
	return pass("思考仍然开启", nil, evidence...)
}

// F-10：四色象限 PNG 识别（P0）。
func f10Image(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{
		"messages":   imageMsg("这张图片四个角分别是什么颜色？按左上、右上、左下、右下顺序回答。", colorPNG()),
		"max_tokens": 128,
	}
	r, retries := chatTimeout(c, body, suite.TimeoutLarge)
	evidence := []suite.Evidence{ev("四色图", body, r, retries)}
	if r.HTTPStatus != 200 {
		return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), []suite.Defect{
			{Level: "P0", Expect: "200", Actual: fmt.Sprintf("%d", r.HTTPStatus), Attribution: "upstream"},
		}, evidence...)
	}
	ok := containsAll(r.Content, "红", "绿", "蓝", "黄") && containsAll(r.Content, "左上", "右上", "左下", "右下")
	if ok {
		return pass("四角颜色与位置全对", nil, evidence...)
	}
	return fail("图片识别错误", []suite.Defect{
		{Level: "P0", Expect: "左上红/右上绿/左下蓝/右下黄", Actual: truncate(r.Content, 200), Attribution: "upstream"},
	}, evidence...)
}

// F-11：无 Authorization 与错误 key 均 401。
func f11Auth(c *suite.CaseCtx) suite.Outcome {
	body := map[string]any{"messages": userMsg("a"), "max_tokens": 32}
	noAuth := c.Client.Chat(c, suite.Target{BaseURL: c.Target.BaseURL, Model: c.Target.Model}, body, suite.TimeoutNormal)
	badKey := c.Client.Chat(c, suite.Target{BaseURL: c.Target.BaseURL, Model: c.Target.Model, APIKey: "sk-invalid-" + nonce()}, body, suite.TimeoutNormal)
	evidence := []suite.Evidence{
		ev("无 Authorization", body, noAuth, 0),
		ev("错误 key", body, badKey, 0),
	}
	if noAuth.HTTPStatus == 401 && badKey.HTTPStatus == 401 {
		return pass("两种非法请求均 401", nil, evidence...)
	}
	return fail(fmt.Sprintf("无鉴权=%d 错key=%d", noAuth.HTTPStatus, badKey.HTTPStatus), []suite.Defect{
		{Level: "P1", Expect: "均 401", Actual: fmt.Sprintf("%d/%d", noAuth.HTTPStatus, badKey.HTTPStatus), Attribution: "gateway"},
	}, evidence...)
}

// F-12：TTFT 观测：复用本档已发请求的计时（0 额外请求），不设通过线。
func f12TTFT(c *suite.CaseCtx) suite.Outcome {
	r, retries := chat(c, map[string]any{"messages": userMsg("数到 3"), "max_tokens": 32, "stream": true})
	evidence := []suite.Evidence{ev("TTFT 观测", map[string]any{"stream": true}, r, retries)}
	if r.HTTPStatus != 200 {
		return fail(fmt.Sprintf("HTTP %d", r.HTTPStatus), nil, evidence...)
	}
	return pass(fmt.Sprintf("TTFT %dms / TTFT-业务 %dms / E2E %dms", r.TTFTMs, r.TTFTContentMs, r.E2EMs),
		map[string]any{"ttft_ms": r.TTFTMs, "ttft_content_ms": r.TTFTContentMs, "e2e_ms": r.E2EMs}, evidence...)
}

// ---- 小工具 ----

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func usageReasoning(r suite.CallResult) int {
	if r.Usage == nil {
		return 0
	}
	return r.Usage.ReasoningTokens
}
