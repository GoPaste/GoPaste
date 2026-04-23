package appguard

import (
	"os"

	"github.com/emersion/go-autostart"
)

// autostartApp 返回统一描述本应用的开机自启条目。
// - Windows: 写入 HKCU\Software\Microsoft\Windows\CurrentVersion\Run
// - macOS:   生成 ~/Library/LaunchAgents/gopaste.plist
// - Linux:   生成 ~/.config/autostart/gopaste.desktop
//
// 启动时附加 --silent-start 参数，配合 main.go 中的静默启动逻辑，
// 开机自启时不会直接弹出窗口，仅加载到后台。
func autostartApp() (*autostart.App, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &autostart.App{
		Name:        "gopaste",
		DisplayName: "GoPaste",
		Exec:        []string{exe, "--silent-start"},
	}, nil
}

// IsAutoStartEnabled 返回当前是否已配置开机自启。
func IsAutoStartEnabled() bool {
	app, err := autostartApp()
	if err != nil {
		return false
	}
	return app.IsEnabled()
}

// SetAutoStart 切换开机自启设置。enabled=true 启用，false 禁用。
func SetAutoStart(enabled bool) error {
	app, err := autostartApp()
	if err != nil {
		return err
	}
	if enabled {
		// 若已启用则无需重复写入，避免重复生成 plist/desktop 文件
		if app.IsEnabled() {
			return nil
		}
		return app.Enable()
	}
	if !app.IsEnabled() {
		return nil
	}
	return app.Disable()
}
