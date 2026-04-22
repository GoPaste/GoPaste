//go:build ignore

// gen_tray_icon 生成 macOS 状态栏用的模板图标（template icon）。
//
// 模板图标规则（macOS 强制）：
//   - 背景必须完全透明（alpha=0）；
//   - 主体像素只保留 alpha 通道，颜色一律为黑（R=G=B=0），
//     由系统按菜单栏深浅主题自动反色。
//
// 本脚本绘制一个"P"形剪影：主干 + 右上半圆洞，尺寸 44×44，
// 对应状态栏 22pt @2x。
//
// 运行方式：go run build/gen_tray_icon.go
// 输出：internal/tray/icon_template.png
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

const (
	size = 44 // 像素边长，@2x 菜单栏尺寸
)

// 黑色像素（完全不透明）——模板图标语义：alpha=255 的部分会被系统着色。
var fg = color.NRGBA{R: 0, G: 0, B: 0, A: 255}

// inP 判断像素 (x,y) 是否属于 "P" 字形的主体。
//
// P 结构：
//   - 左侧竖杠（stem）：垂直矩形
//   - 上半部的圆环（bowl）：外圆环，靠上、靠右
//
// 所有坐标归一化到 [0,1]×[0,1]，最后再乘以 size。
func inP(fx, fy float64) bool {
	// ---- 参数（归一化坐标） ----
	// 整个字形紧凑排布在 [0.12, 0.88] x [0.08, 0.92] 内
	stemLeft := 0.18
	stemRight := 0.42
	stemTop := 0.08
	stemBottom := 0.92

	// bowl（碗）外圆中心与半径
	bowlCx := 0.55
	bowlCy := 0.32 // 靠上
	bowlR := 0.30
	bowlRin := 0.13 // 内圆（洞）

	// 竖杠
	if fx >= stemLeft && fx <= stemRight && fy >= stemTop && fy <= stemBottom {
		return true
	}
	// 圆环：外圆内、内圆外，且在 bowl 所在上半区
	dx := fx - bowlCx
	dy := fy - bowlCy
	d := math.Sqrt(dx*dx + dy*dy)
	if d <= bowlR && d >= bowlRin {
		// 限制到上半部 + 略超出 stem 右侧，避免遮到下方
		if fy <= bowlCy+bowlR+0.01 && fy >= stemTop-0.02 && fx >= stemLeft-0.02 {
			return true
		}
	}
	return false
}

// aaSample 做 4x4 超采样抗锯齿：一个像素内取 16 个子像素点投票，
// 命中比例映射为 alpha（0..255）。
func aaSample(px, py int) uint8 {
	const n = 4
	hits := 0
	for sy := 0; sy < n; sy++ {
		for sx := 0; sx < n; sx++ {
			fx := (float64(px) + (float64(sx)+0.5)/n) / float64(size)
			fy := (float64(py) + (float64(sy)+0.5)/n) / float64(size)
			if inP(fx, fy) {
				hits++
			}
		}
	}
	if hits == 0 {
		return 0
	}
	return uint8(hits * 255 / (n * n))
}

func main() {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			a := aaSample(x, y)
			if a == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: a})
		}
	}
	f, err := os.Create("internal/tray/icon_template.png")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
	_ = fg // 占位：若后续扩展为实心色可用
}
