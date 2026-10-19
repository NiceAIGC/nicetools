package k3

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// soakDuration 允许测试环境缩短 Soak 时长；生产默认执行标准要求的 30 分钟。
var soakDuration = 30 * time.Minute

type sLoadSpec struct {
	id          string
	target      int
	concurrency int
	samples     int
	ttftP50     float64
	ttftP90     float64
	e2eP90      float64
}

type sSample struct {
	label   string
	body    map[string]any
	result  suite.CallResult
	retries int
}

// sCases 返回 §6 的全部 S 档用例。
func sCases() []suite.Case {
	return []suite.Case{
		{ID: "S-T1", Name: "阶梯压测 T1（1k-5k）", Level: suite.LevelRequired, Run: func(c *suite.CaseCtx) suite.Outcome {
			return sLoad(c, sLoadSpec{"T1", 3000, 60, 50, 2500, 4000, 20000})
		}},
		{ID: "S-T2", Name: "阶梯压测 T2（6k-15k）", Level: suite.LevelRequired, Run: func(c *suite.CaseCtx) suite.Outcome {
			return sLoad(c, sLoadSpec{"T2", 10500, 50, 48, 4000, 6000, 30000})
		}},
		{ID: "S-T3", Name: "阶梯压测 T3（16k-30k）", Level: suite.LevelRequired, Run: func(c *suite.CaseCtx) suite.Outcome {
			return sLoad(c, sLoadSpec{"T3", 23000, 40, 36, 4000, 8000, 48000})
		}},
		{ID: "S-T4", Name: "阶梯压测 T4（32k-60k）", Level: suite.LevelRequired, Run: func(c *suite.CaseCtx) suite.Outcome {
			return sLoad(c, sLoadSpec{"T4", 46000, 15, 18, 6000, 12000, 68000})
		}},
		{ID: "S-T5", Name: "阶梯压测 T5（64k-120k）", Level: suite.LevelRequired, Run: func(c *suite.CaseCtx) suite.Outcome {
			return sLoad(c, sLoadSpec{"T5", 92000, 10, 12, 8000, 16000, 92000})
		}},
		{ID: "S-T6", Name: "阶梯压测 T6（128k-240k）", Level: suite.LevelRequired, Run: func(c *suite.CaseCtx) suite.Outcome {
			return sLoad(c, sLoadSpec{"T6", 184000, 5, 8, 10000, 20000, 130000})
		}},
		{ID: "S-C1", Name: "缓存压测命中率", Level: suite.LevelRequired, Run: sCacheHit},
		{ID: "S-C2", Name: "缓存 TTL 压测", Level: suite.LevelRequired, Run: sCacheTTL},
		{ID: "S-C3", Name: "多轮稳态缓存命中", Level: suite.LevelRequired, Run: sCacheMultiturn},
		{ID: "S-SOAK", Name: "Soak 稳定性（30 分钟）", Level: suite.LevelRequired, Run: sSoak},
		{ID: "S-X1", Name: "极限输入观测（256k-512k）", Level: suite.LevelRecord, Run: sExtreme},
	}
}

// sLoad 执行一个六档阶梯压测，随机前缀确保压测不复用缓存。
func sLoad(c *suite.CaseCtx, spec sLoadSpec) suite.Outcome {
	prompt, actualTokens, err := fillPrompt(c, spec.target)
	if err != nil {
		return sFailure(spec.id+" 输入校准失败", nil, nil, []suite.Defect{{Level: "P1", Expect: "可用 token 校准", Actual: err.Error(), Attribution: "gateway"}})
	}
	samples, stopped := sRequests(c, spec.concurrency, spec.samples, func(i int) (string, map[string]any) {
		body := map[string]any{
			"messages": userMsg(fmt.Sprintf("压测随机前缀=%s-%d\n%s\n请简要确认已读。", nonce(), i, prompt)),
			"stream":   true, "max_tokens": 256, "temperature": 1.0,
		}
		return fmt.Sprintf("%s 请求 %d", spec.id, i+1), body
	})
	metrics, evidence, successful, errors, uncontrolled := sLoadMetrics(samples)
	metrics["target_input_tokens"] = spec.target
	metrics["calibrated_input_tokens"] = actualTokens
	metrics["concurrency"] = spec.concurrency
	metrics["minimum_samples"] = spec.samples
	if stopped {
		return sOutcome(spec.id+" 已按控制信号收尾", metrics, evidence, nil)
	}
	var defects []suite.Defect
	if len(samples) < spec.samples {
		defects = append(defects, suite.Defect{Level: "P1", Expect: fmt.Sprintf("样本量≥%d", spec.samples), Actual: fmt.Sprintf("%d", len(samples)), Attribution: "client"})
	}
	if successful < 95 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "成功率≥95%", Actual: fmt.Sprintf("%.2f%%", successful), Attribution: "upstream"})
	}
	if errors > 5 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "错误率≤5%", Actual: fmt.Sprintf("%.2f%%", errors), Attribution: "gateway"})
	}
	if uncontrolled > 0 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "失控请求=0", Actual: fmt.Sprintf("%d", uncontrolled), Attribution: "upstream"})
	}
	if metrics["ttft_p50_ms"].(float64) > spec.ttftP50 || metrics["ttft_p90_ms"].(float64) > spec.ttftP90 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: fmt.Sprintf("TTFT P50/P90≤%.0f/%.0fms", spec.ttftP50, spec.ttftP90), Actual: fmt.Sprintf("%.0f/%.0fms", metrics["ttft_p50_ms"], metrics["ttft_p90_ms"]), Attribution: "upstream"})
	}
	if metrics["tpot_p50_ms"].(float64) > 22 || metrics["tpot_p90_ms"].(float64) > 36 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "TPOT P50/P90≤22/36ms", Actual: fmt.Sprintf("%.2f/%.2fms", metrics["tpot_p50_ms"], metrics["tpot_p90_ms"]), Attribution: "upstream"})
	}
	if metrics["e2e_p90_ms"].(float64) > spec.e2eP90 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: fmt.Sprintf("E2E P90≤%.0fms", spec.e2eP90), Actual: fmt.Sprintf("%.0fms", metrics["e2e_p90_ms"]), Attribution: "upstream"})
	}
	if len(defects) > 0 {
		return sFailure(spec.id+" 未达标", metrics, evidence, defects)
	}
	return pass(spec.id+" 达标", metrics, evidence...)
}

// sRequests 以固定 worker pool 发送请求，完成顺序即样本完成顺序。
func sRequests(c *suite.CaseCtx, concurrency, count int, build func(int) (string, map[string]any)) ([]sSample, bool) {
	jobs := make(chan int, count)
	results := make(chan sSample, count)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if sControlled(c) {
					return
				}
				label, body := build(i)
				r, retries := chatTimeout(c, body, suite.TimeoutLarge)
				results <- sSample{label: label, body: body, result: r, retries: retries}
			}
		}()
	}
	stopped := false
	for i := range count {
		if sControlled(c) {
			stopped = true
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if sControlled(c) {
		stopped = true
	}
	close(results)
	out := make([]sSample, 0, count)
	for sample := range results {
		out = append(out, sample)
	}
	return out, stopped
}

func sLoadMetrics(samples []sSample) (map[string]any, []suite.Evidence, float64, float64, int) {
	ttft := make([]float64, 0, len(samples))
	e2e := make([]float64, 0, len(samples))
	tpot := make([]float64, 0, len(samples))
	evidence := make([]suite.Evidence, 0, len(samples))
	successes, errors, uncontrolled := 0, 0, 0
	for _, sample := range samples {
		r := sample.result
		evidence = append(evidence, ev(sample.label, sample.body, r, sample.retries))
		if r.HTTPStatus == 200 && r.ErrKind == "" {
			successes++
			ttft = append(ttft, float64(r.TTFTMs))
			e2e = append(e2e, float64(r.E2EMs))
			if r.Usage != nil && r.Usage.CompletionTokens >= 2 {
				tpot = append(tpot, float64(r.E2EMs-r.TTFTMs)/float64(r.Usage.CompletionTokens-1))
			}
		}
		if r.HTTPStatus != 200 || r.ErrKind != "" {
			errors++
		}
		if r.FinishReason == "length" && r.Usage != nil && r.Usage.CompletionTokens > 0 && r.Usage.ReasoningTokens*100 >= r.Usage.CompletionTokens*95 {
			uncontrolled++
		}
	}
	metrics := sPercentileMetrics(ttft, tpot, e2e)
	metrics["samples"] = len(samples)
	metrics["success_rate_percent"] = sRate(successes, len(samples))
	metrics["error_rate_percent"] = sRate(errors, len(samples))
	metrics["uncontrolled_requests"] = uncontrolled
	metrics["tpot_samples"] = len(tpot)
	return metrics, evidence, sRate(successes, len(samples)), sRate(errors, len(samples)), uncontrolled
}

func sEvidence(samples []sSample) []suite.Evidence {
	evidence := make([]suite.Evidence, 0, len(samples))
	for _, sample := range samples {
		evidence = append(evidence, ev(sample.label, sample.body, sample.result, sample.retries))
	}
	return evidence
}

func sPercentileMetrics(ttft, tpot, e2e []float64) map[string]any {
	metrics := map[string]any{
		"ttft_p50_ms": suite.Percentile(ttft, 50), "ttft_p90_ms": suite.Percentile(ttft, 90),
		"tpot_p50_ms": suite.Percentile(tpot, 50), "tpot_p90_ms": suite.Percentile(tpot, 90),
		"e2e_p50_ms": suite.Percentile(e2e, 50), "e2e_p90_ms": suite.Percentile(e2e, 90),
	}
	if len(ttft) >= 10 {
		metrics["ttft_p95_ms"] = suite.Percentile(ttft, 95)
	}
	if len(tpot) >= 10 {
		metrics["tpot_p95_ms"] = suite.Percentile(tpot, 95)
	}
	if len(e2e) >= 10 {
		metrics["e2e_p95_ms"] = suite.Percentile(e2e, 95)
	}
	if len(ttft) >= 30 {
		metrics["ttft_p99_ms"] = suite.Percentile(ttft, 99)
	}
	if len(tpot) >= 30 {
		metrics["tpot_p99_ms"] = suite.Percentile(tpot, 99)
	}
	if len(e2e) >= 30 {
		metrics["e2e_p99_ms"] = suite.Percentile(e2e, 99)
	}
	return metrics
}

func sCacheHit(c *suite.CaseCtx) suite.Outcome {
	prefix, _, err := fillPrompt(c, 2048)
	if err != nil {
		return sFailure("缓存输入校准失败", nil, nil, []suite.Defect{{Level: "P1", Expect: "可用 token 校准", Actual: err.Error(), Attribution: "gateway"}})
	}
	type scenario struct {
		name  string
		build func(int) map[string]any
	}
	scenarios := []scenario{
		{"repeat", func(int) map[string]any {
			return map[string]any{"messages": userMsg(prefix + "\n请回答：缓存测试。"), "stream": true, "max_tokens": 256, "temperature": 1.0}
		}},
		{"multiturn", func(i int) map[string]any {
			return map[string]any{"messages": []any{map[string]any{"role": "user", "content": prefix}, map[string]any{"role": "assistant", "content": "我已阅读该前缀。"}, map[string]any{"role": "user", "content": fmt.Sprintf("第 %d 次追问，请简答。", i+1)}}, "stream": true, "max_tokens": 256, "temperature": 1.0}
		}},
		{"suffix", func(i int) map[string]any {
			return map[string]any{"messages": userMsg(prefix + fmt.Sprintf("\n不同尾部 %s-%d，请简答。", nonce(), i)), "stream": true, "max_tokens": 256, "temperature": 1.0}
		}},
	}
	var samples []sSample
	for _, scenario := range scenarios {
		if sControlled(c) {
			return sOutcome("缓存命中率已按控制信号收尾", map[string]any{"measurement_concurrency": 10, "prefix_target_tokens": 2048}, sEvidence(samples), nil)
		}
		warm := scenario.build(-1)
		r, retries := chatTimeout(c, warm, suite.TimeoutLarge)
		samples = append(samples, sSample{label: scenario.name + " 预热", body: warm, result: r, retries: retries})
	}
	measured, stopped := sRequests(c, 10, 15, func(i int) (string, map[string]any) {
		scenario := scenarios[i/5]
		return fmt.Sprintf("%s 测量 %d", scenario.name, i%5+1), scenario.build(i % 5)
	})
	samples = append(samples, measured...)
	if stopped {
		return sOutcome("缓存命中率已按控制信号收尾", map[string]any{"measurement_concurrency": 10, "prefix_target_tokens": 2048}, sEvidence(samples), nil)
	}
	metrics := map[string]any{"measurement_concurrency": 10, "prefix_target_tokens": 2048}
	evidence := make([]suite.Evidence, 0, len(samples))
	var defects []suite.Defect
	for _, scenario := range []string{"repeat", "multiturn", "suffix"} {
		prompt, cached := 0, 0
		for _, sample := range samples {
			if !strings.HasPrefix(sample.label, scenario+" 测量") {
				continue
			}
			evidence = append(evidence, ev(sample.label, sample.body, sample.result, sample.retries))
			if sample.result.Usage != nil {
				prompt += sample.result.Usage.PromptTokens
				cached += sample.result.Usage.CachedTokens
			}
		}
		rate := sRate(cached, prompt)
		metrics[scenario+"_prompt_tokens"] = prompt
		metrics[scenario+"_cached_tokens"] = cached
		metrics[scenario+"_hit_rate_percent"] = rate
		if rate < 80 {
			defects = append(defects, suite.Defect{Level: "P1", Expect: scenario + " token 命中率≥80%", Actual: fmt.Sprintf("%.2f%%", rate), Attribution: "upstream"})
		}
	}
	for _, sample := range samples {
		if strings.Contains(sample.label, "测量") {
			continue
		}
		evidence = append(evidence, ev(sample.label, sample.body, sample.result, sample.retries))
	}
	if len(defects) > 0 {
		return sFailure("缓存命中率未达标", metrics, evidence, defects)
	}
	return pass("三场景缓存命中率均达标", metrics, evidence...)
}

func sCacheTTL(c *suite.CaseCtx) suite.Outcome {
	prefix, _, err := fillPrompt(c, 70000)
	if err != nil {
		return sFailure("TTL 输入校准失败", nil, nil, []suite.Defect{{Level: "P1", Expect: "可用 token 校准", Actual: err.Error(), Attribution: "gateway"}})
	}
	body := func() map[string]any {
		return map[string]any{"messages": userMsg(prefix + "\n请只回答 TTL。"), "stream": true, "max_tokens": 256, "temperature": 1.0}
	}
	var evidence []suite.Evidence
	warm := body()
	r, retries := chatTimeout(c, warm, suite.TimeoutLarge)
	evidence = append(evidence, ev("TTL 预热", warm, r, retries))
	checkpoints := []time.Duration{0, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute}
	curve := make([]map[string]any, 0, len(checkpoints))
	hitAfter30, stopped := false, false
	previous := time.Duration(0)
	for _, checkpoint := range checkpoints {
		if !sWait(c, checkpoint-previous) {
			stopped = true
			break
		}
		previous = checkpoint
		request := body()
		result, attempts := chatTimeout(c, request, suite.TimeoutLarge)
		evidence = append(evidence, ev("TTL "+checkpoint.String(), request, result, attempts))
		hitRate := 0.0
		if result.Usage != nil {
			hitRate = sRate(result.Usage.CachedTokens, result.Usage.PromptTokens)
		}
		curve = append(curve, map[string]any{"checkpoint_seconds": checkpoint.Seconds(), "prompt_tokens": usagePrompt(result), "cached_tokens": usageCached(result), "hit_rate_percent": hitRate, "http_status": result.HTTPStatus})
		if checkpoint >= 30*time.Second && result.Usage != nil && result.Usage.CachedTokens > 0 {
			hitAfter30 = true
		}
	}
	metrics := map[string]any{"decay_curve": curve, "hit_at_or_after_30_seconds": hitAfter30}
	if stopped {
		return sOutcome("缓存 TTL 已按控制信号收尾", metrics, evidence, nil)
	}
	if !hitAfter30 {
		return sFailure("缓存 TTL 不足 30 秒", metrics, evidence, []suite.Defect{{Level: "P1", Expect: "≥30s 有缓存命中", Actual: "所有 ≥30s 检查点 cached_tokens=0", Attribution: "upstream"}})
	}
	return pass("缓存 TTL 在 30 秒后仍命中", metrics, evidence...)
}

func sCacheMultiturn(c *suite.CaseCtx) suite.Outcome {
	prefix, _, err := fillPrompt(c, 2048)
	if err != nil {
		return sFailure("多轮缓存输入校准失败", nil, nil, []suite.Defect{{Level: "P1", Expect: "可用 token 校准", Actual: err.Error(), Attribution: "gateway"}})
	}
	messages := userMsg(prefix + "\n下面开始多轮缓存测试。")
	var evidence []suite.Evidence
	metrics := map[string]any{}
	var defects []suite.Defect
	for round := range 6 {
		if sControlled(c) {
			return sOutcome("多轮缓存已按控制信号收尾", metrics, evidence, nil)
		}
		body := map[string]any{"messages": messages, "stream": true, "max_tokens": 256, "temperature": 1.0}
		r, retries := chatTimeout(c, body, suite.TimeoutLarge)
		evidence = append(evidence, ev(fmt.Sprintf("Round %d", round), body, r, retries))
		rate := 0.0
		if r.Usage != nil {
			rate = sRate(r.Usage.CachedTokens, r.Usage.PromptTokens)
		}
		metrics[fmt.Sprintf("round_%d_hit_rate_percent", round)] = rate
		if round >= 2 && rate < 60 {
			defects = append(defects, suite.Defect{Level: "P1", Expect: fmt.Sprintf("Round %d 命中率≥60%%", round), Actual: fmt.Sprintf("%.2f%%", rate), Attribution: "upstream"})
		}
		messages = append(messages, map[string]any{"role": "assistant", "content": r.Content}, map[string]any{"role": "user", "content": fmt.Sprintf("这是第 %d 轮追问，请继续简答。", round+1)})
	}
	if len(defects) > 0 {
		return sFailure("多轮稳态缓存未达标", metrics, evidence, defects)
	}
	return pass("Round≥2 稳态缓存均达标", metrics, evidence...)
}

func sSoak(c *suite.CaseCtx) suite.Outcome {
	prompt, actualTokens, err := fillPrompt(c, 23000)
	if err != nil {
		return sFailure("Soak 输入校准失败", nil, nil, []suite.Defect{{Level: "P1", Expect: "可用 token 校准", Actual: err.Error(), Attribution: "gateway"}})
	}
	deadline := time.Now().Add(soakDuration)
	results := make(chan sSample, 128)
	var wg sync.WaitGroup
	for worker := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sequence := 0; time.Now().Before(deadline); sequence++ {
				if sControlled(c) {
					return
				}
				body := map[string]any{"messages": userMsg(fmt.Sprintf("Soak=%d-%d-%s\n%s\n请简答。", worker, sequence, nonce(), prompt)), "stream": true, "max_tokens": 256, "temperature": 1.0}
				r, retries := chatTimeout(c, body, suite.TimeoutLarge)
				results <- sSample{label: fmt.Sprintf("worker %d request %d", worker+1, sequence+1), body: body, result: r, retries: retries}
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()
	var samples []sSample
	for sample := range results {
		samples = append(samples, sample)
	}
	metrics, evidence, _, errorRate, _ := sLoadMetrics(samples)
	metrics["duration_seconds"] = soakDuration.Seconds()
	metrics["concurrency"] = 20
	metrics["target_input_tokens"] = 23000
	metrics["calibrated_input_tokens"] = actualTokens
	metrics["ttft_cv"] = sTTFTCV(samples)
	storms, resets := 0, 0
	consecutive5xx := 0
	for _, sample := range samples {
		if sample.result.HTTPStatus >= 500 && sample.result.HTTPStatus <= 599 {
			consecutive5xx++
			if consecutive5xx == 5 {
				storms++
			}
		} else {
			consecutive5xx = 0
		}
		if sample.result.ErrKind == "transport" && strings.Contains(strings.ToLower(sample.result.ErrMsg), "reset") {
			resets++
		}
	}
	metrics["five_xx_storms"] = storms
	metrics["connection_resets"] = resets
	var defects []suite.Defect
	if errorRate > 1 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "错误率≤1%", Actual: fmt.Sprintf("%.2f%%", errorRate), Attribution: "gateway"})
	}
	if storms > 0 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "无连续≥5个 5xx 风暴", Actual: fmt.Sprintf("%d", storms), Attribution: "upstream"})
	}
	if metrics["ttft_cv"].(float64) >= 0.5 {
		defects = append(defects, suite.Defect{Level: "P1", Expect: "TTFT 漂移 CV<0.5", Actual: fmt.Sprintf("%.3f", metrics["ttft_cv"]), Attribution: "upstream"})
	}
	if len(defects) > 0 {
		return sFailure("Soak 未达标", metrics, evidence, defects)
	}
	return pass("Soak 稳定性达标", metrics, evidence...)
}

func sExtreme(c *suite.CaseCtx) suite.Outcome {
	prompt, actualTokens, err := fillPrompt(c, 384000)
	if err != nil {
		return sFailure("极限输入校准失败", nil, nil, []suite.Defect{{Level: "P1", Expect: "可用 token 校准", Actual: err.Error(), Attribution: "gateway"}})
	}
	samples, _ := sRequests(c, 1, 6, func(i int) (string, map[string]any) {
		body := map[string]any{"messages": userMsg(fmt.Sprintf("极限观测=%s-%d\n%s\n请简答。", nonce(), i, prompt)), "stream": true, "max_tokens": 256, "temperature": 1.0}
		return fmt.Sprintf("极限样本 %d", i+1), body
	})
	metrics, evidence, _, _, _ := sLoadMetrics(samples)
	metrics["target_input_tokens"] = 384000
	metrics["calibrated_input_tokens"] = actualTokens
	return pass("极限输入观测已记录（无通过线）", metrics, evidence...)
}

func sOutcome(summary string, metrics map[string]any, evidence []suite.Evidence, defects []suite.Defect) suite.Outcome {
	if len(defects) > 0 {
		return sFailure(summary, metrics, evidence, defects)
	}
	return pass(summary, metrics, evidence...)
}

func sFailure(summary string, metrics map[string]any, evidence []suite.Evidence, defects []suite.Defect) suite.Outcome {
	return suite.Outcome{Status: suite.StatusFailed, Summary: summary, Metrics: metrics, Evidence: evidence, Defects: defects}
}

func sRate(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) * 100 / float64(denominator)
}

func sTTFTCV(samples []sSample) float64 {
	values := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if sample.result.HTTPStatus == 200 && sample.result.ErrKind == "" {
			values = append(values, float64(sample.result.TTFTMs))
		}
	}
	return suite.CV(values)
}

func sControlled(c *suite.CaseCtx) bool {
	return c.CheckControl != nil && c.CheckControl() != nil
}

// sWait 按两秒切片休眠，保证 TTL 等待可被暂停或终止。
func sWait(c *suite.CaseCtx, total time.Duration) bool {
	for total > 0 {
		if sControlled(c) {
			return false
		}
		slice := min(2*time.Second, total)
		select {
		case <-c.Done():
			return false
		case <-time.After(slice):
		}
		total -= slice
	}
	return !sControlled(c)
}

func usagePrompt(r suite.CallResult) int {
	if r.Usage == nil {
		return 0
	}
	return r.Usage.PromptTokens
}
func usageCached(r suite.CallResult) int {
	if r.Usage == nil {
		return 0
	}
	return r.Usage.CachedTokens
}
