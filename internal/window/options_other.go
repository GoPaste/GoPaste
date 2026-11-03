//go:build !windows && !darwin

// Package window 封装窗口相关的跨平台能力。
package window

import "github.com/wailsapp/wails/v3/pkg/application"

// ApplyOptions 在 Linux 下使用无边框窗口（与 v2 一致）。
func ApplyOptions(opts *application.WebviewWindowOptions) {
	opts.Frameless = true
}

// ApplyAppOptions 在 Linux 下无需额外设置。
func ApplyAppOptions(opts *application.Options) {
	_ = opts
}

// AfterWindowCreated 在 Linux 下无事可做。
func AfterWindowCreated(w application.Window) PanelStatus {
	_ = w
	return PanelOK
}
