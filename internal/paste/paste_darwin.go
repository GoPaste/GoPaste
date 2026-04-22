//go:build darwin

package paste

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework ApplicationServices -framework Carbon
#include <ApplicationServices/ApplicationServices.h>
#include <Carbon/Carbon.h>   // kVK_ANSI_V

// -----------------------------------------------------------------------------
// 为什么不用 osascript
// -----------------------------------------------------------------------------
// 历史实现是 fork `osascript -e 'tell application "System Events" to keystroke "v" using command down'`
// 它有两个大坑：
//
//   1) 双重权限：System Events 走的是 AppleEvents / Automation 权限，外加
//      一层 Accessibility，任何一层没给都会 exit status 1，且权限弹窗时机
//      不可控。
//   2) 进程级联崩溃：osascript 出错时的错误路径在某些 macOS 版本上会把
//      调用方（Go 进程）也一起带走——没有 crash report、日志断在 exec 前，
//      正好符合我们反复见到的"before SendPaste 之后进程消失"现象。
//
// CGEventPost 直接向系统 HID 流注入 Cmd+V 键事件：
//   - 只需要 Accessibility 权限（单点授权）
//   - 同步 C 调用，无子进程
//   - 失败不会杀父进程
//
// 额外防护：HID post 要求放在 session 事件流，以便被当前前台 app 收到。
// -----------------------------------------------------------------------------

// 检查 Accessibility 权限；不弹窗，只查询。
static int paste_has_accessibility(void) {
    // kAXTrustedCheckOptionPrompt=false → 不弹系统设置，只检查
    CFStringRef keys[1] = { kAXTrustedCheckOptionPrompt };
    CFBooleanRef values[1] = { kCFBooleanFalse };
    CFDictionaryRef opts = CFDictionaryCreate(
        kCFAllocatorDefault,
        (const void **)keys, (const void **)values, 1,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    Boolean trusted = AXIsProcessTrustedWithOptions(opts);
    CFRelease(opts);
    return trusted ? 1 : 0;
}

// 合成一组 Cmd+V 按下/松开事件。成功返回 0，失败返回非 0。
static int paste_send_cmd_v(void) {
    CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateCombinedSessionState);
    if (!src) return 1;

    // key down: V
    CGEventRef vDown = CGEventCreateKeyboardEvent(src, (CGKeyCode)kVK_ANSI_V, true);
    if (!vDown) { CFRelease(src); return 2; }
    CGEventSetFlags(vDown, kCGEventFlagMaskCommand);

    // key up: V
    CGEventRef vUp = CGEventCreateKeyboardEvent(src, (CGKeyCode)kVK_ANSI_V, false);
    if (!vUp) { CFRelease(vDown); CFRelease(src); return 3; }
    CGEventSetFlags(vUp, kCGEventFlagMaskCommand);

    // 发到 HID 事件流（等价于键盘产生的事件），目标是当前前台 app。
    CGEventPost(kCGHIDEventTap, vDown);
    CGEventPost(kCGHIDEventTap, vUp);

    CFRelease(vDown);
    CFRelease(vUp);
    CFRelease(src);
    return 0;
}
*/
import "C"

import "fmt"

// sendPasteImpl 在 macOS 上通过 CGEventPost 注入 Cmd+V。
// 依赖 Accessibility 权限；若未授权，返回带明确提示的错误，上层 UI 可以引导用户到系统设置。
func sendPasteImpl() error {
	if C.paste_has_accessibility() == 0 {
		return fmt.Errorf("paste: accessibility permission not granted; " +
			"请到 系统设置 → 隐私与安全 → 辅助功能 勾选 GoPaste")
	}
	if rc := C.paste_send_cmd_v(); rc != 0 {
		return fmt.Errorf("paste: CGEventPost failed rc=%d", int(rc))
	}
	return nil
}
