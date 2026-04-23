//go:build windows

package cursor

import (
	"syscall"
	"unsafe"
)

var (
	user32         = syscall.NewLazyDLL("user32.dll")
	getCursorPos   = user32.NewProc("GetCursorPos")
	getDpiForSys   = user32.NewProc("GetDpiForSystem") // Win10 1607+
	procMonitorFromPoint = user32.NewProc("MonitorFromPoint")
	procGetDpiForMonitor = syscall.NewLazyDLL("shcore.dll").NewProc("GetDpiForMonitor")

	gdi32       = syscall.NewLazyDLL("gdi32.dll")
	getDC       = user32.NewProc("GetDC")
	releaseDC   = user32.NewProc("ReleaseDC")
	getDeviceCaps = gdi32.NewProc("GetDeviceCaps")
)

type point struct {
	X, Y int32
}

// Position 返回鼠标位置（逻辑像素，与 Wails WindowSetPosition 同坐标系）。
//
// Wails v2 Windows 的窗口 API 使用逻辑像素；而 GetCursorPos 在 DPI-aware
// 进程里返回物理像素。两者混用会导致"跟随鼠标"窗口偏离屏幕。
// 这里读取系统 DPI（Win10+）或 GetDeviceCaps(LOGPIXELSX)，把物理坐标
// 换算为逻辑坐标。
func Position() (int, int) {
	var pt point
	getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	scale := currentDPIScale(pt)
	if scale <= 0 {
		return int(pt.X), int(pt.Y)
	}
	return int(float64(pt.X) / scale), int(float64(pt.Y) / scale)
}

// currentDPIScale 返回鼠标所在显示器的 DPI 缩放比（例如 1.0 / 1.25 / 1.5 / 2.0）。
// 失败时返回 0。
func currentDPIScale(pt point) float64 {
	// 优先使用 per-monitor DPI（Win8.1+），这样多显示器且缩放不同也能正确。
	if dpiX, ok := perMonitorDPI(pt); ok {
		return float64(dpiX) / 96.0
	}
	// 退一步：系统 DPI（Win10 1607+）
	if r, _, _ := getDpiForSys.Call(); r != 0 {
		return float64(r) / 96.0
	}
	// 最老派的办法：GetDeviceCaps(LOGPIXELSX) on desktop DC
	if dc, _, _ := getDC.Call(0); dc != 0 {
		const LOGPIXELSX = 88
		r, _, _ := getDeviceCaps.Call(dc, LOGPIXELSX)
		releaseDC.Call(0, dc)
		if r != 0 {
			return float64(r) / 96.0
		}
	}
	return 0
}

// perMonitorDPI 读取 pt 所在显示器的 DPI。
func perMonitorDPI(pt point) (uint32, bool) {
	if procMonitorFromPoint.Find() != nil || procGetDpiForMonitor.Find() != nil {
		return 0, false
	}
	const MONITOR_DEFAULTTONEAREST = 0x00000002
	// MonitorFromPoint 入参是 POINT by value（8 字节 = lo:x hi:y）
	ptPacked := uintptr(uint32(pt.X)) | (uintptr(uint32(pt.Y)) << 32)
	hMon, _, _ := procMonitorFromPoint.Call(ptPacked, MONITOR_DEFAULTTONEAREST)
	if hMon == 0 {
		return 0, false
	}
	const MDT_EFFECTIVE_DPI = 0
	var dpiX, dpiY uint32
	r, _, _ := procGetDpiForMonitor.Call(hMon, MDT_EFFECTIVE_DPI,
		uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY)))
	if r != 0 || dpiX == 0 {
		return 0, false
	}
	return dpiX, true
}
