//go:build windows

package cursor

import (
	"unsafe"
)

var (
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
)

type rect struct {
	Left, Top, Right, Bottom int32
}

const (
	spiGetWorkArea = 0x0030
)

// WorkArea 返回主显示器工作区域（排除任务栏），以 Wails 逻辑像素为单位。
//
// 在 per-monitor DPI-aware 进程中，SPI_GETWORKAREA 返回的是**物理像素**；
// 为了与 Wails v2 的 WindowSetPosition / WindowGetSize（逻辑像素）匹配，
// 这里把矩形除以系统 DPI 缩放再返回。
// 返回值：x, y, width, height。失败返回 0,0,0,0。
func WorkArea() (int, int, int, int) {
	var r rect
	ret, _, _ := procSystemParametersInfo.Call(
		spiGetWorkArea,
		0,
		uintptr(unsafe.Pointer(&r)),
		0,
	)
	if ret == 0 {
		return 0, 0, 0, 0
	}

	// 用主显示器 DPI（相对 96 的倍数）进行缩放归一。
	var scale float64
	if dpi, _, _ := getDpiForSys.Call(); dpi != 0 {
		scale = float64(dpi) / 96.0
	}
	if scale <= 0 {
		if dc, _, _ := getDC.Call(0); dc != 0 {
			const LOGPIXELSX = 88
			r2, _, _ := getDeviceCaps.Call(dc, LOGPIXELSX)
			releaseDC.Call(0, dc)
			if r2 != 0 {
				scale = float64(r2) / 96.0
			}
		}
	}
	if scale <= 0 {
		scale = 1
	}

	left := int(float64(r.Left) / scale)
	top := int(float64(r.Top) / scale)
	right := int(float64(r.Right) / scale)
	bottom := int(float64(r.Bottom) / scale)
	return left, top, right - left, bottom - top
}
