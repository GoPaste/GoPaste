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
// ctx 参数仅为签名统一保留；mac 实现里用不到。
func ShowMain(ctx context.Context) {
	_ = ctx
	OrderFront(Title)
}

// HideMain 以面板化方式隐藏主窗口：orderOut:。
// 和 wailsruntime.WindowHide 的区别是：不触发 [NSApp hide] /
// applicationShouldTerminateAfterLastWindowClosed: 链路，也不改变 AppKit
// 的 active app 状态。NonactivatingPanel 必须配对这个。
//
// 注意：Wails 内部维护一份"窗口可见"状态，用 WindowHide 才会翻转。
// 这里不调 WindowHide 意味着 Wails runtime 认为窗口仍然"可见"（但
// 面板已经从屏幕上消失）。只要我们后续只用 ShowMain/HideMain 配对，
// 不再混用 WindowShow/WindowHide，这份不一致不会有实际影响。
func HideMain(ctx context.Context) {
	_ = ctx
	OrderOut(Title)
}

// ResignMainKey 让面板交还键盘焦点给下面的 active app，但**不**隐藏面板。
// EcoPaste 的粘贴流程用这种姿势：不 hide、只 resign，然后立刻发 Cmd+V。
// 我们当前流程是"HideMain + SendPaste"，这个 helper 留作未来可选。
func ResignMainKey(ctx context.Context) {
	_ = ctx
	ResignKey(Title)
}

// 保留未使用的 wailsruntime 引用，便于将来需要 fallback。
var _ = wailsruntime.WindowHide
