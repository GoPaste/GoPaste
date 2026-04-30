//go:build !darwin && !windows

package window

import (
	"context"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Title 主窗口标题，必须与 options.App.Title 保持一致。
const Title = "GoPaste"

// ShowMain 在非 macOS 上直接走 Wails 的 WindowShow。
func ShowMain(ctx context.Context) {
	if ctx == nil {
		return
	}
	wailsruntime.WindowShow(ctx)
}

// HideMain 在非 macOS 上直接走 Wails 的 WindowHide。
func HideMain(ctx context.Context) {
	if ctx == nil {
		return
	}
	wailsruntime.WindowHide(ctx)
}

// ResignMainKey 在非 macOS 上是 no-op（Windows/Linux 的等价概念是
// 把前台让给其他窗口，这里通过 HideMain + RestorePreviousWindow 实现）。
func ResignMainKey(ctx context.Context) {
	_ = ctx
}
