//go:build darwin

package window

import "github.com/wailsapp/wails/v3/pkg/application"

// Title 主窗口标题，必须与 main.go 里 WebviewWindowOptions.Title 保持一致。
// panel_darwin.m 按标题在 [NSApp windows] 中查找主窗口。
const Title = "GoPaste"

// ShowMain 以面板化方式显示主窗口，不抢前台应用的 active 状态。
//
// 只调用 wails 的 Show() + Focus()。对 NonactivatingPanel，wails 的实现是
// orderFrontRegardless + makeKeyWindow（见 wails 的 windowShow / windowFocus），
// 不会激活应用，正是我们需要的语义。
//
// 这里刻意不额外调用原生接口（orderFrontRegardless / makeKeyWindow / activateIgnoringOtherApps:）：
// 两套逻辑并存会让窗口的 key 状态和应用的 active 状态互相打架，也是本文件之前
// “每次显示都重设 collectionBehavior / level”这类兜底逻辑的来源。
// Space 行为、层级、collectionBehavior 全部交给 wails 的选项（见 options_darwin.go）。
func ShowMain(w application.Window) {
	if w == nil {
		return
	}
	w.Show()
	w.Focus()
}

// HideMain 以面板化方式隐藏主窗口。
// v3 的 Hide() 在 macOS 上就是 orderOut:，不触发 [NSApp hide] /
// applicationShouldTerminateAfterLastWindowClosed: 链路，也不改变 active app。
func HideMain(w application.Window) {
	if w == nil {
		return
	}
	w.Hide()
}
