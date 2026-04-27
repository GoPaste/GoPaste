//go:build !darwin

package tray

import (
	"runtime"

	"fyne.io/systray"
)

// startPlatform 在非 macOS 平台上不处理，交由 systray 路径处理。
func startPlatform(_ Callbacks) (cleanup func(), ok bool) {
	return nil, false
}

// currentIconStyle 记录当前图标风格，applyIcon 和 setIconStylePlatform 使用。
var currentIconStyle = "color"

// setIconStylePlatform 切换图标风格。
// Windows：直接调用 systray.SetIcon 实时切换；Linux：no-op。
func setIconStylePlatform(style string) {
	currentIconStyle = style
	if runtime.GOOS != "windows" {
		return
	}
	switch style {
	case "gray":
		systray.SetIcon(iconGrayICO)
	default:
		systray.SetIcon(iconICO)
	}
}
