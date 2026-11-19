package k3

import (
	"fmt"
	"sync"
	"sync/atomic"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// sSample 是一次并发请求样本（含请求体与结果），供证据与指标聚合共用。
type sSample struct {
	label   string
	body    map[string]any
	result  suite.CallResult
	retries int
}

// sRequests 以固定 worker pool 发送请求，完成顺序即样本完成顺序。
func sRequests(c *suite.CaseCtx, concurrency, count int, build func(int) (string, map[string]any)) ([]sSample, bool) {
	jobs := make(chan int, count)
	results := make(chan sSample, count)
	var completed atomic.Int64
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
				if c.Progress != nil {
					c.Progress(int(completed.Add(1)), count, "基准样本")
				}
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

// sLoadMetrics 聚合样本：TTFT/TPOT/E2E 分位、ITL、OTPS、成功率与失控请求。
func sLoadMetrics(samples []sSample) (map[string]any, []suite.Evidence, float64, float64, int) {
	ttft := make([]float64, 0, len(samples))
	e2e := make([]float64, 0, len(samples))
	tpot := make([]float64, 0, len(samples))
	otps := make([]float64, 0, len(samples))
	itl := make([]float64, 0, len(samples))
	evidence := make([]suite.Evidence, 0, len(samples))
	successes, errors, uncontrolled := 0, 0, 0
	for _, sample := range samples {
		r := sample.result
		evidence = append(evidence, ev(sample.label, sample.body, r, sample.retries))
		if r.HTTPStatus == 200 && r.ErrKind == "" {
			successes++
			ttft = append(ttft, float64(r.TTFTMs))
			e2e = append(e2e, float64(r.E2EMs))
			for _, interval := range r.ITLMsa {
				if interval >= 0 {
					itl = append(itl, float64(interval))
				}
			}
			if r.Usage != nil && r.Usage.CompletionTokens > 0 && r.E2EMs > r.TTFTMs {
				seconds := float64(r.E2EMs-r.TTFTMs) / 1000
				if seconds > 0 {
					otps = append(otps, float64(r.Usage.CompletionTokens)/seconds)
				}
				if r.Usage.CompletionTokens >= 2 {
					tpot = append(tpot, float64(r.E2EMs-r.TTFTMs)/float64(r.Usage.CompletionTokens-1))
				}
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
	metrics["itl_p95_ms"] = suite.Percentile(itl, 95)
	metrics["otps_p50"] = suite.Percentile(otps, 50)
	metrics["otps_p90"] = suite.Percentile(otps, 90)
	metrics["samples"] = len(samples)
	metrics["success_rate_percent"] = sRate(successes, len(samples))
	metrics["error_rate_percent"] = sRate(errors, len(samples))
	metrics["uncontrolled_requests"] = uncontrolled
	metrics["tpot_samples"] = len(tpot)
	return metrics, evidence, sRate(successes, len(samples)), sRate(errors, len(samples)), uncontrolled
}

// sPercentileMetrics 输出 TTFT/TPOT/E2E 的 P50/P90/P95/P99。
// 分位数一律输出（含样本不足的 P99），由报告层按样本量标注可信度。
func sPercentileMetrics(ttft, tpot, e2e []float64) map[string]any {
	metrics := map[string]any{}
	for name, values := range map[string][]float64{"ttft": ttft, "tpot": tpot, "e2e": e2e} {
		for _, p := range []float64{50, 90, 95, 99} {
			metrics[fmt.Sprintf("%s_p%.0f_ms", name, p)] = suite.Percentile(values, p)
		}
	}
	return metrics
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

func sControlled(c *suite.CaseCtx) bool {
	return c.CheckControl != nil && c.CheckControl() != nil
}
