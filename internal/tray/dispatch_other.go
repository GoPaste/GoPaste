//go:build !darwin && !windows

package tray

// dispatchOnMain 在非 macOS 平台上直接调用 fn（Linux 的 systray 内部会自行处理线程）。
func dispatchOnMain(fn func()) {
	if fn != nil {
		fn()
	}
}
