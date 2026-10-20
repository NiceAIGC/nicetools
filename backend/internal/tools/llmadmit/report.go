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
	LevelNote   string          `json:"level_note"` // 必过/豁免等含义说明
	Status      string          `json:"status"`
	Summary     string          `json:"summary"`
	HTTPStatus  int             `json:"http_status,omitempty"`
	RequestID   string          `json:"request_id,omitempty"`
	DurationMs  int64           `json:"duration_ms"`
	Retries     int             `json:"retries,omitempty"`
	Doc         suite.CaseDoc   `json:"doc"`
	Metrics     json.RawMessage `json:"metrics"`
	Defects     json.RawMessage `json:"defects"`
	EvidenceSHA string          `json:"evidence_sha"`
	// 仅详细报告包含原文（含脱敏 curl 示例）；脱敏报告剔除。
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

// docMap 返回套件「档位/用例 → 说明」索引，用于报告注入用例文档。
func (m *Module) docMap(suiteID string) map[string]suite.CaseDoc {
	docs := map[string]suite.CaseDoc{}
	if m.runner == nil {
		return docs
	}
	s, ok := m.runner.suites[suiteID]
	if !ok {
		return docs
	}
	for _, tier := range s.Tiers {
		for _, c := range tier.Cases {
			docs[tier.ID+"/"+c.ID] = c.Doc
		}
	}
	return docs
}

// levelNote 是用例级别的人类可读含义（哪些必须达到、哪些可豁免）。
func levelNote(level string) string {
	switch level {
	case string(suite.LevelP0):
		return "P0 否决项（任一失败当档不通过）"
	case string(suite.LevelRequired):
		return "必过项（必须达到）"
	case string(suite.LevelRecord):
		return "记录项（产出数据，不影响通过）"
	case string(suite.LevelOptional):
		return "可选项（声明支持才必须达到，未声明可豁免）"
	case string(suite.LevelGate):
		return "前置闸门（不通过则整次测试无效）"
	}
	return level
}

// curlExample 把一次证据请求还原为可复现的 curl 命令；密钥一律打码，脱敏报告同时隐藏主机。
func curlExample(baseURL string, masked bool, item suite.Evidence) string {
	base := baseURL
	if masked {
		base = maskURL(baseURL)
	}
	headers := "-H 'Authorization: Bearer ***' -H 'Content-Type: application/json'"
	if strings.Contains(item.Request, `"stream":true`) {
		headers += " -H 'Accept: text/event-stream'"
	}
	return fmt.Sprintf("curl -X POST '%s/chat/completions' %s -d '%s'", base, headers, item.Request)
}

// enrichEvidence 给每条证据补 curl 示例；脱敏时剔除原文保留摘要与哈希。
func enrichEvidence(raw string, baseURL string, masked bool) json.RawMessage {
	var items []suite.Evidence
	if json.Unmarshal([]byte(raw), &items) != nil || len(items) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"label":   item.Label,
			"request": item.Request,
			"curl":    curlExample(baseURL, masked, item),
			"result":  item.Result,
			"retries": item.Retries,
		})
	}
	data, _ := json.Marshal(out)
	return data
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
	rep.Cases, rep.Defects = reportRows(rows, run.BaseURL, masked, m.docMap(run.Suite))
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
	rep.Cases, rep.Defects = reportRows(merged, newest.BaseURL, masked, m.docMap(newest.Suite))
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
func reportRows(rows []CaseResult, baseURL string, masked bool, docs map[string]suite.CaseDoc) ([]caseRow, []defectRow) {
	var cases []caseRow
	var defects []defectRow
	for _, row := range rows {
		cr := caseRow{
			CaseID: row.CaseID, Tier: row.Tier, Name: row.Name, Level: row.Level, LevelNote: levelNote(row.Level),
			Status: row.Status, Summary: row.Summary, HTTPStatus: row.HTTPStatus,
			RequestID: row.RequestID, DurationMs: row.DurationMs, Retries: row.Retries,
			Doc:     docs[row.Tier+"/"+row.CaseID],
			Metrics: json.RawMessage(row.Metrics), Defects: json.RawMessage(row.Defects),
			EvidenceSHA: row.EvidenceSHA,
		}
		// 证据原文（含 curl 示例）只进详细报告；脱敏版完全剔除请求/响应体。
		if !masked && row.Evidence != "" {
			if enriched := enrichEvidence(row.Evidence, baseURL, false); enriched != nil {
				cr.Evidence = enriched
			} else {
				cr.Evidence = json.RawMessage(row.Evidence)
			}
		}
		if masked {
			cr.RequestID = ""
			cr.Summary = redact(cr.Summary, baseURL)
			// 缺陷原文也含目标地址，随用例行一并脱敏。
			var rowDefects []suite.Defect
			if json.Unmarshal([]byte(row.Defects), &rowDefects) == nil {
				for i := range rowDefects {
					rowDefects[i].Expect = redact(rowDefects[i].Expect, baseURL)
					rowDefects[i].Actual = redact(rowDefects[i].Actual, baseURL)
				}
				if data, err := json.Marshal(rowDefects); err == nil {
					cr.Defects = data
				}
			}
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
	b.WriteString("## 用例一览\n\n| 用例 | 名称 | 级别 | 结论 | HTTP | 耗时(ms) |\n|---|---|---|---|---|---|\n")
	for _, c := range rep.Cases {
		fmt.Fprintf(&b, "| %s-%s | %s | %s | %s | %d | %d |\n",
			c.Tier, c.CaseID, c.Name, c.LevelNote, statusText(c.Status), c.HTTPStatus, c.DurationMs)
	}
	b.WriteString("\n## 用例详情\n\n")
	for _, c := range rep.Cases {
		renderCaseDetail(&b, c, rep.Masked)
	}
	fmt.Fprintf(&b, "\n---\n\n*generated by %s · %s*\n", Watermark, Watermark)
	return b.String()
}

func statusText(status string) string {
	switch status {
	case "passed":
		return "✅ 通过"
	case "failed":
		return "❌ 失败"
	case "exempt":
		return "⚠️ 豁免"
	case "skipped":
		return "⏭ 跳过"
	case "running":
		return "🔄 运行中"
	default:
		return "⏳ 待执行"
	}
}

// renderCaseDetail 渲染单用例详情：说明、判据、级别含义、实测摘要、请求（脱敏 curl）与响应原文。
func renderCaseDetail(b *strings.Builder, c caseRow, masked bool) {
	fmt.Fprintf(b, "### %s-%s %s\n\n", c.Tier, c.CaseID, c.Name)
	fmt.Fprintf(b, "- **级别**：%s\n- **结论**：%s", c.LevelNote, statusText(c.Status))
	if c.HTTPStatus != 0 {
		fmt.Fprintf(b, "（HTTP %d）", c.HTTPStatus)
	}
	if c.DurationMs > 0 {
		fmt.Fprintf(b, "，耗时 %dms", c.DurationMs)
	}
	b.WriteString("\n")
	if c.Doc.Purpose != "" {
		fmt.Fprintf(b, "- **为什么测**：%s\n", c.Doc.Purpose)
	}
	if c.Doc.Method != "" {
		fmt.Fprintf(b, "- **怎么测**：%s\n", c.Doc.Method)
	}
	if c.Doc.Expect != "" {
		ref := c.Doc.Ref
		if ref != "" {
			ref = "（" + ref + "）"
		}
		fmt.Fprintf(b, "- **预期**：%s%s\n", c.Doc.Expect, ref)
	}
	if c.Summary != "" {
		fmt.Fprintf(b, "- **实测**：%s\n", c.Summary)
	}
	var defects []suite.Defect
	if json.Unmarshal(c.Defects, &defects) == nil && len(defects) > 0 {
		b.WriteString("- **缺陷**：\n")
		for _, d := range defects {
			fmt.Fprintf(b, "  - `%s` 期望：%s；实测：%s；归因：%s\n", d.Level, d.Expect, d.Actual, d.Attribution)
		}
	}
	if !masked {
		var evidence []map[string]any
		if json.Unmarshal(c.Evidence, &evidence) == nil && len(evidence) > 0 {
			for _, item := range evidence {
				fmt.Fprintf(b, "\n**请求**（%s，curl 已脱敏）：\n\n```bash\n%s\n```\n", item["label"], item["curl"])
				if result, ok := item["result"].(map[string]any); ok {
					raw, _ := result["raw"].(string)
					if raw != "" {
						fmt.Fprintf(b, "\n**响应**（HTTP %v，原文%s）：\n\n```\n%s\n```\n", result["http_status"], truncateNote(raw, 2000), truncateRunes(raw, 2000))
					}
				}
			}
		}
	}
	b.WriteString("\n")
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func truncateNote(s string, max int) string {
	if len([]rune(s)) <= max {
		return ""
	}
	return fmt.Sprintf("，已截断至 %d 字", max)
}
