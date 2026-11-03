//go:build darwin

package window

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers -framework WebKit

#include <stdlib.h>   // for free()

extern int GoPasteConfigurePanel(const char *title);
extern void GoPasteActivateForDialog(void);
extern void GoPasteDeactivateAfterDialog(void);
extern char *GoPasteSaveFileDialog(const char *title, const char *defaultName, const char *allowedExt);
*/
import "C"

import "unsafe"

// ConfigurePanel 做主面板的一次性外观配置：隐藏红绿灯，并禁止 AppKit 的
// “按住窗口背景拖动”（窗口拖动由前端 drag-region 负责）。
// 幂等，可在启动阶段重试。返回窗口状态，供调用方写诊断日志。
func ConfigurePanel(title string) PanelStatus {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	return PanelStatus(C.GoPasteConfigurePanel(ct))
}

// ActivateForDialog 在弹出系统对话框（NSAlert / NSSavePanel 等）前调用。
// NonactivatingPanel 不是 active app，系统对话框需要 active app 才能正常显示并接收输入。
// 调用方必须在对话框结束后调用 DeactivateAfterDialog，否则应用会一直保持 active，
// 破坏“面板不抢前台”的前提。
func ActivateForDialog() {
	C.GoPasteActivateForDialog()
}

// DeactivateAfterDialog 对话框关闭后调用，恢复 NonactivatingPanel 的非激活状态。
func DeactivateAfterDialog() {
	C.GoPasteDeactivateAfterDialog()
}

// SaveFileDialog 在主线程原子执行“激活 → NSSavePanel → 恢复”。
// ext 为不带点的扩展名（"json" / "zip" / "png"），空字符串表示不限制类型。
// 返回用户选择的路径，取消返回空字符串。
func SaveFileDialog(title, defaultName, ext string) string {
	ct := C.CString(title)
	cn := C.CString(defaultName)
	ce := C.CString(ext)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cn))
	defer C.free(unsafe.Pointer(ce))
	result := C.GoPasteSaveFileDialog(ct, cn, ce)
	if result == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result)
}

// HasNativeSaveDialog 报告当前平台是否必须用本包的原生保存对话框。
// macOS 上 NonactivatingPanel 不是 active app，v3 的 SaveFile 不会先激活应用，
// 对话框可能弹不出来或无法输入，所以统一走 SaveFileDialog。
func HasNativeSaveDialog() bool { return true }
