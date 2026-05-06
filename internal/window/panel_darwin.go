//go:build darwin

package window

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers

#include <stdlib.h>   // for free()

extern void GoPasteConvertToNonactivatingPanel(const char *title);
extern void GoPasteOrderOut(const char *title);
extern void GoPasteOrderFront(const char *title);
extern void GoPasteResignKey(const char *title);
extern void GoPasteActivateForDialog(void);
extern void GoPasteDeactivateAfterDialog(void);
extern char *GoPasteSaveFileDialog(const char *title, const char *defaultName);
*/
import "C"

import "unsafe"

// ConvertToNonactivatingPanel 把 title 指定的 NSWindow 改造成
// NSPanel + NSWindowStyleMaskNonactivatingPanel。调用一次即可，内部幂等。
//
// 必须在 Wails 窗口已创建之后调用（OnStartup 里就已经可以拿到）。
// 见 panel_darwin.m 顶部注释了解为什么要这么做。
func ConvertToNonactivatingPanel(title string) {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	C.GoPasteConvertToNonactivatingPanel(ct)
}

// OrderOut 以面板化的方式隐藏主窗口（orderOut:）。
// 与 Wails 的 WindowHide 不同：不触发 [NSApp hide] / applicationShould…
// 回调，也不影响当前 active app。NonactivatingPanel 必须用这个。
func OrderOut(title string) {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	C.GoPasteOrderOut(ct)
}

// OrderFront 以面板化的方式显示主窗口（orderFrontRegardless + makeKeyWindow）。
// 不会调 activateIgnoringOtherApps:，所以下层 app 的 active 状态不受影响。
func OrderFront(title string) {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	C.GoPasteOrderFront(ct)
}

// ResignKey 让面板交还键盘输入给下面的 active app。粘贴前调用，
// 使 Cmd+V 能被目标应用收到。
func ResignKey(title string) {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	C.GoPasteResignKey(ct)
}

// ActivateForDialog 在弹出系统对话框（SaveFileDialog 等）前调用。
// NonactivatingPanel 不是 active app，系统对话框需要 active app 才能正常显示。
// 临时调用 activateIgnoringOtherApps: 让 GoPaste 成为前台应用。
func ActivateForDialog() {
	C.GoPasteActivateForDialog()
}

// DeactivateAfterDialog 对话框关闭后调用，恢复 NonactivatingPanel 的非激活状态。
func DeactivateAfterDialog() {
	C.GoPasteDeactivateAfterDialog()
}

// SaveFileDialog 在主线程原子执行"激活 → NSSavePanel → 恢复"。
// 返回用户选择的路径，取消返回空字符串。
func SaveFileDialog(title, defaultName string) string {
	ct := C.CString(title)
	cn := C.CString(defaultName)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cn))
	result := C.GoPasteSaveFileDialog(ct, cn)
	if result == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result)
}
