//go:build darwin

package tray

/*
#cgo LDFLAGS: -framework Foundation

extern void DispatchOnMain(void);
*/
import "C"

var pendingMainFn func()

//export goRunPendingMainFn
func goRunPendingMainFn() {
	fn := pendingMainFn
	pendingMainFn = nil
	if fn != nil {
		fn()
	}
}

// dispatchOnMain 将 fn 调度到 macOS 主线程执行（非阻塞）。
// systray.nativeStart 必须在主线程执行，否则托盘创建会失败/崩溃。
func dispatchOnMain(fn func()) {
	pendingMainFn = fn
	C.DispatchOnMain()
}
