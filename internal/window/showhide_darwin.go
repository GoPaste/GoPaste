//go:build darwin

package window

import (
	"context"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Title 主窗口标题，必须与 options.App.Title 保持一致。
// 这里做成常量以便 app 层直接引用；修改窗口标题时记得同步。
const Title = "GoPaste"

// ShowMain 以面板化方式显示主窗口：orderFrontRegardless + makeKeyWindow。
// 和 wailsruntime.WindowShow 的区别是：不会调用 activateIgnoringOtherApps:，
// 所以下层前台应用的 active 状态保持不变，这正是剪贴板面板类 app 需要的行为。
//
// 注意：Wails 内部维护一份"窗口可见"状态，用 WindowShow 才会翻转。
// 这里不调 WindowShow 意味着 Wails runtime 认为窗口仍然"隐藏"，
// 但 Panel 已经从 orderFront 显示到屏幕上。我们自己在 app.go 的
// windowVisible 里维护真实显隐状态即可。
func ShowMain(ctx context.Context) {
	_ = ctx
	OrderFront(Title)
}

// HideMain 以面板化方式隐藏主窗口：orderOut:。
// 和 wailsruntime.WindowHide 的区别是：不触发 [NSApp hide] /
// applicationShouldTerminateAfterLastWindowClosed: 链路，也不改变 AppKit
// 的 active app 状态。NonactivatingPanel 必须配对这个。
func HideMain(ctx context.Context) {
	_ = ctx
	OrderOut(Title)
}

// ResignMainKey 让面板交还键盘焦点给下面的 active app，但**不**隐藏面板。
// EcoPaste 的粘贴流程用这种姿势：不 hide、只 resign，然后立刻发 Cmd+V。
func ResignMainKey(ctx context.Context) {
	_ = ctx
	ResignKey(Title)
}

// 保留未使用的引用，避免 unused import。
var _ = wailsruntime.WindowHide
