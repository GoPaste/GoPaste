//go:build !windows

package window

// ApplyWin11RoundCorners 在非 Windows 平台下是空操作。
func ApplyWin11RoundCorners(title string) {}
