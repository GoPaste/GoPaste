//go:build darwin

// Package window 封装窗口相关的跨平台能力。
package window

import "github.com/wailsapp/wails/v3/pkg/application"

// panelCollectionBehavior 决定面板如何跟随用户的 Space：
//   - MoveToActiveSpace：每次 show 跟到当前 Space（多全屏 Space 下比 CanJoinAllSpaces 可靠）
//   - FullScreenAuxiliary：能浮在全屏应用之上
//   - Stationary：不参与 Mission Control 动画
const panelCollectionBehavior = application.MacWindowCollectionBehaviorMoveToActiveSpace |
	application.MacWindowCollectionBehaviorFullScreenAuxiliary |
	application.MacWindowCollectionBehaviorStationary

// ApplyOptions 将平台相关选项写入主窗口的 WebviewWindowOptions。
//
// macOS 策略：
//   - Frameless=false：**必须**保留 titled style，否则 NSWindowStyleMaskBorderless
//     不带系统圆角和阴影，窗口会变成直角硬边。
//   - TitleBarHiddenInset：把标题栏条隐藏为内嵌形态，保留 titled mask
//     的圆角/阴影，但视觉上标题栏不可见。
//   - WindowClass=Panel + NonActivating：原生创建带 NSWindowStyleMaskNonactivatingPanel
//     的 NSPanel。wails 的 Show()/Focus() 对非激活面板只做 orderFrontRegardless +
//     makeKeyWindow，不会 activateIgnoringOtherApps:，所以显示面板不抢前台应用的
//     active 状态，粘贴时 Cmd+V 才会落到用户原来的 App。
//   - FloatingPanel + WindowLevel=Floating：面板始终浮在普通应用窗口之上。
//   - BecomesKeyOnlyIfNeeded=false：面板显示后要能直接接收键盘（搜索框 / 方向键 / 回车）。
//   - 红绿灯按钮由 ConfigurePanel 隐藏（AppKit 层兜底，见 panel_darwin.m）。
func ApplyOptions(opts *application.WebviewWindowOptions) {
	opts.Frameless = false
	opts.Mac.TitleBar = application.MacTitleBarHiddenInset
	opts.Mac.WindowClass = application.MacWindowClassPanel
	opts.Mac.PanelPreferences = application.MacPanelPreferences{
		NonActivating:          true,
		FloatingPanel:          true,
		BecomesKeyOnlyIfNeeded: false,
	}
	opts.Mac.WindowLevel = application.MacWindowLevelFloating
	opts.Mac.CollectionBehavior = panelCollectionBehavior
}

// ApplyAppOptions 将平台相关的应用级选项写入 application.Options。
//
// GoPaste 是托盘常驻应用：主窗口隐藏 / 关闭后进程不能退出。v3 默认
// ApplicationShouldTerminateAfterLastWindowClosed=false，这里显式写出以防默认值变化。
// Dock 图标显隐（ActivationPolicy）由 SetTaskbarVisible 在运行期按设置切换。
func ApplyAppOptions(opts *application.Options) {
	opts.Mac.ApplicationShouldTerminateAfterLastWindowClosed = false
}

// AfterWindowCreated 在 v3 主窗口创建完成后调用：隐藏红绿灯、关闭 AppKit 的
// “按住背景拖窗口”，并校验 v3 的 Panel 选项确实生效。
// 返回面板校验结果，供调用方写诊断日志。
func AfterWindowCreated(w application.Window) PanelStatus {
	if w != nil {
		w.SetCloseButtonState(application.ButtonHidden)
		w.SetMinimiseButtonState(application.ButtonHidden)
		w.SetMaximiseButtonState(application.ButtonHidden)
	}
	return ConfigurePanel(Title)
}
