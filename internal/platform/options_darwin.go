//go:build darwin

// Package platform 按操作系统注入 Wails 的平台专属选项。
package platform

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// ApplyOptions 将平台相关选项写入 opts。
// macOS：使用 TitleBarHiddenInset，隐藏标题栏但保留左上角红黄绿三个系统按钮，
// 窗口保留 NSWindow 原生圆角和阴影。
func ApplyOptions(opts *options.App) {
	opts.Frameless = false
	opts.Mac = &mac.Options{
		TitleBar: mac.TitleBarHiddenInset(),
		About: &mac.AboutInfo{
			Title:   "GoPaste",
			Message: "GoPaste — A cross-platform clipboard manager.",
		},
	}
}
