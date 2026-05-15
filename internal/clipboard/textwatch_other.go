//go:build !darwin && !windows

package clipboard

import (
	"context"

	"golang.design/x/clipboard"
)

// startTextWatch 非 darwin/windows 平台沿用 golang.design/x/clipboard 实现。
// darwin 因 NSPasteboard 并发竞态需要自研（见 textwatch_darwin.go）。
// windows 因 readImage access violation 崩溃，整套剪贴板读取都已自研
// （见 textwatch_windows.go / imagewatch_windows.go）。
func startTextWatch(ctx context.Context) <-chan []byte {
	return clipboard.Watch(ctx, clipboard.FmtText)
}
