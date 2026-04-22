//go:build !windows

package cursor

// WorkArea 在非 Windows 平台返回零，调用方应 fallback 到屏幕尺寸。
// macOS 的 NSScreen.visibleFrame 已自动排除 Dock/菜单栏，Wails 返回值即是工作区。
func WorkArea() (int, int, int, int) {
	return 0, 0, 0, 0
}
