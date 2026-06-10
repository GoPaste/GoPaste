//go:build darwin

package extensions

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices -framework IOKit -framework Carbon

#include <stdlib.h>

extern void GoPasteInstallCmdQGuard(void);
extern void GoPasteSetCmdQBehavior(const char *behavior);
extern void GoPasteSetCmdQConfirmWindowMs(int ms);

extern int  GoPasteCmdQTapAuthStatus(void);
extern int  GoPasteCmdQTapRequestAccess(void);
extern int  GoPasteCmdQTapStart(void);
extern void GoPasteCmdQTapStop(void);
extern void GoPasteCmdQOpenInputMonitoringPrefs(void);
extern void GoPasteDebugLog(const char *msg);
*/
import "C"

import (
	"sync"
	"unsafe"
)

var (
	cbMu sync.RWMutex
	cb   OnCmdQHandled
)

// InstallCmdQGuard 在进程内安装 Cmd+Q 拦截钩子（仅 L1/L2，自身前台有效）。幂等。
// 必须在 wails 启动后调用（此时 NSApp.mainMenu 已就绪）。
func InstallCmdQGuard(callback OnCmdQHandled) {
	cbMu.Lock()
	cb = callback
	cbMu.Unlock()
	C.GoPasteInstallCmdQGuard()
}

// SetCmdQBehavior 更新当前 Cmd+Q 策略。可在任意时刻调用。
func SetCmdQBehavior(b CmdQBehavior) {
	cs := C.CString(string(b))
	defer C.free(unsafe.Pointer(cs))
	C.GoPasteSetCmdQBehavior(cs)
}

// SetCmdQConfirmWindowMs 设置二次确认的时间窗口（毫秒）。默认 1500ms。
func SetCmdQConfirmWindowMs(ms int) {
	if ms < 300 {
		ms = 300
	}
	if ms > 10000 {
		ms = 10000
	}
	C.GoPasteSetCmdQConfirmWindowMs(C.int(ms))
}

// Supported 在 macOS 返回 true。
func Supported() bool { return true }

// --- L0 全局拦截（CGEventTap + 输入监控权限） ---

// CmdQTapAuthStatus 查询输入监控授权状态。
func CmdQTapAuthStatus() TapAuthStatus {
	switch int(C.GoPasteCmdQTapAuthStatus()) {
	case 1:
		return TapAuthGranted
	case 2:
		return TapAuthDenied
	default:
		return TapAuthUnknown
	}
}

// CmdQTapRequestAccess 首次触发系统授权弹窗；
// 用户之前已选过（允许或拒绝），调用无弹窗，返回当前状态的 granted 值。
func CmdQTapRequestAccess() bool {
	return C.GoPasteCmdQTapRequestAccess() == 1
}

// CmdQTapStart 启用 L0 全局 Cmd+Q 拦截。
// 未授权时返回 false；调用方应先判权限并引导用户。
func CmdQTapStart() bool {
	return C.GoPasteCmdQTapStart() == 1
}

// CmdQTapStop 停用 L0 全局拦截，回退到 L1/L2（仅 GoPaste 自身前台生效）。
func CmdQTapStop() {
	C.GoPasteCmdQTapStop()
}

// OpenInputMonitoringPrefs 打开系统设置 → 输入监控面板，引导用户授权。
func OpenInputMonitoringPrefs() {
	C.GoPasteCmdQOpenInputMonitoringPrefs()
}

// DebugLog 让外部代码能直接走 NSLog 打日志（绕过任何 Go 侧 sink）。
// 仅用于排错。
func DebugLog(msg string) {
	cs := C.CString(msg)
	defer C.free(unsafe.Pointer(cs))
	C.GoPasteDebugLog(cs)
}

// GoPasteCmdQNotify C 侧在 Cmd+Q 被拦截时调用此函数，通知 Go 侧当前原因。
//
//export GoPasteCmdQNotify
func GoPasteCmdQNotify(creason *C.char) {
	reason := ""
	if creason != nil {
		reason = C.GoString(creason)
	}
	cbMu.RLock()
	fn := cb
	cbMu.RUnlock()
	if fn != nil {
		// 回调里可能会走 wailsruntime.EventsEmit，放到 goroutine 里避免阻塞主线程。
		go fn(reason)
	}
}
