// Package tray 提供跨平台系统托盘能力。
//
// 基于 fyne.io/systray：
//   - 左键单击图标：触发 OnShow（显示/隐藏主面板）
//   - 右键单击图标：弹出菜单
//
// 图标显隐（SetVisible）:
//
//	Windows 上通过 Shell_NotifyIcon NIM_DELETE/NIM_ADD 平滑切换，
//	不调用 systray.Quit()，避免 sync.Once 限制。这和 Tauri 的
//	tray_icon.set_visible(false/true) 是同等效果。
//
//	macOS/Linux 上暂不支持平滑切换（fyne.io/systray 未暴露 API），
//	关闭后再开启仍走 restartApp 兜底。
package tray

import (
	_ "embed"
	"runtime"
	"sync"

	"fyne.io/systray"
)

var (
	started   bool      // systray 是否已经启动过
	startedMu sync.Mutex
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

// iconTemplatePNG 专供 macOS 状态栏使用的模板图标：
//   - 背景完全透明，主体为黑色 "P" 剪影；
//   - 系统会按菜单栏深浅主题自动反色渲染。
// 这张图由 build/gen_tray_icon.go 生成，尺寸 44×44（22pt @2x）。
// 原 icon.png 是彩色 app 图标，直接喂给 SetTemplateIcon 会被系统全染成
// 一整块纯色，表现为"状态栏上一个白色/黑色方块"。
//
//go:embed icon_template.png
var iconTemplatePNG []byte

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
	startedMu.Lock()
	started = true
	startedMu.Unlock()

	onReady := func() {
		applyIcon()
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
		mAbout := systray.AddMenuItem("关于", "版本与项目信息")
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

// applyIcon 设置平台对应的图标。
// 抽成独立函数，在 Start 的 onReady 和 SetVisible(true) 中都需要调用。
func applyIcon() {
	if runtime.GOOS == "windows" {
		systray.SetIcon(iconICO)
	} else if runtime.GOOS == "darwin" {
		systray.SetTemplateIcon(iconTemplatePNG, iconTemplatePNG)
	} else {
		systray.SetIcon(iconPNG)
	}
}

// CanToggle 返回当前平台是否支持"不重启进程"地切换托盘图标显隐。
//
//   - Windows：true（通过 Shell_NotifyIcon NIM_DELETE/NIM_ADD 平滑切换）
//   - macOS/Linux：false（fyne.io/systray 未暴露隐藏 API，需要重启）
func CanToggle() bool {
	return runtime.GOOS == "windows"
}

// SetVisible 平滑切换托盘图标的可见性（不销毁 systray 消息循环）。
//
// 仅在 CanToggle() 返回 true 的平台上真正生效（目前只有 Windows）。
// 其他平台上调用此函数是 no-op，调用方应检查 CanToggle() 并走 restart 兜底。
//
// visible=true 时会在重新添加图标后立即调用 systray.SetIcon / SetTooltip，
// 因为 NIM_ADD 创建的图标不携带之前设置的属性。
func SetVisible(show bool) {
	startedMu.Lock()
	ok := started
	startedMu.Unlock()
	if !ok {
		return // systray 尚未启动
	}

	setIconVisible(show)

	// 重新添加图标后恢复图标和 tooltip
	if show {
		applyIcon()
		systray.SetTooltip("GoPaste · 剪切板管理")
	}
}
