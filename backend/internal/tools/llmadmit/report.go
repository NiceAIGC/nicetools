package llmadmit

import (
	"encoding/json"
	"fmt"
	"sort"
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
	// Merged 非空时是合并报告：列出参与合并的任务。
	Merged  []mergedRun `json:"merged,omitempty"`
	Defects []defectRow `json:"defects"`
	Cases   []caseRow   `json:"cases"`
}

type mergedRun struct {
	ID         string `json:"id"`
	Tier       string `json:"tier"`
	Verdict    string `json:"verdict"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type environment struct {
	Channel   string `json:"channel,omitempty"`
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

// buildReport 装配单任务导出；channel 非空时覆盖任务上的渠道名。
func (m *Module) buildReport(run Run, rows []CaseResult, masked bool, channel string) report {
	rep := report{
		Watermark:  Watermark,
		Masked:     masked,
		Standard:   run.SuiteName,
		Verdict:    run.Verdict,
		StopReason: run.StopReason,
		Environment: environment{
			Channel:   channel,
			BaseURL:   run.BaseURL,
			Model:     run.Model,
			Suite:     run.Suite,
			Tier:      run.Tier,
			CreatedBy: run.CreatedBy,
			Timezone:  time.Local.String(),
		},
	}
	if rep.Environment.Channel == "" {
		rep.Environment.Channel = run.Channel
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
	rep.Cases, rep.Defects = reportRows(rows, run.BaseURL, masked)
	return rep
}

// buildMergedReport 合并多次任务为一份报告：同一用例取最新一次结果，
// 环境与渠道名取最新任务（channel 非空时覆盖）；裁决按合并视图重算。
func (m *Module) buildMergedReport(runs []Run, rowsByID map[string][]CaseResult, masked bool, channel string) report {
	// runs 按创建时间升序；最新任务提供环境。
	newest := runs[len(runs)-1]
	rep := m.buildReport(newest, nil, masked, channel)
	if rep.Environment.Channel == "" {
		for i := len(runs) - 1; i >= 0; i-- {
			if runs[i].Channel != "" {
				rep.Environment.Channel = runs[i].Channel
				break
			}
		}
	}
	rep.Merged = []mergedRun{}
	for _, run := range runs {
		item := mergedRun{ID: run.ID, Tier: run.Tier, Verdict: run.Verdict}
		if run.FinishedAt != nil {
			item.FinishedAt = run.FinishedAt.Format(time.RFC3339)
		}
		rep.Merged = append(rep.Merged, item)
	}
	byKey := map[string]CaseResult{}
	for _, run := range runs {
		for _, row := range rowsByID[run.ID] {
			byKey[row.Tier+"/"+row.CaseID] = row
		}
	}
	merged := make([]CaseResult, 0, len(byKey))
	for _, row := range byKey {
		merged = append(merged, row)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Tier != merged[j].Tier {
			return tierRank(merged[i].Tier) < tierRank(merged[j].Tier)
		}
		return merged[i].Seq < merged[j].Seq
	})
	rep.Cases, rep.Defects = reportRows(merged, newest.BaseURL, masked)
	rep.Verdict = mergedVerdict(rep.Cases)
	if rep.Verdict != "通过" {
		rep.StopReason = "合并视图存在未通过档位"
	}
	return rep
}

func tierRank(tier string) int {
	switch tier {
	case "F":
		return 0
	case "M":
		return 1
	default:
		return 2
	}
}

// mergedVerdict 重算合并视图裁决：某档有 P0/required 失败或 P1 ≥ 3 → 不通过。
func mergedVerdict(cases []caseRow) string {
	p1 := map[string]int{}
	failed := false
	for _, c := range cases {
		if c.Status == suite.StatusFailed && (c.Level == string(suite.LevelP0) || c.Level == string(suite.LevelRequired)) {
			failed = true
		}
		var defects []suite.Defect
		if json.Unmarshal(c.Defects, &defects) == nil {
			for _, d := range defects {
				if d.Level == "P1" {
					p1[c.Tier]++
				}
			}
		}
	}
	for _, n := range p1 {
		if n >= 3 {
			failed = true
		}
	}
	if failed {
		return "不通过"
	}
	return "通过"
}

// reportRows 把用例行转为导出结构；masked 时脱敏。
func reportRows(rows []CaseResult, baseURL string, masked bool) ([]caseRow, []defectRow) {
	var cases []caseRow
	var defects []defectRow
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
		cases = append(cases, cr)
		var rowDefects []suite.Defect
		if json.Unmarshal([]byte(row.Defects), &rowDefects) == nil {
			for _, d := range rowDefects {
				dr := defectRow{CaseID: row.CaseID, Level: d.Level, Expect: d.Expect, Actual: d.Actual, Attribution: d.Attribution}
				if masked {
					dr.Expect, dr.Actual = redact(dr.Expect, baseURL), redact(dr.Actual, baseURL)
				}
				defects = append(defects, dr)
			}
		}
	}
	return cases, defects
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
	channel := rep.Environment.Channel
	if channel == "" {
		channel = "（未命名渠道）"
	}
	fmt.Fprintf(&b, "| 项 | 值 |\n|---|---|\n| 渠道 | %s |\n| 网关 | `%s` |\n| 模型 | `%s` |\n| 档位 | %s |\n| 结论 | **%s** |\n| 时段 | %s ~ %s (%s) |\n\n",
		channel, rep.Environment.BaseURL, rep.Environment.Model, rep.Environment.Tier, rep.Verdict, rep.Environment.StartedAt, rep.Environment.EndedAt, rep.Environment.Timezone)
	if len(rep.Merged) > 0 {
		fmt.Fprintf(&b, "本报告合并自 %d 次测试：\n\n| 任务 | 档位 | 完成时间 | 结论 |\n|---|---|---|---|\n", len(rep.Merged))
		for _, item := range rep.Merged {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", item.ID, item.Tier, item.FinishedAt, item.Verdict)
		}
		b.WriteString("\n")
	}
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
