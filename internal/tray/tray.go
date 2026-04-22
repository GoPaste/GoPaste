// Package tray 提供跨平台系统托盘能力。
//
// 基于 fyne.io/systray：
//   - 左键单击图标：触发 OnShow（显示/隐藏主面板）
//   - 右键单击图标：弹出菜单
package tray

import (
	_ "embed"
	"runtime"

	"fyne.io/systray"
)

// Callbacks 托盘菜单项 / 交互行为。
type Callbacks struct {
	OnShow    func() // 左键单击托盘图标 / 菜单"显示主面板"
	OnAbout   func() // 菜单"关于"
	OnRestart func() // 菜单"重启"
	OnQuit    func() // 菜单"退出"
}

//go:embed icon.ico
var iconICO []byte

//go:embed icon.png
var iconPNG []byte

// Start 启动系统托盘。
//
//   - macOS：NSApp 已被 Wails 持有主线程，必须使用 RunWithExternalLoop，
//     并把返回的 start() 调度到主线程执行，否则 NSStatusBar 会在非主线程
//     构建，表现为"菜单栏无图标且无报错"。
//   - Linux：同样使用 RunWithExternalLoop（GTK 主循环由 Wails 持有）。
//   - Windows：systray 自己跑消息循环，放到独立 goroutine 里即可。
//
// 返回 cleanup 回调：调用后关闭托盘。
func Start(cb Callbacks) (cleanup func()) {
	onReady := func() {
		if runtime.GOOS == "windows" {
			systray.SetIcon(iconICO)
		} else {
			// macOS：使用模板图标，系统会按菜单栏深浅主题自动着色，
			// 也能得到正确的菜单栏显示尺寸（≈22pt）。
			systray.SetTemplateIcon(iconPNG, iconPNG)
		}
		systray.SetTitle("")
		systray.SetTooltip("GoPaste · 剪切板管理")

		// 左键单击图标 → 显示/隐藏主面板
		systray.SetOnTapped(func() {
			if cb.OnShow != nil {
				cb.OnShow()
			}
		})

		// 右键菜单项
		mShow := systray.AddMenuItem("显示主面板", "打开 GoPaste 主窗口")
		systray.AddSeparator()
		mAbout := systray.AddMenuItem("关于 GoPaste", "版本与项目信息")
		systray.AddSeparator()
		mRestart := systray.AddMenuItem("重启 GoPaste", "重新启动应用")
		mQuit := systray.AddMenuItem("退出 GoPaste", "完全关闭应用")

		go func() {
			for {
				select {
				case <-mShow.ClickedCh:
					if cb.OnShow != nil {
						cb.OnShow()
					}
				case <-mAbout.ClickedCh:
					if cb.OnAbout != nil {
						cb.OnAbout()
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
		// Windows 上 systray.Run 会内部创建消息线程，放 goroutine 里跑即可。
		go systray.Run(onReady, onExit)
		return func() { systray.Quit() }
	}

	// macOS / Linux：外部主循环集成。
	start, end := systray.RunWithExternalLoop(onReady, onExit)
	// start() 必须在主线程执行（NSStatusBar 要求）。dispatch_darwin.go 提供
	// 了主线程调度；Linux 上 dispatchOnMain 为 no-op 直接同步执行。
	dispatchOnMain(start)
	return func() {
		end()
	}
}
