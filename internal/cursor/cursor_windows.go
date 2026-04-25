//go:build windows

package cursor

import (
	"syscall"
	"unsafe"
)

// 本包的 Windows 定位约定：
//
// Wails v2 Windows 内部 `WindowSetPosition(x, y)` 的实现是：
//
//	SetWindowPos(hwnd, HWND_TOP, workRect.Left + x, workRect.Top + y, …)
//
// 即 x/y 是「相对工作区」的**物理像素**（没有经过 DPI 缩放），
// workRect 由 `GetMonitorInfo` 返回也是物理像素。
//
// 而 `WindowGetSize` 内部调了 `scaleToDefaultDPI`，返回的是**逻辑像素**。
//
// 因此本项目统一用「物理像素」做坐标/工作区运算，
// 仅在读取窗口尺寸时乘以 DPI 比例换算成物理像素，这样 clamp 之后的坐标
// 可以原封不动传给 `WindowSetPosition`（再减去工作区原点即可）。
//
// 为了避免各处重复查询 DPI，ScaleForPoint 按"鼠标所在显示器"返回缩放比。

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	getCursorPos         = user32.NewProc("GetCursorPos")
	getDpiForSys         = user32.NewProc("GetDpiForSystem") // Win10 1607+
	procMonitorFromPoint = user32.NewProc("MonitorFromPoint")
	procGetDpiForMonitor = syscall.NewLazyDLL("shcore.dll").NewProc("GetDpiForMonitor")

	gdi32         = syscall.NewLazyDLL("gdi32.dll")
	getDC         = user32.NewProc("GetDC")
	releaseDC     = user32.NewProc("ReleaseDC")
	getDeviceCaps = gdi32.NewProc("GetDeviceCaps")
)

type point struct {
	X, Y int32
}

// Position 返回鼠标位置，单位：**物理像素**（Win32 GetCursorPos 原值）。
//
// 为什么不做 DPI 缩放？
// 因为 Wails 的 WindowSetPosition 在 Windows 下内部用的就是物理像素
// （没有 scaleWithDPI），所以让它们处于同一个坐标系最省事。
func Position() (int, int) {
	var pt point
	getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}

// ScaleForPoint 返回 (x, y) 所在显示器的 DPI 缩放比（96 基准）。
// 失败返回 1.0。
func ScaleForPoint(x, y int) float64 {
	pt := point{X: int32(x), Y: int32(y)}
	if s := currentDPIScale(pt); s > 0 {
		return s
	}
	return 1.0
}

// currentDPIScale 返回鼠标所在显示器的 DPI 缩放比。失败返回 0。
func currentDPIScale(pt point) float64 {
	if dpiX, ok := perMonitorDPI(pt); ok {
		return float64(dpiX) / 96.0
	}
	if r, _, _ := getDpiForSys.Call(); r != 0 {
		return float64(r) / 96.0
	}
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

func perMonitorDPI(pt point) (uint32, bool) {
	if procMonitorFromPoint.Find() != nil || procGetDpiForMonitor.Find() != nil {
		return 0, false
	}
	const MONITOR_DEFAULTTONEAREST = 0x00000002
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
