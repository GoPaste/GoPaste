//go:build !windows && !darwin

// Package platform 按操作系统注入 Wails 的平台专属选项。
package platform

import "github.com/wailsapp/wails/v2/pkg/options"

// ApplyOptions 在非 Windows/macOS 平台下保持默认（Frameless 可按需开关）。
func ApplyOptions(opts *options.App) {
	opts.Frameless = true
}
