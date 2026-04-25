//go:build darwin

package tray

// setIconVisible 在 macOS 上通过 GoPasteStatusItemInstall / Uninstall
// 平滑切换菜单栏图标的显隐，无需重启进程。
// show=true 时重新安装（幂等），show=false 时从菜单栏移除。
func setIconVisible(show bool) {
	if show {
		showStatusItem()
	} else {
		hideStatusItem()
	}
}
