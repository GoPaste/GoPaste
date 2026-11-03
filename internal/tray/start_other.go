//go:build !darwin

package tray

import (
	"runtime"

	"fyne.io/systray"
)

// startPlatform 使用 fyne.io/systray 启动托盘（Windows / Linux）。
func startPlatform(cb Callbacks) (cleanup func()) {
	onReady := func() {
		applyIcon()
		systray.SetTitle("")
		systray.SetTooltip("GoPaste · 剪切板管理")

		systray.SetOnTapped(func() {
			if cb.OnShow != nil {
				cb.OnShow()
			}
		})

		mShow := systray.AddMenuItem("显示主面板", "打开 GoPaste 主窗口")
		mSettings := systray.AddMenuItem("打开设置", "打开偏好设置")
		systray.AddSeparator()
		mAbout := systray.AddMenuItem("关于", "版本与项目信息")
		mWebsite := systray.AddMenuItem("打开官网", "在浏览器中打开 GoPaste 官网")
		systray.AddSeparator()
		mRestart := systray.AddMenuItem("重启", "重新启动应用")
		mQuit := systray.AddMenuItem("退出", "完全关闭应用")

		go func() {
			for {
				select {
				case <-mShow.ClickedCh:
					if cb.OnShow != nil {
						cb.OnShow()
					}
				case <-mSettings.ClickedCh:
					if cb.OnSettings != nil {
						cb.OnSettings()
					}
				case <-mAbout.ClickedCh:
					if cb.OnAbout != nil {
						cb.OnAbout()
					}
				case <-mWebsite.ClickedCh:
					if cb.OnWebsite != nil {
						cb.OnWebsite()
					}
				case <-mRestart.ClickedCh:
					systray.Quit()
					if cb.OnRestart != nil {
						cb.OnRestart()
					}
					return
				case <-mQuit.ClickedCh:
					systray.Quit()
					if cb.OnQuit != nil {
						cb.OnQuit()
					}
					return
				}
			}
		}()
	}
	onExit := func() {}

	if runtime.GOOS == "windows" {
		go systray.Run(onReady, onExit)
		return func() { systray.Quit() }
	}

	// Linux
	start, end := systray.RunWithExternalLoop(onReady, onExit)
	SetPendingStart(start)
	return func() { end() }
}

// applyIcon 设置平台对应的图标，感知当前图标风格。
func applyIcon() {
	if runtime.GOOS == "windows" {
		if currentIconStyle == "gray" {
			systray.SetIcon(iconGrayICO)
		} else {
			systray.SetIcon(iconICO)
		}
	} else {
		systray.SetIcon(iconPNG)
	}
}

// restoreAfterShow 在图标重新显示后恢复图标和 tooltip
// （NIM_ADD 创建的图标不携带之前设置的属性）。
func restoreAfterShow() {
	applyIcon()
	systray.SetTooltip("GoPaste · 剪切板管理")
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
