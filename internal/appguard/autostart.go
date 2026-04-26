package appguard

// 开机自启 API（跨平台，由平台相关文件提供具体实现）：
//   - autostart_windows.go  写入 HKCU\Software\Microsoft\Windows\CurrentVersion\Run（纯 Go，无 cgo）
//   - autostart_unix.go     使用 github.com/emersion/go-autostart
//                           ·macOS: ~/Library/LaunchAgents/gopaste.plist
//                           ·Linux: ~/.config/autostart/gopaste.desktop
//
// 启动时附加 --silent-start 参数，配合 main.go 中的静默启动逻辑，
// 开机自启时不会直接弹出窗口，仅加载到后台。

const (
	autostartName        = "gopaste"
	autostartDisplayName = "GoPaste"
	autostartSilentArg   = "--silent-start"
)

// IsAutoStartEnabled 返回当前是否已配置开机自启。
func IsAutoStartEnabled() bool {
	return platformIsAutoStartEnabled()
}

// SetAutoStart 切换开机自启设置。enabled=true 启用，false 禁用。
func SetAutoStart(enabled bool) error {
	if enabled {
		if platformIsAutoStartEnabled() {
			return nil
		}
		return platformEnableAutoStart()
	}
	if !platformIsAutoStartEnabled() {
		return nil
	}
	return platformDisableAutoStart()
}
