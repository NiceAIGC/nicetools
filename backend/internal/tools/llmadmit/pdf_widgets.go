package llmadmit

import "strings"

// 版式组件：表格、药丸标签、统计卡、分组标题、段落。全部使用精确字宽排版。

type tableCell struct {
	Text string
	// Color 为文字颜色；Bg 非空时整格填色。
	Color [3]uint8
	Bg    [3]uint8
	HasBg bool
	// Pill 为真时把 Text 画成彩色药丸（用于结论/等级）。
	Pill           bool
	PillFg, PillBg [3]uint8
	Align          int // 0 左对齐，1 居中，2 右对齐
	Size           float64
	Dot            [3]uint8
	HasDot         bool
}

type tableStyle struct {
	widths     []float64
	header     []tableCell
	rows       [][]tableCell
	bodySize   float64
	headerSize float64
	lineH      float64
	minRowH    float64
	padX       float64
	zebra      bool
	grid       bool
}

func textCell(s string) tableCell {
	return tableCell{Text: s, Color: colorBody}
}

func strongCell(s string) tableCell {
	return tableCell{Text: s, Color: colorInk}
}

func rightCell(s string) tableCell {
	return tableCell{Text: s, Color: colorBody, Align: 2}
}

func pillCell(text string, fg, bg [3]uint8) tableCell {
	return tableCell{Text: text, Pill: true, PillFg: fg, PillBg: bg, Align: 1}
}

func dotCell(text string, dot [3]uint8) tableCell {
	return tableCell{Text: text, Color: colorInk, Dot: dot, HasDot: true}
}

func (c tableCell) size(def float64) float64 {
	if c.Size > 0 {
		return c.Size
	}
	return def
}

func maxLines(cells [][]string) int {
	count := 1
	for _, cell := range cells {
		if len(cell) > count {
			count = len(cell)
		}
	}
	return count
}

// tableView 输出无竖线（可选浅网格）的现代表格：表头浅底、行间细线、结论用色块药丸。
// 超大单元格会在行内按页切片，表头在续页重复。
func (w *pdfWriter) tableView(style tableStyle) {
	if w.err != nil || len(style.rows) == 0 {
		return
	}
	widths := fitWidths(style.widths)
	if style.bodySize == 0 {
		style.bodySize = 8.5
	}
	if style.headerSize == 0 {
		style.headerSize = 7.6
	}
	if style.lineH == 0 {
		style.lineH = style.bodySize + 3
	}
	if style.minRowH == 0 {
		style.minRowH = 22
	}
	if style.padX == 0 {
		style.padX = 5
	}
	wrapRow := func(cells []tableCell, def float64) [][]string {
		lines := make([][]string, len(widths))
		for i := range widths {
			if i < len(cells) && cells[i].Text != "" {
				size := cells[i].size(def)
				usable := (widths[i] - 2*style.padX) / size
				lines[i] = w.wrapUnits(cells[i].Text, usable)
			}
			if len(lines[i]) == 0 {
				lines[i] = []string{""}
			}
		}
		return lines
	}
	headerLines := wrapRow(style.header, style.headerSize)
	headerH := float64(maxLines(headerLines))*style.lineH + 10
	firstRowH := style.minRowH
	if len(style.rows) > 0 {
		firstRowH = float64(maxLines(wrapRow(style.rows[0], style.bodySize)))*style.lineH + 10
	}
	// 超大首行不应把「表头 + 至少一行」的整块高度算进 ensure，否则每次续页都会多翻一页。
	if firstRowH > 80 {
		firstRowH = 80
	}
	drawHeader := func() {
		w.ensure(headerH + firstRowH)
		w.fillRect(pdfMargin, w.y, pdfWidth-pdfMargin, w.y+headerH, colorPanel, 3)
		x := pdfMargin
		for i, cell := range style.header {
			lines := headerLines[i]
			size := cell.size(style.headerSize)
			for j, line := range lines {
				lx := x + style.padX
				if cell.Align == 2 {
					lx = x + widths[i] - style.padX - w.textWidth(line, size)
				} else if cell.Align == 1 {
					lx = x + (widths[i]-w.textWidth(line, size))/2
				}
				w.drawText(line, lx, w.y+5+float64(j)*style.lineH, size, colorMuted)
			}
			x += widths[i]
		}
		w.y += headerH
		w.hline(w.y)
	}
	drawFragment := func(cells []tableCell, lines [][]string, start, count int, zebra bool) {
		height := float64(count)*style.lineH + 10
		x := pdfMargin
		for i := range widths {
			if i < len(cells) && cells[i].HasBg {
				w.fillRect(x, w.y, x+widths[i], w.y+height, cells[i].Bg, 0)
			} else if zebra {
				w.fillRect(x, w.y, x+widths[i], w.y+height, colorZebra, 0)
			}
			x += widths[i]
		}
		if style.grid {
			x = pdfMargin
			for i := 0; i < len(widths)-1; i++ {
				x += widths[i]
				w.line(x, w.y, x, w.y+height, colorTrack, 0.4)
			}
		}
		x = pdfMargin
		for i := range widths {
			if i >= len(cells) {
				x += widths[i]
				continue
			}
			cell := cells[i]
			size := cell.size(style.bodySize)
			if cell.Pill {
				pillH := size + 6
				if start == 0 {
					label := w.clipUnits(cell.Text, (widths[i]-2*style.padX)/size)
					px := x + style.padX
					pw := w.textWidth(label, size) + 10
					if cell.Align == 1 {
						px = x + (widths[i]-pw)/2
					} else if cell.Align == 2 {
						px = x + widths[i] - style.padX - pw
					}
					w.pill(px, w.y+(height-pillH)/2, label, cell.PillFg, cell.PillBg, size)
				}
			} else {
				textX := x + style.padX
				if cell.HasDot {
					w.dot(textX+2, w.y+(height-4)/2, 4, cell.Dot)
					textX += 10
				}
				for j := start; j < start+count && j < len(lines[i]); j++ {
					line := lines[i][j]
					lx := textX
					if cell.Align == 2 {
						lx = x + widths[i] - style.padX - w.textWidth(line, size)
					} else if cell.Align == 1 {
						lx = x + (widths[i]-w.textWidth(line, size))/2
					}
					w.drawText(line, lx, w.y+5+float64(j-start)*style.lineH, size, cell.Color)
				}
			}
			x += widths[i]
		}
		w.y += height
		w.hline(w.y)
	}
	drawHeader()
	for idx, row := range style.rows {
		lines := wrapRow(row, style.bodySize)
		total := maxLines(lines)
		start := 0
		for start < total {
			available := int((pdfContentBottom - w.y - 10) / style.lineH)
			if available < 1 {
				w.newPage()
				drawHeader()
				continue
			}
			count := tableFragmentLines(total-start, available)
			drawFragment(row, lines, start, count, style.zebra && idx%2 == 1)
			start += count
			if start < total {
				w.newPage()
				drawHeader()
			}
		}
	}
}

// pill 画圆角药丸，返回宽度。
func (w *pdfWriter) pill(x, top float64, text string, fg, bg [3]uint8, size float64) float64 {
	h := size + 6
	width := w.textWidth(text, size) + 10
	w.fillRect(x, top, x+width, top+h, bg, h/2)
	w.drawText(text, x+5, top+3, size, fg)
	return width
}

func (w *pdfWriter) dot(x, y, d float64, color [3]uint8) {
	w.fillRect(x, y, x+d, y+d, color, d/2)
}

func (w *pdfWriter) statusPill(text string, x, top float64) float64 {
	fg, bg := statusColors(text)
	return w.pill(x, top, text, fg, bg, 7.2)
}

// statusColors 返回状态/结论的文本色与浅底色。
func statusColors(label string) (fg, bg [3]uint8) {
	switch label {
	case "通过", "达标", "已达标", "全部通过":
		return colorGreen, colorGreenBg
	case "失败", "未通过", "未达标", "有失败项":
		return colorRed, colorRedBg
	case "能力跳过", "部分跳过", "需关注", "仅供参考":
		return colorAmber, colorAmberBg
	default:
		return colorMuted, colorPanel
	}
}

func statusLabel(status string) string {
	switch status {
	case "passed":
		return "通过"
	case "failed":
		return "失败"
	case "exempt":
		return "能力跳过"
	case "skipped":
		return "跳过"
	case "running":
		return "运行中"
	default:
		return "待执行"
	}
}

func perfStatusLabel(status string) string {
	if status == "passed" {
		return "达标"
	}
	if status == "failed" {
		return "未达标"
	}
	return statusLabel(status)
}

// levelBadgeText 是等级药的短文本（闸门/P0/P1…）。
func levelBadgeText(level string) string {
	switch normalizeLevel(level) {
	case "gate":
		return "闸门"
	case "P0", "P1", "P2", "P3":
		return normalizeLevel(level)
	}
	if level == "" {
		return "—"
	}
	return level
}

// levelPillStyle 给 P0/P1/P2/P3 与历史级别着色。
func levelPillStyle(level string) (fg, bg [3]uint8) {
	switch normalizeLevel(level) {
	case "P0":
		return colorRed, colorRedBg
	case "P1":
		return colorAmber, colorAmberBg
	case "P2":
		return colorBlue, colorBlueBg
	case "P3":
		return colorMuted, colorPanel
	case "gate":
		return colorViolet, colorVioletBg
	}
	return colorMuted, colorPanel
}

// sectionHeader 画带绿色竖条的章节标题与说明。
func (w *pdfWriter) sectionHeader(title, note string) {
	w.ensure(54)
	w.gap(10)
	w.fillRect(pdfMargin, w.y+2, pdfMargin+3.5, w.y+16, colorBrand, 1.5)
	w.drawText(title, pdfMargin+10, w.y, 13.5, colorInk)
	w.y += 19
	if note != "" {
		w.textColor(note, 8, 12, colorMuted)
	}
	w.gap(4)
}

// groupHeader 画分组标题：色块 + 名称 + 右侧统计。
func (w *pdfWriter) groupHeader(label, note string, color [3]uint8) {
	w.ensure(30)
	w.gap(6)
	w.fillRect(pdfMargin, w.y+3, pdfMargin+8, w.y+11, color, 2)
	w.drawText(label, pdfMargin+13, w.y-1, 11, colorInk)
	if note != "" {
		w.drawText(note, pdfMargin+13+w.textWidth(label, 11)+8, w.y+1, 7.5, colorMuted)
	}
	w.y += 18
	w.hline(w.y)
	w.gap(4)
}

// statCard 画诊断数字卡：顶部色条 + 标签 + 大数字 + 副标题。
func (w *pdfWriter) statCard(x, y, width, height float64, label, value, sub string, accent, bg [3]uint8) {
	w.panelRect(x, y, x+width, y+height, bg, colorBorder, 6, 0.6)
	w.fillRect(x+1, y+1, x+width-1, y+3, accent, 1.5)
	w.drawText(label, x+9, y+8, 7.5, colorMuted)
	w.drawText(value, x+9, y+20, 18, accent)
	w.drawText(sub, x+9, y+44, 6.8, colorMuted)
}

// labeledParagraph 输出「标签 + 多行正文」，正文行首与标签右侧对齐。
func (w *pdfWriter) labeledParagraph(label, text string, x, width, labelW, size, leading float64, bodyColor [3]uint8) {
	if text == "" || w.err != nil {
		return
	}
	lines := w.wrapUnits(text, (width-labelW)/size)
	for i, line := range lines {
		w.ensure(leading)
		if i == 0 {
			w.drawText(label, x, w.y, size, colorMuted)
		}
		w.drawText(line, x+labelW, w.y, size, bodyColor)
		w.y += leading
	}
}

// chip 画声明/标签小胶囊，返回宽度。
func (w *pdfWriter) chip(x, top float64, text string, fg, bg [3]uint8) float64 {
	return w.pill(x, top, text, fg, bg, 7.2)
}

// flowChips 从左到右排布胶囊，自动换行。
func (w *pdfWriter) flowChips(items []string, x, width float64) {
	if len(items) == 0 {
		return
	}
	cx, top := x, w.y
	for _, item := range items {
		fg, bg := colorTeal, colorGreenBg
		if strings.Contains(item, "未声明") {
			fg, bg = colorMuted, colorPanel
		}
		textW := w.textWidth(item, 7.2) + 10
		if cx+textW > x+width {
			cx = x
			top += 18
		}
		w.chip(cx, top, item, fg, bg)
		cx += textW + 5
	}
	w.y = top + 15
}
