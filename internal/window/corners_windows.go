//go:build windows

package window

import (
	"syscall"
	"time"
	"unsafe"
)

// ApplyWin11RoundCorners 为窗口启用 Windows 11 DWM 原生圆角。
// 在 Win10 上 DwmSetWindowAttribute 会返回非零 hr，但不会崩溃，窗口表现为直角。
// 通过窗口标题 FindWindowW 找到句柄，在新 goroutine 里重试几次，确保在 wails 创建窗口后执行。
func ApplyWin11RoundCorners(title string) {
	go func() {
		defer func() { _ = recover() }()
		user32 := syscall.NewLazyDLL("user32.dll")
		dwmapi := syscall.NewLazyDLL("dwmapi.dll")
		findWindow := user32.NewProc("FindWindowW")
		dwmSetAttr := dwmapi.NewProc("DwmSetWindowAttribute")

		titlePtr, err := syscall.UTF16PtrFromString(title)
		if err != nil {
			return
		}

		// DWMWA_WINDOW_CORNER_PREFERENCE = 33
		// DWMWCP_ROUND = 2 (圆角)
		const DWMWA_WINDOW_CORNER_PREFERENCE = 33
		const DWMWCP_ROUND = int32(2)
		pref := DWMWCP_ROUND

		for i := 0; i < 40; i++ { // 最多尝试约 4 秒
			hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))
			if hwnd != 0 {
				dwmSetAttr.Call(
					hwnd,
					uintptr(DWMWA_WINDOW_CORNER_PREFERENCE),
					uintptr(unsafe.Pointer(&pref)),
					unsafe.Sizeof(pref),
				)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}
