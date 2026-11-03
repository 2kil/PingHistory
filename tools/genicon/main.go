// Command genicon 生成应用与托盘图标的 .ico 文件（32 位 BMP 格式，兼容 Shell_NotifyIcon）。
// 用法：go run ./tools/genicon assets/app.ico
package main

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"log"
	"math"
	"os"
	"path/filepath"
)

var sizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}

// 心跳折线的归一化坐标（相对图标边长），用来表达"延迟监测"的语义。
var pulse = [][2]float64{
	{0.19, 0.55}, {0.35, 0.55}, {0.44, 0.30}, {0.55, 0.77}, {0.63, 0.43}, {0.70, 0.55}, {0.82, 0.55},
}

func main() {
	out := "assets/app.ico"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	images := make([][]byte, 0, len(sizes))
	for _, s := range sizes {
		images = append(images, encodeDIB(render(s), s))
	}

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(len(sizes)))

	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		w := byte(s)
		if s >= 256 {
			w = 0
		}
		buf.WriteByte(w) // width
		buf.WriteByte(w) // height
		buf.WriteByte(0) // color count
		buf.WriteByte(0) // reserved
		_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
		_ = binary.Write(&buf, binary.LittleEndian, uint16(32))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(images[i])))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(images[i])
	}
	for _, img := range images {
		buf.Write(img)
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		log.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		log.Fatalf("写入图标失败: %v", err)
	}
	log.Printf("已生成 %s（%d 字节，尺寸 %v）", out, buf.Len(), sizes)
}

// render 使用 4 倍超采样绘制图标，返回自上而下的 RGBA 像素。
func render(size int) []color.NRGBA {
	const ss = 4
	w := size * ss
	hi := make([]color.NRGBA, w*w)

	cx, cy := float64(w)/2, float64(w)/2
	radius := float64(w)/2 - float64(w)*0.035
	stroke := float64(w) * 0.075

	for y := 0; y < w; y++ {
		for x := 0; x < w; x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			if math.Hypot(px-cx, py-cy) > radius {
				continue
			}
			t := py / float64(w)
			bg := lerp(color.NRGBA{34, 211, 238, 255}, color.NRGBA{47, 107, 255, 255}, t)

			d := distanceToPolyline(px, py, float64(w))
			if d < stroke/2 {
				bg = color.NRGBA{255, 255, 255, 255}
			}
			hi[y*w+x] = bg
		}
	}

	// 盒式降采样得到抗锯齿效果。
	out := make([]color.NRGBA, size*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					c := hi[(y*ss+dy)*w+(x*ss+dx)]
					af := float64(c.A) / 255
					r += float64(c.R) * af
					g += float64(c.G) * af
					b += float64(c.B) * af
					a += af
				}
			}
			n := float64(ss * ss)
			px := color.NRGBA{}
			if a > 0 {
				px = color.NRGBA{
					R: uint8(clamp(r / a)),
					G: uint8(clamp(g / a)),
					B: uint8(clamp(b / a)),
					A: uint8(clamp(a / n * 255)),
				}
			}
			out[y*size+x] = px
		}
	}
	return out
}

func distanceToPolyline(x, y, scale float64) float64 {
	best := math.Inf(1)
	for i := 0; i < len(pulse)-1; i++ {
		x1, y1 := pulse[i][0]*scale, pulse[i][1]*scale
		x2, y2 := pulse[i+1][0]*scale, pulse[i+1][1]*scale
		d := distanceToSegment(x, y, x1, y1, x2, y2)
		if d < best {
			best = d
		}
	}
	return best
}

func distanceToSegment(px, py, x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	lengthSq := dx*dx + dy*dy
	if lengthSq == 0 {
		return math.Hypot(px-x1, py-y1)
	}
	t := ((px-x1)*dx + (py-y1)*dy) / lengthSq
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(x1+t*dx), py-(y1+t*dy))
}

// encodeDIB 将像素编码为 ICO 内嵌的 32 位 BMP（自下而上 BGRA + AND 掩码）。
func encodeDIB(pixels []color.NRGBA, size int) []byte {
	var buf bytes.Buffer

	_ = binary.Write(&buf, binary.LittleEndian, uint32(40))    // biSize
	_ = binary.Write(&buf, binary.LittleEndian, int32(size))   // biWidth
	_ = binary.Write(&buf, binary.LittleEndian, int32(size*2)) // biHeight（含掩码）
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))     // biPlanes
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32))    // biBitCount
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))     // biCompression = BI_RGB
	_ = binary.Write(&buf, binary.LittleEndian, uint32(size*size*4))
	_ = binary.Write(&buf, binary.LittleEndian, int32(0))
	_ = binary.Write(&buf, binary.LittleEndian, int32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))

	for y := size - 1; y >= 0; y-- {
		for x := 0; x < size; x++ {
			c := pixels[y*size+x]
			buf.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}

	maskRow := ((size + 31) / 32) * 4
	mask := make([]byte, maskRow*size)
	buf.Write(mask)

	return buf.Bytes()
}

func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t)
	}
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 255}
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
