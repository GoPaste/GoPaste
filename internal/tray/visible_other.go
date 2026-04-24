//go:build !windows

package tray

// setIconVisible 在非 Windows 平台上是 no-op。
//
// macOS / Linux 上 fyne.io/systray 没有暴露隐藏通知图标的公开 API，
// 且 systray.Quit() 受 sync.Once 限制无法复用。
// 这些平台上"关闭后再开启菜单栏图标"仍走 restartApp 兜底。
//
// Windows 上由 visible_windows.go 通过 Shell_NotifyIcon 实现平滑切换。
func setIconVisible(show bool) {
	// no-op on macOS / Linux
}
