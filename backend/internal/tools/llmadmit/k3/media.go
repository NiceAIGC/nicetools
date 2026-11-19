package k3

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
)

// 红/蓝纯色对照视频（M-30 防瞎猜双对照），构建期嵌入。
//
//go:embed media/red.mp4
var redMP4 []byte

//go:embed media/blue.mp4
var blueMP4 []byte

// RedMP4/BlueMP4 供模块的媒体托管端点使用（任务令牌保护）。
func RedMP4() []byte  { return redMP4 }
func BlueMP4() []byte { return blueMP4 }

func videoDataURI(data []byte) string {
	return "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(data)
}

// PadMP4 在 MP4 末尾追加合法 free box 到指定字节数；解码器会忽略 free box。
func PadMP4(data []byte, size int) []byte {
	if size <= len(data) {
		return data
	}
	freeLen := size - len(data)
	if freeLen < 8 {
		freeLen = 8
	}
	box := make([]byte, 8)
	binary.BigEndian.PutUint32(box, uint32(freeLen))
	copy(box[4:], "free")
	out := make([]byte, 0, len(data)+freeLen)
	out = append(out, data...)
	out = append(out, box...)
	out = append(out, make([]byte, freeLen-8)...)
	return out
}

// colorPNG 生成四色象限 PNG（F-10：左上红/右上绿/左下蓝/右下黄）。
func colorPNG() string {
	const size = 240
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	quadrants := [2][2]color.RGBA{
		{{R: 220, G: 30, B: 30, A: 255}, {R: 30, G: 170, B: 60, A: 255}},
		{{R: 30, G: 80, B: 210, A: 255}, {R: 235, G: 200, B: 30, A: 255}},
	}
	for y := range size {
		for x := range size {
			img.Set(x, y, quadrants[y/(size/2)][x/(size/2)])
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// sixColorPNG 生成横向六色条 PNG（M-28：红橙黄绿蓝紫顺序）。
func sixColorPNG() string {
	const w, h = 360, 120
	colors := []color.RGBA{
		{R: 220, G: 30, B: 30, A: 255}, {R: 240, G: 140, B: 20, A: 255},
		{R: 235, G: 210, B: 30, A: 255}, {R: 40, G: 170, B: 60, A: 255},
		{R: 30, G: 90, B: 210, A: 255}, {R: 130, G: 60, B: 180, A: 255},
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, colors[x/(w/6)])
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// imageMsg 构造 图片 + 文本 的多模态 user 消息（与官方/网关实测顺序一致）。
func imageMsg(text, dataURI string) []any {
	return []any{map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
			map[string]any{"type": "text", "text": text},
		},
	}}
}

// videoMsg 构造 视频 + 文本 的多模态 user 消息（与官方/网关实测顺序一致）。
func videoMsg(text, dataURI string) []any {
	return []any{map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{"type": "video_url", "video_url": map[string]any{"url": dataURI}},
			map[string]any{"type": "text", "text": text},
		},
	}}
}
