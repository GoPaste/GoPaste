//go:build !windows && !darwin

package tray

// setIconVisible 在 Linux 上是 no-op（fyne.io/systray 未暴露隐藏 API）。
func setIconVisible(show bool) {}
