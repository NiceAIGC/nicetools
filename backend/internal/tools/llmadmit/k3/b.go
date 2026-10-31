package k3

import (
	"fmt"
	"time"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// B 档：基准测试（基本延迟 + 缓存），约 15 分钟。
// 8 个输入长度梯度场景（对齐业界基准的档位划分）+ 缓存冷热与 TTL 快检。

type bScenario struct {
	id, label                      string
	tokens, concurrency, samples   int
	ttftP50Max, ttftP90Max, e2eP90 float64
}

func bScenarios() []bScenario {
	return []bScenario{
		{"B-4K", "<8K@10", 4000, 10, 50, 2500, 4000, 20000},
		{"B-12K", "8-16K@8", 12000, 8, 48, 4000, 6000, 30000},
		{"B-16K", "16-32K@6", 24000, 6, 36, 4000, 8000, 48000},
		{"B-32K", "32-64K@6", 48000, 6, 18, 6000, 12000, 68000},
		{"B-64K", "64-128K@4", 96000, 4, 12, 8000, 16000, 92000},
		{"B-128K", "128-256K@2", 192000, 2, 8, 10000, 20000, 130000},
		{"B-256K", "256-512K@1", 384000, 1, 6, 12000, 25000, 190000},
		{"B-512K", "512K-1M@1", 768000, 1, 6, 16000, 32000, 260000},
	}
}

// bLoadCases 返回延迟基准场景（8 个输入梯度）。
func bLoadCases() []suite.Case {
	cases := []suite.Case{}
	for _, sc := range bScenarios() {
		scenario := sc
		cases = append(cases, suite.Case{
			ID: scenario.id, Name: "基准 " + scenario.label, Level: suite.LevelRequired,
			Doc: suite.CaseDoc{
				Purpose: fmt.Sprintf("在 %s 输入、并发 %d 下测 %d 个样本，验证该档延迟与稳定性是否达准入线（同语料复用同时观测缓存命中）。", scenario.label, scenario.concurrency, scenario.samples),
				Method:  "同一语料发满样本，流式收 TTFT/TPOT/E2E/OTPS 分位数与缓存命中（usage.cached_tokens token 口径）。",
				Expect:  fmt.Sprintf("成功率≥95%% 且错误率≤5%%；TTFT P50≤%.0fms P90≤%.0fms；TPOT P50≤22ms P90≤36ms；E2E P90≤%.0fms。", scenario.ttftP50Max, scenario.ttftP90Max, scenario.e2eP90),
				Ref:     "§6.1 阶梯判据（基准档）",
			},
			Run: func(c *suite.CaseCtx) suite.Outcome { return bRunScenario(c, scenario) },
		})
	}
	return cases
}

// bCacheCases 返回基础缓存用例（进准入测试档）。
func bCacheCases() []suite.Case {
	return []suite.Case{
		{ID: "B-C1", Name: "缓存冷热三场景", Level: suite.LevelRequired, Run: mB4CacheScenarios,
			Doc: suite.CaseDoc{
				Purpose: "缓存在 repeat/multiturn/suffix 三种真实使用形态下都要稳定命中，单一场景通过不代表可用。",
				Method:  "2048 token 公共前缀，各场景 1 预热 + 5 测量，场景间独立 nonce 防串扰。",
				Expect:  "三场景 token 覆盖率均 ≥80%；suffix 首测不命中记 P2。",
				Ref:     "§5.2 M-B4",
			}},
		{ID: "B-C2", Name: "缓存 TTL 快检", Level: suite.LevelRequired, Run: bCacheTTL,
			Doc: suite.CaseDoc{
				Purpose: "缓存存活时间是命中率的前提，过短 TTL 会导致长会话冷启动。",
				Method:  "70k token 前缀预热，0/30/60/120s 检查点探测 cached_tokens 衰减。",
				Expect:  "TTL ≥ 30s（≥30s 检查点仍有命中），产出衰减曲线。",
				Ref:     "§5.2 M-B3（快检版）",
			}},
	}
}

// bRunScenario 跑一个基准场景：同语料样本（首个自然预热），统计延迟分位与缓存命中。
func bRunScenario(c *suite.CaseCtx, sc bScenario) suite.Outcome {
	prompt, actualTokens, err := fillPrompt(c, sc.tokens)
	if err != nil {
		return sFailure(sc.id+" 输入校准失败", nil, nil, []suite.Defect{{Level: "P1", Expect: "可用 token 校准", Actual: err.Error(), Attribution: "gateway"}})
	}
	samples, stopped := sRequests(c, sc.concurrency, sc.samples, func(i int) (string, map[string]any) {
		body := map[string]any{
			"messages": userMsg(prompt + "\n请直接回答：OK"), "max_tokens": 256, "temperature": 1.0, "stream": true,
		}
		return fmt.Sprintf("%s 样本 %d", sc.id, i+1), body
	})
	metrics, evidence, successRate, errorRate, uncontrolled := sLoadMetrics(samples)
	metrics["calibrated_input_tokens"] = actualTokens
	metrics["concurrency"] = sc.concurrency

	// 缓存命中（token 口径，剔除首个预热样本）；OTPS（输出 token/秒，含思考）。
	promptTokens, cachedTokens := 0, 0
	otps := make([]float64, 0, len(samples))
	for i, sample := range samples {
		r := sample.result
		if r.HTTPStatus == 200 && r.ErrKind == "" && r.Usage != nil {
			if i > 0 {
				promptTokens += r.Usage.PromptTokens
				cachedTokens += r.Usage.CachedTokens
			}
			if r.E2EMs > r.TTFTMs && r.Usage.CompletionTokens > 0 {
				otps = append(otps, float64(r.Usage.CompletionTokens)*1000/float64(r.E2EMs-r.TTFTMs))
			}
		}
	}
	metrics["cache_hit_rate_percent"] = sRate(cachedTokens, promptTokens)
	metrics["otps_p50"] = suite.Percentile(otps, 50)
	metrics["otps_p90"] = suite.Percentile(otps, 90)

	var defects []suite.Defect
	if stopped {
		defects = append(defects, suite.Defect{Level: "P1", Expect: fmt.Sprintf("样本量≥%d", sc.samples), Actual: fmt.Sprintf("%d", len(samples)), Attribution: "client"})
	}
	if successRate < 95 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "成功率≥95%", Actual: fmt.Sprintf("%.2f%%", successRate), Attribution: "upstream"})
	}
	if errorRate > 5 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "错误率≤5%", Actual: fmt.Sprintf("%.2f%%", errorRate), Attribution: "gateway"})
	}
	if uncontrolled > 0 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "失控请求=0", Actual: fmt.Sprintf("%d", uncontrolled), Attribution: "upstream"})
	}
	if metrics["ttft_p50_ms"].(float64) > sc.ttftP50Max || metrics["ttft_p90_ms"].(float64) > sc.ttftP90Max {
		defects = append(defects, suite.Defect{Level: "P1", Expect: fmt.Sprintf("TTFT P50/P90≤%.0f/%.0fms", sc.ttftP50Max, sc.ttftP90Max), Actual: fmt.Sprintf("%.0f/%.0fms", metrics["ttft_p50_ms"], metrics["ttft_p90_ms"]), Attribution: "upstream"})
	}
	if metrics["tpot_p50_ms"].(float64) > 22 || metrics["tpot_p90_ms"].(float64) > 36 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "TPOT P50/P90≤22/36ms", Actual: fmt.Sprintf("%.2f/%.2fms", metrics["tpot_p50_ms"], metrics["tpot_p90_ms"]), Attribution: "upstream"})
	}
	if metrics["e2e_p90_ms"].(float64) > sc.e2eP90 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: fmt.Sprintf("E2E P90≤%.0fms", sc.e2eP90), Actual: fmt.Sprintf("%.0fms", metrics["e2e_p90_ms"]), Attribution: "upstream"})
	}
	summary := fmt.Sprintf("%s：成功率 %.1f%% TTFT P50 %.0fms TPOT P50 %.1fms 缓存命中 %.1f%%",
		sc.label, successRate, metrics["ttft_p50_ms"], metrics["tpot_p50_ms"], metrics["cache_hit_rate_percent"])
	if len(defects) > 0 {
		return sFailure(sc.id+" 未达标："+summary, metrics, evidence, defects)
	}
	return pass(sc.id+" 达标："+summary, metrics, evidence...)
}

// bCacheTTL 缓存 TTL 快检（0/30/60/120s 检查点）。
func bCacheTTL(c *suite.CaseCtx) suite.Outcome {
	prefix, err := cachedPrefix(c, 70000)
	if err != nil {
		return fail("前缀构造失败: "+err.Error(), nil)
	}
	body := func() map[string]any {
		return map[string]any{"messages": userMsg(prefix + "\n请只回答：TTL"), "max_tokens": 32}
	}
	var evidence []suite.Evidence
	r, retries := chatTimeout(c, body(), suite.TimeoutLarge)
	evidence = append(evidence, ev("预热", body(), r, retries))
	curve := []map[string]any{}
	hitAt30 := false
	previous := 0
	for _, sec := range []int{0, 30, 60, 120} {
		for remaining := sec - previous; remaining > 0; remaining -= 2 {
			if err := c.CheckControl(); err != nil {
				return pass("TTL 探测被中断", map[string]any{"decay_curve": curve}, evidence...)
			}
			time.Sleep(2 * time.Second)
		}
		previous = sec
		r, retries := chatTimeout(c, body(), suite.TimeoutLarge)
		evidence = append(evidence, ev(fmt.Sprintf("%ds", sec), body(), r, retries))
		rate := 0.0
		if r.Usage != nil && r.Usage.PromptTokens > 0 {
			rate = float64(r.Usage.CachedTokens) * 100 / float64(r.Usage.PromptTokens)
		}
		curve = append(curve, map[string]any{"seconds": sec, "hit_rate_percent": rate})
		if sec >= 30 && rate > 0 {
			hitAt30 = true
		}
	}
	metrics := map[string]any{"decay_curve": curve}
	if !hitAt30 {
		return fail("缓存 TTL < 30s", []suite.Defect{{Level: "P1", Expect: "≥30s 有命中", Actual: "30s 起 cached=0", Attribution: "upstream"}}, evidence...)
	}
	return pass("缓存 TTL ≥ 30s", metrics, evidence...)
}
