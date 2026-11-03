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
	OnShow     func() // 左键单击托盘图标 / 菜单"显示主面板"
	OnSettings func() // 菜单"打开设置"
	OnAbout    func() // 菜单"关于"
	OnWebsite  func() // 菜单"打开官网"
	OnRestart  func() // 菜单"重启"
	OnQuit     func() // 菜单"退出"
}

//go:embed icons/tray.ico
var iconICO []byte

//go:embed icons/tray-gray.ico
var iconGrayICO []byte

//go:embed icons/tray.png
var iconPNG []byte

// iconTemplatePNG 模板图标（保留备用）：
//   - 背景完全透明，主体为黑色 "P" 剪影；
//   - 系统会按菜单栏深浅主题自动反色渲染。
// 由 scripts/gen_tray_icon.go 生成，尺寸 44×44（22pt @2x）。
//
// 当前 darwin 走的是 iconColorPNG（彩色），与 dock 图标视觉一致。
// 如要恢复"自适应深浅模式"的规范做法，把 applyIcon() 里 darwin 分支
// 改回 SetTemplateIcon(iconTemplatePNG, iconTemplatePNG) 即可。
//
//go:embed icons/tray-template.png
var iconTemplatePNG []byte

// iconColorPNG 彩色 macOS 状态栏图标：
//   - 由 scripts/gen_appicon.py 从 build/appicon.src.png 缩放生成；
//   - 尺寸 88×88（22pt @4x），自带圆角；
//   - 走 SetIcon（非 template），系统不会再染色，所见即所得。
//
//go:embed icons/tray-color.png
var iconColorPNG []byte

// iconGrayPNG 灰色系 macOS 状态栏图标：
//   - 由 scripts/gen_tray_icon_gray.py 从 build/appicon.src.png 灰度转换生成；
//   - 尺寸 44×44，适合不需要彩色强调色的场景。
//
//go:embed icons/tray-gray.png
var iconGrayPNG []byte

// Start 启动系统托盘。
//
//   - macOS：纯 cgo NSStatusItem 实现（start_darwin.go），完全不链接 fyne.io/systray。
//     fyne/systray 在 macOS 上会把 Go runtime 分配的对象设为 NSStatusItem 的 ObjC
//     target，Go GC 可能回收该对象导致 SIGSEGV；同时其 ObjC 类 MenuItem 与 wails
//     的 MenuItem 符号冲突，会导致链接失败（duplicate symbol）。
//   - Linux / Windows：fyne.io/systray（start_other.go）。
//
// 返回 cleanup 回调：调用后关闭托盘。
func Start(cb Callbacks) (cleanup func()) {
	startedMu.Lock()
	started = true
	startedMu.Unlock()

	return startPlatform(cb)
}

// CanToggle 返回当前平台是否支持"不重启进程"地切换托盘图标显隐。
//
//   - Windows：true（通过 Shell_NotifyIcon NIM_DELETE/NIM_ADD 平滑切换）
//   - macOS：true（通过 GoPasteStatusItemInstall/Uninstall 平滑切换）
//   - Linux：false（fyne.io/systray 未暴露隐藏 API，需要重启）
func CanToggle() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
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
		restoreAfterShow()
	}
}

// SetIconStyle 切换托盘/菜单栏图标风格。
// style: "color"（彩色）| "gray"（灰色）。
// macOS 和 Windows 生效；Linux 为 no-op。
func SetIconStyle(style string) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return
	}
	setIconStylePlatform(style)
}
