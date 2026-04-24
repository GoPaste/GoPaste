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

	// pendingStart: systray.RunWithExternalLoop 返回的 start()，延迟到
	// wails OnDomReady 之后由 FlushPendingStart 调度到主线程执行。
	pendingStart   func()
	pendingStartMu sync.Mutex
)

// SetPendingStart 存储待调度的 systray start 函数。由 tray 包内部调用。
func SetPendingStart(fn func()) {
	pendingStartMu.Lock()
	pendingStart = fn
	pendingStartMu.Unlock()
}

// FlushPendingStart 由 App 在 wails OnDomReady 后调用，把 systray.start()
// 调度到主线程执行。无平台差异：Linux 上 dispatchOnMain 同步执行。
func FlushPendingStart() {
	pendingStartMu.Lock()
	fn := pendingStart
	pendingStart = nil
	pendingStartMu.Unlock()
	if fn == nil {
		return
	}
	dispatchOnMain(fn)
}

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

// iconTemplatePNG 模板图标（保留备用）：
//   - 背景完全透明，主体为黑色 "P" 剪影；
//   - 系统会按菜单栏深浅主题自动反色渲染。
// 由 build/gen_tray_icon.go 生成，尺寸 44×44（22pt @2x）。
//
// 当前 darwin 走的是 iconColorPNG（彩色），与 dock 图标视觉一致。
// 如要恢复"自适应深浅模式"的规范做法，把 applyIcon() 里 darwin 分支
// 改回 SetTemplateIcon(iconTemplatePNG, iconTemplatePNG) 即可。
//
//go:embed icon_template.png
var iconTemplatePNG []byte

// iconColorPNG 彩色 macOS 状态栏图标：
//   - 由 build/gen_appicon.py 从 build/appicon.src.png 缩放生成；
//   - 尺寸 44×44（22pt @2x），自带圆角；
//   - 走 SetIcon（非 template），系统不会再染色，所见即所得。
// 代价：dark mode 下不会自动反色。这是用户主动选择的取舍
// （希望菜单栏图标和 dock 图标完全一致），不是 bug。
//
//go:embed icon_color.png
var iconColorPNG []byte

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
	// 注意：start() 必须在主线程执行，且必须在 wails 完成启动序列（包括
	// 首次 _ExecJS 注入 runtime.js）之后。否则 NSStatusBar 创建和 wails
	// 的 WKWebView/主窗口初始化在主队列里交错，会导致 objc_msgSend 野
	// 指针崩溃 (PC=libobjc+0x28, addr=0x20)。
	//
	// 稳妥做法：延迟到 wails.OnDomReady 之后几百 ms 再调 start()。
	// 我们把 start 存到 pendingStart，由 App.OnDomReady 触发真正调度。
	SetPendingStart(start)
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
		// 用户选择菜单栏图标与 dock 图标视觉一致：用彩色 SetIcon，
		// 而非 template。如要恢复自适应深浅模式的规范做法，改回
		// SetTemplateIcon(iconTemplatePNG, iconTemplatePNG) 即可。
		systray.SetIcon(iconColorPNG)
		// 注：v1/v2 两版 setTrayIconSizePt 都会让主线程 RunMainLoop
		// SIGSEGV(addr=0x20)。临时全部禁用以确认基线稳定。换思路用
		// "改 PNG 内容比例（让图形撑满更多画布）"来视觉放大图标。
		// setTrayIconSizePt(22)
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
