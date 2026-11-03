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
//
// 注意：启用分支必须始终重写自启项，不能依据"键是否已存在"提前返回。
// 否则升级/移动到新路径（如旧版本装到 Program Files、新建绿色版到别处）
// 后，注册表/启动项仍指向旧 exe，下次开机拉起的是旧版本而非当前版本。
// 各平台的 platformEnableAutoStart 都是覆盖式写入（Windows SetStringValue /
// macOS·Linux os.Create），重复调用安全且正是我们期望的"路径刷新"行为。
func SetAutoStart(enabled bool) error {
	if enabled {
		return platformEnableAutoStart()
	}
	if !platformIsAutoStartEnabled() {
		return nil
	}
	return platformDisableAutoStart()
}
