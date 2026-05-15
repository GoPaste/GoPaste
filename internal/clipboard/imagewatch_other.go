//go:build !darwin && !windows

package clipboard

import (
	"context"

	"golang.design/x/clipboard"
)

// startImageWatch 非 macOS/Windows 平台沿用 golang.design/x/clipboard 的 PNG 监听。
// Linux 下 PNG 是剪切板图片的默认格式，第三方库的实现足以覆盖。
// （darwin 因系统截图写入 TIFF，需要自研轮询 + TIFF→PNG 转换，见 imagewatch_darwin.go）
// （windows 因 readImage 存在 access violation 崩溃，自研实现见 imagewatch_windows.go）
func startImageWatch(ctx context.Context) <-chan []byte {
	return clipboard.Watch(ctx, clipboard.FmtImage)
}
