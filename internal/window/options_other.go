//go:build !windows && !darwin

// Package window 封装窗口相关的跨平台能力。
package window

import "github.com/wailsapp/wails/v2/pkg/options"

// ApplyOptions 在非 Windows/macOS 平台下保持默认（Frameless 可按需开关）。
func ApplyOptions(opts *options.App) {
	opts.Frameless = true
}
