//go:build darwin

// Package window 封装窗口相关的跨平台能力。
package window

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// ApplyOptions 将平台相关选项写入 opts。
// macOS 策略：
//   - Frameless=false：**必须**保留 titled style，否则 NSWindowStyleMaskBorderless
//     不带系统圆角和阴影，窗口会变成直角硬边。
//   - TitleBarHiddenInset：把标题栏条隐藏为内嵌形态，保留 titled mask
//     的圆角/阴影，但视觉上标题栏不可见。
//   - 三个红绿灯按钮的隐藏由 panel_darwin.m 里 standardWindowButton.hidden=YES
//     完成，不需要在这里 Frameless。
//   - 真实的面板化 (NonactivatingPanel + FloatingLevel + 跨 Space) 由
//     panel_darwin.m 的 isa swap + setStyleMask(原mask | NonactivatingPanel)
//     完成，见 app.startup 里 convertMainWindowToPanelWithRetry。
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
