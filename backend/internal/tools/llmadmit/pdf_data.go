package llmadmit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// 报告的数据派生：性能场景、门槛判定、通过率与时间格式。PDF 与 Markdown 共用同一口径。

type performanceRow struct {
	label   string
	name    string
	level   string
	status  string
	metrics map[string]any
}

type cachePerformanceRow struct {
	label        string
	rate         float64
	cachedTokens string
	promptTokens string
	seconds      float64
	hasSeconds   bool
}

func collectPerformance(cases []caseRow) ([]performanceRow, []cachePerformanceRow) {
	rows := make([]performanceRow, 0, len(cases))
	cacheRows := []cachePerformanceRow{}
	dedicatedCache := []cachePerformanceRow{}
	for _, c := range cases {
		var metrics map[string]any
		if json.Unmarshal(c.Metrics, &metrics) != nil || len(metrics) == 0 {
			continue
		}
		label := c.CaseID
		benchmark := hasPerformanceMetrics(metrics)
		if benchmark {
			rows = append(rows, performanceRow{label: label, name: c.Name, level: c.Level, status: c.Status, metrics: metrics})
		}
		for _, key := range sortedKeys(metrics) {
			if strings.HasPrefix(key, "target_") || (key != "cache_hit_rate_percent" && key != "hit_rate_percent" && !strings.HasSuffix(key, "_hit_rate_percent")) {
				continue
			}
			rate, ok := numberMetric(metrics, key)
			if !ok {
				continue
			}
			name := "命中率"
			cachedKey, promptKey := "cached_tokens", "prompt_tokens"
			if key != "cache_hit_rate_percent" && key != "hit_rate_percent" {
				prefix := strings.TrimSuffix(key, "_hit_rate_percent")
				name = strings.ReplaceAll(prefix, "_", " ")
				cachedKey, promptKey = prefix+"_cached_tokens", prefix+"_prompt_tokens"
			}
			entry := cachePerformanceRow{label: label + " · " + name, rate: rate, cachedTokens: numberText(metrics, cachedKey, 0), promptTokens: numberText(metrics, promptKey, 0)}
			if benchmark && name == "命中率" {
				cacheRows = append(cacheRows, entry)
			} else {
				dedicatedCache = append(dedicatedCache, entry)
			}
		}
		if curve, ok := metrics["decay_curve"].([]any); ok {
			for _, point := range curve {
				values, ok := point.(map[string]any)
				if !ok {
					continue
				}
				seconds, hasSeconds := numberMetric(values, "seconds")
				if !hasSeconds {
					seconds, hasSeconds = numberMetric(values, "checkpoint_seconds")
				}
				rate, hasRate := numberMetric(values, "hit_rate_percent")
				if hasSeconds && hasRate {
					dedicatedCache = append(dedicatedCache, cachePerformanceRow{
						label: fmt.Sprintf("%s · TTL %.0fs", label, seconds), rate: rate,
						cachedTokens: numberText(values, "cached_tokens", 0), promptTokens: numberText(values, "prompt_tokens", 0),
						seconds: seconds, hasSeconds: true,
					})
				}
			}
		}
	}
	// 有专门缓存用例时只用它们，否则回退到基准场景的缓存命中，保证图不空也不重复。
	if len(dedicatedCache) > 0 {
		cacheRows = dedicatedCache
	}
	return rows, cacheRows
}

func hasPerformanceMetrics(metrics map[string]any) bool {
	for _, key := range []string{
		"ttft_p50_ms", "ttft_p90_ms", "tpot_p50_ms", "tpot_p90_ms", "e2e_p50_ms", "e2e_p90_ms",
		"otps_p50", "otps_p90", "success_rate_percent",
	} {
		if _, ok := numberMetric(metrics, key); ok {
			return true
		}
	}
	return false
}

func numberMetric(metrics map[string]any, key string) (float64, bool) {
	value, ok := metrics[key]
	if !ok {
		return 0, false
	}
	switch n := value.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func numberText(metrics map[string]any, key string, decimals int) string {
	value, ok := numberMetric(metrics, key)
	if !ok {
		return "—"
	}
	if decimals == 0 {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.*f", decimals, value)
}

func positiveMetricText(metrics map[string]any, key string, decimals int) string {
	value, ok := numberMetric(metrics, key)
	if !ok || value <= 0 {
		return "—"
	}
	return numberText(metrics, key, decimals)
}

func percentMetricText(metrics map[string]any, key string) string {
	value, ok := numberMetric(metrics, key)
	if !ok {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", value)
}

func benchmarkInputText(metrics map[string]any) string {
	for _, key := range []string{"calibrated_input_tokens", "target_input_tokens", "input_tokens"} {
		if value := numberText(metrics, key, 0); value != "—" {
			return value
		}
	}
	return "—"
}

// perfGate 是一项性能门槛：实测 key、门槛 key、标签与方向。
type perfGate struct {
	key, target, label, unit string
	decimals                 int
	higher                   bool
}

// 准入结论只以四项延迟门槛同时达标为准（与历史报告口径一致）。
var perfLatencyGates = []perfGate{
	{"ttft_p50_ms", "target_ttft_p50_ms", "TTFT P50", "ms", 0, false},
	{"ttft_p95_ms", "target_ttft_p95_ms", "TTFT P95", "ms", 0, false},
	{"tpot_p50_ms", "target_tpot_p50_ms", "TPOT P50", "ms", 1, false},
	{"e2e_p90_ms", "target_e2e_p90_ms", "E2E P90", "ms", 0, false},
}

// 场景卡里的完整校验还覆盖 TPOT P95 与成功率。
var perfCardGates = append(append([]perfGate{}, perfLatencyGates...),
	perfGate{"tpot_p95_ms", "target_tpot_p95_ms", "TPOT P95", "ms", 1, false},
	perfGate{"success_rate_percent", "target_success_rate_percent", "成功率", "%", 1, true},
)

func gatePass(value, target float64, higher bool) bool {
	if higher {
		return value >= target
	}
	return value <= target
}

// perfVerdict 统计所有档位中四项延迟全部达标的数量，以及各门槛未达标档位数。
func perfVerdict(rows []performanceRow) (total, allPass int, failedByGate []int) {
	failedByGate = make([]int, len(perfLatencyGates))
	for _, row := range rows {
		checked, failed := false, false
		for i, gate := range perfLatencyGates {
			value, hasValue := numberMetric(row.metrics, gate.key)
			target, hasTarget := numberMetric(row.metrics, gate.target)
			if !hasValue || !hasTarget {
				continue
			}
			checked = true
			if !gatePass(value, target, gate.higher) {
				failed = true
				failedByGate[i]++
			}
		}
		if !checked {
			continue
		}
		total++
		if !failed {
			allPass++
		}
	}
	return total, allPass, failedByGate
}

// compliance 返回通过率（豁免与跳过不计入分母）。
func compliance(s summary) (passed, assessed int, rate float64) {
	assessed = s.Total - s.Exempt - s.Skipped
	if assessed < 0 {
		assessed = 0
	}
	if assessed == 0 {
		return s.Passed, 0, 0
	}
	return s.Passed, assessed, float64(s.Passed) / float64(assessed) * 100
}

type levelStat struct {
	total, passed, failed int
}

func levelStats(rep report) map[string]levelStat {
	stats := map[string]levelStat{}
	for _, c := range rep.Cases {
		level := normalizeLevel(c.Level)
		if level == "" {
			level = "其他"
		}
		st := stats[level]
		st.total++
		switch c.Status {
		case "passed":
			st.passed++
		case "failed":
			st.failed++
		}
		stats[level] = st
	}
	return stats
}

func (s levelStat) label() string {
	if s.total == 0 {
		return "—"
	}
	if s.failed > 0 {
		return "失败"
	}
	return "通过"
}

func (s levelStat) colors() (fg, bg [3]uint8) {
	if s.total == 0 {
		return colorFaint, colorPanel
	}
	if s.failed > 0 {
		return colorRed, colorRedBg
	}
	return colorGreen, colorGreenBg
}

// rateColors 按通过率给出绿/橙/红，配合浅底大数字。
func rateColors(rate float64) (fg, bg [3]uint8) {
	switch {
	case rate >= 90:
		return colorGreen, colorGreenBg
	case rate >= 70:
		return colorAmber, colorAmberBg
	default:
		return colorRed, colorRedBg
	}
}

func verdictFailed(verdict string) bool {
	return strings.Contains(verdict, "未通过") || strings.Contains(verdict, "不通过") ||
		strings.Contains(verdict, "失败") || strings.Contains(verdict, "无效")
}

func verdictColors(verdict string) (fg, bg [3]uint8) {
	if verdictFailed(verdict) {
		return colorRed, colorRedBg
	}
	if strings.Contains(verdict, "通过") || strings.Contains(verdict, "0 项") {
		return colorGreen, colorGreenBg
	}
	return colorAmber, colorAmberBg
}

// durationText 把两个 RFC3339 时间换算为「3m 12s」。
func durationText(start, end string) string {
	if start == "" || end == "" {
		return "—"
	}
	t0, err0 := time.Parse(time.RFC3339, start)
	t1, err1 := time.Parse(time.RFC3339, end)
	if err0 != nil || err1 != nil || t1.Before(t0) {
		return "—"
	}
	d := t1.Sub(t0)
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

var reportLocation = time.FixedZone("CST", 8*60*60)

// stampText 把 RFC3339 转为东八区「2026-11-09 15:34」，解析失败时原样返回。
func stampText(s string) string {
	if s == "" {
		return "—"
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(reportLocation).Format("2006-01-02 15:04")
	}
	return s
}

// isDashboardMetric 判断指标是否已在性能看板/场景卡中展示，避免用例明细里重复。
func isDashboardMetric(key string) bool {
	if key == "p99_sample_note" || strings.HasPrefix(key, "target_") {
		return true
	}
	switch key {
	case "concurrency", "samples", "minimum_samples", "duration_ms", "success_rate_percent", "error_rate_percent", "uncontrolled_requests",
		"tpot_samples", "http_200_count", "finish_stop_count", "finish_length_count", "conn_reused_samples", "conn_new_samples",
		"prompt_tokens", "cached_tokens", "cache_read_tokens", "output_tokens", "input_tokens_total", "output_tokens_total", "cache_read_tokens_total",
		"cache_hit_rate_percent", "cache_coverage_percent", "cache_hit_requests", "cache_total_requests", "input_tokens_per_request", "output_tokens_per_request", "cache_read_tokens_per_request",
		"min_latency_p50_ms", "ttft_cv", "latency_p99_p50_ratio", "ttft_p99_p50_ratio", "decay_curve", "sample_ttft_ms":
		return true
	}
	for _, prefix := range []string{"ttft_", "ttfb_", "tpot_", "e2e_", "net_prefill_", "max_itl_", "otps_", "conn_", "itl_", "repeat_", "multiturn_", "suffix_", "target_input_", "calibrated_input_"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return strings.HasSuffix(key, "_hit_rate_percent") || strings.HasSuffix(key, "_cached_tokens") || strings.HasSuffix(key, "_prompt_tokens")
}

// videoLimitInfo 是可选视频体积上限用例的关键指标，供 PDF 首页展示。
type videoLimitInfo struct {
	base64MiB     float64
	base64BodyMiB float64
	urlMiB        float64
	urlBodyMiB    float64
	urlSupported  bool
	urlNote       string
	base64Probes  float64
	urlProbes     float64
}

func collectVideoLimit(cases []caseRow) (videoLimitInfo, bool) {
	for _, c := range cases {
		if c.CaseID != "M-B2" {
			continue
		}
		var m map[string]any
		if json.Unmarshal(c.Metrics, &m) != nil {
			continue
		}
		base64, ok := numberMetric(m, "base64_limit_mib")
		if !ok {
			continue
		}
		v := videoLimitInfo{base64MiB: base64}
		v.base64BodyMiB, _ = numberMetric(m, "base64_max_request_bytes")
		v.base64BodyMiB /= 1 << 20
		v.urlMiB, _ = numberMetric(m, "url_limit_mib")
		v.urlBodyMiB, _ = numberMetric(m, "url_max_request_bytes")
		v.urlBodyMiB /= 1 << 20
		v.urlSupported, _ = m["url_supported"].(bool)
		v.urlNote, _ = m["url_note"].(string)
		v.base64Probes, _ = numberMetric(m, "base64_probes")
		v.urlProbes, _ = numberMetric(m, "url_probes")
		return v, true
	}
	return videoLimitInfo{}, false
}
