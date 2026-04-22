//go:build darwin

package paste

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>

// 返回当前最前台应用的 PID（不含本进程自身）。0 表示失败/无。
static int paste_get_frontmost_pid(void) {
    NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
    if (!app) return 0;
    return (int)[app processIdentifier];
}

// 把指定 PID 的应用切到前台。返回 1 成功 / 0 失败。
static int paste_activate_pid(int pid) {
    NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
    if (!app) return 0;
    BOOL ok = [app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
    return ok ? 1 : 0;
}
*/
import "C"

import (
	"fmt"
	"time"
)

// PreviousWindow 跨平台句柄。Mac 上是被遮挡的应用 PID。
type PreviousWindow struct {
	pid int
}

// IsValid 是否有效。
func (p PreviousWindow) IsValid() bool { return p.pid > 0 }

// CapturePreviousWindow 抓当前最前台 app 的 PID。
func CapturePreviousWindow() (PreviousWindow, error) {
	pid := int(C.paste_get_frontmost_pid())
	if pid <= 0 {
		return PreviousWindow{}, fmt.Errorf("focus: no frontmost app")
	}
	return PreviousWindow{pid: pid}, nil
}

// RestorePreviousWindow 把焦点切回先前的应用。
func RestorePreviousWindow(p PreviousWindow) error {
	if !p.IsValid() {
		return fmt.Errorf("focus: invalid pid")
	}
	if C.paste_activate_pid(C.int(p.pid)) == 0 {
		return fmt.Errorf("focus: activate pid=%d failed", p.pid)
	}
	// 等焦点切完
	time.Sleep(80 * time.Millisecond)
	return nil
}
