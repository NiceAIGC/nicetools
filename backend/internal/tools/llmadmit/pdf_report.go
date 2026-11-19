package llmadmit

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/signintech/gopdf"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// renderPDF 把报告渲染为 PDF：封面结论 + 诊断卡 + 分组结论 + 性能仪表盘 + 场景卡 + 能力验证。
func renderPDF(rep report) ([]byte, error) {
	w, err := renderPDFWriter(rep)
	if err != nil {
		return nil, err
	}
	if w.err != nil {
		return nil, w.err
	}
	return w.pdf.GetBytesPdf(), nil
}

func renderPDFWriter(rep report) (*pdfWriter, error) {
	w := &pdfWriter{pdf: &gopdf.GoPdf{}, widths: map[rune]float64{}, watermark: rep.Watermark != ""}
	w.pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := w.pdf.AddTTFFontData("wqy", pdfFont); err != nil {
		return nil, err
	}
	w.pdf.SetInfo(gopdf.PdfInfo{
		Title:        rep.Environment.Model + " 大模型准入测试报告",
		Author:       Watermark,
		Subject:      rep.Standard + " · " + rep.Environment.PresetName,
		Creator:      "niceaigc.cn 大模型准入测试",
		Producer:     "niceaigc.cn",
		CreationDate: time.Now(),
	})
	w.header = rep.Environment.Model + " · " + rep.Standard + " · " + rep.Environment.PresetName
	w.header = w.clipUnits(w.header, 250/7)
	if rep.Masked && rep.Brief {
		w.footerNote = "脱敏精简版 · 已隐去地址、密钥、原文与 request_id"
	} else if rep.Masked {
		w.footerNote = "脱敏版 · 已隐去地址、密钥与 request_id"
	} else if rep.Brief {
		w.footerNote = "精简版 · 不含请求/响应原文"
	}
	w.newPage()

	w.hero(rep)
	w.verdictPanel(rep)
	w.diagnostics(rep)
	if limit, ok := collectVideoLimit(rep.Cases); ok {
		w.videoLimitPanel(limit)
	}
	w.resultsTable(rep)
	perfRows, cacheRows := collectPerformance(rep.Cases)
	w.overviewTable(perfRows)
	if len(rep.Defects) > 0 {
		w.defectSection(rep)
	}
	if len(perfRows) > 0 {
		w.performanceSection(perfRows)
		w.scenarioSection(perfRows)
	}
	if len(cacheRows) > 0 {
		w.cacheSection(cacheRows)
	}
	w.capabilitySection(rep)
	w.appendix(rep)
	w.finishPage()
	return w, nil
}

// hero 是封面头：品牌、标题、环境摘要、右上的准入结论徽章。
func (w *pdfWriter) hero(rep report) {
	top := w.y
	w.drawText("niceaigc.cn", pdfMargin, top+2, 9.5, colorBrand)
	w.drawText("大模型准入基准", pdfMargin, top+14, 6.5, colorMuted)
	w.verdictBadge(rep, pdfWidth-pdfMargin-118, top+5, 118, 76)

	w.y = top + 26
	model := dashIfEmpty(rep.Environment.Model)
	title := model + " · 大模型准入测试报告"
	titleSize := 18.0
	for titleSize > 13 && w.textWidth(title, titleSize) > 365 {
		titleSize -= 0.5
	}
	w.drawText(w.clipUnits(title, 365/titleSize), pdfMargin, w.y, titleSize, colorInk)
	w.y += 27

	scope := fmt.Sprintf("任务 ID：%s ｜ 渠道：%s ｜ 范围：%s ｜ 标准：%s", dashIfEmpty(rep.Environment.TaskID), orDash(rep.Environment.Channel), dashIfEmpty(rep.Environment.PresetName), dashIfEmpty(rep.Standard))
	w.paragraphAt(pdfMargin, 365, scope, 8.2, 12, colorMuted)
	when := fmt.Sprintf("测试时间：%s ~ %s ｜ 用时 %s", stampText(rep.Environment.StartedAt), stampText(rep.Environment.EndedAt), durationText(rep.Environment.StartedAt, rep.Environment.EndedAt))
	w.paragraphAt(pdfMargin, 365, when, 8.2, 12, colorMuted)
	if top+94 > w.y {
		w.y = top + 94
	}
	w.hline(w.y)
	w.y += 10
}

// verdictBadge 画右上角的大结论徽章，附通过率进度条。
func (w *pdfWriter) verdictBadge(rep report, x, y, width, height float64) {
	passed, assessed, rate := compliance(rep.Summary)
	fg, bg := colorMuted, colorPanel
	value := "—"
	if assessed > 0 {
		fg, bg = rateColors(rate)
		value = fmt.Sprintf("%.1f%%", rate)
	}
	w.panelRect(x, y, x+width, y+height, bg, fg, 8, 1.1)
	w.drawText("通过率", x+10, y+7, 6.5, colorMuted)
	w.drawText(value, x+10, y+18, 20, fg)
	trackY := y + 44
	w.fillRect(x+10, trackY, x+width-10, trackY+4, colorBorder, 2)
	if rate > 0 {
		w.fillRect(x+10, trackY, x+10+(width-20)*rate/100, trackY+4, fg, 2)
	}
	sub := "无可执行用例"
	if assessed > 0 {
		sub = fmt.Sprintf("%s · %d / %d 项通过", rep.Grade, passed, assessed)
	}
	w.drawText(sub, x+10, y+58, 5.8, colorMuted)
}

// verdictPanel 是封面结论区：总体结论 + P0–P3 四个级别状态。
func (w *pdfWriter) verdictPanel(rep report) {
	top := w.y
	height := 70.0
	w.panelRect(pdfMargin, top, pdfWidth-pdfMargin, top+height, colorPanel, colorBorder, 8, 0.6)
	leftX := pdfMargin + 12
	leftW := 300.0
	s := rep.Summary
	_, assessed, rate := compliance(s)
	rateFg, _ := rateColors(rate)
	rateText := "—"
	if assessed > 0 {
		rateText = fmt.Sprintf("%.1f%%", rate)
	}
	w.drawText("通过率", leftX, top+8, 8, colorMuted)
	w.drawText(rateText, leftX, top+19, 12, rateFg)
	line := fmt.Sprintf("共 %d 项：通过 %d · 失败 %d · 能力跳过 %d · 手动跳过 %d", s.Total, s.Passed, s.Failed, s.Exempt, s.Skipped)
	w.drawText(w.clipUnits(line, leftW/7.2), leftX, top+38, 7.2, colorBody)
	w.drawText("通过率 = 通过 ÷（总数 − 跳过）", leftX, top+52, 6.5, colorMuted)

	stats := levelStats(rep)
	cellW, gap := 36.0, 3.0
	levels := []struct{ key, label string }{{"gate", "闸门"}, {"P0", "P0"}, {"P1", "P1"}, {"P2", "P2"}, {"P3", "P3"}}
	startX := pdfWidth - pdfMargin - 10 - 5*cellW - 4*gap
	for i, level := range levels {
		st := stats[level.key]
		cellFg, cellBg := st.colors()
		x := startX + float64(i)*(cellW+gap)
		w.panelRect(x, top+8, x+cellW, top+60, cellBg, cellFg, 5, 0.5)
		w.drawText(level.label, x+5, top+13, 6.5, colorMuted)
		w.drawText(st.label(), x+5, top+24, 8.4, cellFg)
		countText := fmt.Sprintf("%d 项", st.total)
		if st.failed > 0 {
			countText = fmt.Sprintf("%d/%d", st.failed, st.total)
		}
		w.drawText(countText, x+5, top+42, 5.8, colorMuted)
	}
	w.y = top + height + 12
}

// diagnostics 是「评测诊断」四张数字卡。
func (w *pdfWriter) diagnostics(rep report) {
	w.sectionHeader("评测诊断", "先看四个数：通过率、用例规模、未通过项、性能达标档位。")
	s := rep.Summary
	passed, assessed, rate := compliance(s)
	rateFg, rateBg := rateColors(rate)

	levelCount := map[string]int{}
	levelFailed := map[string]int{}
	for _, c := range rep.Cases {
		level := normalizeLevel(c.Level)
		levelCount[level]++
		if c.Status == "failed" {
			levelFailed[level]++
		}
	}
	perfRows, _ := collectPerformance(rep.Cases)
	perfTotal, perfPass, _ := perfVerdict(perfRows)

	rateValue, rateSub := fmt.Sprintf("%.1f%%", rate), fmt.Sprintf("通过 %d / 考核 %d 项", passed, assessed)
	if assessed == 0 {
		rateValue, rateSub, rateFg, rateBg = "—", "无可执行用例", colorMuted, colorPanel
	}
	cards := []struct {
		label, value, sub string
		fg, bg            [3]uint8
	}{
		{"通过率", rateValue, rateSub, rateFg, rateBg},
		{"用例总数", fmt.Sprintf("%d", s.Total), fmt.Sprintf("闸门 %d · P0 %d · P1 %d · P2 %d · P3 %d", levelCount["gate"], levelCount["P0"], levelCount["P1"], levelCount["P2"], levelCount["P3"]), colorBlue, colorBlueBg},
	}
	failedSub := "无失败项"
	if s.Failed > 0 {
		parts := []string{}
		for _, level := range []string{"P0", "P1", "P2", "P3"} {
			if levelFailed[level] > 0 {
				parts = append(parts, fmt.Sprintf("%s×%d", level, levelFailed[level]))
			}
		}
		failedSub = strings.Join(parts, " · ")
	}
	failedFg, failedBg := colorGreen, colorGreenBg
	if s.Failed > 0 {
		failedFg, failedBg = colorRed, colorRedBg
	}
	cards = append(cards, struct {
		label, value, sub string
		fg, bg            [3]uint8
	}{"未通过", fmt.Sprintf("%d 项", s.Failed), failedSub, failedFg, failedBg})

	if len(perfRows) > 0 {
		pf, pb := colorGreen, colorGreenBg
		value := fmt.Sprintf("%d / %d", perfPass, perfTotal)
		sub := "TTFT / TPOT / E2E"
		if perfTotal == 0 {
			pf, pb, value, sub = colorMuted, colorPanel, "—", "无完整门槛数据"
		} else if perfPass == 0 {
			pf, pb = colorRed, colorRedBg
		} else if perfPass < perfTotal {
			pf, pb = colorAmber, colorAmberBg
		}
		cards = append(cards, struct {
			label, value, sub string
			fg, bg            [3]uint8
		}{"延迟四项达标", value, sub, pf, pb})
	} else {
		groupsPassed := 0
		for _, g := range s.Groups {
			if g.Failed == 0 {
				groupsPassed++
			}
		}
		gf, gb := colorTeal, colorGreenBg
		if len(s.Groups) == 0 || groupsPassed == 0 {
			gf, gb = colorRed, colorRedBg
		} else if groupsPassed < len(s.Groups) {
			gf, gb = colorAmber, colorAmberBg
		}
		cards = append(cards, struct {
			label, value, sub string
			fg, bg            [3]uint8
		}{"分组通过", fmt.Sprintf("%d / %d", groupsPassed, len(s.Groups)), "前置闸门/协议/缓存", gf, gb})
	}

	const gap = 6.0
	cardW := (pdfWidth - 2*pdfMargin - 3*gap) / 4
	top := w.y
	for i, card := range cards {
		w.statCard(pdfMargin+float64(i)*(cardW+gap), top, cardW, 62, card.label, card.value, card.sub, card.fg, card.bg)
	}
	w.y = top + 62
	w.gap(6)
	w.textColor("口径：通过率 = 通过 ÷（总数 − 跳过）；性能档位以 TTFT P50/P95、TPOT P50、E2E P90 四项门槛同时达标为准。", 7, 10, colorFaint)
	w.gap(2)
}

// videoLimitPanel 在首页展示可选视频体积上限的关键指标（仅勾选并执行后出现）。
func (w *pdfWriter) videoLimitPanel(v videoLimitInfo) {
	const height = 48.0
	w.ensure(height + 8)
	top := w.y
	w.panelRect(pdfMargin, top, pdfWidth-pdfMargin, top+height, colorPanel, colorBorder, 6, 0.6)
	w.drawText("视频理解体积上限（二分实测）", pdfMargin+10, top+7, 8.5, colorInk)
	base := "Base64：未通过/未测"
	if v.base64MiB > 0 {
		base = fmt.Sprintf("Base64：%.1f MiB", v.base64MiB)
		if v.base64BodyMiB > 0 {
			base += fmt.Sprintf("（请求体 %.1f MiB）", v.base64BodyMiB)
		}
	}
	w.drawText(base, pdfMargin+10, top+20, 7.6, colorBlue)
	url := "URL：未测"
	switch {
	case v.urlSupported:
		url = fmt.Sprintf("URL：%.1f MiB", v.urlMiB)
		if v.urlBodyMiB > 0 {
			url += fmt.Sprintf("（请求体 %.1f MiB）", v.urlBodyMiB)
		}
	case v.urlNote != "":
		url = "URL：" + v.urlNote
	default:
		url = "URL：未通过"
	}
	w.drawText(w.clipUnits(url, 220/7.6), pdfMargin+270, top+20, 7.6, colorTeal)
	w.drawText(fmt.Sprintf("范围 1–100 MiB · 颜色识别校验 · base64 %d 次 / URL %d 次探测", int(v.base64Probes), int(v.urlProbes)), pdfMargin+10, top+33, 6.8, colorMuted)
	w.y = top + height + 8
}

// resultsTable 是「测试结果」分组表，结论用彩色药丸。
func (w *pdfWriter) resultsTable(rep report) {
	w.sectionHeader("测试结果", "按分组汇总；结论由 P0 否决项与 P1 必过项决定。")
	rows := make([][]tableCell, 0, len(rep.Summary.Groups))
	for _, g := range rep.Summary.Groups {
		assessed := g.Total - g.Exempt - g.Skipped
		rate := 0.0
		rateText := "—"
		rateFg := colorMuted
		if assessed > 0 {
			rate = float64(g.Passed) / float64(assessed) * 100
			rateText = fmt.Sprintf("%.1f%%", rate)
			rateFg, _ = rateColors(rate)
		}
		passFg := colorMuted
		if g.Passed > 0 {
			passFg = colorGreen
		}
		failFg := colorMuted
		if g.Failed > 0 {
			failFg = colorRed
		}
		conclusion := "通过"
		if g.Failed > 0 {
			conclusion = "有失败项"
		} else if g.Passed == 0 && g.Exempt > 0 {
			conclusion = "部分跳过"
		}
		misc := g.Exempt + g.Skipped
		row := []tableCell{
			dotCell(g.Label, groupColor(g.Group)),
			rightCell(fmt.Sprint(g.Total)),
			{Text: fmt.Sprint(g.Passed), Color: passFg, Align: 2},
			{Text: fmt.Sprint(g.Failed), Color: failFg, Align: 2},
			rightCell(fmt.Sprint(misc)),
			{Text: rateText, Color: rateFg, Align: 2},
			statusTableCell(conclusion),
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return
	}
	w.tableView(tableStyle{
		widths: []float64{150, 50, 45, 45, 75, 65, 85},
		header: []tableCell{textCell("测试分组"), rightCell("用例数"), rightCell("通过"), rightCell("失败"), rightCell("跳过"), rightCell("通过率"), tableCell{Text: "结论", Align: 1}},
		rows:   rows,
		zebra:  true,
	})
	w.gap(6)
}

func statusTableCell(label string) tableCell {
	fg, bg := statusColors(label)
	return pillCell(label, fg, bg)
}

func groupColor(group string) [3]uint8 {
	switch group {
	case suite.GroupGate:
		return colorViolet
	case suite.GroupFormat:
		return colorBlue
	case suite.GroupCache:
		return colorTeal
	case suite.GroupLatency:
		return colorGreen
	}
	return colorMuted
}

// overviewTable 是「对比总览」：一眼看出每档哪些指标达标。
func (w *pdfWriter) overviewTable(rows []performanceRow) {
	if len(rows) == 0 {
		return
	}
	w.sectionHeader("对比总览", "TTFT / E2E 用秒，TPOT 用毫秒/token；数值写作「实测 / 门槛」，绿色达标、红色未达标。")
	tableRows := make([][]tableCell, 0, len(rows))
	for _, row := range rows {
		m := row.metrics
		cells := []tableCell{strongCell(row.label), rightCell(benchmarkInputText(m))}
		cells = append(cells,
			scoreCell(m, "ttft_p50_ms", "target_ttft_p50_ms", 0.001, 1, "s", false),
			scoreCell(m, "ttft_p95_ms", "target_ttft_p95_ms", 0.001, 1, "s", false),
			scoreCell(m, "tpot_p50_ms", "target_tpot_p50_ms", 1, 1, "ms", false),
			scoreCell(m, "e2e_p90_ms", "target_e2e_p90_ms", 0.001, 1, "s", false),
			scoreCell(m, "success_rate_percent", "target_success_rate_percent", 1, 1, "%", true),
		)
		cache := tableCell{Text: percentMetricText(m, "cache_hit_rate_percent"), Color: colorTeal, Align: 2}
		cells = append(cells, cache, statusTableCell(perfStatusLabel(row.status)))
		tableRows = append(tableRows, cells)
	}
	w.tableView(tableStyle{
		widths:   []float64{52, 44, 60, 60, 58, 58, 44, 44, 58},
		header:   []tableCell{textCell("场景"), rightCell("输入 tok"), rightCell("TTFT P50 (s)"), rightCell("TTFT P95 (s)"), rightCell("TPOT P50 (ms)"), rightCell("E2E P90 (s)"), rightCell("成功率"), rightCell("缓存"), tableCell{Text: "判定", Align: 1}},
		rows:     tableRows,
		bodySize: 7.2,
		lineH:    9.6,
		zebra:    true,
	})
	budget := numberText(rows[0].metrics, "output_budget_tokens", 0)
	if budget == "—" {
		budget = "默认"
	}
	w.ensure(22)
	w.gap(4)
	w.textColor(fmt.Sprintf("本次基准输出预算：%s tokens/请求。", budget), 7, 10, colorMuted)
	w.gap(6)
}

// scoreCell 生成「实测 / 门槛」并按是否达标着色。
func scoreCell(metrics map[string]any, key, targetKey string, scale float64, decimals int, unit string, higher bool) tableCell {
	value, ok := numberMetric(metrics, key)
	if !ok {
		return tableCell{Text: "—", Color: colorFaint, Align: 2}
	}
	text := fmt.Sprintf("%.*f", decimals, value*scale)
	if unit == "%" {
		text += "%"
	}
	color := colorBody
	if target, hasTarget := numberMetric(metrics, targetKey); hasTarget {
		pass := gatePass(value, target, higher)
		color = colorRed
		if pass {
			color = colorGreen
		}
		if unit != "%" {
			text += fmt.Sprintf(" / %.*f", decimals, target*scale)
		}
	}
	return tableCell{Text: text, Color: color, Align: 2}
}

// valueCell 只显示实测值但按门槛着色，用于列宽不足的精确数值表。
func valueCell(metrics map[string]any, key, targetKey string, scale float64, decimals int, unit string, higher bool) tableCell {
	value, ok := numberMetric(metrics, key)
	if !ok {
		return tableCell{Text: "—", Color: colorFaint, Align: 2}
	}
	text := fmt.Sprintf("%.*f", decimals, value*scale)
	if unit == "%" {
		text += "%"
	}
	color := colorBody
	if target, hasTarget := numberMetric(metrics, targetKey); hasTarget {
		color = colorRed
		if gatePass(value, target, higher) {
			color = colorGreen
		}
	}
	return tableCell{Text: text, Color: color, Align: 2}
}

// performanceSection 是性能仪表盘：结论条 + 2×2 指标卡 + 精确数值表。
func (w *pdfWriter) performanceSection(rows []performanceRow) {
	w.ensure(410)
	w.sectionHeader("性能对比", "横轴为输入档位；绿色达标、红色未达标，虚线为门槛；TTFT/E2E 用秒，TPOT 用毫秒/token。")
	w.chartLegend()
	w.perfBanner(rows)

	type metricSpec struct {
		title, note, key, target, unit string
		scale                          float64
		higher                         bool
		color                          [3]uint8
	}
	specs := []metricSpec{
		{"首字延迟 TTFT P50（秒）", "单位：秒 · 用户发出请求后等第一个字", "ttft_p50_ms", "target_ttft_p50_ms", "s", 0.001, false, colorBlue},
		{"慢请求首字 TTFT P95（秒）", "单位：秒 · 最慢 5% 请求的首字等待", "ttft_p95_ms", "target_ttft_p95_ms", "s", 0.001, false, colorViolet},
		{"输出间隔 TPOT P50（毫秒/token）", "单位：毫秒 · 生成每个 token 的平均间隔", "tpot_p50_ms", "target_tpot_p50_ms", "ms", 1, false, colorTeal},
		{"端到端 E2E P90（秒）", "单位：秒 · 从发出请求到回答结束", "e2e_p90_ms", "target_e2e_p90_ms", "s", 0.001, false, colorAmber},
	}
	labels := make([]string, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, row.label)
	}
	const gap = 6.0
	cardW := (pdfWidth - 2*pdfMargin - gap) / 2
	cardH := 150.0
	type chart struct {
		spec            metricSpec
		values, targets []float64
	}
	charts := make([]chart, 0, len(specs))
	for _, spec := range specs {
		values := make([]float64, 0, len(rows))
		targets := make([]float64, 0, len(rows))
		hasAny := false
		for _, row := range rows {
			v, ok := numberMetric(row.metrics, spec.key)
			if !ok {
				v = 0
			} else if v > 0 {
				hasAny = true
			}
			t, hasTarget := numberMetric(row.metrics, spec.target)
			if !hasTarget {
				t = 0
			}
			v *= spec.scale
			t *= spec.scale
			values = append(values, v)
			targets = append(targets, t)
		}
		if hasAny {
			charts = append(charts, chart{spec: spec, values: values, targets: targets})
		}
	}
	if len(charts) > 0 {
		chartRows := (len(charts) + 1) / 2
		w.ensure(float64(chartRows)*(cardH+gap) + 4)
		startY := w.y
		for index, item := range charts {
			x := pdfMargin + float64(index%2)*(cardW+gap)
			y := startY + float64(index/2)*(cardH+gap)
			w.barChartCard(x, y, cardW, cardH, item.spec.title, item.spec.note, labels, item.values, item.targets, item.spec.higher, item.spec.color, item.spec.unit)
		}
		w.y = startY + float64(chartRows)*(cardH+gap) + 2
	}

	w.gap(4)
	w.drawText("精确数值", pdfMargin, w.y, 10.5, colorInk)
	w.y += 16
	tableRows := make([][]tableCell, 0, len(rows))
	for _, row := range rows {
		m := row.metrics
		tableRows = append(tableRows, []tableCell{
			strongCell(row.label),
			rightCell(benchmarkInputText(m)),
			valueCell(m, "ttft_p50_ms", "target_ttft_p50_ms", 0.001, 1, "s", false),
			valueCell(m, "ttft_p95_ms", "target_ttft_p95_ms", 0.001, 1, "s", false),
			valueCell(m, "tpot_p50_ms", "target_tpot_p50_ms", 1, 1, "ms", false),
			valueCell(m, "tpot_p95_ms", "target_tpot_p95_ms", 1, 1, "ms", false),
			valueCell(m, "e2e_p90_ms", "target_e2e_p90_ms", 0.001, 1, "s", false),
			valueCell(m, "success_rate_percent", "target_success_rate_percent", 1, 1, "%", true),
			tableCell{Text: positiveMetricText(m, "otps_p50", 1), Color: colorTeal, Align: 2},
			tableCell{Text: positiveMetricText(m, "itl_p95_ms", 1), Color: colorBody, Align: 2},
			tableCell{Text: percentMetricText(m, "cache_hit_rate_percent"), Color: colorTeal, Align: 2},
		})
	}
	w.tableView(tableStyle{
		widths:     []float64{48, 42, 50, 50, 50, 50, 50, 44, 48, 42, 41},
		header:     []tableCell{textCell("场景"), rightCell("输入 tok"), rightCell("TTFT P50 (s)"), rightCell("TTFT P95 (s)"), rightCell("TPOT P50\n(ms)"), rightCell("TPOT P95\n(ms)"), rightCell("E2E P90 (s)"), rightCell("成功率"), rightCell("输出 tok/s"), rightCell("ITL P95"), rightCell("缓存")},
		rows:       tableRows,
		bodySize:   7.1,
		headerSize: 6.6,
		lineH:      9.4,
		zebra:      true,
	})
	w.gap(4)
}

// perfBanner 是性能结论条：多少档位四项延迟全部达标。
func (w *pdfWriter) perfBanner(rows []performanceRow) {
	total, allPass, failedByGate := perfVerdict(rows)
	if total == 0 {
		return
	}
	fg, bg := colorGreen, colorGreenBg
	if allPass == 0 {
		fg, bg = colorRed, colorRedBg
	} else if allPass < total {
		fg, bg = colorAmber, colorAmberBg
	}
	text := fmt.Sprintf("%d 档中 %d 档四项延迟全部达标", total, allPass)
	if allPass < total {
		details := []string{}
		for i, gate := range perfLatencyGates {
			if failedByGate[i] > 0 {
				details = append(details, fmt.Sprintf("%s %d 档未达标", gate.label, failedByGate[i]))
			}
		}
		text += "（" + strings.Join(details, "，") + "）"
	}
	lines := w.wrapUnits(text, (pdfWidth-2*pdfMargin-40)/8.2)
	height := float64(len(lines))*11 + 12
	w.ensure(height + 6)
	w.panelRect(pdfMargin, w.y, pdfWidth-pdfMargin, w.y+height, bg, fg, 6, 0.6)
	w.dot(pdfMargin+9, w.y+height/2-3, 6, fg)
	for i, line := range lines {
		w.drawText(line, pdfMargin+24, w.y+6+float64(i)*11, 8.2, fg)
	}
	w.y += height + 8
}

// scenarioSection 是逐档场景卡：配置、分位、门槛、样本趋势。
func (w *pdfWriter) scenarioSection(rows []performanceRow) {
	w.ensure(300)
	w.sectionHeader("场景明细", "每个输入档位一张卡：测试配置、延迟分位、门槛校验与 TTFT 样本趋势。")
	for _, row := range rows {
		w.scenarioCard(row)
	}
}

func (w *pdfWriter) scenarioCard(row performanceRow) {
	m := row.metrics
	samples := numberSlice(m, "sample_ttft_ms")
	cardH := 224.0
	if len(samples) == 0 {
		cardH = 158.0
	}
	w.ensure(cardH + 10)
	x := pdfMargin
	width := pdfWidth - 2*pdfMargin
	top := w.y
	fg, bg := statusColors(perfStatusLabel(row.status))
	w.panelRect(x, top, x+width, top+cardH, [3]uint8{0xff, 0xff, 0xff}, colorBorder, 7, 0.6)
	w.fillRect(x+1, top+1, x+width-1, top+23, bg, 6)
	w.dot(x+12, top+9, 5, fg)
	w.drawText(w.clipUnits(row.label+" · "+row.name, 300/10), x+21, top+6, 10, colorInk)
	statusText := perfStatusLabel(row.status)
	statusW := w.textWidth(statusText, 7.2) + 10
	level := levelBadgeText(row.level)
	levelFg, levelBg := levelPillStyle(row.level)
	levelW := w.textWidth(level, 7.2) + 10
	w.pill(x+width-10-statusW-6-levelW, top+6, level, levelFg, levelBg, 7.2)
	w.statusPill(statusText, x+width-10-statusW, top+6)

	// 左：测试配置；右：延迟分位表。
	configX := x + 12
	w.drawText("测试配置", configX, top+30, 7, colorMuted)
	configPairs := [][2]string{
		{"并发 × 样本", numberText(m, "concurrency", 0) + " × " + numberText(m, "samples", 0)},
		{"输入 tokens", benchmarkInputText(m)},
		{"成功率", percentMetricText(m, "success_rate_percent")},
		{"缓存命中", percentMetricText(m, "cache_hit_rate_percent")},
		{"输出预算", numberText(m, "output_budget_tokens", 0) + " tok"},
		{"测试耗时", shortDuration(numberMetricValue(m, "duration_ms"))},
	}
	for i, pair := range configPairs {
		y := top + 42 + float64(i)*11
		w.drawText(pair[0], configX, y, 7.2, colorMuted)
		w.drawText(w.clipUnits(pair[1], 100/8), configX+62, y, 8, colorBody)
	}

	tableX := x + 248
	tableW := width - 260
	w.drawText("延迟统计（TTFT/E2E 秒，TPOT ms）", tableX, top+30, 7, colorMuted)
	percentiles := []struct {
		label    string
		p50, p95 float64
		p99, max float64
		scale    float64
		decimals int
	}{
		{"TTFT", numberMetricValue(m, "ttft_p50_ms"), numberMetricValue(m, "ttft_p95_ms"), numberMetricValue(m, "ttft_p99_ms"), numberMetricValue(m, "ttft_max_ms"), 0.001, 2},
		{"TPOT", numberMetricValue(m, "tpot_p50_ms"), numberMetricValue(m, "tpot_p95_ms"), numberMetricValue(m, "tpot_p99_ms"), numberMetricValue(m, "tpot_max_ms"), 1, 1},
		{"E2E", numberMetricValue(m, "e2e_p50_ms"), numberMetricValue(m, "e2e_p95_ms"), numberMetricValue(m, "e2e_p99_ms"), numberMetricValue(m, "e2e_max_ms"), 0.001, 2},
	}
	cols := []float64{52, 48, 48, 48, 52}
	headers := []string{"阶段", "P50", "P95", "P99", "Max"}
	w.fillRect(tableX, top+41, tableX+tableW, top+53, colorPanel, 2)
	cx := tableX
	for i, h := range headers {
		al := 0
		if i > 0 {
			al = 2
		}
		tx := cx + 4
		if al == 2 {
			tx = cx + cols[i] - 4 - w.textWidth(h, 6.5)
		}
		w.drawText(h, tx, top+43.5, 6.5, colorMuted)
		cx += cols[i]
	}
	for r, item := range percentiles {
		y := top + 55 + float64(r)*13
		w.hlineAt(tableX, tableX+tableW, y)
		w.drawText(item.label, tableX+4, y+2.5, 7.2, colorMuted)
		vals := []float64{item.p50, item.p95, item.p99, item.max}
		for i, v := range vals {
			text := "—"
			if v > 0 {
				text = fmt.Sprintf("%.*f", item.decimals, v*item.scale)
			}
			px := tableX + sum(cols[:i+1]) - 4 - w.textWidth(text, 7.2)
			w.drawText(text, px, y+2.5, 7.2, colorBody)
		}
	}
	w.hlineAt(tableX, tableX+tableW, top+55+3*13)

	// 门槛校验：两列三行。
	gateTop := top + 100
	w.drawText("门槛校验", configX, gateTop, 7, colorMuted)
	gateX := configX
	gateW := (width - 24) / 2
	gateRow := 0
	for _, gate := range perfCardGates {
		value, hasValue := numberMetric(m, gate.key)
		target, hasTarget := numberMetric(m, gate.target)
		if !hasValue || !hasTarget {
			continue
		}
		col := gateRow % 2
		rowIdx := gateRow / 2
		gx := gateX + float64(col)*(gateW+8)
		gy := gateTop + 12 + float64(rowIdx)*12
		pass := gatePass(value, target, gate.higher)
		color := colorRed
		if pass {
			color = colorGreen
		}
		scale, decimals, unit := 1.0, gate.decimals, gate.unit
		if gate.unit == "ms" && (strings.HasPrefix(gate.key, "ttft_") || strings.HasPrefix(gate.key, "e2e_")) {
			scale, decimals, unit = 0.001, 2, "s"
		}
		w.dot(gx+1, gy+3, 4, color)
		w.drawText(gate.label, gx+10, gy, 7.2, colorInk)
		actual := fmt.Sprintf("%.*f%s", decimals, value*scale, unit)
		ix := gx + 10 + w.textWidth(gate.label, 7.2) + 5
		w.drawText(actual, ix, gy, 7.2, color)
		rel := "≤"
		if gate.higher {
			rel = "≥"
		}
		targetText := fmt.Sprintf("%s%.*f%s", rel, decimals, target*scale, unit)
		w.drawText(targetText, ix+w.textWidth(actual, 7.2)+3, gy, 6.6, colorFaint)
		gateRow++
	}

	if len(samples) > 0 {
		threshold, _ := numberMetric(m, "target_ttft_p95_ms")
		w.linePlot(x+12, top+146, width-24, 70, "TTFT 样本趋势（ms）", "虚线为 P95 门槛", samples, threshold, colorRed, "ms")
	}
	w.y = top + cardH + 8
}

// hlineAt 在指定横坐标范围画细线。
func (w *pdfWriter) hlineAt(x0, x1, y float64) {
	w.line(x0, y, x1, y, colorTrack, 0.5)
}

func numberMetricValue(metrics map[string]any, key string) float64 {
	v, _ := numberMetric(metrics, key)
	return v
}

func numberSlice(metrics map[string]any, key string) []float64 {
	raw, ok := metrics[key].([]any)
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(raw))
	for _, item := range raw {
		if v, ok := item.(float64); ok && v >= 0 {
			out = append(out, v)
		}
	}
	return out
}

func shortDuration(ms float64) string {
	if ms <= 0 {
		return "—"
	}
	seconds := ms / 1000
	if seconds < 60 {
		return fmt.Sprintf("%.0fs", seconds)
	}
	return fmt.Sprintf("%dm %02ds", int(seconds)/60, int(seconds)%60)
}

// cacheSection 画缓存命中率：TTL 曲线优先，否则横向条形。
func (w *pdfWriter) cacheSection(rows []cachePerformanceRow) {
	w.ensure(170)
	w.sectionHeader("缓存命中", "命中率越高，重复前缀的计费与首字延迟越低；绿色 ≥80%，橙色 50–79%，红色 <50%。")
	hasCurve := false
	points := make([]cachePerformanceRow, 0, len(rows))
	bars := make([]cachePerformanceRow, 0, len(rows))
	for _, row := range rows {
		if row.hasSeconds {
			hasCurve = true
			points = append(points, row)
		} else {
			bars = append(bars, row)
		}
	}
	if hasCurve {
		w.decayPlot(points)
	}
	if len(bars) > 0 {
		curvePrefixes := map[string]bool{}
		for _, point := range points {
			curvePrefixes[cacheLabelPrefix(point.label)] = true
		}
		filtered := bars[:0]
		for _, bar := range bars {
			if !curvePrefixes[cacheLabelPrefix(bar.label)] {
				filtered = append(filtered, bar)
			}
		}
		if len(filtered) > 0 {
			w.cacheBars(filtered)
		}
	}
}

// capabilitySection 是「能力验证」：非性能用例按分组逐个出卡。
func (w *pdfWriter) capabilitySection(rep report) {
	cases := make([]caseRow, 0, len(rep.Cases))
	for _, c := range rep.Cases {
		var m map[string]any
		if json.Unmarshal(c.Metrics, &m) == nil && hasPerformanceMetrics(m) {
			continue
		}
		cases = append(cases, c)
	}
	if len(cases) == 0 {
		return
	}
	w.ensure(330)
	w.sectionHeader("能力验证", "逐项核对协议、格式与缓存；每张卡给出为什么测、怎么测、预期与实测。")
	counts := map[string]int{}
	for _, c := range cases {
		counts[c.Status]++
	}
	const gap = 6.0
	cardW := (pdfWidth - 2*pdfMargin - 3*gap) / 4
	top := w.y
	stats := []struct {
		label, value, sub string
		fg, bg            [3]uint8
	}{
		{"通过", fmt.Sprint(counts["passed"]), "符合预期", colorGreen, colorGreenBg},
		{"失败", fmt.Sprint(counts["failed"]), "需修复或复核", colorRed, colorRedBg},
		{"能力跳过", fmt.Sprint(counts["exempt"]), "能力未声明", colorAmber, colorAmberBg},
		{"跳过", fmt.Sprint(counts["skipped"]), "本次未执行", colorMuted, colorPanel},
	}
	for i, st := range stats {
		w.statCard(pdfMargin+float64(i)*(cardW+gap), top, cardW, 54, st.label, st.value, st.sub, st.fg, st.bg)
	}
	w.y = top + 54 + 8

	current := ""
	for _, c := range cases {
		if c.Group != current {
			current = c.Group
			count := 0
			failed := 0
			for _, item := range cases {
				if item.Group == c.Group {
					count++
					if item.Status == "failed" {
						failed++
					}
				}
			}
			note := fmt.Sprintf("%d 项 · 通过 %d · 失败 %d", count, count-failed, failed)
			w.ensure(140)
			w.groupHeader(c.GroupLabel, note, groupColor(c.Group))
		}
		w.capabilityCard(c, rep.Masked)
	}
}

func (w *pdfWriter) capabilityCard(c caseRow, masked bool) {
	estimate, huge := w.capabilityCardEstimate(c, masked)
	if huge {
		w.ensure(72)
	} else {
		w.ensure(estimate)
	}
	x := pdfMargin
	width := pdfWidth - 2*pdfMargin
	top := w.y
	fg, bg := statusColors(statusLabel(c.Status))
	w.fillRect(x, top, x+width, top+22, bg, 4)
	w.fillRect(x, top+2, x+3, top+20, fg, 1.5)
	w.dot(x+11, top+8, 5, fg)
	title := c.CaseID + " · " + c.Name
	statusText := statusLabel(c.Status)
	statusW := w.textWidth(statusText, 7.2) + 10
	level := levelBadgeText(c.Level)
	levelFg, levelBg := levelPillStyle(c.Level)
	levelW := w.textWidth(level, 7.2) + 10
	pillsW := statusW + levelW + 12
	w.drawText(w.clipUnits(title, (width-pillsW-24)/9.5), x+20, top+5, 9.5, colorInk)
	lx := x + width - 8 - statusW - 6 - levelW
	w.pill(lx, top+5, level, levelFg, levelBg, 7.2)
	w.statusPill(statusText, x+width-8-statusW, top+5)
	w.y = top + 24

	meta := levelShortNote(c.Level)
	if c.HTTPStatus != 0 {
		meta += fmt.Sprintf(" · HTTP %d", c.HTTPStatus)
	}
	if c.DurationMs > 0 {
		meta += fmt.Sprintf(" · 耗时 %dms", c.DurationMs)
	}
	if c.Retries > 0 {
		meta += fmt.Sprintf(" · 重试 %d 次", c.Retries)
	}
	if c.RequestID != "" && !masked {
		meta += " · request_id " + c.RequestID
	}
	w.drawText(w.clipUnits(meta, width/7), x, w.y, 7, colorMuted)
	w.y += 11

	w.labeledParagraph("为什么测", c.Doc.Purpose, x, width, 52, 8.5, 12, colorBody)
	w.labeledParagraph("怎么测", c.Doc.Method, x, width, 52, 8.5, 12, colorBody)
	expect := c.Doc.Expect
	if c.Doc.Ref != "" {
		expect += "（" + c.Doc.Ref + "）"
	}
	w.labeledParagraph("预期", expect, x, width, 52, 8.5, 12, colorBody)
	if c.Summary != "" {
		w.labeledParagraph("实测", c.Summary, x, width, 52, 8.5, 12, fg)
	}
	w.gap(2)
	w.defectBox(c)
	w.metricTable(c)
	w.evidence(c, masked)
	w.gap(5)
	w.hline(w.y)
	w.gap(6)
}

func levelShortNote(level string) string {
	switch normalizeLevel(level) {
	case "P0":
		return "P0 否决项"
	case "P1":
		return "P1 必过项"
	case "P2":
		return "P2 记录项"
	case "P3":
		return "P3 可选项"
	case "gate":
		return "前置闸门"
	}
	return level
}

// defectBox 在用例卡里渲染失败原因；无缺陷时不占空间。
func (w *pdfWriter) defectBox(c caseRow) {
	var defects []suite.Defect
	if json.Unmarshal(c.Defects, &defects) != nil || len(defects) == 0 {
		return
	}
	x := pdfMargin
	width := pdfWidth - 2*pdfMargin
	type line struct {
		text  string
		color [3]uint8
	}
	lines := []line{}
	for _, d := range defects {
		lines = append(lines, line{text: normalizeLevel(d.Level) + " 期望：" + d.Expect, color: colorBody})
		lines = append(lines, line{text: "实测：" + d.Actual, color: colorRed})
	}
	wrapped := make([][]string, len(lines))
	height := 8.0
	for i, item := range lines {
		wrapped[i] = w.wrapUnits(item.text, (width-24)/7.5)
		height += float64(len(wrapped[i]))*10 + 2
	}
	w.ensure(height + 4)
	w.panelRect(x, w.y, x+width, w.y+height, colorRedBg, colorBorder, 4, 0.5)
	w.fillRect(x+1, w.y+1, x+3, w.y+height-1, colorRed, 1)
	top := w.y + 6
	for i, item := range lines {
		for _, text := range wrapped[i] {
			w.drawText(text, x+9, top, 7.5, item.color)
			top += 10
		}
		top += 2
	}
	w.y += height + 4
}

// metricTable 输出用例的非看板指标；看板指标已在性能区展示，不重复。
func (w *pdfWriter) metricTable(c caseRow) {
	var metrics map[string]any
	if json.Unmarshal(c.Metrics, &metrics) != nil || len(metrics) == 0 {
		return
	}
	keys := []string{}
	for _, key := range sortedKeys(metrics) {
		if isDashboardMetric(key) {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return
	}
	rows := make([][]tableCell, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []tableCell{strongCell(key), textCell(metricText(metrics[key]))})
	}
	w.ensure(30)
	w.drawText("指标明细", pdfMargin, w.y, 7.5, colorMuted)
	w.y += 10
	w.tableView(tableStyle{
		widths:   []float64{210, 305},
		header:   []tableCell{textCell("指标"), textCell("值")},
		rows:     rows,
		bodySize: 7.3,
		lineH:    9.6,
		zebra:    true,
	})
	w.gap(4)
}

// evidence 输出完整报告的请求/响应证据；脱敏版与精简版无内容。
func (w *pdfWriter) evidence(c caseRow, masked bool) {
	if masked || len(c.Evidence) == 0 {
		return
	}
	var items []map[string]any
	if json.Unmarshal(c.Evidence, &items) != nil {
		return
	}
	for _, item := range items {
		label, _ := item["label"].(string)
		curl, _ := item["curl"].(string)
		w.drawText("请求证据（"+label+"，curl 已脱敏）", pdfMargin, w.y, 7.5, colorMuted)
		w.y += 10
		w.box(curl, 7.2)
		if result, ok := item["result"].(map[string]any); ok {
			raw, _ := result["raw"].(string)
			if raw != "" {
				w.drawText(fmt.Sprintf("响应证据（HTTP %v%s）", result["http_status"], truncateNote(raw, 1200)), pdfMargin, w.y, 7.5, colorMuted)
				w.y += 10
				w.box(truncateRunes(raw, 1200), 7.2)
			}
		}
	}
}

// defectSection 是独立缺陷清单表。
func (w *pdfWriter) defectSection(rep report) {
	w.ensure(150)
	rows := make([][]tableCell, 0, len(rep.Defects))
	for _, d := range rep.Defects {
		fg, bg := levelPillStyle(d.Level)
		rows = append(rows, []tableCell{
			pillCell(levelBadgeText(d.Level), fg, bg),
			strongCell(d.CaseID),
			textCell(w.clipUnits(d.Expect, 170/7.4)),
			{Text: w.clipUnits(d.Actual, 220/7.4), Color: colorRed},
		})
	}
	w.sectionHeader("缺陷清单", "失败项的期望与实测对照。")
	w.tableView(tableStyle{
		widths:   []float64{40, 58, 200, 217},
		header:   []tableCell{tableCell{Text: "级别", Align: 1}, textCell("用例"), textCell("期望"), textCell("实测")},
		rows:     rows,
		bodySize: 7.4,
		lineH:    9.8,
		zebra:    true,
	})
	w.gap(6)
}

// appendix 汇总环境、声明与合并任务，作为报告的可追溯附录。
func (w *pdfWriter) appendix(rep report) {
	w.ensure(280)
	w.sectionHeader("报告说明", "环境信息与能力声明，用于复现与归档。")
	env := rep.Environment
	reportType := "详细版"
	if rep.Masked && rep.Brief {
		reportType = "脱敏精简版"
	} else if rep.Masked {
		reportType = "脱敏版"
	} else if rep.Brief {
		reportType = "精简版"
	}
	pairs := [][2]string{
		{"任务 ID", dashIfEmpty(env.TaskID)},
		{"渠道", orDash(env.Channel)},
		{"模型", dashIfEmpty(env.Model)},
		{"评级", dashIfEmpty(rep.Grade)},
		{"测试标准", dashIfEmpty(rep.Standard)},
		{"测试范围", dashIfEmpty(env.PresetName)},
		{"网关地址", dashIfEmpty(env.BaseURL)},
		{"创建人", dashIfEmpty(env.CreatedBy)},
		{"开始时间", stampText(env.StartedAt)},
		{"结束时间", stampText(env.EndedAt)},
		{"测试用时", durationText(env.StartedAt, env.EndedAt)},
		{"时区", dashIfEmpty(env.Timezone)},
		{"报告类型", reportType},
		{"水印", func() string {
			if rep.Watermark == "" {
				return "无水印"
			}
			return rep.Watermark
		}()},
	}
	if rep.StopReason != "" {
		pairs = append(pairs, [2]string{"停止原因", rep.StopReason})
	}
	left := pairs
	right := [][2]string{}
	if len(pairs) > 6 {
		left = pairs[:6]
		right = pairs[6:]
	}
	colW := (pdfWidth - 2*pdfMargin - 40) / 2
	leftH := w.kvHeight(left, colW, 54, 8.2, 10)
	rightH := w.kvHeight(right, colW, 54, 8.2, 10)
	bodyH := leftH
	if rightH > bodyH {
		bodyH = rightH
	}
	panelH := bodyH + 34
	w.ensure(panelH + 8)
	top := w.y
	w.panelRect(pdfMargin, top, pdfWidth-pdfMargin, top+panelH, colorPanel, colorBorder, 6, 0.6)
	w.drawText("测试环境", pdfMargin+12, top+9, 8.5, colorMuted)
	w.kvColumn(left, pdfMargin+12, top+24, colW, 54, 8.2, 10)
	if len(right) > 0 {
		w.kvColumn(right, pdfMargin+12+colW+16, top+24, colW, 54, 8.2, 10)
	}
	w.y = top + panelH + 10

	if len(rep.Declarations) > 0 {
		w.drawText("能力声明", pdfMargin, w.y, 8.5, colorMuted)
		w.y += 12
		keys := make([]string, 0, len(rep.Declarations))
		for k := range rep.Declarations {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		items := make([]string, 0, len(keys))
		for _, key := range keys {
			state := "未声明"
			if rep.Declarations[key] {
				state = "声明支持"
			}
			items = append(items, key+"："+state)
		}
		w.flowChips(items, pdfMargin, pdfWidth-2*pdfMargin)
		w.gap(6)
	}
	if len(rep.Merged) > 0 {
		w.drawText(fmt.Sprintf("合并自 %d 次测试", len(rep.Merged)), pdfMargin, w.y, 8.5, colorMuted)
		w.y += 12
		rows := make([][]tableCell, 0, len(rep.Merged))
		for _, item := range rep.Merged {
			rows = append(rows, []tableCell{strongCell(item.ID), textCell(item.PresetName), statusTextCell(item.Verdict), textCell(stampText(item.FinishedAt))})
		}
		w.tableView(tableStyle{
			widths:   []float64{110, 150, 120, 135},
			header:   []tableCell{textCell("任务"), textCell("范围"), textCell("结论"), textCell("完成时间")},
			rows:     rows,
			bodySize: 7.5,
			lineH:    9.8,
			zebra:    true,
		})
	}
	w.gap(8)
	w.textColor("本报告由 niceaigc.cn 自动生成；结论仅对本次测试样本、该渠道与模型负责。", 7, 10, colorFaint)
}

func statusTextCell(verdict string) tableCell {
	fg, bg := verdictColors(verdict)
	return tableCell{Text: verdict, Color: fg, Bg: bg, HasBg: true}
}

// kvHeight 预估一组键值对的高度，供面板先占位。
func (w *pdfWriter) kvHeight(pairs [][2]string, width, labelW, size, leading float64) float64 {
	total := 0.0
	for _, pair := range pairs {
		lines := w.wrapUnits(pair[1], (width-labelW)/size)
		if len(lines) == 0 {
			lines = []string{""}
		}
		total += float64(len(lines)) * leading
	}
	return total
}

func (w *pdfWriter) kvColumn(pairs [][2]string, x, top, width, labelW, size, leading float64) {
	y := top
	for _, pair := range pairs {
		lines := w.wrapUnits(pair[1], (width-labelW)/size)
		if len(lines) == 0 {
			lines = []string{""}
		}
		for i, line := range lines {
			if i == 0 {
				w.drawText(pair[0], x, y, 7.2, colorMuted)
			}
			w.drawText(line, x+labelW, y, size, colorBody)
			y += leading
		}
	}
}

// paragraphAt 按固定宽度输出段落，用于封面副标题等不宜通栏的位置。
func (w *pdfWriter) paragraphAt(x, width float64, s string, size, leading float64, color [3]uint8) {
	if s == "" || w.err != nil {
		return
	}
	lines := w.wrapUnits(s, width/size)
	for _, line := range lines {
		w.ensure(leading)
		w.drawText(line, x, w.y, size, color)
		w.y += leading
	}
}

// thresholdText 把门槛值写成适合卡片阅读的形式：2000ms 而不是 2.0kms。
func thresholdText(v float64, unit string) string {
	switch unit {
	case "s":
		switch {
		case v >= 10:
			return fmt.Sprintf("%.0fs", v)
		case v >= 1:
			return fmt.Sprintf("%.1fs", v)
		default:
			return fmt.Sprintf("%.2fs", v)
		}
	case "ms":
		return fmt.Sprintf("%.0fms", v)
	}
	return shortNumber(v) + unit
}

func cacheLabelPrefix(label string) string {
	if i := strings.Index(label, " · "); i >= 0 {
		return label[:i]
	}
	return label
}

// capabilityCardEstimate 预估一张能力卡的高度；证据过长时允许跨页（huge=true）。
func (w *pdfWriter) capabilityCardEstimate(c caseRow, masked bool) (float64, bool) {
	const size, leading = 8.5, 12.0
	width := pdfWidth - 2*pdfMargin
	height := 24.0 + 11.0 + 6.0
	expect := c.Doc.Expect
	if c.Doc.Ref != "" {
		expect += "（" + c.Doc.Ref + "）"
	}
	for _, text := range []string{c.Doc.Purpose, c.Doc.Method, expect, c.Summary} {
		if text == "" {
			continue
		}
		height += float64(len(w.wrapUnits(text, (width-52)/size))) * leading
	}
	height += w.defectBoxHeight(c) + 4
	height += w.metricTableHeight(c)
	if !masked && len(c.Evidence) > 0 {
		if len(c.Evidence) > 2400 {
			return height, true
		}
		height += 36
	}
	height += 16
	if height > pdfContentBottom-pdfContentTop-4 {
		return height, true
	}
	return height, false
}

// defectBoxHeight 与 defectBox 同一折行口径。
func (w *pdfWriter) defectBoxHeight(c caseRow) float64 {
	var defects []suite.Defect
	if json.Unmarshal(c.Defects, &defects) != nil || len(defects) == 0 {
		return 0
	}
	width := pdfWidth - 2*pdfMargin - 24
	height := 8.0
	for _, d := range defects {
		for _, text := range []string{normalizeLevel(d.Level) + " 期望：" + d.Expect, "实测：" + d.Actual} {
			height += float64(len(w.wrapUnits(text, width/7.5))) * 10
		}
		height += 2
	}
	return height + 4
}

// metricTableHeight 预估指标明细表高度（含表头）。
func (w *pdfWriter) metricTableHeight(c caseRow) float64 {
	var metrics map[string]any
	if json.Unmarshal(c.Metrics, &metrics) != nil || len(metrics) == 0 {
		return 0
	}
	keys := []string{}
	for _, key := range sortedKeys(metrics) {
		if !isDashboardMetric(key) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return 0
	}
	height := 10.0 + 22.0
	for _, key := range keys {
		lines := len(w.wrapUnits(metricText(metrics[key]), 295/7.3))
		row := float64(lines)*9.6 + 10
		if row < 20 {
			row = 20
		}
		height += row
	}
	return height + 4
}
