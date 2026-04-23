//go:build windows

// Package window 封装窗口相关的跨平台能力：
// 包括 Wails 启动选项、Win11 DWM 原生圆角、Windows 任务栏显隐等。
package window

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// ApplyOptions 将平台相关选项写入 opts。
// Windows：Frameless + 非透明 + DWM 原生圆角。
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
