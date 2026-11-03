//go:build windows

// Package window 封装窗口相关的跨平台能力：
// 包括 Wails 窗口选项、Win11 DWM 原生圆角、Windows 任务栏显隐等。
package window

import "github.com/wailsapp/wails/v3/pkg/application"

// ApplyOptions 将平台相关选项写入主窗口的 WebviewWindowOptions。
// Windows：Frameless + 实色背景（非透明 WebView）+ DWM 原生圆角。
//
// BackgroundType 必须是 Solid（v2 时代的 WebviewIsTransparent=false）：
//
//	设成透明时拉伸窗口会露出窗口底色，而 BackgroundColour 仅在进程启动时
//	按配置文件读取一次，运行中切换主题后不会更新——导致暗色→浅色后拉伸出黑色，
//	浅色→暗色后拉伸出白色。
//	拉伸白边/黑边的真正解决方案放在前端 JS：通过 CSS 把 html/body 的 background
//	设成 var(--bg)，并在 resize 事件里强制同步 backgroundColor，让 WebView2 自身
//	的背景始终跟随当前主题，不依赖 Wails 静态的 BackgroundColour。
//
// v3 下 BackdropType 只在 BackgroundTypeTranslucent 时生效，这里保持实色即可。
// Frameless 默认带 DWM 阴影/缩放热区（DisableFramelessWindowDecorations=false），
// 与 v2 的 Frameless 行为一致。
func ApplyOptions(opts *application.WebviewWindowOptions) {
	opts.Frameless = true
	opts.BackgroundType = application.BackgroundTypeSolid
	opts.Windows.DisableIcon = true
	// 在 Wails 创建窗口后，通过 DWM API 为 Win11 添加原生圆角（内部按标题轮询 HWND）
	ApplyWin11RoundCorners(opts.Title)
	// 屏蔽 Alt+Space 弹出的 Win32 系统菜单。
	// 触发场景：用户用全局热键 Alt+5 唤起窗口后，OS 内部 Alt 的 keyup
	// 没派发到窗口消息循环 → 窗口"以为 Alt 还按着" → 接下来按 Space 被
	// 解读为 Alt+Space → 弹出还原/移动/最小化/最大化/关闭菜单。
	// 详见 sysmenu_windows.go 内的注释。
	DisableAltSpaceSysMenu(opts.Title)
}

// ApplyAppOptions 将平台相关的应用级选项写入 application.Options。
// Windows 目前无需额外设置：关闭即隐藏由 WindowClosing hook 处理，
// 窗口从不真正销毁，因此不会触发 v3 的"最后一个窗口关闭即退出"。
func ApplyAppOptions(opts *application.Options) {
	_ = opts
}

// AfterWindowCreated 在 v3 主窗口创建完成（ApplicationStarted 之后）调用，
// 用于只能在原生窗口存在后才能做的设置。Windows 下圆角 / 系统菜单屏蔽
// 已在 ApplyOptions 里以"轮询 HWND"的方式异步完成，这里无事可做。
func AfterWindowCreated(w application.Window) PanelStatus {
	_ = w
	return PanelOK
}
