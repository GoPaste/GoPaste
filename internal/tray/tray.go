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
	OnOpenDir func() // 菜单"打开数据目录"
	OnClear   func() // 菜单"清空历史"
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
// macOS / Linux：Wails 已持有 NSApp / GTK 主循环，
// 因此使用 systray.RunWithExternalLoop，并把 start() 调度到主线程执行。
// Windows：systray 在独立 goroutine 中通过 systray.Run 运行即可。
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
		systray.SetTooltip("gopaste · 剪切板管理")

		// 左键单击图标 → 显示/隐藏主面板
		systray.SetOnTapped(func() {
			if cb.OnShow != nil {
				cb.OnShow()
			}
		})

		// 右键菜单项
		mShow := systray.AddMenuItem("显示主面板", "打开 gopaste 主窗口")
		systray.AddSeparator()
		mOpenDir := systray.AddMenuItem("打开数据目录", "在文件管理器中打开")
		mClear := systray.AddMenuItem("清空历史", "清空未收藏/未置顶的历史")
		systray.AddSeparator()
		mAbout := systray.AddMenuItem("关于 gopaste", "版本与项目信息")
		systray.AddSeparator()
		mRestart := systray.AddMenuItem("重启 gopaste", "重新启动应用")
		mQuit := systray.AddMenuItem("退出 gopaste", "完全关闭应用")

		go func() {
			for {
				select {
				case <-mShow.ClickedCh:
					if cb.OnShow != nil {
						cb.OnShow()
					}
				case <-mOpenDir.ClickedCh:
					if cb.OnOpenDir != nil {
						cb.OnOpenDir()
					}
				case <-mClear.ClickedCh:
					if cb.OnClear != nil {
						cb.OnClear()
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
		go systray.Run(onReady, onExit)
		return func() { systray.Quit() }
	}

	// macOS / Linux：使用 external loop 并把 start 派到主线程
	start, end := systray.RunWithExternalLoop(onReady, onExit)
	dispatchOnMain(start)
	return end
}
