//go:build !windows

package window

// FindMainWindow 在非 Windows 平台永远返回 0。
func FindMainWindow(title string) uintptr { return 0 }

// SetTaskbarVisible 在非 Windows 平台为空操作：
// macOS 有 LSUIElement / NSApp.setActivationPolicy，Linux 由 WM 决定，
// 都不在本函数职责内。
func SetTaskbarVisible(hwnd uintptr, visible bool) {}
