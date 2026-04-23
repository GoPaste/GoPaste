//go:build windows

// Package platform 按操作系统注入 Wails 的平台专属选项。
package platform

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"gopaste/internal/winx"
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
	winx.ApplyWin11RoundCorners(opts.Title)
}
