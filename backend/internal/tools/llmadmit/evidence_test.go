package llmadmit

import (
	"strings"
	"testing"
	"time"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

func TestExplainCaseReasoningExhausted(t *testing.T) {
	row := CaseResult{
		Status:  suite.StatusFailed,
		Summary: "B-12K 未达标",
		Metrics: `{"uncontrolled_requests":7,"samples":24}`,
	}
	text := explainCase(row, suite.CaseDoc{Expect: "响应完整输出=0"}, []suite.Defect{{Level: "P1", Expect: "响应完整输出=0", Actual: "7 次"}})
	if !strings.Contains(text, "响应完整输出") || strings.Contains(text, "回答没写完") {
		t.Fatalf("explanation=%q", text)
	}
}

func TestExplainCasePassed(t *testing.T) {
	row := CaseResult{Status: suite.StatusPassed, Summary: "红答红、蓝答蓝"}
	text := explainCase(row, suite.CaseDoc{Expect: "双对照通过"}, nil)
	if !strings.Contains(text, "通过") || !strings.Contains(text, "双对照通过") {
		t.Fatalf("explanation=%q", text)
	}
}

func TestCaseEvidenceItemShape(t *testing.T) {
	// 仅校验脱敏相关类型不会丢掉传输层字段。
	item := caseEvidenceItem{Label: "x", Request: "{}", Response: "{}", HTTPStatus: 200, DurationMs: (2 * time.Second).Milliseconds()}
	if item.HTTPStatus != 200 || item.DurationMs != 2000 {
		t.Fatalf("item=%+v", item)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://api.example.com":           "https://api.example.com/v1",
		"https://api.example.com/":          "https://api.example.com/v1",
		"https://api.example.com/v1":        "https://api.example.com/v1",
		"https://api.example.com/v1/":       "https://api.example.com/v1",
		"https://api.example.com/v2/llm/v1": "https://api.example.com/v2/llm/v1",
	}
	for in, want := range cases {
		if got := normalizeBaseURL(in); got != want {
			t.Errorf("normalizeBaseURL(%q)=%q want %q", in, got, want)
		}
	}
}
