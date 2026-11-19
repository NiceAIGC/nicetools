package llmadmit

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/signintech/gopdf"
)

// 文泉驿微米黑（提取自 wqy-microhei.ttc，授权见 fonts/LICENSE.txt），用于 CJK 渲染。
//
//go:embed fonts/wqy-microhei-sc.ttf
var pdfFont []byte

const (
	pdfMargin        = 40.0
	pdfWidth         = 595.28 // A4 宽（pt）
	pdfHeight        = 841.89 // A4 高（pt）
	pdfHeaderRuleY   = 30.0
	pdfContentTop    = 52.0
	pdfFooterY       = pdfHeight - 34
	pdfContentBottom = pdfFooterY - 12
)

// 配色取自常用的 Tailwind 色板，保证屏幕阅读与彩色打印都清晰。
var (
	colorInk      = [3]uint8{0x0f, 0x17, 0x2a}
	colorBody     = [3]uint8{0x33, 0x41, 0x55}
	colorMuted    = [3]uint8{0x64, 0x74, 0x8b}
	colorFaint    = [3]uint8{0x94, 0xa3, 0xb8}
	colorBorder   = [3]uint8{0xe2, 0xe8, 0xf0}
	colorPanel    = [3]uint8{0xf8, 0xfa, 0xfc}
	colorZebra    = [3]uint8{0xfb, 0xfc, 0xfe}
	colorBrand    = [3]uint8{0x0f, 0x9d, 0x70}
	colorBlue     = [3]uint8{0x25, 0x63, 0xeb}
	colorViolet   = [3]uint8{0x7c, 0x3a, 0xed}
	colorTeal     = [3]uint8{0x0d, 0x94, 0x88}
	colorAmber    = [3]uint8{0xd9, 0x77, 0x06}
	colorRed      = [3]uint8{0xdc, 0x26, 0x26}
	colorGreen    = [3]uint8{0x16, 0xa3, 0x4a}
	colorGreenBg  = [3]uint8{0xec, 0xfd, 0xf5}
	colorRedBg    = [3]uint8{0xfe, 0xf2, 0xf2}
	colorAmberBg  = [3]uint8{0xff, 0xfb, 0xeb}
	colorBlueBg   = [3]uint8{0xef, 0xf6, 0xff}
	colorVioletBg = [3]uint8{0xf5, 0xf3, 0xff}
	colorTrack    = [3]uint8{0xee, 0xf2, 0xf7}
)

type pdfWriter struct {
	pdf *gopdf.GoPdf
	y   float64
	// page 从 1 开始，页眉页脚据此渲染页码。
	page int
	err  error
	// widths 是 size=100 基准下的逐字宽度缓存：折行/截断全部用字体精确度量。
	widths map[rune]float64
	// header 是页眉右侧的报告标识；footerNote 是页脚中间的脱敏/精简标记。
	header     string
	footerNote string
	// watermark 控制本页是否绘制斜向水印。
	watermark bool
	// curFontSize 缓存当前字号；fontReady 后改用 SetFontSize，避免重复扫描字体对象。
	curFontSize float64
	fontReady   bool
	// curTextColor 缓存当前文字色，减少重复颜色指令。
	curTextColor [3]uint8
	hasTextColor bool
}

// runeWidth 返回字符在 size=100 下的字体精确宽度（懒缓存）。
func (w *pdfWriter) runeWidth(r rune) float64 {
	if rw, ok := w.widths[r]; ok {
		return rw
	}
	if w.err != nil {
		return 100
	}
	_ = w.setFont(100)
	width, err := w.pdf.MeasureTextWidth(string(r))
	if err != nil || width <= 0 {
		width = 100
	}
	w.widths[r] = width
	return width
}

// textWidth 是字符串在指定字号下的精确宽度。
func (w *pdfWriter) textWidth(s string, size float64) float64 {
	total := 0.0
	for _, r := range s {
		total += w.runeWidth(r)
	}
	return total * size / 100
}

// setFont 只在字号变化时真正调用 gopdf.SetFont（该调用会扫描字体对象）。
func (w *pdfWriter) setFont(size float64) error {
	if w.err != nil {
		return w.err
	}
	if w.curFontSize == size {
		return nil
	}
	if !w.fontReady {
		if err := w.pdf.SetFont("wqy", "", size); err != nil {
			w.err = err
			return err
		}
		w.fontReady = true
	} else if err := w.pdf.SetFontSize(size); err != nil {
		w.err = err
		return err
	}
	w.curFontSize = size
	return nil
}

// inFont 判断字符是否在内嵌字体子集内（与 fonts/ 子集化范围一致）；
// 不在的字符统一替换为 ?，杜绝缺字豆腐块（emoji/罕见符号）。
func inFont(r rune) bool {
	switch {
	case r >= 0x20 && r <= 0x7E, r >= 0xA0 && r <= 0xFF,
		r >= 0x2000 && r <= 0x206F, r >= 0x20A0 && r <= 0x20BF,
		r >= 0x2100 && r <= 0x214F, r >= 0x2190 && r <= 0x22FF,
		r >= 0x2300 && r <= 0x23FF, r >= 0x2460 && r <= 0x26FF,
		r >= 0x3000 && r <= 0x303F, r >= 0xFF00 && r <= 0xFFEF,
		r >= 0x4E00 && r <= 0x9FA5:
		return true
	}
	return false
}

// wrapUnits 按精确宽度折行（usable 以 size 字号计）。
// 断行规则：新行不得以收尾标点开头（。，、；：！？）》」』）— 标点后移不产生孤行。
func (w *pdfWriter) wrapUnits(s string, usable float64) []string {
	lines := []string{}
	line := ""
	lineWidth := 0.0
	limit := usable * 100
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, line)
			line = ""
			lineWidth = 0
			continue
		}
		if !inFont(r) {
			r = '?'
		}
		rw := w.runeWidth(r)
		if lineWidth+rw > limit && line != "" {
			if strings.ContainsRune("。，、；：！？）》」』）％‰…", r) {
				// 标点跟上一行走，宁可轻微越界也不孤行。
				line += string(r)
				lineWidth += rw
				continue
			}
			lines = append(lines, line)
			line = string(r)
			lineWidth = rw
			continue
		}
		line += string(r)
		lineWidth += rw
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// clipUnits 按精确宽度截断并加省略号。
func (w *pdfWriter) clipUnits(s string, usable float64) string {
	line := ""
	lineWidth := 0.0
	limit := usable * 100
	ellipsisWidth := w.runeWidth('…')
	for _, r := range s {
		if !inFont(r) {
			r = '?'
		}
		rw := w.runeWidth(r)
		if lineWidth+rw+ellipsisWidth > limit {
			return line + "…"
		}
		line += string(r)
		lineWidth += rw
	}
	return s
}

func (w *pdfWriter) gap(h float64) { w.y += h }

func (w *pdfWriter) ensure(h float64) {
	if w.y+h > pdfContentBottom {
		w.newPage()
	}
}

// newPage 开新页并绘制统一的页眉、页脚；页码在绘制时即已知。
func (w *pdfWriter) newPage() {
	if w.page > 0 {
		w.drawWatermark()
	}
	w.page++
	w.pdf.AddPage()
	w.y = pdfContentTop
	if w.page > 1 {
		w.drawText("niceaigc.cn", pdfMargin, 14, 8, colorBrand)
		w.drawText("大模型准入基准", pdfMargin+50, 14.5, 6.8, colorMuted)
	}
	if w.header != "" {
		x := pdfWidth - pdfMargin - w.textWidth(w.header, 7)
		w.drawText(w.header, x, 15, 7, colorFaint)
	}
	w.pdf.SetStrokeColor(colorBorder[0], colorBorder[1], colorBorder[2])
	w.pdf.SetLineWidth(0.6)
	w.pdf.SetLineType("solid")
	w.pdf.Line(pdfMargin, pdfHeaderRuleY, pdfWidth-pdfMargin, pdfHeaderRuleY)

	w.pdf.Line(pdfMargin, pdfFooterY, pdfWidth-pdfMargin, pdfFooterY)
	w.drawText(Watermark+" · 大模型准入测试系统", pdfMargin, pdfFooterY+6, 7, colorMuted)
	if w.footerNote != "" {
		x := (pdfWidth - w.textWidth(w.footerNote, 7)) / 2
		w.drawText(w.footerNote, x, pdfFooterY+6, 7, colorAmber)
	}
	pageLabel := fmt.Sprintf("第 %d 页", w.page)
	w.drawText(pageLabel, pdfWidth-pdfMargin-w.textWidth(pageLabel, 7), pdfFooterY+6, 7, colorMuted)
}

// drawWatermark 在当前页最上层覆盖低透明度斜向水印；密度高但颜色很浅。
// gopdf 的 Text 不支持透明度，必须用 CellWithOption 携带 ExtGState。
func (w *pdfWriter) drawWatermark() {
	if !w.watermark || w.page == 0 || w.err != nil {
		return
	}
	const (
		text  = "niceaigc.cn"
		size  = 13.0
		cols  = 6
		rows  = 8
		angle = -30.0
	)
	tr := gopdf.Transparency{Alpha: 0.06, BlendModeType: gopdf.NormalBlendMode}
	w.pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
	_ = w.setFont(size)
	stepX := pdfWidth / float64(cols)
	stepY := pdfHeight / float64(rows)
	textW := w.textWidth(text, size)
	// textWidth 会临时切到 100 号量字，CellWithOption 前必须切回来，否则水印会被画成 100pt。
	_ = w.setFont(size)
	for row := range rows {
		for col := range cols {
			cx := stepX*(float64(col)+0.5) + float64(row%2)*stepX*0.22
			cy := stepY * (float64(row) + 0.5)
			w.pdf.Rotate(angle, cx, cy)
			w.pdf.SetXY(cx-textW/2, cy-size/2)
			_ = w.pdf.CellWithOption(&gopdf.Rect{W: textW + 2, H: size + 2}, text, gopdf.CellOption{Transparency: &tr})
			w.pdf.RotateReset()
		}
	}
}

// finishPage 在最后一页落水印。
func (w *pdfWriter) finishPage() {
	w.drawWatermark()
}

// fillRect 画纯色矩形/圆角块。
func (w *pdfWriter) fillRect(x0, y0, x1, y1 float64, fill [3]uint8, radius float64) {
	if w.err != nil || x1 <= x0 || y1 <= y0 {
		return
	}
	w.pdf.SetFillColor(fill[0], fill[1], fill[2])
	_ = w.pdf.Rectangle(x0, y0, x1, y1, "F", radius, 8)
}

// panelRect 画带描边的圆角面板。
func (w *pdfWriter) panelRect(x0, y0, x1, y1 float64, fill, stroke [3]uint8, radius, lineWidth float64) {
	if w.err != nil || x1 <= x0 || y1 <= y0 {
		return
	}
	w.pdf.SetFillColor(fill[0], fill[1], fill[2])
	w.pdf.SetStrokeColor(stroke[0], stroke[1], stroke[2])
	w.pdf.SetLineWidth(lineWidth)
	_ = w.pdf.Rectangle(x0, y0, x1, y1, "FD", radius, 8)
}

func (w *pdfWriter) line(x0, y0, x1, y1 float64, color [3]uint8, width float64) {
	if w.err != nil {
		return
	}
	w.pdf.SetStrokeColor(color[0], color[1], color[2])
	w.pdf.SetLineWidth(width)
	w.pdf.Line(x0, y0, x1, y1)
}

func (w *pdfWriter) dashedLine(x0, y0, x1, y1 float64, color [3]uint8, width float64) {
	if w.err != nil {
		return
	}
	w.pdf.SetLineType("dashed")
	w.line(x0, y0, x1, y1, color, width)
	w.pdf.SetLineType("solid")
}

func (w *pdfWriter) hline(y float64) {
	w.line(pdfMargin, y, pdfWidth-pdfMargin, y, colorBorder, 0.6)
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

// tableFragmentLines 计算一页还能放多少行表格内容。
func tableFragmentLines(remaining, available int) int {
	if remaining < available {
		return remaining
	}
	return available
}

// fitWidths 把列宽等比收缩到可用宽度内。
func fitWidths(widths []float64) []float64 {
	usable := pdfWidth - 2*pdfMargin
	total := sum(widths)
	if total <= usable {
		return widths
	}
	out := make([]float64, len(widths))
	for i, v := range widths {
		out[i] = v * usable / total
	}
	return out
}

func (w *pdfWriter) text(s string, size, leading float64) {
	w.textColor(s, size, leading, colorInk)
}

// textColor 输出通栏段落（自动分页、按精确宽度折行）。
func (w *pdfWriter) textColor(s string, size, leading float64, color [3]uint8) {
	if w.err != nil {
		return
	}
	lines := w.wrapUnits(s, (pdfWidth-2*pdfMargin)/size)
	for _, line := range lines {
		w.ensure(leading)
		w.drawText(line, pdfMargin, w.y, size, color)
		w.y += leading
	}
}

// textColored 是 textColor 的语义别名。
func (w *pdfWriter) textColored(s string, size, leading float64, fg [3]uint8) {
	w.textColor(s, size, leading, fg)
}

// box 画灰色证据块：左侧强调线 + 浅底 + 自动分页。
func (w *pdfWriter) box(text string, size float64) {
	if w.err != nil || text == "" {
		return
	}
	lines := w.wrapUnits(text, (pdfWidth-2*pdfMargin-16)/size)
	lineH := size + 2.6
	for start := 0; start < len(lines); {
		available := int((pdfContentBottom - w.y - 8) / lineH)
		if available < 1 {
			w.newPage()
			continue
		}
		count := tableFragmentLines(len(lines)-start, available)
		height := float64(count)*lineH + 8
		w.panelRect(pdfMargin, w.y, pdfWidth-pdfMargin, w.y+height, colorPanel, colorBorder, 4, 0.5)
		w.fillRect(pdfMargin, w.y+1, pdfMargin+3, w.y+height-1, colorFaint, 1)
		for i := 0; i < count; i++ {
			w.drawText(lines[start+i], pdfMargin+8, w.y+4+float64(i)*lineH, size, colorBody)
		}
		w.y += height + 3
		start += count
		if start < len(lines) {
			w.newPage()
		}
	}
}

func orDash(s string) string {
	if s == "" {
		return "（未命名渠道）"
	}
	return s
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func pdfTextBaseline(top, size float64) float64 {
	return top + size
}

func (w *pdfWriter) drawText(text string, x, top, size float64, color [3]uint8) error {
	if w.err != nil {
		return w.err
	}
	if err := w.setFont(size); err != nil {
		return err
	}
	if !w.hasTextColor || w.curTextColor != color {
		w.pdf.SetTextColor(color[0], color[1], color[2])
		w.curTextColor = color
		w.hasTextColor = true
	}
	w.pdf.SetXY(x, pdfTextBaseline(top, size))
	if err := w.pdf.Text(text); err != nil {
		w.err = err
		return err
	}
	return nil
}
