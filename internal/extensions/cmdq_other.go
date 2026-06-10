//go:build !darwin

package extensions

// InstallCmdQGuard 非 macOS 平台不提供 Cmd+Q 拦截，调用为 no-op。
func InstallCmdQGuard(cb OnCmdQHandled) {}

// SetCmdQBehavior 非 macOS 平台为 no-op。
func SetCmdQBehavior(b CmdQBehavior) {}

// SetCmdQConfirmWindowMs 非 macOS 平台为 no-op。
func SetCmdQConfirmWindowMs(ms int) {}

// Supported 非 macOS 平台返回 false，前端据此隐藏相关设置项。
func Supported() bool { return false }

// --- L0 全局拦截（CGEventTap）—— 非 macOS 平台 no-op ---

// CmdQTapAuthStatus 非 macOS 平台始终返回 TapAuthGranted（以便调用方可无条件尝试）。
func CmdQTapAuthStatus() TapAuthStatus { return TapAuthGranted }

// CmdQTapRequestAccess 非 macOS 平台为 no-op，返回 true。
func CmdQTapRequestAccess() bool { return true }

// CmdQTapStart 非 macOS 平台为 no-op，返回 false 表示未启用。
func CmdQTapStart() bool { return false }

// CmdQTapStop 非 macOS 平台为 no-op。
func CmdQTapStop() {}

// OpenInputMonitoringPrefs 非 macOS 平台为 no-op。
func OpenInputMonitoringPrefs() {}

// DebugLog 非 macOS 平台 no-op。
func DebugLog(msg string) {}
