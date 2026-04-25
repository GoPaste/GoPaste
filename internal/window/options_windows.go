//go:build windows

// Package window 封装窗口相关的跨平台能力：
// 包括 Wails 启动选项、Win11 DWM 原生圆角、Windows 任务栏显隐等。
package window

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// ApplyOptions 将平台相关选项写入 opts。
// Windows：Frameless + 非透明 WebView + DWM 原生圆角。
//
// WebviewIsTransparent 必须为 false：
//   设成 true 时拉伸窗口会露出 Wails 的 BackgroundColour，而该值仅在进程启动时
//   读取一次配置文件，运行中切换主题后不会更新——导致暗色→浅色后拉伸出黑色，
//   浅色→暗色后拉伸出白色。
//   拉伸白边/黑边的真正解决方案放在前端 JS：通过 CSS 把 html/body 的 background
//   设成 var(--bg)，并在 resize 事件里强制同步 backgroundColor，让 WebView2 自身
//   的背景始终跟随当前主题，不依赖 Wails 静态的 BackgroundColour。
func ApplyOptions(opts *options.App) {
	opts.Frameless = true
	opts.Windows = &windows.Options{
		WebviewIsTransparent: false,
		WindowIsTranslucent:  false,
		DisableWindowIcon:    true,
	}
	// 在 wails 创建窗口后，通过 DWM API 为 Win11 添加原生圆角
	ApplyWin11RoundCorners(opts.Title)
}
