//go:build !windows && !darwin

package window

// FindMainWindow 在非 Windows/darwin 平台永远返回 0。
func FindMainWindow(title string) uintptr { return 0 }

// SetTaskbarVisible 在 Linux 上为空操作（由 WM 决定）。
func SetTaskbarVisible(hwnd uintptr, visible bool) {}
