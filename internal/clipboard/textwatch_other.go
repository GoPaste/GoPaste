//go:build !darwin

package clipboard

import (
	"context"

	"golang.design/x/clipboard"
)

// startTextWatch 非 darwin 平台沿用 golang.design/x/clipboard 实现。
// 仅 darwin 因 NSPasteboard 并发竞态需要自研（见 textwatch_darwin.go）。
func startTextWatch(ctx context.Context) <-chan []byte {
	return clipboard.Watch(ctx, clipboard.FmtText)
}
