package k3

import (
	"fmt"
	"time"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// 性能基准档：8 个语料梯度，单并发画像（1–10 并发），产出完整延迟/吞吐/缓存分位数据。
// 并发与样本量对齐业界基准报告；语料值取各档名义长度（4k…512k）。

type bScenario struct {
	id, label                    string
	tokens, concurrency, samples int
	ttftP50Max, ttftP95Max       float64
	e2eP90                       float64
}

// bMinSamples 是每档「最小推理延迟」基线采样次数（决定净 PreFill 的参照点）。
func bMinSamples() int { return 6 }

func bScenarios() []bScenario {
	return []bScenario{
		{"B-4K", "<8K@10", 4000, 10, 30, 2500, 4000, 20000},
		{"B-12K", "8-16K@8", 12000, 8, 24, 4000, 6000, 30000},
		{"B-16K", "16-32K@6", 16000, 6, 18, 4000, 8000, 48000},
		{"B-32K", "32-64K@6", 32000, 6, 12, 6000, 12000, 68000},
		{"B-64K", "64-128K@4", 64000, 4, 8, 8000, 16000, 92000},
		{"B-128K", "128-256K@2", 128000, 2, 6, 10000, 20000, 130000},
		{"B-256K", "256-512K@1", 256000, 1, 6, 12000, 25000, 190000},
		{"B-512K", "512K-1M@1", 512000, 1, 6, 16000, 32000, 260000},
	}
}

// bLoadCases 返回性能基准场景（8 个语料梯度），属于 latency 分组。
func bLoadCases() []suite.Case {
	cases := make([]suite.Case, 0, len(bScenarios()))
	for _, sc := range bScenarios() {
		scenario := sc
		cases = append(cases, suite.Case{
			ID: scenario.id, Name: "基准 " + scenario.label, Level: suite.LevelRequired, Group: suite.GroupLatency,
			Doc: suite.CaseDoc{
				Purpose: fmt.Sprintf("在 %s 语料、%d 并发下测 %d 个样本，产出该档延迟分位、输出速度与缓存命中（同语料复用同时观测缓存）。", scenario.label, scenario.concurrency, scenario.samples),
				Method:  fmt.Sprintf("校准 %dk token 语料发满样本，流式收 TTFT/TTFB/TPOT/Tokens/s/最大 ITL 分位；另发 %d 次最小请求测最小推理延迟。", scenario.tokens/1000, bMinSamples()),
				Expect:  fmt.Sprintf("成功率≥95%% 且错误率≤5%%；TTFT P50≤%.0fms P95≤%.0fms；TPOT P50≤22ms P95≤36ms；E2E P90≤%.0fms。", scenario.ttftP50Max, scenario.ttftP95Max, scenario.e2eP90),
				Ref:     "§6.1 性能基准",
			},
			Run: func(c *suite.CaseCtx) suite.Outcome { return bRunScenario(c, scenario) },
		})
	}
	return cases
}

// benchmarkMaxTokens 是性能基准单请求的输出预算。
// K3 会先输出思考链；预算过小会让思考吃掉全部 token，最终回答为空（finish=length）。
const benchmarkMaxTokens = 4096

// bRunScenario 跑一个基准场景：先测最小推理延迟基线，再发满样本聚合全部分位指标。
func bRunScenario(c *suite.CaseCtx, sc bScenario) suite.Outcome {
	prompt, actualTokens, err := fillPrompt(c, sc.tokens)
	if err != nil {
		return sFailure(sc.id+" 输入校准失败", nil, nil, []suite.Defect{{Level: "P1", Expect: "可用 token 校准", Actual: err.Error(), Attribution: "gateway"}})
	}
	started := time.Now()
	minTTFT := bMinLatency(c)
	if sControlled(c) {
		return sOutcome(sc.id+" 已按控制信号收尾", map[string]any{"calibrated_input_tokens": actualTokens}, nil, nil)
	}
	samples, stopped := sRequests(c, sc.concurrency, sc.samples, func(i int) (string, map[string]any) {
		body := map[string]any{
			"messages": userMsg(prompt + "\n请直接回答：OK"), "max_tokens": benchmarkMaxTokens, "stream": true,
		}
		return fmt.Sprintf("%s 样本 %d", sc.id, i+1), body
	})
	metrics, evidence, successRate, errorRate, uncontrolled := sLoadMetrics(samples)
	metrics["calibrated_input_tokens"] = actualTokens
	metrics["concurrency"] = sc.concurrency
	metrics["minimum_samples"] = sc.samples
	metrics["duration_ms"] = time.Since(started).Milliseconds()
	metrics["min_latency_p50_ms"] = minTTFT
	metrics["target_ttft_p50_ms"] = sc.ttftP50Max
	metrics["target_ttft_p95_ms"] = sc.ttftP95Max
	metrics["target_e2e_p90_ms"] = sc.e2eP90
	metrics["target_tpot_p50_ms"] = 22
	metrics["target_tpot_p95_ms"] = 36
	metrics["target_success_rate_percent"] = 95
	metrics["target_error_rate_percent"] = 5
	metrics["target_itl_p95_ms"] = 80
	metrics["output_budget_tokens"] = benchmarkMaxTokens
	metrics["target_uncontrolled_requests"] = 0
	bEnrichMetrics(metrics, samples, minTTFT)

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
		defects = append(defects, suite.Defect{Level: "P1", Expect: "响应完整输出=0", Actual: fmt.Sprintf("%d 次", uncontrolled), Attribution: "upstream"})
	}
	if metrics["ttft_p50_ms"].(float64) > sc.ttftP50Max || metrics["ttft_p95_ms"].(float64) > sc.ttftP95Max {
		defects = append(defects, suite.Defect{Level: "P1", Expect: fmt.Sprintf("TTFT P50/P95≤%.0f/%.0fms", sc.ttftP50Max, sc.ttftP95Max), Actual: fmt.Sprintf("%.0f/%.0fms", metrics["ttft_p50_ms"], metrics["ttft_p95_ms"]), Attribution: "upstream"})
	}
	if metrics["tpot_p50_ms"].(float64) > 22 || metrics["tpot_p95_ms"].(float64) > 36 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "TPOT P50/P95≤22/36ms", Actual: fmt.Sprintf("%.2f/%.2fms", metrics["tpot_p50_ms"], metrics["tpot_p95_ms"]), Attribution: "upstream"})
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

// bMinLatency 测最小推理延迟基线：短 prompt + max_tokens=8，取 TTFT 中位数。
// 这一项代表「网关 + 模型固定开销」，净 PreFill = TTFT − 该基线。
func bMinLatency(c *suite.CaseCtx) float64 {
	values := make([]float64, 0, bMinSamples())
	for i := 0; i < bMinSamples(); i++ {
		if sControlled(c) {
			break
		}
		body := map[string]any{"messages": userMsg("Hi"), "max_tokens": 8, "stream": true}
		r, _ := chat(c, body)
		if r.HTTPStatus == 200 && r.ErrKind == "" && r.TTFTMs > 0 {
			values = append(values, float64(r.TTFTMs))
		}
	}
	return suite.Percentile(values, 50)
}

// bEnrichMetrics 汇总 B 档全部分位与用量指标（延迟、吞吐、ITL、缓存、Token/请求）。
func bEnrichMetrics(metrics map[string]any, samples []sSample, minLatency float64) {
	ttft := make([]float64, 0, len(samples))
	ttfb := make([]float64, 0, len(samples))
	netPrefill := make([]float64, 0, len(samples))
	maxITL := make([]float64, 0, len(samples))
	conn := make([]float64, 0, len(samples))
	connReused := 0
	http200, stop, length, okCount := 0, 0, 0, 0
	promptTokens, cachedTokens, outputTokens := 0, 0, 0
	cacheHitRequests := 0
	scatter := make([]float64, 0, len(samples))

	for _, sample := range samples {
		r := sample.result
		if r.HTTPStatus == 200 {
			http200++
		}
		switch r.FinishReason {
		case "stop":
			stop++
		case "length":
			length++
		}
		if r.ConnReused {
			connReused++
		} else if r.ConnMs > 0 {
			conn = append(conn, float64(r.ConnMs))
		}
		if r.HTTPStatus != 200 || r.ErrKind != "" {
			continue
		}
		okCount++
		ttft = append(ttft, float64(r.TTFTMs))
		scatter = append(scatter, float64(r.TTFTMs))
		if r.TTFBMs > 0 {
			ttfb = append(ttfb, float64(r.TTFBMs))
		}
		if minLatency > 0 && float64(r.TTFTMs) > minLatency {
			netPrefill = append(netPrefill, float64(r.TTFTMs)-minLatency)
		}
		if r.MaxITLMs > 0 {
			maxITL = append(maxITL, float64(r.MaxITLMs))
		}
		if r.Usage == nil {
			continue
		}
		promptTokens += r.Usage.PromptTokens
		cachedTokens += r.Usage.CachedTokens
		outputTokens += r.Usage.CompletionTokens
		if r.Usage.CachedTokens > 0 {
			cacheHitRequests++
		}
	}
	e2e := make([]float64, 0, okCount)
	otps := make([]float64, 0, okCount)
	for _, sample := range samples {
		r := sample.result
		if r.HTTPStatus != 200 || r.ErrKind != "" {
			continue
		}
		e2e = append(e2e, float64(r.E2EMs))
		if r.Usage != nil && r.Usage.CompletionTokens > 0 && r.E2EMs > r.TTFTMs {
			seconds := float64(r.E2EMs-r.TTFTMs) / 1000
			if seconds > 0 {
				otps = append(otps, float64(r.Usage.CompletionTokens)/seconds)
			}
		}
	}

	// 分位族：P50/P95/P99/Max。
	bSetPercentiles(metrics, "ttft", ttft, true)
	bSetPercentiles(metrics, "ttfb", ttfb, true)
	bSetPercentiles(metrics, "net_prefill", netPrefill, false)
	bSetPercentiles(metrics, "max_itl", maxITL, false)
	bSetPercentiles(metrics, "otps", otps, true)
	metrics["e2e_p50_ms"] = suite.Percentile(e2e, 50)
	metrics["e2e_p90_ms"] = suite.Percentile(e2e, 90)
	metrics["e2e_p95_ms"] = suite.Percentile(e2e, 95)
	metrics["e2e_p99_ms"] = suite.Percentile(e2e, 99)
	metrics["e2e_max_ms"] = bMax(e2e)
	bSetPercentiles(metrics, "conn", conn, true)
	metrics["conn_reused_samples"] = connReused
	metrics["conn_new_samples"] = len(conn)

	// 稳定性与抖动。
	metrics["ttft_cv"] = suite.CV(ttft)
	metrics["latency_p99_p50_ratio"] = bRatio(suite.Percentile(e2e, 99), suite.Percentile(e2e, 50))
	metrics["ttft_p99_p50_ratio"] = bRatio(suite.Percentile(ttft, 99), suite.Percentile(ttft, 50))

	// 状态分布与成功率。
	metrics["http_200_count"] = http200
	metrics["finish_stop_count"] = stop
	metrics["finish_length_count"] = length

	// 缓存与 Token 用量。
	metrics["prompt_tokens"] = promptTokens
	metrics["cached_tokens"] = cachedTokens
	metrics["output_tokens"] = outputTokens
	metrics["cache_read_tokens"] = cachedTokens
	metrics["input_tokens_total"] = promptTokens
	metrics["output_tokens_total"] = outputTokens
	metrics["cache_read_tokens_total"] = cachedTokens
	metrics["cache_hit_rate_percent"] = sRate(cachedTokens, promptTokens)
	metrics["cache_coverage_percent"] = sRate(cacheHitRequests, okCount)
	metrics["cache_hit_requests"] = cacheHitRequests
	metrics["cache_total_requests"] = okCount
	if okCount > 0 {
		metrics["input_tokens_per_request"] = float64(promptTokens) / float64(okCount)
		metrics["output_tokens_per_request"] = float64(outputTokens) / float64(okCount)
		metrics["cache_read_tokens_per_request"] = float64(cachedTokens) / float64(okCount)
	}
	// P99 可靠性标注：样本不足 30 时 P99 仅供参照。
	if okCount < 30 {
		metrics["p99_sample_note"] = fmt.Sprintf("n=%d <30，P99 仅供参考", okCount)
	}
	// 散点图数据：每样本 TTFT（最多 60 点，等距抽样）。
	metrics["sample_ttft_ms"] = bDownsample(scatter, 60)
}

// bSetPercentiles 写入 <name>_p50/p95/p99/max；suffix_ms 指定键名单位。
func bSetPercentiles(metrics map[string]any, name string, values []float64, withMs bool) {
	suffix := ""
	if withMs {
		suffix = "_ms"
	}
	metrics[name+"_p50"+suffix] = suite.Percentile(values, 50)
	metrics[name+"_p95"+suffix] = suite.Percentile(values, 95)
	metrics[name+"_p99"+suffix] = suite.Percentile(values, 99)
	metrics[name+"_max"+suffix] = bMax(values)
}

func bMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	best := values[0]
	for _, v := range values[1:] {
		if v > best {
			best = v
		}
	}
	return best
}

func bRatio(numerator, denominator float64) float64 {
	if denominator <= 0 {
		return 0
	}
	return numerator / denominator
}

// bDownsample 等距抽取原始请求顺序样本，保留首尾，供散点图使用。
func bDownsample(values []float64, limit int) []float64 {
	if limit <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) <= limit {
		return append([]float64(nil), values...)
	}
	if limit == 1 {
		return []float64{values[0]}
	}
	out := make([]float64, 0, limit)
	for i := 0; i < limit; i++ {
		index := i * (len(values) - 1) / (limit - 1)
		out = append(out, values[index])
	}
	return out
}
