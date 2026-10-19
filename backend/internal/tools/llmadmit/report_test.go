package llmadmit

import (
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

	full := m.buildReport(run, rows, false)
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

	masked := m.buildReport(run, rows, true)
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

func TestRenderMarkdownWatermark(t *testing.T) {
	run, rows := sampleRunAndRows()
	m := &Module{}
	md := renderMarkdown(m.buildReport(run, rows, true))
	if !strings.Contains(md, Watermark) {
		t.Fatal("MD 缺水印")
	}
	if !strings.Contains(md, "脱敏") {
		t.Fatal("masked MD 缺脱敏标识")
	}
	if strings.Contains(md, "secret-gateway") {
		t.Fatal("masked MD 泄露目标地址")
	}
	full := renderMarkdown(m.buildReport(run, rows, false))
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
