//go:build darwin

package paste

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework ApplicationServices

#include <ApplicationServices/ApplicationServices.h>

// 查询 Accessibility 授权状态。
//   prompt == 0：静默查询，不弹框（用于条件分支）
//   prompt != 0：未授权时系统会弹"请到 设置 → 隐私与安全 → 辅助功能"的提示框
// 返回值：1 = trusted，0 = not trusted。
static int paste_check_accessibility(int prompt) {
    CFStringRef keys[1]   = { kAXTrustedCheckOptionPrompt };
    CFBooleanRef values[1] = { prompt ? kCFBooleanTrue : kCFBooleanFalse };
    CFDictionaryRef opts = CFDictionaryCreate(
        kCFAllocatorDefault,
        (const void **)keys, (const void **)values, 1,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    Boolean trusted = AXIsProcessTrustedWithOptions(opts);
    CFRelease(opts);
    return trusted ? 1 : 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
)

// -----------------------------------------------------------------------------
// 为什么现在用 osascript 而不是 CGEventPost
// -----------------------------------------------------------------------------
// 历史实现用 CGEventPost(kCGHIDEventTap) 直接注入 Cmd+V 键事件。看起来很优雅，
// 但实测有两类致命问题：
//
//   1) 事件目标不可控。
//      CGEventPost 是"全局注入 HID 流"，没有"目标 app"概念——谁当 key window
//      事件就落到谁。NonactivatingPanel 化之后面板虽然不抢 active app，但
//      orderOut: 之后它**依然是 keyWindow**（AppKit 设计如此，不会因为视觉
//      隐藏就自动让位）。结果：注入的 Cmd+V 回到 GoPaste 自己的 WebView，
//      触发前端某个事件，又调回 PasteItem，形成死循环。
//
//   2) macOS HID throttle / WindowServer kill。
//      死循环一旦跑起来，每秒数次的 HID 注入会被 WindowServer 识别为异常，
//      可能直接 SIGKILL 发起者。表现：进程"无声消失"，没有 crash report、
//      没有 `log show` 事件。我们反复在日志里看到的"最后一条 sent 之后就
//      没声了"就是这个。
//
// EcoPaste（Tauri 生态里最成熟的剪贴板工具）在 macOS 上采用的做法：
//   - 不用 CGEventPost
//   - 先 resignKeyWindow 让 AppKit 把 key 状态交回"上一个 key window"的 app
//   - 然后用 `osascript -e 'tell application "System Events" to keystroke "v"
//     using command down'` 发 Cmd+V
//
// osascript 这条路线的优点：
//   - System Events 发送的是"面向当前前台 app 的按键"，不是盲注 HID
//   - AppleEvent IPC 频率低，不会被 WindowServer throttle / SIGKILL
//   - 权限要求仍然只是 Accessibility（虽然 System Events 在新系统上有时
//     会叠一层 Automation 权限，但首次调用会同样通过 Accessibility 弹框
//     引导，单次授权即可）
//
// 换路线的直接收益：
//   - 解决了"粘贴后反复触发 PasteItem"的死循环
//   - 解决了"进程无声消失"的闪退
//   - 实现简单到 15 行
// -----------------------------------------------------------------------------

// ErrNoAccessibility 当系统未授予 Accessibility 权限时返回。
// 上层看到这个错误应当：
//   - 不要隐藏面板（面板隐藏后用户还以为"粘贴成功了但没贴出来"）
//   - 弹一个引导框告诉用户去 系统设置 → 隐私与安全 → 辅助功能 勾选 GoPaste
//   - 可以用 `errors.Is(err, paste.ErrNoAccessibility)` 判断
var ErrNoAccessibility = errors.New("paste: accessibility permission not granted")

// HasAccessibility 静默查询是否授权。不弹框，可在任意时机调用（包括 UI 线程）。
// 用于：
//   - PasteItem 在隐藏面板之前预检，决定要不要直接走引导流程
//   - 启动/托盘菜单等地方显示权限状态指示
func HasAccessibility() bool {
	return C.paste_check_accessibility(0) == 1
}

// promptOnce 保证整个进程生命周期内 prompt=YES 只发生一次——
// 反复调用 AXIsProcessTrustedWithOptions(prompt=YES) 本身不会崩，但会让系统
// 弹多次"未授权"通知，视觉打扰用户。
var promptOnce sync.Once

// PromptAccessibility 在首次调用时触发系统权限弹框，让 GoPaste 被加入
// "系统设置 → 隐私与安全 → 辅助功能"列表。已经授权过则是 no-op。
// 线程安全；可以被 PasteItem 和任何 RPC 方法调用。
func PromptAccessibility() (trusted bool) {
	promptOnce.Do(func() {
		C.paste_check_accessibility(1) // 忽略返回值：主要目的是触发系统弹框
	})
	return HasAccessibility()
}

// sendPasteImpl 在 macOS 上通过 `osascript` 让 System Events 投递 Cmd+V。
//
// 契约：
//   - 未授权返回 ErrNoAccessibility（不触发弹框，由调用方决定引导时机）
//   - osascript 非零退出返回带 stderr 的错误
//
// 为什么在这里再查一次 HasAccessibility：即便 PasteItem 层已经预检过，
// 用户有可能在预检和真正 osascript 调用之间手动关闭授权。多一次 check 几乎
// 零成本（C 函数调用），能给出更清晰的错误。
func sendPasteImpl() error {
	if C.paste_check_accessibility(0) == 0 {
		return fmt.Errorf("%w; 请到 系统设置 → 隐私与安全 → 辅助功能 勾选 GoPaste", ErrNoAccessibility)
	}

	// 保留 cmd.Run() 而非 exec.Command(...).Output()：
	// 我们不关心 stdout；stderr 才包含 osascript 的报错信息。
	cmd := exec.Command("osascript", "-e",
		`tell application "System Events" to keystroke "v" using command down`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("paste: osascript failed: %w; output=%q", err, string(out))
	}
	return nil
}
