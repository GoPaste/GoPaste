//go:build darwin

package window

/*
#cgo LDFLAGS: -framework Cocoa

extern void GoPasteSetDockVisible(int visible);
*/
import "C"

// FindMainWindow 在 macOS 上不需要 HWND，返回 0。
func FindMainWindow(title string) uintptr { return 0 }

// SetTaskbarVisible 在 macOS 上切换 Dock 图标的显隐。
// visible=true  → NSApplicationActivationPolicyRegular（有 Dock 图标）
// visible=false → NSApplicationActivationPolicyAccessory（无 Dock 图标）
func SetTaskbarVisible(hwnd uintptr, visible bool) {
	v := C.int(0)
	if visible {
		v = 1
	}
	C.GoPasteSetDockVisible(v)
}
