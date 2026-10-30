package llmadmit

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/signintech/gopdf"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

// 文泉驿微米黑（提取自 wqy-microhei.ttc，授权见 fonts/LICENSE.txt），用于 CJK 渲染。
//
//go:embed fonts/wqy-microhei-sc.ttf
var pdfFont []byte

const (
	pdfMargin = 40.0
	pdfWidth  = 595.28 // A4 宽（pt）
	pdfHeight = 841.89 // A4 高（pt）
)

var (
	colorTitle  = [3]uint8{0x1c, 0x2b, 0x4a}
	colorMuted  = [3]uint8{0x64, 0x74, 0x8b}
	colorBorder = [3]uint8{0xd4, 0xd7, 0xdd}
	colorFill   = [3]uint8{0xf1, 0xf5, 0xf9}
	colorPass   = [3]uint8{0x16, 0xa3, 0x4a}
	colorFail   = [3]uint8{0xdc, 0x26, 0x26}
	colorWarn   = [3]uint8{0xb4, 0x53, 0x09}
)

type pdfWriter struct {
	pdf *gopdf.GoPdf
	y   float64
	err error
}

// renderPDF 把报告渲染为 PDF；排版参考基准报告：摘要总览 + 分档表格 + 用例详情卡。
func renderPDF(rep report) ([]byte, error) {
	w := &pdfWriter{pdf: &gopdf.GoPdf{}}
	w.pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := w.pdf.AddTTFFontData("wqy", pdfFont); err != nil {
		return nil, err
	}
	w.newPage()

	// 标题区
	w.text(rep.Standard+" 准入测试报告", 16, 24)
	w.rule()
	if rep.Masked {
		w.text("（脱敏版：已隐去测试目标地址、密钥与 request_id）", 8.5, 12)
	}
	if rep.Brief {
		w.text("（精简版：不含请求/响应原文，保留说明、判据与缺陷解释）", 8.5, 12)
	}
	w.kvGrid([][2]string{
		{"渠道", orDash(rep.Environment.Channel)},
		{"模型", rep.Environment.Model},
		{"网关", rep.Environment.BaseURL},
		{"档位", rep.Environment.Tier},
		{"裁决", rep.Verdict},
		{"时段", rep.Environment.StartedAt + " ~ " + rep.Environment.EndedAt},
	})
	if rep.StopReason != "" {
		w.text("说明："+rep.StopReason, 9, 13)
	}
	if len(rep.Merged) > 0 {
		rows := [][]string{}
		for _, item := range rep.Merged {
			rows = append(rows, []string{item.ID, item.Tier, item.Verdict, item.FinishedAt})
		}
		w.gap(4)
		w.text(fmt.Sprintf("合并自 %d 次测试", len(rep.Merged)), 10, 15)
		w.table([]string{"任务", "档位", "结论", "完成时间"}, rows, []float64{120, 40, 130, 225})
	}
	if len(rep.Declarations) > 0 {
		decls := []string{}
		for k, v := range rep.Declarations {
			decls = append(decls, fmt.Sprintf("%s=%v", k, v))
		}
		w.text("能力声明："+strings.Join(decls, "，"), 9, 13)
	}

	// 摘要（对应基准报告的总览区）
	s := rep.Summary
	w.gap(8)
	w.text("摘要", 13, 20)
	w.text(fmt.Sprintf("共 %d 项：通过 %d · 失败 %d · 豁免 %d · 跳过 %d", s.Total, s.Passed, s.Failed, s.Exempt, s.Skipped), 10, 15)
	if len(s.FailedByLevel) > 0 {
		parts := []string{}
		for _, level := range []string{"P0", "P1", "P2", "P3"} {
			if s.FailedByLevel[level] > 0 {
				parts = append(parts, fmt.Sprintf("%s×%d", level, s.FailedByLevel[level]))
			}
		}
		w.text("未通过级别分布："+strings.Join(parts, "、"), 9, 13)
	}
	tierRows := [][]string{}
	for _, t := range s.Tiers {
		tierRows = append(tierRows, []string{t.Tier, fmt.Sprint(t.Total), fmt.Sprint(t.Passed), fmt.Sprint(t.Failed), fmt.Sprint(t.Exempt), fmt.Sprint(t.Skipped)})
	}
	w.table([]string{"档位", "用例数", "通过", "失败", "豁免", "跳过"}, tierRows, []float64{60, 85, 85, 85, 85, 85})

	if len(rep.Defects) > 0 {
		w.gap(8)
		w.text("缺陷清单", 13, 20)
		rows := [][]string{}
		for _, d := range rep.Defects {
			rows = append(rows, []string{d.Level, d.CaseID, d.Expect, d.Actual, d.Attribution})
		}
		w.table([]string{"级别", "用例", "期望", "实测", "归因"}, rows, []float64{36, 60, 140, 190, 89})
	}

	// 用例详情
	w.gap(8)
	w.text("用例详情", 13, 20)
	currentTier := ""
	for _, c := range rep.Cases {
		if c.Tier != currentTier {
			currentTier = c.Tier
			w.gap(6)
			w.text(currentTier+" 档", 12, 18)
		}
		w.renderCase(c, rep.Masked)
	}
	return w.pdf.GetBytesPdf(), w.err
}

// kvGrid 两列键值网格（环境信息）。
func (w *pdfWriter) kvGrid(pairs [][2]string) {
	for i := 0; i+1 < len(pairs); i += 2 {
		w.ensure(14)
		w.pdf.SetX(pdfMargin)
		_ = w.pdf.SetFont("wqy", "", 9)
		w.pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
		_ = w.pdf.Text(pairs[i][0] + "：")
		w.pdf.SetX(pdfMargin + 40)
		w.pdf.SetTextColor(colorTitle[0], colorTitle[1], colorTitle[2])
		_ = w.pdf.Text(clip(pairs[i][1], 60))
		w.pdf.SetX(pdfMargin + 265)
		w.pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
		_ = w.pdf.Text(pairs[i+1][0] + "：")
		w.pdf.SetX(pdfMargin + 305)
		w.pdf.SetTextColor(colorTitle[0], colorTitle[1], colorTitle[2])
		_ = w.pdf.Text(clip(pairs[i+1][1], 55))
		w.y += 14
	}
	if len(pairs)%2 == 1 {
		last := pairs[len(pairs)-1]
		w.text(last[0]+"："+last[1], 9, 14)
	}
}

// table 渲染带表头底色与边框的表格（单元格单行，超长截断）。
func (w *pdfWriter) table(headers []string, rows [][]string, widths []float64) {
	if w.err != nil {
		return
	}
	const rowH = 16.0
	x0 := pdfMargin
	w.ensure(rowH * float64(len(rows)+1))
	// 表头
	w.pdf.SetFillColor(colorFill[0], colorFill[1], colorFill[2])
	w.pdf.Rectangle(x0, w.y, x0+sum(widths), w.y+rowH, "F", 0, 0)
	w.cellRow(headers, widths, rowH, colorTitle)
	w.y += rowH
	// 数据行
	for _, row := range rows {
		w.cellRow(row, widths, rowH, colorTitle)
		w.y += rowH
		w.hline(w.y)
	}
	w.y += 4
}

func (w *pdfWriter) cellRow(cells []string, widths []float64, rowH float64, fg [3]uint8) {
	x := pdfMargin
	_ = w.pdf.SetFont("wqy", "", 8.5)
	for i, cell := range cells {
		width := widths[i]
		if i < len(widths)-1 && x+width > pdfWidth-pdfMargin {
			width = pdfWidth - pdfMargin - x
		}
		w.pdf.SetFillColor(255, 255, 255)
		w.pdf.SetStrokeColor(colorBorder[0], colorBorder[1], colorBorder[2])
		w.pdf.Rectangle(x, w.y, x+width, w.y+rowH, "D", 0, 0)
		w.pdf.SetTextColor(fg[0], fg[1], fg[2])
		w.pdf.SetXY(x+4, w.y+4)
		_ = w.pdf.Text(clipUnits(cell, (width-8)/8.5))
		x += width
	}
}

func (w *pdfWriter) hline(y float64) {
	w.pdf.SetStrokeColor(colorBorder[0], colorBorder[1], colorBorder[2])
	w.pdf.Line(pdfMargin, y, pdfWidth-pdfMargin, y)
}

func (w *pdfWriter) rule() {
	w.hline(w.y)
	w.y += 4
}

func sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

// renderCase 渲染单用例卡片：标题条（状态着色）+ 说明/判据/实测 + 指标表 + 缺陷表 + 证据盒。
func (w *pdfWriter) renderCase(c caseRow, masked bool) {
	if w.err != nil {
		return
	}
	const barH = 18.0
	w.ensure(barH + 60)
	// 标题条
	w.pdf.SetFillColor(colorFill[0], colorFill[1], colorFill[2])
	w.pdf.Rectangle(pdfMargin, w.y, pdfWidth-pdfMargin, w.y+barH, "F", 0, 0)
	w.pdf.SetXY(pdfMargin+4, w.y+4)
	_ = w.pdf.SetFont("wqy", "", 10)
	w.pdf.SetTextColor(colorTitle[0], colorTitle[1], colorTitle[2])
	_ = w.pdf.Text(fmt.Sprintf("%s-%s %s", c.Tier, c.CaseID, c.Name))
	statusColor := colorPass
	switch c.Status {
	case suite.StatusFailed:
		statusColor = colorFail
	case suite.StatusExempt, suite.StatusSkipped:
		statusColor = colorWarn
	}
	statusLabel := map[string]string{"passed": "通过", "failed": "失败", "exempt": "豁免", "skipped": "跳过", "pending": "待执行", "running": "运行中"}[c.Status]
	w.pdf.SetXY(pdfWidth-pdfMargin-60, w.y+4)
	w.pdf.SetTextColor(statusColor[0], statusColor[1], statusColor[2])
	_ = w.pdf.Text(statusLabel)
	w.y += barH + 2

	meta := c.LevelNote
	if c.HTTPStatus != 0 {
		meta += fmt.Sprintf(" ｜ HTTP %d", c.HTTPStatus)
	}
	if c.DurationMs > 0 {
		meta += fmt.Sprintf(" ｜ %dms", c.DurationMs)
	}
	w.textColored(meta, 8.5, 12, colorMuted)
	if c.Doc.Purpose != "" {
		w.text("为什么测："+c.Doc.Purpose, 9, 13)
	}
	if c.Doc.Method != "" {
		w.text("怎么测："+c.Doc.Method, 9, 13)
	}
	if c.Doc.Expect != "" {
		ref := c.Doc.Ref
		if ref != "" {
			ref = "（" + ref + "）"
		}
		w.text("预期："+c.Doc.Expect+ref, 9, 13)
	}
	if c.Summary != "" {
		w.textColored("实测："+c.Summary, 9, 13, statusColor)
	}

	// 指标表
	var metrics map[string]any
	if json.Unmarshal(c.Metrics, &metrics) == nil && len(metrics) > 0 {
		rows := [][]string{}
		for _, key := range sortedKeys(metrics) {
			rows = append(rows, []string{key, metricText(metrics[key])})
		}
		w.table([]string{"指标", "值"}, rows, []float64{230, 285})
	}
	// 缺陷表
	var defects []suite.Defect
	if json.Unmarshal(c.Defects, &defects) == nil && len(defects) > 0 {
		rows := [][]string{}
		for _, d := range defects {
			rows = append(rows, []string{d.Level, d.Expect, d.Actual, d.Attribution})
		}
		w.table([]string{"级别", "期望", "实测", "归因"}, rows, []float64{36, 150, 240, 89})
	}
	// 证据盒（精简/脱敏版无）
	if !masked {
		var evidence []map[string]any
		if json.Unmarshal(c.Evidence, &evidence) == nil {
			for _, item := range evidence {
				label, _ := item["label"].(string)
				curl, _ := item["curl"].(string)
				w.textColored("请求（"+label+"，curl 已脱敏）", 8, 11, colorMuted)
				w.box(curl, 7.5)
				if result, ok := item["result"].(map[string]any); ok {
					raw, _ := result["raw"].(string)
					if raw != "" {
						w.textColored(fmt.Sprintf("响应（HTTP %v，原文%s）", result["http_status"], truncateNote(raw, 1200)), 8, 11, colorMuted)
						w.box(truncateRunes(raw, 1200), 7.5)
					}
				}
			}
		}
	}
	w.y += 6
}

// box 灰底圆角文本盒（请求/响应原文）。
func (w *pdfWriter) box(text string, size float64) {
	if w.err != nil {
		return
	}
	lines := w.wrapUnits(text, (pdfWidth-2*pdfMargin-12)/size)
	for _, line := range lines {
		w.ensure(11)
		w.pdf.SetFillColor(colorFill[0], colorFill[1], colorFill[2])
		w.pdf.Rectangle(pdfMargin, w.y-1, pdfWidth-pdfMargin, w.y+10, "F", 0, 0)
		w.pdf.SetXY(pdfMargin+6, w.y+1)
		_ = w.pdf.SetFont("wqy", "", size)
		w.pdf.SetTextColor(colorTitle[0], colorTitle[1], colorTitle[2])
		_ = w.pdf.Text(line)
		w.y += 10
	}
	w.y += 2
}

// textColored 彩色段落文本。
func (w *pdfWriter) textColored(s string, size, leading float64, fg [3]uint8) {
	if w.err != nil {
		return
	}
	w.pdf.SetTextColor(fg[0], fg[1], fg[2])
	w.text(s, size, leading)
	w.pdf.SetTextColor(colorTitle[0], colorTitle[1], colorTitle[2])
}

func orDash(s string) string {
	if s == "" {
		return "（未命名渠道）"
	}
	return s
}

// newPage 开新页：页眉标题 + 页脚水印。
func (w *pdfWriter) newPage() {
	w.pdf.AddPage()
	w.y = pdfMargin
	_ = w.pdf.SetFont("wqy", "", 8)
	w.pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
	w.pdf.SetXY(pdfMargin, 20)
	_ = w.pdf.Text("NiceTools 准入测试报告")
	w.pdf.SetStrokeColor(colorBorder[0], colorBorder[1], colorBorder[2])
	w.pdf.Line(pdfMargin, 30, pdfWidth-pdfMargin, 30)
	w.pdf.SetXY(pdfMargin, pdfHeight-28)
	_ = w.pdf.Text(fmt.Sprintf("generated by %s · %s", Watermark, Watermark))
	w.pdf.SetTextColor(colorTitle[0], colorTitle[1], colorTitle[2])
}

func (w *pdfWriter) gap(h float64) { w.y += h }

func (w *pdfWriter) ensure(h float64) {
	if w.y+h > pdfHeight-pdfMargin-20 {
		w.newPage()
	}
}

// text 按可用宽度折行写一段文字；超页自动翻页。
// 宽度用估算（CJK≈1em、ASCII≈0.5em），避免逐字 MeasureTextWidth 的 O(n²) 开销。
func (w *pdfWriter) text(s string, size, leading float64) {
	if w.err != nil {
		return
	}
	if err := w.pdf.SetFont("wqy", "", size); err != nil {
		w.err = err
		return
	}
	for _, line := range w.wrapUnits(s, (pdfWidth-2*pdfMargin)/size) {
		w.ensure(leading)
		w.pdf.SetXY(pdfMargin, w.y)
		if err := w.pdf.Text(line); err != nil {
			w.err = err
			return
		}
		w.y += leading
	}
}

// wrapUnits 按估算宽度折行（CJK=1，其他=0.55）。
func (w *pdfWriter) wrapUnits(s string, usable float64) []string {
	lines := []string{}
	units := 0.0
	line := ""
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, line)
			line, units = "", 0
			continue
		}
		w_ := 0.55
		if r > 0x2E7F {
			w_ = 1.0
		}
		if units+w_ > usable && line != "" {
			lines = append(lines, line)
			line, units = string(r), w_
		} else {
			line += string(r)
			units += w_
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// clip 按字符数截断（kv 网格用）。
func clip(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// clipUnits 按估算宽度截断（表格单元格用）。
func clipUnits(s string, usable float64) string {
	units := 0.0
	for i, r := range s {
		w_ := 0.55
		if r > 0x2E7F {
			w_ = 1.0
		}
		if units+w_ > usable {
			return s[:i] + "…"
		}
		units += w_
	}
	return s
}
