//go:build windows

package window

import (
	"syscall"
	"time"
	"unsafe"
)

// ApplyWin11RoundCorners 为窗口启用 Windows 11 DWM 原生圆角，同时去掉 DWM 外边框线。
// 在 Win10 上 DwmSetWindowAttribute 会返回非零 hr，但不会崩溃，窗口表现为直角。
// 通过窗口标题 FindWindowW 找到句柄，在新 goroutine 里重试几次，确保在 wails 创建窗口后执行。
//
// 为什么要额外设 BORDER_COLOR=NONE：
//   Win11 在启用 DWMWCP_ROUND 后，除了把四角做圆角裁剪，还会沿窗口整条外轮廓
//   绘制一条 1px 的 DWM 边框线（默认跟随系统强调色/灰色）。在 Frameless + WebView
//   透明 + 暗色主题组合下，这条边框 + DWM 抗锯齿在特定 DPI/尺寸会出现一条斜向
//   延伸的灰色亚像素线（用户报告"界面上出现不是分割线也不是边框的灰色斜线"）。
//   DWMWA_BORDER_COLOR=DWMWA_COLOR_NONE(0xFFFFFFFE) 让 DWM 不画这条线，圆角仍保留。
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

		// DWMWA_WINDOW_CORNER_PREFERENCE = 33, DWMWCP_ROUND = 2 (圆角)
		// DWMWA_BORDER_COLOR            = 34, DWMWA_COLOR_NONE = 0xFFFFFFFE (不画边框)
		const (
			DWMWA_WINDOW_CORNER_PREFERENCE = 33
			DWMWA_BORDER_COLOR             = 34
			DWMWCP_ROUND                   = int32(2)
			DWMWA_COLOR_NONE               = uint32(0xFFFFFFFE)
		)
		pref := DWMWCP_ROUND
		borderColor := DWMWA_COLOR_NONE

		for i := 0; i < 40; i++ { // 最多尝试约 4 秒
			hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))
			if hwnd != 0 {
				dwmSetAttr.Call(
					hwnd,
					uintptr(DWMWA_WINDOW_CORNER_PREFERENCE),
					uintptr(unsafe.Pointer(&pref)),
					unsafe.Sizeof(pref),
				)
				// Win10 不认识这个属性，返回 E_INVALIDARG 即可，不影响圆角。
				dwmSetAttr.Call(
					hwnd,
					uintptr(DWMWA_BORDER_COLOR),
					uintptr(unsafe.Pointer(&borderColor)),
					unsafe.Sizeof(borderColor),
				)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}
