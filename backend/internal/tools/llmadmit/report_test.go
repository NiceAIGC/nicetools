package llmadmit

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleRunAndRows() (Run, []CaseResult) {
	now := time.Now()
	run := Run{
		ID: "r1", CreatedBy: "admin", Suite: "k3-v1", SuiteName: "K3 标准",
		BaseURL: "https://secret-gateway.example.com/v1", Model: "kimi-k3", Tier: "F",
		Declarations: `{"video":false}`, Status: runSucceeded, Verdict: "通过",
		StartedAt: &now, FinishedAt: &now,
	}
	rows := []CaseResult{{
		RunID: "r1", Seq: 1, CaseID: "F-01", Tier: "F", Name: "真伪指纹", Level: "P0",
		Status: "passed", HTTPStatus: 200, RequestID: "req-secret-123", DurationMs: 120,
		Summary: "增量 +7/+10", Metrics: "{}", Defects: "[]",
		Evidence:    `[{"label":"a","request":"{}","result":{"http_status":200,"raw":"ok"}}]`,
		EvidenceSHA: "abc",
	}, {
		RunID: "r1", Seq: 2, CaseID: "F-11", Tier: "F", Name: "鉴权", Level: "required",
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
		{RunID: "r", Seq: 1, CaseID: "B-4K", Tier: "B", Name: "基准 <8K", Level: "P1", Status: "passed",
			Metrics: `{"ttft_p50_ms":1240,"ttft_p90_ms":1965,"cache_hit_rate_percent":96.4}`, Defects: "[]"},
		{RunID: "r", Seq: 2, CaseID: "B-12K", Tier: "B", Name: "基准 8-16K", Level: "P1", Status: "passed",
			Metrics: `{"ttft_p50_ms":1645,"ttft_p90_ms":2579,"cache_hit_rate_percent":96.7}`, Defects: "[]"},
	}
	m := &Module{}
	rep := m.buildReport(Run{ID: "r", Suite: "k3-v1", BaseURL: "http://gw", Model: "kimi-k3", Declarations: "{}"}, rows, false, "")
	md := renderMarkdown(rep)
	if !strings.Contains(md, "TTFT 对比") || !strings.Contains(md, "缓存命中率") {
		t.Fatal("MD 缺图表区")
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
	run1 := Run{ID: "r1", Suite: "k3-v1", SuiteName: "K3", BaseURL: "https://gw-a.example.com/v1", Model: "kimi-k3", Tier: "F", Channel: "张三的K3渠道", Declarations: "{}", Status: runFailed, Verdict: "不通过", CreatedAt: t1, FinishedAt: &t1}
	run2 := Run{ID: "r2", Suite: "k3-v1", SuiteName: "K3", BaseURL: "https://gw-a.example.com/v1", Model: "kimi-k3", Tier: "M", Declarations: "{}", Status: runSucceeded, Verdict: "通过", CreatedAt: t2, FinishedAt: &t2}
	rows1 := []CaseResult{
		{RunID: "r1", Seq: 1, CaseID: "F-01", Tier: "F", Name: "真伪指纹", Level: "P0", Status: "failed", Summary: "旧结果", Metrics: "{}", Defects: `[{"level":"P0","expect":"e","actual":"a","attribution":"upstream"}]`},
		{RunID: "r1", Seq: 2, CaseID: "F-02", Tier: "F", Name: "身份", Level: "P0", Status: "passed", Summary: "ok", Metrics: "{}", Defects: "[]"},
	}
	rows2 := []CaseResult{
		{RunID: "r2", Seq: 1, CaseID: "F-01", Tier: "F", Name: "真伪指纹", Level: "P0", Status: "passed", Summary: "新结果", Metrics: "{}", Defects: "[]"},
		{RunID: "r2", Seq: 2, CaseID: "M-01", Tier: "M", Name: "Stream", Level: "required", Status: "passed", Summary: "ok", Metrics: "{}", Defects: "[]"},
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
	if rep.Cases[0].Tier != "F" || rep.Cases[2].Tier != "M" {
		t.Fatalf("排序错误: %s/%s", rep.Cases[0].Tier, rep.Cases[2].Tier)
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
