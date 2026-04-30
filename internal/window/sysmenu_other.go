//go:build !windows

package window

// DisableAltSpaceSysMenu 在非 Windows 平台下是空操作。
// macOS / Linux 没有 Win32 系统菜单的概念，对应问题不存在。
func DisableAltSpaceSysMenu(title string) {}

// EnsureSysMenuSubclass 同样是空操作（仅 Windows 需要）。
func EnsureSysMenuSubclass(title string) {}
