//go:build darwin

package tray

// startPlatform 在 macOS 上使用纯 CGO NSStatusItem 实现托盘。
func startPlatform(cb Callbacks) (cleanup func(), ok bool) {
	end := installStatusItem(cb, iconColorPNG)
	return end, true
}

// setIconStylePlatform 切换 macOS 菜单栏图标风格。
func setIconStylePlatform(style string) {
	currentIconStyle = style
	switch style {
	case "gray":
		setStatusItemIcon(iconGrayPNG, false)
	default: // "color"
		setStatusItemIcon(iconColorPNG, false)
	}
}
