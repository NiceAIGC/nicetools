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
	Brief        bool            `json:"brief,omitempty"` // 精简版：无请求/响应原文
	Standard     string          `json:"standard"`
	Environment  environment     `json:"environment"`
	Verdict      string          `json:"verdict"`
	StopReason   string          `json:"stop_reason,omitempty"`
	Summary      summary         `json:"summary"`
	Declarations map[string]bool `json:"declarations"`
	// Merged 非空时是合并报告：列出参与合并的任务。
	Merged  []mergedRun `json:"merged,omitempty"`
	Defects []defectRow `json:"defects"`
	Cases   []caseRow   `json:"cases"`
}

// summary 是报告摘要：总体与分档位的用例计数、未通过级别分布（参考基准报告的总览页）。
type summary struct {
	Total         int            `json:"total"`
	Passed        int            `json:"passed"`
	Failed        int            `json:"failed"`
	Exempt        int            `json:"exempt"`
	Skipped       int            `json:"skipped"`
	FailedByLevel map[string]int `json:"failed_by_level"`
	Tiers         []tierSummary  `json:"tiers"`
}

type tierSummary struct {
	Tier    string `json:"tier"`
	Total   int    `json:"total"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Exempt  int    `json:"exempt"`
	Skipped int    `json:"skipped"`
}

// buildSummary 从用例行统计摘要。
func buildSummary(rows []CaseResult) summary {
	s := summary{FailedByLevel: map[string]int{}}
	byTier := map[string]*tierSummary{}
	order := []string{}
	for _, row := range rows {
		s.Total++
		ts, ok := byTier[row.Tier]
		if !ok {
			ts = &tierSummary{Tier: row.Tier}
			byTier[row.Tier] = ts
			order = append(order, row.Tier)
		}
		ts.Total++
		switch row.Status {
		case suite.StatusPassed:
			s.Passed++
			ts.Passed++
		case suite.StatusFailed:
			s.Failed++
			ts.Failed++
			s.FailedByLevel[normalizeLevel(row.Level)]++
		case suite.StatusExempt:
			s.Exempt++
			ts.Exempt++
		case suite.StatusSkipped:
			s.Skipped++
			ts.Skipped++
		}
	}
	for _, id := range order {
		s.Tiers = append(s.Tiers, *byTier[id])
	}
	return s
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
// 兼容历史库中的 required/record/optional 写法。
func levelNote(level string) string {
	switch level {
	case "P0":
		return "P0 否决项（任一失败当档不通过）"
	case "P1", "required":
		return "P1 必过项（必须达到）"
	case "P2", "record":
		return "P2 记录项（产出数据，不影响通过）"
	case "P3", "optional":
		return "P3 可选项（声明支持才必须达到，未声明可豁免）"
	case "gate":
		return "前置闸门（不通过则整次测试无效）"
	}
	return level
}

// displayTrim 把缺陷的期望/实测文本截断到适合表格展示的长度：
// 多行取首行，超长加省略号（完整原文仍在用例证据里）。
func displayTrim(s string, max int) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// normalizeLevel 把历史级别名归一到 P0–P3，供计数统计。
func normalizeLevel(level string) string {
	switch level {
	case "required":
		return "P1"
	case "record":
		return "P2"
	case "optional":
		return "P3"
	}
	return level
}

// verdictText 按级别统计未通过项；全部通过时为「0 项未通过」。
func verdictText(counts map[string]int) string {
	total := 0
	for _, n := range counts {
		total += n
	}
	if total == 0 {
		return "0 项未通过"
	}
	parts := []string{}
	for _, level := range []string{"P0", "P1", "P2", "P3"} {
		if counts[level] > 0 {
			parts = append(parts, fmt.Sprintf("%s×%d", level, counts[level]))
		}
	}
	return fmt.Sprintf("%d 项未通过（%s）", total, strings.Join(parts, "、"))
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
func (m *Module) buildReport(run Run, rows []CaseResult, masked bool, channel string, brief ...bool) report {
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
		// 脱敏版连渠道名也隐去（渠道名可能暴露客户/供应商）。
		rep.Environment.Channel = "已隐去"
	}
	if run.StartedAt != nil {
		rep.Environment.StartedAt = run.StartedAt.Format(time.RFC3339)
	}
	if run.FinishedAt != nil {
		rep.Environment.EndedAt = run.FinishedAt.Format(time.RFC3339)
	}
	_ = json.Unmarshal([]byte(run.Declarations), &rep.Declarations)
	isBrief := len(brief) > 0 && brief[0]
	rep.Cases, rep.Defects = reportRows(rows, run.BaseURL, masked, m.docMap(run.Suite), isBrief)
	rep.Brief = isBrief
	rep.Summary = buildSummary(rows)
	return rep
}

// buildMergedReport 合并多次任务为一份报告：同一用例取最新一次结果，
// 环境与渠道名取最新任务（channel 非空时覆盖）；裁决按合并视图重算。
func (m *Module) buildMergedReport(runs []Run, rowsByID map[string][]CaseResult, masked bool, channel string, brief ...bool) report {
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
	isBrief := len(brief) > 0 && brief[0]
	rep.Cases, rep.Defects = reportRows(merged, newest.BaseURL, masked, m.docMap(newest.Suite), isBrief)
	rep.Brief = isBrief
	rep.Summary = buildSummary(merged)
	rep.Verdict = mergedVerdict(rep.Cases)
	return rep
}

// tierRank 档位排序；兼容历史库中的 F/B/M/S 旧档位。
func tierRank(tier string) int {
	switch tier {
	case "准入", "F":
		return 0
	case "B":
		return 1
	case "M":
		return 2
	default: // 压测 / S / 未知
		return 3
	}
}

// mergedVerdict 重算合并视图裁决：按级别统计未通过项。
func mergedVerdict(cases []caseRow) string {
	counts := map[string]int{}
	for _, c := range cases {
		if c.Status == suite.StatusFailed {
			counts[normalizeLevel(c.Level)]++
		}
	}
	return verdictText(counts)
}

// reportRows 把用例行转为导出结构；masked 时脱敏。
func reportRows(rows []CaseResult, baseURL string, masked bool, docs map[string]suite.CaseDoc, brief ...bool) ([]caseRow, []defectRow) {
	isBrief := len(brief) > 0 && brief[0]
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
		// 证据原文（含 curl 示例）只进「详细且未脱敏」的报告；
		// 脱敏版与精简版都不带请求/响应体（精简版保留说明/判据/缺陷解释）。
		if !masked && !isBrief && row.Evidence != "" {
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
				dr := defectRow{CaseID: row.CaseID, Level: d.Level, Expect: displayTrim(d.Expect, 80), Actual: displayTrim(d.Actual, 160), Attribution: d.Attribution}
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
	if rep.Brief {
		b.WriteString("> 📄 精简版：不含请求/响应原文，保留用例说明、判据与缺陷解释\n\n")
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
	renderSummaryMD(&b, rep)
	renderChartsMD(&b, rep.Cases)
	if len(rep.Declarations) > 0 {
		b.WriteString("## 能力声明\n\n| 能力 | 声明支持 |\n|---|---|\n")
		for k, v := range rep.Declarations {
			fmt.Fprintf(&b, "| %s | %v |\n", k, v)
		}
		b.WriteString("\n")
	}
	if len(rep.Defects) > 0 {
		b.WriteString("## 缺陷清单\n\n| 用例 | 级别 | 期望 | 实测 |\n|---|---|---|---|\n")
		for _, d := range rep.Defects {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", d.CaseID, d.Level, d.Expect, d.Actual)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## 用例一览\n\n")
	currentTier := ""
	for _, c := range rep.Cases {
		if c.Tier != currentTier {
			currentTier = c.Tier
			fmt.Fprintf(&b, "\n### %s 档\n\n| 用例 | 名称 | 级别 | 结论 | HTTP | 耗时(ms) |\n|---|---|---|---|---|---|\n", currentTier)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %d |\n",
			c.CaseID, c.Name, c.LevelNote, statusText(c.Status), c.HTTPStatus, c.DurationMs)
	}
	b.WriteString("\n## 用例详情\n\n")
	for _, c := range rep.Cases {
		renderCaseDetail(&b, c, rep.Masked)
	}
	fmt.Fprintf(&b, "\n---\n\n*generated by %s · %s*\n", Watermark, Watermark)
	return b.String()
}

// renderSummaryMD 渲染摘要区：总计、未通过级别分布、分档位计数表。
func renderSummaryMD(b *strings.Builder, rep report) {
	b.WriteString("## 摘要\n\n")
	s := rep.Summary
	fmt.Fprintf(b, "共 **%d** 项：✅ 通过 %d · ❌ 失败 %d · ⚠️ 豁免 %d · ⏭ 跳过 %d\n\n",
		s.Total, s.Passed, s.Failed, s.Exempt, s.Skipped)
	if len(s.FailedByLevel) > 0 {
		b.WriteString("未通过级别分布：")
		for _, level := range []string{"P0", "P1", "P2", "P3"} {
			if s.FailedByLevel[level] > 0 {
				fmt.Fprintf(b, " **%s×%d**", level, s.FailedByLevel[level])
			}
		}
		b.WriteString("\n\n")
	}
	b.WriteString("| 档位 | 用例数 | 通过 | 失败 | 豁免 | 跳过 |\n|---|---|---|---|---|---|\n")
	for _, t := range s.Tiers {
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d |\n", t.Tier, t.Total, t.Passed, t.Failed, t.Exempt, t.Skipped)
	}
	b.WriteString("\n")
}

// renderChartsMD 渲染文本条形图（与 PDF 图表同源：TTFT 对比与缓存命中率）。
func renderChartsMD(b *strings.Builder, cases []caseRow) {
	type bar struct {
		label    string
		p50, p90 float64
	}
	var ttfts []bar
	var caches []struct {
		label string
		rate  float64
	}
	for _, c := range cases {
		var m map[string]any
		if json.Unmarshal(c.Metrics, &m) != nil {
			continue
		}
		p50, ok1 := m["ttft_p50_ms"].(float64)
		p90, ok2 := m["ttft_p90_ms"].(float64)
		if ok1 && ok2 && p50 > 0 {
			ttfts = append(ttfts, bar{chartLabel(c), p50, p90})
		}
		if rate, ok := m["cache_hit_rate_percent"].(float64); ok {
			caches = append(caches, struct {
				label string
				rate  float64
			}{chartLabel(c), rate})
		}
	}
	textBar := func(frac float64, width int) string {
		filled := int(frac*float64(width) + 0.5)
		if filled > width {
			filled = width
		}
		return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	}
	if len(ttfts) >= 2 {
		b.WriteString("\n## TTFT 对比（P50 / P90，ms）\n\n```\n")
		max := 1.0
		for _, t := range ttfts {
			if t.p90 > max {
				max = t.p90
			}
		}
		for _, t := range ttfts {
			fmt.Fprintf(b, "%-14s %s %s %.0f/%.0f\n", t.label, textBar(t.p50/max, 24), textBar(t.p90/max, 24), t.p50, t.p90)
		}
		b.WriteString("```\n")
	}
	if len(caches) >= 2 {
		b.WriteString("\n## 缓存命中率（token 口径，%）\n\n```\n")
		for _, c := range caches {
			fmt.Fprintf(b, "%-14s %s %.1f%%\n", c.label, textBar(c.rate/100, 40), c.rate)
		}
		b.WriteString("```\n")
	}
}

// chartLabel 图表行标签：用例 ID 已含档位前缀时不重复拼接。
func chartLabel(c caseRow) string {
	if strings.HasPrefix(c.CaseID, c.Tier) {
		return c.CaseID
	}
	return c.Tier + "-" + c.CaseID
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
			fmt.Fprintf(b, "  - `%s` 期望：%s；实测：%s\n", d.Level, displayTrim(d.Expect, 80), displayTrim(d.Actual, 160))
		}
	}
	// 指标表：压测/缓存等用例的量化结果（对应基准报告的判定明细区）。
	var metrics map[string]any
	if json.Unmarshal(c.Metrics, &metrics) == nil && len(metrics) > 0 {
		b.WriteString("\n**指标明细**：\n\n| 指标 | 值 |\n|---|---|\n")
		for _, key := range sortedKeys(metrics) {
			fmt.Fprintf(b, "| %s | %s |\n", key, metricText(metrics[key]))
		}
		b.WriteString("\n")
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

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// metricText 把指标值渲染为紧凑文本（嵌套结构转 JSON）。
func metricText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%.2f", t)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
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
