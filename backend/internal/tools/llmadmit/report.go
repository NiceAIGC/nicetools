package llmadmit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// Watermark 是所有导出报告的归属水印。
const Watermark = "niceaigc"

// report 是导出文档结构（JSON 即此结构的序列化）。
type report struct {
	Watermark    string          `json:"watermark"`
	Masked       bool            `json:"masked"`
	Standard     string          `json:"standard"`
	Environment  environment     `json:"environment"`
	Verdict      string          `json:"verdict"`
	StopReason   string          `json:"stop_reason,omitempty"`
	Declarations map[string]bool `json:"declarations"`
	Defects      []defectRow     `json:"defects"`
	Cases        []caseRow       `json:"cases"`
}

type environment struct {
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	Suite     string `json:"suite"`
	Tier      string `json:"tier"`
	CreatedBy string `json:"created_by"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"finished_at,omitempty"`
	Timezone  string `json:"timezone"`
}

type caseRow struct {
	CaseID      string          `json:"case_id"`
	Tier        string          `json:"tier"`
	Name        string          `json:"name"`
	Level       string          `json:"level"`
	Status      string          `json:"status"`
	Summary     string          `json:"summary"`
	HTTPStatus  int             `json:"http_status,omitempty"`
	RequestID   string          `json:"request_id,omitempty"`
	DurationMs  int64           `json:"duration_ms"`
	Retries     int             `json:"retries,omitempty"`
	Metrics     json.RawMessage `json:"metrics"`
	Defects     json.RawMessage `json:"defects"`
	EvidenceSHA string          `json:"evidence_sha"`
	// 仅详细报告包含原文；脱敏报告剔除。
	Evidence json.RawMessage `json:"evidence,omitempty"`
}

type defectRow struct {
	CaseID      string `json:"case_id"`
	Level       string `json:"level"`
	Expect      string `json:"expect"`
	Actual      string `json:"actual"`
	Attribution string `json:"attribution"`
}

// maskURL 脱敏网关地址：只保留 scheme 与路径，主机整体打码。
func maskURL(raw string) string {
	scheme := ""
	rest := raw
	if i := strings.Index(raw, "://"); i >= 0 {
		scheme = raw[:i+3]
		rest = raw[i+3:]
	}
	path := ""
	if i := strings.Index(rest, "/"); i >= 0 {
		path = rest[i:]
	}
	return scheme + "***" + path
}

// buildReport 装配导出结构；masked=true 时剔除目标敏感信息。
func (m *Module) buildReport(run Run, rows []CaseResult, masked bool) report {
	rep := report{
		Watermark:  Watermark,
		Masked:     masked,
		Standard:   run.SuiteName,
		Verdict:    run.Verdict,
		StopReason: run.StopReason,
		Environment: environment{
			BaseURL:   run.BaseURL,
			Model:     run.Model,
			Suite:     run.Suite,
			Tier:      run.Tier,
			CreatedBy: run.CreatedBy,
			Timezone:  time.Local.String(),
		},
	}
	if masked {
		rep.Environment.BaseURL = maskURL(run.BaseURL)
	}
	if run.StartedAt != nil {
		rep.Environment.StartedAt = run.StartedAt.Format(time.RFC3339)
	}
	if run.FinishedAt != nil {
		rep.Environment.EndedAt = run.FinishedAt.Format(time.RFC3339)
	}
	_ = json.Unmarshal([]byte(run.Declarations), &rep.Declarations)

	baseURL := run.BaseURL
	for _, row := range rows {
		cr := caseRow{
			CaseID: row.CaseID, Tier: row.Tier, Name: row.Name, Level: row.Level,
			Status: row.Status, Summary: row.Summary, HTTPStatus: row.HTTPStatus,
			RequestID: row.RequestID, DurationMs: row.DurationMs, Retries: row.Retries,
			Metrics: json.RawMessage(row.Metrics), Defects: json.RawMessage(row.Defects),
			EvidenceSHA: row.EvidenceSHA,
		}
		if !masked && row.Evidence != "" {
			cr.Evidence = json.RawMessage(row.Evidence)
		}
		if masked {
			cr.RequestID = ""
			cr.Summary = redact(cr.Summary, baseURL)
		}
		rep.Cases = append(rep.Cases, cr)
		var defects []suite.Defect
		if json.Unmarshal([]byte(row.Defects), &defects) == nil {
			for _, d := range defects {
				dr := defectRow{CaseID: row.CaseID, Level: d.Level, Expect: d.Expect, Actual: d.Actual, Attribution: d.Attribution}
				if masked {
					dr.Expect, dr.Actual = redact(dr.Expect, baseURL), redact(dr.Actual, baseURL)
				}
				rep.Defects = append(rep.Defects, dr)
			}
		}
	}
	return rep
}

// redact 抹掉文本中出现的目标地址与 Bearer 密钥片段。
func redact(s, baseURL string) string {
	out := strings.ReplaceAll(s, baseURL, maskURL(baseURL))
	if i := strings.Index(out, "Bearer "); i >= 0 {
		j := i + len("Bearer ")
		end := strings.IndexAny(out[j:], " \"\n")
		if end < 0 {
			out = out[:j] + "***"
		} else {
			out = out[:j] + "***" + out[j+end:]
		}
	}
	return out
}

// renderMarkdown 把报告渲染为可读格式（§7），每页首行与结尾嵌入水印。
func renderMarkdown(rep report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- %s -->\n", Watermark)
	fmt.Fprintf(&b, "# %s 准入测试报告\n\n> 本报告由 **%s** 准入测试系统生成\n\n", rep.Standard, Watermark)
	if rep.Masked {
		b.WriteString("> ⚠️ 脱敏版：已隐去测试目标地址、密钥与 request_id\n\n")
	}
	b.WriteString("## 环境\n\n")
	fmt.Fprintf(&b, "| 项 | 值 |\n|---|---|\n| 网关 | `%s` |\n| 模型 | `%s` |\n| 档位 | %s |\n| 结论 | **%s** |\n| 时段 | %s ~ %s (%s) |\n\n",
		rep.Environment.BaseURL, rep.Environment.Model, rep.Environment.Tier, rep.Verdict, rep.Environment.StartedAt, rep.Environment.EndedAt, rep.Environment.Timezone)
	if len(rep.Declarations) > 0 {
		b.WriteString("## 能力声明\n\n| 能力 | 声明支持 |\n|---|---|\n")
		for k, v := range rep.Declarations {
			fmt.Fprintf(&b, "| %s | %v |\n", k, v)
		}
		b.WriteString("\n")
	}
	if len(rep.Defects) > 0 {
		b.WriteString("## 缺陷清单\n\n| 用例 | 级别 | 期望 | 实测 | 归因 |\n|---|---|---|---|---|\n")
		for _, d := range rep.Defects {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", d.CaseID, d.Level, d.Expect, d.Actual, d.Attribution)
		}
		b.WriteString("\n")
	}
	b.WriteString("## 用例明细\n\n| 用例 | 名称 | 级别 | 结论 | HTTP | 耗时(ms) | 摘要 |\n|---|---|---|---|---|---|---|\n")
	for _, c := range rep.Cases {
		fmt.Fprintf(&b, "| %s-%s | %s | %s | %s | %d | %d | %s |\n",
			c.Tier, c.CaseID, c.Name, c.Level, c.Status, c.HTTPStatus, c.DurationMs, strings.ReplaceAll(c.Summary, "|", "\\|"))
	}
	fmt.Fprintf(&b, "\n---\n\n*generated by %s · %s*\n", Watermark, Watermark)
	return b.String()
}
