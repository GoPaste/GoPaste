//go:build windows

package tray

// dispatchOnMain 在 Windows 上直接同步调用。
// Windows 分支实际不会用到此函数（tray.Start 里走 go systray.Run 路径），
// 这里提供实现仅为了同包编译通过。
func dispatchOnMain(fn func()) {
	if fn != nil {
		fn()
	}
}
