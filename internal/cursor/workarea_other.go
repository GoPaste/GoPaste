//go:build !windows && !darwin

package cursor

// WorkArea 在非 Windows 平台返回零，调用方应 fallback 到屏幕尺寸。
// macOS 的 NSScreen.visibleFrame 已自动排除 Dock/菜单栏，Wails 返回值即是工作区。
func WorkArea() (int, int, int, int) {
	return 0, 0, 0, 0
}

// ScaleForPoint 在非 Windows 平台恒为 1.0。
// macOS/Linux 下 Wails 的坐标 API 已经是逻辑像素，Position() 也返回逻辑像素，
// 无需额外缩放。
func ScaleForPoint(x, y int) float64 {
	return 1.0
}
