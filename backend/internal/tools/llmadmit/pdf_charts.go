package llmadmit

import (
	"fmt"
	"math"
)

// 图表：全部用矢量矩形/折线绘制，不依赖图片，打印与缩放都清晰。

// barChartCard 画一张指标对比卡：标题 + 门槛 + 绿色达标/红色未达标的柱状图。
func (w *pdfWriter) barChartCard(x, y, width, height float64, title, note string, labels []string, values, targets []float64, higher bool, series [3]uint8, unit string) {
	if w.err != nil || len(values) == 0 {
		return
	}
	w.panelRect(x, y, x+width, y+height, [3]uint8{0xff, 0xff, 0xff}, colorBorder, 6, 0.6)
	w.fillRect(x+1, y+1, x+width-1, y+3, series, 1.5)
	targetLabel := ""
	if len(targets) > 0 && targets[0] > 0 {
		targetLabel = "门槛 " + thresholdText(targets[0], unit)
	}
	titleWidth := width - 20
	if targetLabel != "" {
		titleWidth -= w.textWidth(targetLabel, 6.2) + 8
	}
	w.drawText(w.clipUnits(title, titleWidth/9), x+10, y+8, 9, colorInk)
	if targetLabel != "" {
		w.drawText(targetLabel, x+width-10-w.textWidth(targetLabel, 6.2), y+8, 6.2, colorFaint)
	}
	if note != "" {
		w.drawText(w.clipUnits(note, (width-20)/6.8), x+10, y+21, 6.8, colorMuted)
	}
	plotX := x + 30
	plotW := width - 42
	plotTop := y + 38
	plotBottom := y + height - 22
	if plotBottom <= plotTop || plotW <= 0 {
		return
	}
	maxValue := 0.0
	for i, v := range values {
		if v > maxValue {
			maxValue = v
		}
		if i < len(targets) && targets[i] > maxValue {
			maxValue = targets[i]
		}
	}
	if maxValue <= 0 {
		maxValue = 1
	}
	maxValue *= 1.12
	// 三条水平网格线，标签靠左。
	for _, frac := range []float64{0, 0.5, 1} {
		gy := plotBottom - frac*(plotBottom-plotTop)
		w.line(plotX, gy, plotX+plotW, gy, colorTrack, 0.5)
		label := chartValueText(maxValue*frac, unit)
		w.drawText(label, plotX-3-w.textWidth(label, 6), gy-3, 6, colorFaint)
	}
	slot := plotW / float64(len(values))
	barW := math.Min(slot*0.56, 18)
	for i, v := range values {
		cx := plotX + slot*(float64(i)+0.5)
		h := 0.0
		if maxValue > 0 {
			h = v / maxValue * (plotBottom - plotTop)
		}
		target := 0.0
		if i < len(targets) {
			target = targets[i]
		}
		color := series
		hasTarget := target > 0
		if hasTarget {
			pass := v <= target
			if higher {
				pass = v >= target
			}
			color = colorRed
			if pass {
				color = colorGreen
			}
		}
		if h < 1 {
			h = 1
		}
		w.fillRect(cx-barW/2, plotBottom-h, cx+barW/2, plotBottom, color, 1.5)
		valueLabel := chartValueText(v, unit)
		w.drawText(valueLabel, cx-w.textWidth(valueLabel, 6.2)/2, plotBottom-h-8, 6.2, color)
		if i < len(labels) {
			label := w.clipUnits(labels[i], (slot-2)/6.5)
			w.drawText(label, cx-w.textWidth(label, 6.5)/2, plotBottom+5, 6.5, colorMuted)
		}
	}
	if len(targets) > 0 && targets[0] > 0 {
		ty := plotBottom - targets[0]/maxValue*(plotBottom-plotTop)
		w.dashedLine(plotX, ty, plotX+plotW, ty, colorFaint, 0.7)
	}
}

// linePlot 画样本趋势折线：浅网格 + 彩色折线 + 门槛虚线。
func (w *pdfWriter) linePlot(x, y, width, height float64, title, note string, values []float64, threshold float64, color [3]uint8, unit string) {
	if w.err != nil || len(values) == 0 {
		return
	}
	if title != "" {
		w.drawText(title, x, y, 8.5, colorInk)
	}
	if note != "" {
		w.drawText(note, x+width-w.textWidth(note, 6.5), y+1, 6.5, colorFaint)
	}
	plotX := x + 30
	plotW := width - 40
	plotTop := y + 13
	plotBottom := y + height - 12
	if plotBottom <= plotTop || plotW <= 0 {
		return
	}
	maxValue := 0.0
	for _, v := range values {
		if v > maxValue {
			maxValue = v
		}
	}
	if threshold > maxValue {
		maxValue = threshold
	}
	if maxValue <= 0 {
		maxValue = 1
	}
	maxValue *= 1.12
	for _, frac := range []float64{0, 0.5, 1} {
		gy := plotBottom - frac*(plotBottom-plotTop)
		w.line(plotX, gy, plotX+plotW, gy, colorTrack, 0.5)
		label := chartValueText(maxValue*frac, unit)
		w.drawText(label, plotX-3-w.textWidth(label, 6), gy-3, 6, colorFaint)
	}
	point := func(i int, v float64) (float64, float64) {
		px := plotX
		if len(values) > 1 {
			px = plotX + float64(i)/float64(len(values)-1)*plotW
		} else {
			px = plotX + plotW/2
		}
		py := plotBottom - v/maxValue*(plotBottom-plotTop)
		return px, py
	}
	w.pdf.SetStrokeColor(color[0], color[1], color[2])
	w.pdf.SetLineWidth(1.3)
	for i := range values {
		px, py := point(i, values[i])
		if i > 0 {
			prevX, prevY := point(i-1, values[i-1])
			w.pdf.Line(prevX, prevY, px, py)
		}
		w.fillRect(px-1.4, py-1.4, px+1.4, py+1.4, color, 1.4)
	}
	if threshold > 0 {
		ty := plotBottom - threshold/maxValue*(plotBottom-plotTop)
		w.dashedLine(plotX, ty, plotX+plotW, ty, colorAmber, 0.7)
	}
	// x 轴首尾标签，避免每个点都写字造成拥挤。
	w.drawText("样本 1", plotX, plotBottom+4, 6, colorFaint)
	end := fmt.Sprintf("样本 %d", len(values))
	w.drawText(end, plotX+plotW-w.textWidth(end, 6), plotBottom+4, 6, colorFaint)
}

// cacheBars 画缓存命中率横条：标签 + 轨道 + 数值，80% 处画门槛线。
func (w *pdfWriter) cacheBars(rows []cachePerformanceRow) {
	const labelW = 165.0
	barX := pdfMargin + labelW
	barW := pdfWidth - 2*pdfMargin - labelW - 55
	for _, row := range rows {
		w.ensure(18)
		color := colorAmber
		if row.rate >= 80 {
			color = colorGreen
		}
		if row.rate < 50 {
			color = colorRed
		}
		label := w.clipUnits(row.label, (labelW-8)/7.5)
		w.drawText(label, pdfMargin, w.y+3, 7.5, colorInk)
		w.fillRect(barX, w.y+3, barX+barW, w.y+12, colorTrack, 4.5)
		if row.rate > 0 {
			w.fillRect(barX, w.y+3, barX+barW*math.Min(row.rate, 100)/100, w.y+12, color, 4.5)
		}
		tx := barX + barW*0.8
		w.dashedLine(tx, w.y+1, tx, w.y+14, colorFaint, 0.6)
		w.drawText(fmt.Sprintf("%.1f%%", row.rate), barX+barW+8, w.y+3, 7.5, color)
		w.y += 18
	}
	w.drawText("虚线为 80% 参考线；绿色 ≥80%，橙色 50–79%，红色 <50%。", pdfMargin, w.y+2, 7, colorFaint)
	w.y += 16
}

// decayPlot 画缓存命中率随 TTL 衰减的折线。
func (w *pdfWriter) decayPlot(rows []cachePerformanceRow) {
	type point struct {
		sec  float64
		rate float64
	}
	points := make([]point, 0, len(rows))
	for _, row := range rows {
		if row.hasSeconds {
			points = append(points, point{row.seconds, row.rate})
		}
	}
	if len(points) == 0 {
		return
	}
	for i := 1; i < len(points); i++ {
		for j := i; j > 0 && points[j].sec < points[j-1].sec; j-- {
			points[j], points[j-1] = points[j-1], points[j]
		}
	}
	const height = 118.0
	w.ensure(height + 18)
	x := pdfMargin
	width := pdfWidth - 2*pdfMargin
	y := w.y
	w.panelRect(x, y, x+width, y+height, [3]uint8{0xff, 0xff, 0xff}, colorBorder, 6, 0.6)
	w.fillRect(x+1, y+1, x+width-1, y+3, colorTeal, 1.5)
	w.drawText("缓存命中率随 TTL 衰减", x+10, y+8, 9, colorInk)
	w.drawText("横轴为距离写入的秒数；虚线为 80% 参考线。", x+10, y+21, 6.8, colorMuted)
	plotX := x + 34
	plotW := width - 46
	plotTop := y + 38
	plotBottom := y + height - 20
	maxSec := points[len(points)-1].sec
	if maxSec <= 0 {
		maxSec = 1
	}
	for _, frac := range []float64{0, 0.5, 1} {
		gy := plotBottom - frac*(plotBottom-plotTop)
		w.line(plotX, gy, plotX+plotW, gy, colorTrack, 0.5)
		label := fmt.Sprintf("%.0f%%", 100*frac)
		w.drawText(label, plotX-3-w.textWidth(label, 6), gy-3, 6, colorFaint)
	}
	px := func(i int) float64 {
		if len(points) == 1 {
			return plotX + plotW/2
		}
		return plotX + points[i].sec/maxSec*plotW
	}
	py := func(rate float64) float64 { return plotBottom - rate/100*(plotBottom-plotTop) }
	w.pdf.SetStrokeColor(colorTeal[0], colorTeal[1], colorTeal[2])
	w.pdf.SetLineWidth(1.3)
	for i := range points {
		if i > 0 {
			w.pdf.Line(px(i-1), py(points[i-1].rate), px(i), py(points[i].rate))
		}
		w.fillRect(px(i)-1.6, py(points[i].rate)-1.6, px(i)+1.6, py(points[i].rate)+1.6, colorTeal, 1.6)
		w.drawText(fmt.Sprintf("%.1f%%", points[i].rate), px(i)-w.textWidth(fmt.Sprintf("%.1f%%", points[i].rate), 6)/2, py(points[i].rate)-9, 6, colorTeal)
	}
	ty := py(80)
	w.dashedLine(plotX, ty, plotX+plotW, ty, colorAmber, 0.7)
	w.drawText("80%", plotX+plotW-14, ty-8, 6, colorAmber)
	// TTL 轴标签：首、中、尾。
	w.drawText("0s", plotX, plotBottom+4, 6, colorFaint)
	mid := fmt.Sprintf("%.0fs", maxSec/2)
	w.drawText(mid, plotX+plotW/2-w.textWidth(mid, 6)/2, plotBottom+4, 6, colorFaint)
	end := fmt.Sprintf("%.0fs", maxSec)
	w.drawText(end, plotX+plotW-w.textWidth(end, 6), plotBottom+4, 6, colorFaint)
	w.y = y + height + 8
}

// chartValueText 把图表数值写成带单位的人类可读单位（秒/毫秒），不再用 9.6k。
func chartValueText(v float64, unit string) string {
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
		switch {
		case v >= 100:
			return fmt.Sprintf("%.0fms", v)
		case v >= 10:
			return fmt.Sprintf("%.0fms", v)
		default:
			return fmt.Sprintf("%.1fms", v)
		}
	case "%":
		return fmt.Sprintf("%.1f%%", v)
	}
	return shortNumber(v)
}

// chartLegend 说明柱色与门槛虚线，避免读者猜图例。
func (w *pdfWriter) chartLegend() {
	w.ensure(16)
	y := w.y
	w.dot(pdfMargin, y+2, 5, colorGreen)
	w.drawText("达标", pdfMargin+9, y, 7, colorMuted)
	x := pdfMargin + 42
	w.dot(x, y+2, 5, colorRed)
	w.drawText("未达标", x+9, y, 7, colorMuted)
	x += 62
	w.dashedLine(x, y+4.5, x+14, y+4.5, colorFaint, 0.8)
	w.drawText("门槛", x+19, y, 7, colorMuted)
	w.y += 13
}

// shortNumber 把大数字压缩成 1.2k / 24k 之类的短标签。
func shortNumber(v float64) string {
	if v == 0 {
		return "0"
	}
	switch {
	case v >= 10000:
		return fmt.Sprintf("%.0fk", v/1000)
	case v >= 1000:
		return fmt.Sprintf("%.1fk", v/1000)
	case v >= 100:
		return fmt.Sprintf("%.0f", v)
	case v >= 10:
		return fmt.Sprintf("%.1f", v)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}
