//go:build darwin

package paste

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <dispatch/dispatch.h>
#include <pthread.h>

// -----------------------------------------------------------------------------
// 背景 / 为什么这样写
// -----------------------------------------------------------------------------
// macOS 上粘贴前恢复焦点踩过三种坑：
//
//   1) 在自建串行队列上跑 AppKit API
//      → 新版 macOS 会 __builtin_trap，进程被内核 kill、无日志。
//
//   2) dispatch_sync(main_queue, ...) 从 cgo 线程等主队列
//      → 在 Wails v2 下，调用链里 WindowHide 等操作可能暂时让主线程无法
//        pump main queue，于是 dispatch_sync 永远不返回，系统看门狗
//        最终把进程杀掉（同样不落 crash report）。
//
//   3) 直接在 cgo 线程调 -[NSRunningApplication activateWithOptions:]
//      → 等同于 (1)。
//
// 结论：用 dispatch_async 异步扔到主队列、不阻塞调用方。我们只是"让
// 前一个 app 前台起来"，本来就不在乎它是否瞬时完成——Go 这边会睡
// 80ms 让焦点切换落地，已经足够。
//
// 为什么不用 pthread_main_np + 就地执行的组合：cgo 线程绝大多数
// 情况下不是主线程，这个分支白写；而若真在主线程，dispatch_async
// 也能正确执行（runloop 下一轮 tick 就跑），不会死锁。统一一条路径
// 更稳。
// -----------------------------------------------------------------------------

// 返回当前最前台应用的 PID。
// frontmostApplication 的内部读取对外线程安全，这里不强制走主队列，
// 避免被主线程卡住时连抓取都失败。
static int paste_get_frontmost_pid(void) {
    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app) {
            return (int)[app processIdentifier];
        }
    }
    return 0;
}

// 把指定 PID 的应用切到前台。
// 异步派发到主队列执行；Go 侧等 80ms 让 AppKit 处理完。
// 返回值恒为 1（"已调度"），真正的成功与否靠上层看粘贴效果。
static int paste_activate_pid(int pid) {
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
            if (app) {
                [app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
            }
        }
    });
    return 1;
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
