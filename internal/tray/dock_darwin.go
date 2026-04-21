//go:build darwin

package tray

/*
#cgo LDFLAGS: -framework Cocoa

extern void InstallDockDelegate(void);
extern void goDockClicked(void);
*/
import "C"

var globalDockCallback func()

//export goDockClicked
func goDockClicked() {
	if globalDockCallback != nil {
		globalDockCallback()
	}
}

// SetDockClickCallback 注册 Dock 图标点击回调并安装 ObjC delegate。
// 需在 Wails startup（NSApp 已初始化后）调用。
func SetDockClickCallback(fn func()) {
	globalDockCallback = fn
	C.InstallDockDelegate()
}
