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
const Watermark = "niceaigc.cn"

// report 是导出文档结构（JSON 即此结构的序列化）。
type report struct {
	Watermark    string          `json:"watermark"`
	Masked       bool            `json:"masked"`
	Brief        bool            `json:"brief,omitempty"` // 精简版：无请求/响应原文
	Standard     string          `json:"standard"`
	Environment  environment     `json:"environment"`
	Verdict      string          `json:"verdict"`
	Grade        string          `json:"grade,omitempty"`
	PassRate     float64         `json:"pass_rate,omitempty"`
	StopReason   string          `json:"stop_reason,omitempty"`
	Summary      summary         `json:"summary"`
	Declarations map[string]bool `json:"declarations"`
	// Merged 非空时是合并报告：列出参与合并的任务。
	Merged  []mergedRun `json:"merged,omitempty"`
	Defects []defectRow `json:"defects"`
	Cases   []caseRow   `json:"cases"`
}

// summary 是报告摘要：总体与分组用例计数、未通过级别分布。
type summary struct {
	Total         int            `json:"total"`
	Passed        int            `json:"passed"`
	Failed        int            `json:"failed"`
	Exempt        int            `json:"exempt"`
	Skipped       int            `json:"skipped"`
	FailedByLevel map[string]int `json:"failed_by_level"`
	Groups        []groupSummary `json:"groups"`
}

type groupSummary struct {
	Group   string `json:"group"`
	Label   string `json:"label"`
	Total   int    `json:"total"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Exempt  int    `json:"exempt"`
	Skipped int    `json:"skipped"`
}

// groupOrder 是分组展示顺序；未登记分组按字典序追加在后。
var groupOrder = []struct{ key, label string }{
	{suite.GroupGate, "前置闸门"},
	{suite.GroupFormat, "格式与协议"},
	{suite.GroupCache, "缓存"},
	{suite.GroupLatency, "性能基准"},
}

func groupRank(key string) int {
	for i, g := range groupOrder {
		if g.key == key {
			return i
		}
	}
	return len(groupOrder)
}

func groupLabel(key string) string {
	for _, g := range groupOrder {
		if g.key == key {
			return g.label
		}
	}
	if strings.HasPrefix(key, "legacy-") {
		return "历史档位 " + strings.TrimPrefix(key, "legacy-")
	}
	if key == "" {
		return "未分组"
	}
	return key
}

// gradeForRate 是报告评级：A+ ≥95%，A 90–95%，B 80–90%，C 70–80%，其余 D。
func gradeForRate(rate float64) string {
	switch {
	case rate >= 95:
		return "A+"
	case rate >= 90:
		return "A"
	case rate >= 80:
		return "B"
	case rate >= 70:
		return "C"
	default:
		return "D"
	}
}

// scoreRun 把用例结果折算成通过率与评级；跳过/豁免不计分母。
func scoreRun(run *Run, rows []CaseResult) {
	passed, assessed := 0, 0
	for _, row := range rows {
		switch row.Status {
		case "passed":
			passed++
			assessed++
		case "failed":
			assessed++
		}
	}
	run.Passed = passed
	run.Assessed = assessed
	run.Failed = assessed - passed
	if assessed == 0 {
		run.PassRate = 0
		run.Grade = "—"
		return
	}
	run.PassRate = float64(passed) / float64(assessed) * 100
	run.Grade = gradeForRate(run.PassRate)
}

// buildSummary 从用例行统计摘要。
func buildSummary(rows []CaseResult) summary {
	s := summary{FailedByLevel: map[string]int{}}
	byGroup := map[string]*groupSummary{}
	for _, row := range rows {
		s.Total++
		group := row.Group
		if group == "" && row.LegacyTier != "" {
			group = "legacy-" + row.LegacyTier
		}
		if group == "" {
			group = "legacy-unknown"
		}
		gs, ok := byGroup[group]
		if !ok {
			gs = &groupSummary{Group: group, Label: groupLabel(group)}
			byGroup[group] = gs
		}
		gs.Total++
		switch row.Status {
		case suite.StatusPassed:
			s.Passed++
			gs.Passed++
		case suite.StatusFailed:
			s.Failed++
			gs.Failed++
			s.FailedByLevel[normalizeLevel(row.Level)]++
		case suite.StatusExempt:
			s.Exempt++
			gs.Exempt++
		case suite.StatusSkipped:
			s.Skipped++
			gs.Skipped++
		}
	}
	for _, g := range groupOrder {
		if gs, ok := byGroup[g.key]; ok {
			s.Groups = append(s.Groups, *gs)
			delete(byGroup, g.key)
		}
	}
	rest := make([]string, 0, len(byGroup))
	for key := range byGroup {
		rest = append(rest, key)
	}
	sort.Strings(rest)
	for _, key := range rest {
		s.Groups = append(s.Groups, *byGroup[key])
	}
	return s
}

type mergedRun struct {
	ID         string `json:"id"`
	Preset     string `json:"preset"`
	PresetName string `json:"preset_name,omitempty"`
	Verdict    string `json:"verdict"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type environment struct {
	TaskID     string `json:"task_id,omitempty"`
	Channel    string `json:"channel,omitempty"`
	TaskNote   string `json:"task_note,omitempty"`
	BaseURL    string `json:"base_url"`
	Model      string `json:"model"`
	Suite      string `json:"suite"`
	Preset     string `json:"preset"`
	PresetName string `json:"preset_name"`
	CreatedBy  string `json:"created_by"`
	StartedAt  string `json:"started_at,omitempty"`
	EndedAt    string `json:"finished_at,omitempty"`
	Timezone   string `json:"timezone"`
}

type caseRow struct {
	CaseID      string          `json:"case_id"`
	Group       string          `json:"group"`
	GroupLabel  string          `json:"group_label"`
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

// docMap 返回套件「用例 → 说明」索引，用于报告注入用例文档。
func (m *Module) docMap(suiteID string) map[string]suite.CaseDoc {
	docs := map[string]suite.CaseDoc{}
	if m.runner == nil {
		return docs
	}
	s, ok := m.runner.suites[suiteID]
	if !ok {
		return docs
	}
	for _, c := range s.Cases {
		docs[c.ID] = c.Doc
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
		return "P3 可选项（声明支持才必须达到，未声明可跳过）"
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
	presetName := m.presetName(run.Suite, run.Preset)
	if run.Preset == "" && run.LegacyTier != "" {
		presetName = "旧档位 " + run.LegacyTier
	}
	rep := report{
		Watermark:  Watermark,
		Masked:     masked,
		Standard:   run.SuiteName,
		Verdict:    run.Verdict,
		StopReason: run.StopReason,
		Environment: environment{
			TaskID:     run.ID,
			Channel:    channel,
			TaskNote:   run.TaskNote,
			BaseURL:    run.BaseURL,
			Model:      run.Model,
			Suite:      run.Suite,
			Preset:     run.Preset,
			PresetName: presetName,
			CreatedBy:  run.CreatedBy,
			Timezone:   "Asia/Shanghai (UTC+08:00)",
		},
	}
	if rep.Environment.Channel == "" {
		rep.Environment.Channel = run.Channel
	}
	if masked {
		rep.Environment.BaseURL = maskURL(run.BaseURL)
		// 脱敏版连渠道名/任务备注也隐去（可能暴露客户/供应商）。
		rep.Environment.Channel = "已隐去"
		if rep.Environment.TaskNote != "" {
			rep.Environment.TaskNote = "已隐去"
		}
	}
	if run.StartedAt != nil {
		rep.Environment.StartedAt = run.StartedAt.In(reportLocation).Format(time.RFC3339)
	}
	if run.FinishedAt != nil {
		rep.Environment.EndedAt = run.FinishedAt.In(reportLocation).Format(time.RFC3339)
	}
	_ = json.Unmarshal([]byte(run.Declarations), &rep.Declarations)
	isBrief := len(brief) > 0 && brief[0]
	rep.Cases, rep.Defects = reportRows(rows, run.BaseURL, masked, m.docMap(run.Suite), isBrief)
	rep.Brief = isBrief
	rep.Summary = buildSummary(rows)
	_, assessed, rate := compliance(rep.Summary)
	if assessed > 0 {
		rep.PassRate = rate
		rep.Grade = gradeForRate(rate)
	} else {
		rep.Grade = "—"
	}
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
		item := mergedRun{ID: run.ID, Preset: run.Preset, PresetName: m.presetName(run.Suite, run.Preset), Verdict: run.Verdict}
		if run.FinishedAt != nil {
			item.FinishedAt = run.FinishedAt.In(reportLocation).Format(time.RFC3339)
		}
		rep.Merged = append(rep.Merged, item)
	}
	byKey := map[string]CaseResult{}
	for _, run := range runs {
		for _, row := range rowsByID[run.ID] {
			byKey[row.CaseID] = row
		}
	}
	merged := make([]CaseResult, 0, len(byKey))
	for _, row := range byKey {
		merged = append(merged, row)
	}
	sort.Slice(merged, func(i, j int) bool {
		if a, b := groupRank(merged[i].Group), groupRank(merged[j].Group); a != b {
			return a < b
		}
		if merged[i].Seq != merged[j].Seq {
			return merged[i].Seq < merged[j].Seq
		}
		return merged[i].CaseID < merged[j].CaseID
	})
	isBrief := len(brief) > 0 && brief[0]
	rep.Cases, rep.Defects = reportRows(merged, newest.BaseURL, masked, m.docMap(newest.Suite), isBrief)
	rep.Brief = isBrief
	rep.Summary = buildSummary(merged)
	_, assessed, rate := compliance(rep.Summary)
	if assessed > 0 {
		rep.PassRate = rate
		rep.Grade = gradeForRate(rate)
	} else {
		rep.Grade = "—"
	}
	rep.Verdict = mergedVerdict(rep.Cases)
	return rep
}

// presetName 返回预设可读名；历史任务无新预设字段时按旧档位或全测展示。
func (m *Module) presetName(suiteID, preset string) string {
	if preset == "" {
		return "全部测试"
	}
	if m.runner != nil {
		if s, ok := m.runner.suites[suiteID]; ok {
			for _, p := range s.Presets {
				if string(p.Key) == preset {
					return p.Label
				}
			}
		}
	}
	if preset == "format" {
		return "只测格式与协议"
	}
	if preset == "cache" {
		return "只测缓存"
	}
	if preset == "latency" {
		return "只测延迟与吞吐"
	}
	return preset
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
	// 详情按分组顺序输出（闸门 → 格式 → 缓存 → 性能），组内保持执行序号；
	// 这样短分组不会孤零零落到最后一页。
	rows = append([]CaseResult(nil), rows...)
	sort.SliceStable(rows, func(i, j int) bool {
		if a, b := groupRank(rows[i].Group), groupRank(rows[j].Group); a != b {
			return a < b
		}
		return rows[i].Seq < rows[j].Seq
	})
	var cases []caseRow
	var defects []defectRow
	for _, row := range rows {
		group := row.Group
		if group == "" && row.LegacyTier != "" {
			group = "legacy-" + row.LegacyTier
		}
		if group == "" {
			group = "legacy-unknown"
		}
		cr := caseRow{
			CaseID: row.CaseID, Group: group, GroupLabel: groupLabel(group),
			Name: row.Name, Level: row.Level, LevelNote: levelNote(row.Level),
			Status: row.Status, Summary: row.Summary, HTTPStatus: row.HTTPStatus,
			RequestID: row.RequestID, DurationMs: row.DurationMs, Retries: row.Retries,
			Doc:     docs[row.CaseID],
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
			var metrics map[string]any
			if json.Unmarshal([]byte(row.Metrics), &metrics) == nil {
				if data, err := json.Marshal(redactAny(metrics, baseURL)); err == nil {
					cr.Metrics = data
				}
			}
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

// redactAny 递归脱敏 metrics 中的字符串；端点/URL 常藏在右上角指标里。
func redactAny(v any, baseURL string) any {
	switch t := v.(type) {
	case string:
		return redact(t, baseURL)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = redactAny(item, baseURL)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = redactAny(item, baseURL)
		}
		return out
	default:
		return v
	}
}

// renderMarkdown 把报告渲染为可读格式（§7），每页首行与结尾嵌入水印。
func renderMarkdown(rep report) string {
	var b strings.Builder
	if rep.Watermark != "" {
		fmt.Fprintf(&b, "<!-- %s -->\n", rep.Watermark)
	}
	fmt.Fprintf(&b, "# %s 测试报告\n\n> 本报告由 **%s** 准入测试系统生成\n\n", rep.Standard, Watermark)
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
	passed, assessed, rate := compliance(rep.Summary)
	rateText := "—"
	if assessed > 0 {
		rateText = fmt.Sprintf("%.1f%%（通过 %d / 考核 %d 项）", rate, passed, assessed)
	}
	fmt.Fprintf(&b, "| 项 | 值 |\n|---|---|\n| 任务 ID | `%s` |\n| 渠道 | %s |\n| 网关 | `%s` |\n| 模型 | `%s` |\n| 范围 | %s |\n| 通过率 | **%s** |\n| 评级 | **%s** |\n| 时段 | %s ~ %s (%s) |\n",
		rep.Environment.TaskID, channel, rep.Environment.BaseURL, rep.Environment.Model, rep.Environment.PresetName, rateText, rep.Grade, rep.Environment.StartedAt, rep.Environment.EndedAt, rep.Environment.Timezone)
	if rep.Environment.TaskNote != "" {
		fmt.Fprintf(&b, "| 任务备注 | %s |\n", rep.Environment.TaskNote)
	}
	b.WriteString("\n")
	if len(rep.Merged) > 0 {
		fmt.Fprintf(&b, "本报告合并自 %d 次测试：\n\n| 任务 | 范围 | 完成时间 | 结论 |\n|---|---|---|---|\n", len(rep.Merged))
		for _, item := range rep.Merged {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", item.ID, item.PresetName, item.FinishedAt, item.Verdict)
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
	currentGroup := ""
	for _, c := range rep.Cases {
		if c.Group != currentGroup {
			currentGroup = c.Group
			fmt.Fprintf(&b, "\n### %s\n\n| 用例 | 名称 | 级别 | 结论 | HTTP | 耗时(ms) |\n|---|---|---|---|---|---|\n", c.GroupLabel)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %d |\n",
			c.CaseID, c.Name, c.LevelNote, statusText(c.Status), c.HTTPStatus, c.DurationMs)
	}
	b.WriteString("\n## 用例详情\n\n")
	for _, c := range rep.Cases {
		renderCaseDetail(&b, c, rep.Masked)
	}
	if rep.Watermark != "" {
		fmt.Fprintf(&b, "\n---\n\n*generated by %s · %s*\n", rep.Watermark, rep.Watermark)
	}
	return b.String()
}

// renderSummaryMD 渲染摘要区：总计、未通过级别分布、分分组计数表。
func renderSummaryMD(b *strings.Builder, rep report) {
	b.WriteString("## 摘要\n\n")
	s := rep.Summary
	fmt.Fprintf(b, "共 **%d** 项：✅ 通过 %d · ❌ 失败 %d · ⏭ 能力跳过 %d · ⏭ 手动跳过 %d\n\n",
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
	b.WriteString("| 分组 | 用例数 | 通过 | 失败 | 能力跳过 | 手动跳过 |\n|---|---|---|---|---|---|\n")
	for _, g := range s.Groups {
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d |\n", g.Label, g.Total, g.Passed, g.Failed, g.Exempt, g.Skipped)
	}
	b.WriteString("\n")
}

// renderChartsMD 输出和 PDF 同源的延迟分位、吞吐/token 表、缓存曲线和 TTFT 散点。
func renderChartsMD(b *strings.Builder, cases []caseRow) {
	var rows []benchmarkRowMD
	var caches []cachePerformanceRow
	for _, c := range cases {
		var m map[string]any
		if json.Unmarshal(c.Metrics, &m) != nil || len(m) == 0 {
			continue
		}
		if hasPerformanceMetrics(m) {
			rows = append(rows, benchmarkRowMD{label: c.CaseID, m: m})
		}
		for _, key := range sortedKeys(m) {
			if strings.HasPrefix(key, "target_") || (key != "cache_hit_rate_percent" && !strings.HasSuffix(key, "_hit_rate_percent")) {
				continue
			}
			rate, ok := numberMetric(m, key)
			if !ok {
				continue
			}
			prefix := strings.TrimSuffix(key, "_hit_rate_percent")
			cachedKey, promptKey, label := "cached_tokens", "prompt_tokens", c.CaseID
			if prefix != "cache" {
				cachedKey, promptKey = prefix+"_cached_tokens", prefix+"_prompt_tokens"
				label += " · " + prefix
			}
			caches = append(caches, cachePerformanceRow{label: label, rate: rate, cachedTokens: numberText(m, cachedKey, 0), promptTokens: numberText(m, promptKey, 0)})
		}
		if curve, ok := m["decay_curve"].([]any); ok {
			for _, item := range curve {
				p, ok := item.(map[string]any)
				if !ok {
					continue
				}
				sec, hasSec := numberMetric(p, "seconds")
				rate, hasRate := numberMetric(p, "hit_rate_percent")
				if hasSec && hasRate {
					caches = append(caches, cachePerformanceRow{label: fmt.Sprintf("%s · TTL %.0fs", c.CaseID, sec), rate: rate, cachedTokens: numberText(p, "cached_tokens", 0), promptTokens: numberText(p, "prompt_tokens", 0)})
				}
			}
		}
	}
	if len(rows) > 0 {
		b.WriteString("## 性能对比\n\n")
		fmt.Fprintf(b, "**%s**\n\n", benchmarkVerdictAllMD(rows))
		b.WriteString("颜色说明：🟢 达到门槛 · 🔴 未达到门槛 · ⚪ 无门槛/数据不足；每列数值格式为「实测 / 门槛」。\n\n")
		b.WriteString("| 场景 | 输入 | TTFT P50 | TTFT P95 | TPOT P50 | E2E P90 | 成功率 | 缓存命中 | 直观结论 |\n|---|---:|---:|---:|---:|---:|---:|---:|---|\n")
		for _, r := range rows {
			m := r.m
			fmt.Fprintf(b, "| %s | %s tok | %s | %s | %s | %s | %s | %s | %s |\n", r.label, benchmarkInputText(m),
				scoredMetricMD(m, "ttft_p50_ms", "target_ttft_p50_ms", 0, "ms", false),
				scoredMetricMD(m, "ttft_p95_ms", "target_ttft_p95_ms", 0, "ms", false),
				scoredMetricMD(m, "tpot_p50_ms", "target_tpot_p50_ms", 1, "ms/token", false),
				scoredMetricMD(m, "e2e_p90_ms", "target_e2e_p90_ms", 0, "ms", false),
				scoredMetricMD(m, "success_rate_percent", "target_success_rate_percent", 1, "%", true),
				scoredMetricMD(m, "cache_hit_rate_percent", "target_cache_hit_rate_percent", 1, "%", true),
				benchmarkVerdictMD(m))
		}
		budget := numberText(rows[0].m, "output_budget_tokens", 0)
		if budget == "—" {
			budget = "默认"
		}
		fmt.Fprintf(b, "\n本次基准输出预算：%s tokens/请求。\n\n", budget)
		b.WriteString("| 场景 | 样本 | 输出速度 | ITL P95 | 错误率 | 输入总量 | 输出总量 | 缓存读取 |\n|---|---:|---:|---:|---:|---:|---:|---:|\n")
		for _, r := range rows {
			m := r.m
			fmt.Fprintf(b, "| %s | %s | %s token/s | %s ms | %s | %s | %s | %s |\n", r.label,
				numberText(m, "samples", 0), numberText(m, "otps_p50", 1), numberText(m, "itl_p95_ms", 1), percentMetricText(m, "error_rate_percent"),
				numberText(m, "input_tokens_total", 0), numberText(m, "output_tokens_total", 0), numberText(m, "cache_read_tokens_total", 0))
		}
		b.WriteString("\n")
	}
	if len(caches) > 0 {
		b.WriteString("## 缓存命中\n\n")
		b.WriteString("绿色 ≥80%，橙色 50–79.9%，红色 <50%。\n\n| 场景 | 命中率 | 直观状态 | cached tokens | prompt tokens |\n|---|---:|---|---:|---:|\n")
		for _, c := range caches {
			state := "🟢 稳定"
			if c.rate < 80 {
				state = "🟠 偏低"
			}
			if c.rate < 50 {
				state = "🔴 很低"
			}
			fmt.Fprintf(b, "| %s | %.1f%% | %s | %s | %s |\n", c.label, c.rate, state, c.cachedTokens, c.promptTokens)
		}
		b.WriteString("\n")
	}
}

// benchmarkRowMD 是 Markdown 图表用的最小场景视图。
type benchmarkRowMD struct {
	label string
	m     map[string]any
}

// benchmarkVerdictAllMD 与 PDF 的一眼结论同源：多少档位四项延迟全部达标。
func benchmarkVerdictAllMD(rows []benchmarkRowMD) string {
	total, allPass := 0, 0
	for _, row := range rows {
		checked, failed := false, false
		for _, item := range []struct{ key, target string }{
			{"ttft_p50_ms", "target_ttft_p50_ms"},
			{"ttft_p95_ms", "target_ttft_p95_ms"},
			{"tpot_p50_ms", "target_tpot_p50_ms"},
			{"e2e_p90_ms", "target_e2e_p90_ms"},
		} {
			value, hasValue := numberMetric(row.m, item.key)
			target, hasTarget := numberMetric(row.m, item.target)
			if !hasValue || !hasTarget {
				continue
			}
			checked = true
			if value > target {
				failed = true
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
	if total == 0 {
		return "⚪ 暂无可判定的延迟门槛"
	}
	if allPass == total {
		return fmt.Sprintf("🟢 %d 档四项延迟全部达标", total)
	}
	return fmt.Sprintf("🔴 %d 档中仅 %d 档四项延迟全部达标", total, allPass)
}

func scoredMetricMD(metrics map[string]any, key, targetKey string, decimals int, suffix string, higher bool) string {
	value, ok := numberMetric(metrics, key)
	if !ok {
		return "⚪ —"
	}
	text := fmt.Sprintf("%."+fmt.Sprint(decimals)+"f%s", value, suffix)
	if target, hasTarget := numberMetric(metrics, targetKey); targetKey != "" && hasTarget {
		passed := value >= target
		if !higher {
			passed = value <= target
		}
		if passed {
			return "🟢 " + text
		}
		return "🔴 " + text
	}
	return "⚪ " + text
}

func benchmarkVerdictMD(metrics map[string]any) string {
	checks := 0
	passed := 0
	for _, item := range []struct {
		key, target string
		higher      bool
	}{
		{"ttft_p50_ms", "target_ttft_p50_ms", false},
		{"ttft_p95_ms", "target_ttft_p95_ms", false},
		{"tpot_p50_ms", "target_tpot_p50_ms", false},
		{"e2e_p90_ms", "target_e2e_p90_ms", false},
		{"success_rate_percent", "target_success_rate_percent", true},
		{"cache_hit_rate_percent", "target_cache_hit_rate_percent", true},
	} {
		value, ok := numberMetric(metrics, item.key)
		target, hasTarget := numberMetric(metrics, item.target)
		if !ok || !hasTarget {
			continue
		}
		checks++
		pass := value >= target
		if !item.higher {
			pass = value <= target
		}
		if pass {
			passed++
		}
	}
	if checks == 0 {
		return "⚪ 暂无完整判定"
	}
	if passed == checks {
		return fmt.Sprintf("🟢 达标（%d/%d）", passed, checks)
	}
	return fmt.Sprintf("🔴 需关注（%d/%d）", passed, checks)
}

func statusText(status string) string {
	switch status {
	case "passed":
		return "✅ 通过"
	case "failed":
		return "❌ 失败"
	case "exempt":
		return "⏭ 跳过"
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
	fmt.Fprintf(b, "### %s %s\n\n", c.CaseID, c.Name)
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
