//go:build !darwin && !windows

package window

import "github.com/wailsapp/wails/v3/pkg/application"

// Title 主窗口标题，必须与 main.go 里 WebviewWindowOptions.Title 保持一致。
const Title = "GoPaste"

// ShowMain 在 Linux 上直接走 v3 的 Show + Focus。
func ShowMain(w application.Window) {
	if w == nil {
		return
	}
	w.Show()
	w.Focus()
}

// HideMain 在 Linux 上直接走 v3 的 Hide。
func HideMain(w application.Window) {
	if w == nil {
		return
	}
	w.Hide()
}

// ResignMainKey 在非 macOS 上是 no-op（Windows/Linux 的等价概念是
// 把前台让给其他窗口，这里通过 HideMain + RestorePreviousWindow 实现）。
func ResignMainKey(w application.Window) {
	_ = w
}
