package llmadmit

import (
	"encoding/json"
	"nicetools/backend/internal/tools/llmadmit/suite"
	"strings"
	"testing"
	"time"
)

func sampleRunAndRows() (Run, []CaseResult) {
	now := time.Now()
	run := Run{
		ID: "r1", CreatedBy: "admin", Suite: "k3-v1", SuiteName: "K3 标准",
		BaseURL: "https://secret-gateway.example.com/v1", Model: "kimi-k3", Preset: "format",
		Declarations: `{"video":false}`, Status: runSucceeded, Verdict: "通过",
		StartedAt: &now, FinishedAt: &now,
	}
	rows := []CaseResult{{
		RunID: "r1", Seq: 1, CaseID: "F-01", Group: "format", Name: "真伪指纹", Level: "P0",
		Status: "passed", HTTPStatus: 200, RequestID: "req-secret-123", DurationMs: 120,
		Summary: "增量 +7/+10", Metrics: "{}", Defects: "[]",
		Evidence:    `[{"label":"a","request":"{}","result":{"http_status":200,"raw":"ok"}}]`,
		EvidenceSHA: "abc",
	}, {
		RunID: "r1", Seq: 2, CaseID: "F-11", Group: "format", Name: "鉴权", Level: "required",
		Status: "failed", HTTPStatus: 403, RequestID: "req-secret-456", DurationMs: 30,
		Summary: "访问 https://secret-gateway.example.com/v1 返回 403",
		Metrics: "{}", Defects: `[{"level":"P1","expect":"401","actual":"403 from https://secret-gateway.example.com/v1","attribution":"gateway"}]`,
		Evidence: `[{"label":"无鉴权","request":"{}","result":{"http_status":403}}]`, EvidenceSHA: "def",
	}}
	return run, rows
}

// 详细报告：保留地址与证据原文；脱敏报告：地址打码、request_id 剔除、证据剔除，两者都有水印。
func TestReportMasking(t *testing.T) {
	run, rows := sampleRunAndRows()
	rows[0].Metrics = `{"endpoint":"https://secret-gateway.example.com/v1/messages","context_length":1048576}`
	m := &Module{}

	full := m.buildReport(run, rows, false, "")
	if full.Environment.BaseURL != "https://secret-gateway.example.com/v1" {
		t.Fatalf("full base_url=%s", full.Environment.BaseURL)
	}
	if full.Cases[0].RequestID != "req-secret-123" {
		t.Fatal("full 应保留 request_id")
	}
	if len(full.Cases[0].Evidence) == 0 {
		t.Fatal("full 应保留证据原文")
	}
	if full.Watermark != Watermark {
		t.Fatal("缺水印")
	}

	masked := m.buildReport(run, rows, true, "")
	if strings.Contains(masked.Environment.BaseURL, "secret-gateway") {
		t.Fatalf("masked base_url 泄露: %s", masked.Environment.BaseURL)
	}
	if masked.Cases[0].RequestID != "" {
		t.Fatal("masked 应剔除 request_id")
	}
	if len(masked.Cases[0].Evidence) != 0 {
		t.Fatal("masked 应剔除证据原文")
	}
	if strings.Contains(masked.Cases[1].Summary, "secret-gateway") {
		t.Fatalf("masked summary 泄露: %s", masked.Cases[1].Summary)
	}
	if strings.Contains(masked.Defects[0].Actual, "secret-gateway") {
		t.Fatalf("masked defect 泄露: %s", masked.Defects[0].Actual)
	}
	if strings.Contains(string(masked.Cases[0].Metrics), "secret-gateway") {
		t.Fatalf("masked metrics 泄露: %s", masked.Cases[0].Metrics)
	}
	if masked.Watermark != Watermark || !masked.Masked {
		t.Fatal("masked 报告缺水印或标记")
	}
}

// 详细报告：证据带脱敏 curl（密钥打码、含请求体），无泄露。
func TestFullReportContainsMaskedCurl(t *testing.T) {
	run, rows := sampleRunAndRows()
	run.APIKey = "sk-super-secret"
	m := &Module{}
	rep := m.buildReport(run, rows, false, "")
	var ev []map[string]any
	if err := json.Unmarshal(rep.Cases[0].Evidence, &ev); err != nil || len(ev) == 0 {
		t.Fatal("详细报告缺证据")
	}
	curl, _ := ev[0]["curl"].(string)
	if !strings.Contains(curl, "curl -X POST 'https://secret-gateway.example.com/v1/chat/completions'") {
		t.Fatalf("curl 示例异常: %s", curl)
	}
	if strings.Contains(curl, "sk-super-secret") {
		t.Fatal("curl 泄露密钥")
	}
}

// PDF：能产出非空 PDF 文件头与 CJK 内容。
func TestRenderPDF(t *testing.T) {
	run, rows := sampleRunAndRows()
	m := &Module{}
	data, err := renderPDF(m.buildReport(run, rows, false, "张三的K3渠道"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 10*1024 || !strings.HasPrefix(string(data[:5]), "%PDF-") {
		t.Fatalf("PDF 输出异常: %d bytes", len(data))
	}
}

// 图表：有 TTFT/缓存指标的用例会产出图表区（PDF 与 MD 同数据源）。
func TestReportCharts(t *testing.T) {
	rows := []CaseResult{
		{RunID: "r", Seq: 1, CaseID: "B-4K", Group: "latency", Name: "基准 <8K", Level: "P1", Status: "passed",
			Metrics: `{"ttft_p50_ms":1240,"ttft_p90_ms":1965,"ttft_p95_ms":2200,"ttft_p99_ms":2450,"sample_ttft_ms":[1000,1200,1400,1600],"cache_hit_rate_percent":96.4,"cached_tokens":9640,"prompt_tokens":10000}`, Defects: "[]"},
		{RunID: "r", Seq: 2, CaseID: "B-12K", Group: "latency", Name: "基准 8-16K", Level: "P1", Status: "passed",
			Metrics: `{"ttft_p50_ms":1645,"ttft_p90_ms":2579,"ttft_p95_ms":2800,"ttft_p99_ms":3100,"sample_ttft_ms":[1300,1600,1900,2200],"cache_hit_rate_percent":96.7,"cached_tokens":9670,"prompt_tokens":10000}`, Defects: "[]"},
	}
	m := &Module{}
	rep := m.buildReport(Run{ID: "r", Suite: "k3-v1", BaseURL: "http://gw", Model: "kimi-k3", Declarations: "{}"}, rows, false, "")
	md := renderMarkdown(rep)
	if !strings.Contains(md, "## 性能对比") || !strings.Contains(md, "TTFT P95") || !strings.Contains(md, "## 缓存命中") {
		t.Fatal("Markdown 缺少可视化性能或缓存总览")
	}
	if !strings.Contains(md, "直观结论") || !strings.Contains(md, "B-4K") || !strings.Contains(md, "B-12K") {
		t.Fatal("Markdown 未呈现直观状态和场景")
	}
	pdf, err := renderPDF(rep)
	if err != nil {
		t.Fatal(err)
	}
	// PDF 是压缩流，图表内容在冒烟中用 pdftotext 验证；这里保证含图表数据的报告能渲染。
	if len(pdf) < 10*1024 {
		t.Fatalf("PDF 输出异常: %d bytes", len(pdf))
	}
	// 摘要统计：2 通过。
	if rep.Summary.Passed != 2 || rep.Summary.Total != 2 {
		t.Fatalf("summary=%+v", rep.Summary)
	}
}

func TestCollectPerformanceIncludesSingleScenarioAndCacheCurve(t *testing.T) {
	rows := []caseRow{{
		CaseID: "B-4K", Group: "latency", Metrics: json.RawMessage(`{"calibrated_input_tokens":4096,"ttft_p50_ms":120,"ttft_p90_ms":240,"ttft_p95_ms":300,"ttft_p99_ms":360,"tpot_p50_ms":20.5,"tpot_p90_ms":30,"tpot_p95_ms":35,"tpot_p99_ms":40,"e2e_p50_ms":800,"e2e_p90_ms":1200,"e2e_p95_ms":1400,"e2e_p99_ms":1600,"otps_p50":50.5,"otps_p90":70.5,"itl_p95_ms":11.5,"cache_hit_rate_percent":91.25,"cached_tokens":9125,"prompt_tokens":10000,"success_rate_percent":100,"error_rate_percent":0,"samples":50,"decay_curve":[{"seconds":0,"hit_rate_percent":99.1,"cached_tokens":991,"prompt_tokens":1000},{"seconds":30,"hit_rate_percent":88.2,"cached_tokens":882,"prompt_tokens":1000}]}`),
	}}
	performance, cache := collectPerformance(rows)
	if len(performance) != 1 {
		t.Fatalf("single benchmark scenario omitted: got %d rows", len(performance))
	}
	for key, want := range map[string]float64{"ttft_p99_ms": 360, "tpot_p95_ms": 35, "e2e_p90_ms": 1200, "otps_p50": 50.5, "itl_p95_ms": 11.5} {
		if got, ok := numberMetric(performance[0].metrics, key); !ok || got != want {
			t.Errorf("%s = %v, present=%v; want %v", key, got, ok, want)
		}
	}
	// 有专门 TTL 曲线时，该场景的点命中率不再单独占一行，避免同一数字出现两次。
	if len(cache) != 2 || cache[0].rate != 99.1 || cache[1].rate != 88.2 || cache[1].cachedTokens != "882" {
		t.Fatalf("cache curve rows wrong: %+v", cache)
	}
	// 只有基准场景（无专门缓存用例）时回退到场景命中率，保证缓存图不为空。
	_, fallback := collectPerformance([]caseRow{{CaseID: "B-8K", Metrics: json.RawMessage(`{"ttft_p50_ms":120,"cache_hit_rate_percent":91.25,"cached_tokens":9125,"prompt_tokens":10000}`)}})
	if len(fallback) != 1 || fallback[0].rate != 91.25 {
		t.Fatalf("benchmark cache fallback wrong: %+v", fallback)
	}
}

func TestDashboardMetricsAreNotRepeatedInCaseDetails(t *testing.T) {
	for _, key := range []string{"ttft_p50_ms", "tpot_p95_ms", "otps_p90", "cache_hit_rate_percent", "repeat_cached_tokens", "decay_curve", "prompt_tokens", "success_rate_percent", "samples"} {
		if !isDashboardMetric(key) {
			t.Errorf("%q should be included in the dashboard, not repeated in case details", key)
		}
	}
	for _, key := range []string{"model", "context_length", "finish_reason"} {
		if isDashboardMetric(key) {
			t.Errorf("non-performance metric %q was filtered", key)
		}
	}
}

func TestTableFragmentLines(t *testing.T) {
	for _, tc := range []struct {
		remaining int
		available int
		want      int
	}{
		{remaining: 3, available: 10, want: 3},
		{remaining: 10, available: 10, want: 10},
		{remaining: 25, available: 10, want: 10},
	} {
		if got := tableFragmentLines(tc.remaining, tc.available); got != tc.want {
			t.Errorf("tableFragmentLines(%d, %d) = %d, want %d", tc.remaining, tc.available, got, tc.want)
		}
	}
}

func TestRenderPDFPaginatesLongBlocks(t *testing.T) {
	run, rows := sampleRunAndRows()
	rep := (&Module{}).buildReport(run, rows, false, strings.Repeat("渠道", 40))
	long := strings.Repeat("跨页内容验证，检查分页与文本对齐。", 500)
	rep.Cases[0].Doc.Purpose = long
	rep.Cases[0].Doc.Method = long
	metrics, err := json.Marshal(map[string]any{"long_metric": long})
	if err != nil {
		t.Fatal(err)
	}
	rep.Cases[0].Metrics = metrics
	evidence, err := json.Marshal([]map[string]any{{
		"label": "长请求",
		"curl":  "curl " + strings.Repeat("x", 12000),
		"result": map[string]any{
			"http_status": 200,
			"raw":         long,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rep.Cases[0].Evidence = evidence
	w, err := renderPDFWriter(rep)
	if err != nil {
		t.Fatal(err)
	}
	if pages := w.pdf.GetNumberOfPages(); pages < 4 {
		t.Fatalf("long report rendered to %d pages, want at least 4", pages)
	}
}

func TestPDFTextBaselinesStayInsideLayoutBoxes(t *testing.T) {
	if got := pdfTextBaseline(pdfContentTop, 16); got != 68 {
		t.Fatalf("title baseline = %.1f, want 68", got)
	}
	if pdfContentTop < pdfHeaderRuleY+16+6 {
		t.Fatalf("title can overlap header rule: top=%g rule=%g", pdfContentTop, pdfHeaderRuleY)
	}
	cellTop, fontSize, rowHeight := 3.0, 8.5, 16.0
	baseline := pdfTextBaseline(cellTop, fontSize)
	if baseline-fontSize < cellTop || baseline > rowHeight {
		t.Fatalf("table text escapes row: top=%.1f baseline=%.1f rowHeight=%.1f", cellTop, baseline, rowHeight)
	}
}

func TestRenderMarkdownWatermark(t *testing.T) {
	run, rows := sampleRunAndRows()
	m := &Module{}
	md := renderMarkdown(m.buildReport(run, rows, true, ""))
	if !strings.Contains(md, Watermark) {
		t.Fatal("MD 缺水印")
	}
	if !strings.Contains(md, "脱敏") {
		t.Fatal("masked MD 缺脱敏标识")
	}
	if strings.Contains(md, "secret-gateway") {
		t.Fatal("masked MD 泄露目标地址")
	}
	full := renderMarkdown(m.buildReport(run, rows, false, ""))
	if !strings.Contains(full, "| 用例 |") {
		t.Fatal("MD 缺用例明细表")
	}
}

func TestLegacyTierRowsRemainReadable(t *testing.T) {
	run := Run{ID: "legacy", SuiteName: "旧标准", LegacyTier: "F", Declarations: "{}"}
	rows := []CaseResult{{RunID: "legacy", CaseID: "F-01", LegacyTier: "F", Name: "旧用例", Status: suite.StatusPassed, Metrics: "{}", Defects: "[]"}}
	rep := (&Module{}).buildReport(run, rows, false, "")
	if rep.Environment.PresetName != "旧档位 F" {
		t.Fatalf("legacy scope=%q", rep.Environment.PresetName)
	}
	if len(rep.Summary.Groups) != 1 || rep.Summary.Groups[0].Label != "历史档位 F" {
		t.Fatalf("legacy group summary=%+v", rep.Summary.Groups)
	}
}
func TestMaskURL(t *testing.T) {
	if got := maskURL("https://api.example.com/v1"); strings.Contains(got, "example") {
		t.Fatalf("maskURL 泄露: %s", got)
	}
	if got := maskURL("http://192.168.1.1:8080/v1"); !strings.HasPrefix(got, "http://") {
		t.Fatalf("maskURL=%s", got)
	}
}

// 合并报告：同一用例取最新任务结果；渠道名取最新任务/覆盖参数；裁决按合并视图重算。
func TestMergedReport(t *testing.T) {
	m := &Module{}
	t1 := time.Now().Add(-2 * time.Hour)
	t2 := time.Now().Add(-time.Hour)
	run1 := Run{ID: "r1", Suite: "k3-v1", SuiteName: "K3", BaseURL: "https://gw-a.example.com/v1", Model: "kimi-k3", Preset: "format", Channel: "张三的K3渠道", Declarations: "{}", Status: runFailed, Verdict: "不通过", CreatedAt: t1, FinishedAt: &t1}
	run2 := Run{ID: "r2", Suite: "k3-v1", SuiteName: "K3", BaseURL: "https://gw-a.example.com/v1", Model: "kimi-k3", Preset: "format", Declarations: "{}", Status: runSucceeded, Verdict: "通过", CreatedAt: t2, FinishedAt: &t2}
	rows1 := []CaseResult{
		{RunID: "r1", Seq: 1, CaseID: "F-01", Group: "format", Name: "真伪指纹", Level: "P0", Status: "failed", Summary: "旧结果", Metrics: "{}", Defects: `[{"level":"P0","expect":"e","actual":"a","attribution":"upstream"}]`},
		{RunID: "r1", Seq: 2, CaseID: "F-02", Group: "format", Name: "身份", Level: "P0", Status: "passed", Summary: "ok", Metrics: "{}", Defects: "[]"},
	}
	rows2 := []CaseResult{
		{RunID: "r2", Seq: 1, CaseID: "F-01", Group: "format", Name: "真伪指纹", Level: "P0", Status: "passed", Summary: "新结果", Metrics: "{}", Defects: "[]"},
		{RunID: "r2", Seq: 2, CaseID: "M-01", Group: "format", Name: "Stream", Level: "required", Status: "passed", Summary: "ok", Metrics: "{}", Defects: "[]"},
	}
	rep := m.buildMergedReport([]Run{run1, run2}, map[string][]CaseResult{"r1": rows1, "r2": rows2}, false, "")
	if len(rep.Cases) != 3 {
		t.Fatalf("merged cases=%d want 3", len(rep.Cases))
	}
	for _, c := range rep.Cases {
		if c.CaseID == "F-01" && c.Summary != "新结果" {
			t.Fatalf("F-01 未取最新: %s", c.Summary)
		}
	}
	if got := []string{rep.Cases[0].CaseID, rep.Cases[1].CaseID, rep.Cases[2].CaseID}; got[0] != "F-01" || got[1] != "F-02" || got[2] != "M-01" {
		t.Fatalf("合并排序错误: %v", got)
	}
	if rep.Environment.Channel != "张三的K3渠道" {
		t.Fatalf("channel=%s", rep.Environment.Channel)
	}
	if rep.Verdict != "0 项未通过" {
		t.Fatalf("verdict=%s want 0 项未通过", rep.Verdict)
	}
	if len(rep.Merged) != 2 {
		t.Fatalf("merged runs=%d", len(rep.Merged))
	}
	// 覆盖渠道名。
	rep = m.buildMergedReport([]Run{run1, run2}, map[string][]CaseResult{"r1": rows1, "r2": rows2}, false, "李四的K3渠道")
	if rep.Environment.Channel != "李四的K3渠道" {
		t.Fatalf("override channel=%s", rep.Environment.Channel)
	}
}
