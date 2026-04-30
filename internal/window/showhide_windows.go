//go:build windows

package window

import (
	"context"
	goruntime "runtime"
	"syscall"
	"unsafe"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Title 主窗口标题，必须与 options.App.Title 保持一致。
const Title = "GoPaste"

// ===========================================================================
// Windows 唤起窗口踩坑史 —— 必读
// ===========================================================================
//
// 现象：全局快捷键 (Alt+`/Alt+1..6) 唤起窗口后，前端收不到任何 keydown
// 事件（左右方向键切 tab 失灵），但用鼠标点一下窗口任意位置，键盘立刻通了。
//
// === 弯路 ===
//
//	v1: 给 Wails host HWND 子类化，在 WM_SETFOCUS 时 SetFocus 到 WebView2
//	    叶子窗口。失败：跨进程/跨线程 SetFocus 是 no-op。
//	v2: 用 GW_CHILD 沿 Z-order 顶层一路向下找叶子。
//	    失败：找到的是 WebView2 的 "Intermediate D3D Window"，不接收键盘。
//	v3: 改用 EnumChildWindows 按 class name 匹配 Chrome_RenderWidgetHostHWND。
//	    失败：焦点根本不在我的进程里，匹配再准也是 no-op。
//	v4: 引入 forceForeground (AttachThreadInput) 抢前台。
//	    全局热键路径修通了，但**冷启动**时窗口可见但键盘失灵——WebView2 的
//	    WM_ACTIVATE 早期处理已跑完，不会再下放焦点。
//	v5: 冷启动 domReady 时主动调 ShowMain + SetFocus 到 Chrome_WidgetWin_0。
//	    失败：SetFocus 返回 ERROR_ACCESS_DENIED (5)。原因：WebView2 host 跑
//	    在自己的 UI 线程上，跟 Go 主线程不同；同进程跨线程 SetFocus 也违法。
//	v6: 在 SetFocus 前 AttachThreadInput 到目标窗口的线程。
//	    失败：AttachThreadInput 返回 0（attach 失败）。原因：Go runtime 的
//	    worker 线程没有创建 message queue，AttachThreadInput 要求两端都有
//	    input queue。即使 LockOSThread 也救不了已经失败的 attach。
//	v7 (current): 改用 SendMessage(host, WM_ACTIVATE+WM_SETFOCUS)。
//	    SendMessage 是 OS 跨线程同步消息派发，不需要 attach、不需要调用方
//	    有 message queue。WebView2 host 收到 WM_ACTIVATE 时跑自己的激活
//	    流程，把 keyboard focus 下放到内部 RenderWidget。这条路径等价于
//	    用户鼠标点击窗口时 OS 走的相同代码。
//
// === 真正的根因 ===
//
// Windows 自 Win98+SR1 起为防止 "焦点抢夺" 加了限制：当一个进程不是当前
// 前台进程时，它调 SetForegroundWindow 会被 OS **静默忽略**，返回 FALSE
// 但不报错。
//
// Wails 的 WindowShow 在 Windows 上调的是 ShowWindow + SetForegroundWindow。
// 我们应用平时是 background process（用户在用别的 app），全局快捷键回调里
// 调 WindowShow 时，那个 SetForegroundWindow 直接被 OS 拒掉。结果：
//
//   - ShowWindow 让窗口显示出来 ✓
//   - WindowSetAlwaysOnTop 让它压在所有窗口之上 ✓
//   - SetForegroundWindow 被拒 ✗
//   - **窗口可见但不是 foreground window，键盘焦点仍在原 app**
//   - 用户鼠标点窗口 → Windows 把前台切给我 → 键盘通了
//
// === 修复 ===
//
// 用 AttachThreadInput trick 绕过前台限制：附加到当前前台线程的 input queue
// 后，OS 认为我们 "和它是同一个线程"，SetForegroundWindow 就能成功。
// 这正是 internal/paste/focus_windows.go 里 RestorePreviousWindow 已经在
// 用的同一招（方向相反：那边是把前台让给其他 app，这边是抢回给自己）。
//
// AttachThreadInput 副作用是两个线程的 input state 临时共享，所以做完
// SetForegroundWindow 必须立刻 Detach，否则会污染键盘/鼠标状态。
//
// ===========================================================================

var (
	u32                      = syscall.NewLazyDLL("user32.dll")
	procGetForegroundWnd     = u32.NewProc("GetForegroundWindow")
	procSetForegroundWnd     = u32.NewProc("SetForegroundWindow")
	procAttachThreadInputFG  = u32.NewProc("AttachThreadInput")
	procGetWindowThreadPIDFG = u32.NewProc("GetWindowThreadProcessId")
	procFindWindowW          = u32.NewProc("FindWindowW")
	procShowWindowAsync      = u32.NewProc("ShowWindowAsync")
	procBringWindowToTop     = u32.NewProc("BringWindowToTop")
	procIsIconic             = u32.NewProc("IsIconic")
	procEnumChildWindows     = u32.NewProc("EnumChildWindows")
	procGetClassNameW        = u32.NewProc("GetClassNameW")
	procKeybdEvent           = u32.NewProc("keybd_event")
	procSendMessageW         = u32.NewProc("SendMessageW")

	k32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentTIDFG = k32.NewProc("GetCurrentThreadId")
	// procGetCurrentPID 在 taskbar_windows.go 已声明，此处复用
)

const (
	swRestore = 9
	swShow    = 5

	vkMenu        = 0x12 // VK_MENU = ALT
	keyeventfUp   = 0x0002
	keyeventfDown = 0x0000

	wmActivate    = 0x0006
	wmSetfocus    = 0x0007
	waActive      = 1
	waClickactive = 2
)

// findMainHwnd 通过窗口标题找到本应用的主 HWND。
// Wails v2 没有公开 HWND，只能靠标题查。Title 与 options.App.Title 一致。
func findMainHwnd() uintptr {
	titlePtr, _ := syscall.UTF16PtrFromString(Title)
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	return hwnd
}

// sendAltTap 模拟一次 ALT 按下抬起。
//
// 这是 Windows 上绕过前台限制的另一招（Raymond Chen 经典 trick）：
// OS 内部用 "用户最近输入" 的时间戳判断 SetForegroundWindow 是否合法，
// 当我们模拟一次 ALT 键，OS 会把当前时间戳归到我们进程，之后 SetForegroundWindow
// 不再被静默忽略。
//
// 选 ALT 是因为它不会触发任何菜单（除非窗口有菜单栏，而我们没有），
// 也不会被前端当成快捷键修饰符（释放后状态归零）。
func sendAltTap() {
	procKeybdEvent.Call(vkMenu, 0, keyeventfDown, 0)
	procKeybdEvent.Call(vkMenu, 0, keyeventfUp, 0)
}

// forceForeground 用 AttachThreadInput 绕过 SetForegroundWindow 的前台限制。
// 这是 Windows 上从 background process 抢前台的标准做法。
//
// 必要性：见文件顶部 "踩坑史 → 真正的根因"。
//
// 流程：
//  1. 找到当前前台窗口所属线程
//  2. AttachThreadInput 附加到该线程
//  3. 若窗口被最小化，先 ShowWindowAsync(SW_RESTORE)
//  4. BringWindowToTop + SetForegroundWindow（这次不会被拒）
//  5. AttachThreadInput Detach（必须做，否则污染线程 input state）
func forceForeground(hwnd uintptr) {
	if hwnd == 0 {
		return
	}

	fg, _, _ := procGetForegroundWnd.Call()
	curTID, _, _ := procGetCurrentTIDFG.Call()

	var fgTID uintptr
	if fg != 0 {
		fgTID, _, _ = procGetWindowThreadPIDFG.Call(fg, 0)
	}

	// 关键：模拟一次 ALT 按下抬起，刷新 OS 的 "最后输入时间戳"，
	// 让我们进程满足 SetForegroundWindow 的合法性检查。
	// 即使 AttachThreadInput 也成功，加这个也无副作用。
	sendAltTap()

	attached := false
	if fgTID != 0 && fgTID != curTID {
		r, _, _ := procAttachThreadInputFG.Call(curTID, fgTID, 1)
		attached = r != 0
	}
	defer func() {
		if attached {
			procAttachThreadInputFG.Call(curTID, fgTID, 0)
		}
	}()

	// 若被最小化，先恢复
	iconic, _, _ := procIsIconic.Call(hwnd)
	if iconic != 0 {
		procShowWindowAsync.Call(hwnd, swRestore)
	} else {
		procShowWindowAsync.Call(hwnd, swShow)
	}
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWnd.Call(hwnd)
}

// ShowMain 在 Windows 上唤起主窗口，确保窗口真正成为 foreground window。
//
// 必须做的事（缺一不可）：
//  1. wailsruntime.WindowShow(ctx) —— 让 Wails 翻转内部 visibility 状态、
//     发 EventsEmit 等。如果跳过，Wails 仍认为窗口隐藏，后续 WindowHide
//     不会工作。
//  2. forceForeground(hwnd) —— 用 AttachThreadInput trick 真的把窗口抢到
//     前台。这一步是关键：单靠 WindowShow 在 background process 状态下
//     不能可靠地拿到键盘焦点。
//  3. focusWebView(hwnd) —— 把 keyboard focus 显式下放到 WebView2 子窗口。
//     冷启动场景下，WebView2 的 WM_ACTIVATE 内置焦点处理在我们抢前台
//     之前就已经走完了，不会再触发；必须由我们主动 SetFocus 到子窗口，
//     键盘消息才会派给前端 (keydown/搜索框输入)。
//
// 时序：先 WindowShow（让窗口可见），再 forceForeground（抢前台），
// 最后 focusWebView（下放焦点）。反过来不行：focusWebView 必须在抢前台
// 成功后做，否则跨进程 SetFocus 是 no-op（见踩坑史 v1）。
func ShowMain(ctx context.Context) {
	if ctx == nil {
		return
	}
	// 关键：锁定 OS 线程。AttachThreadInput 是 per-thread 的状态，
	// 整个 forceForeground + focusWebView 流程必须在同一个 OS 线程上完成。
	// 否则 Go 调度器可能在两个 syscall 之间把 goroutine 切到别的线程，
	// 导致 attach 的线程和后续 SetFocus 调用的线程不是一个，操作全部无效。
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()

	wailsruntime.WindowShow(ctx)

	hwnd := findMainHwnd()
	if hwnd == 0 {
		return
	}
	forceForeground(hwnd)
	focusWebView(hwnd)
}

// HideMain 在 Windows 上直接走 Wails 的 WindowHide。
func HideMain(ctx context.Context) {
	if ctx == nil {
		return
	}
	wailsruntime.WindowHide(ctx)
}

// focusWebView 把键盘焦点显式 SetFocus 到 WebView2 的子窗口。
//
// 背景：
// 文件顶部 "踩坑史" v1~v3 之所以全部失败，是因为当时项目还没做 forceForeground，
// 进程层面就不在前台 → 跨进程 SetFocus 永远 no-op。
//
// 自从 v5 引入 forceForeground 后，热启动路径上：抢前台成功 → WebView2 自己
// 在 WM_ACTIVATE 处理里把焦点下放到 Chrome_WidgetWin → 键盘正常。
//
// 但**冷启动**时这条路断了：wails 启动早期就调了 ShowWindow（此时还不是前台
// 进程），WebView2 的 WM_ACTIVATE/初始焦点逻辑此刻就跑完了一次（没拿到焦点）；
// 等 domReady 时我们 forceForeground 抢成功，**WebView2 不会再来一次**，必须
// 我们主动 SetFocus。
//
// 此时 SetFocus 是合法的：
//   - 我们已经是前台进程（forceForeground 已完成）
//   - 调用线程是我们自己的 main thread
//   - 目标 HWND 是 WebView2 子窗口，属于本进程
//   → 同进程同线程 SetFocus 不是 no-op
//
// WebView2 的窗口树（自外向内）：
//   GoPaste (主 HWND)
//     └─ Chrome_WidgetWin_0 / Chrome_WidgetWin_1 (host)
//          └─ Chrome_RenderWidgetHostHWND  ← 这个才是真正接收键盘的窗口
//          └─ Intermediate D3D Window      (D3D 渲染层，不接收键盘)
//
// 优先 SetFocus 到 Chrome_RenderWidgetHostHWND；找不到则退回 Chrome_WidgetWin。
func focusWebView(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	// 只关心本进程的子窗口，避免误命中嵌入的其它进程窗口
	myPID, _, _ := procGetCurrentPID.Call()

	type found struct {
		render uintptr // Chrome_RenderWidgetHostHWND
		host   uintptr // Chrome_WidgetWin_*
	}
	var f found

	// 递归枚举：EnumChildWindows 默认只枚举直接子窗口，但传 hwnd=0 给它枚举
	// 桌面也不对。改用手动递归。
	var enumRec func(parent uintptr)
	enumRec = func(parent uintptr) {
		cb := syscall.NewCallback(func(child uintptr, _ uintptr) uintptr {
			// PID 过滤
			var pid uintptr
			procGetWindowThreadPIDFG.Call(child, uintptr(unsafe.Pointer(&pid)))
			if pid != myPID {
				return 1 // 继续枚举同级
			}
			// 拿 class name
			buf := make([]uint16, 128)
			n, _, _ := procGetClassNameW.Call(
				child,
				uintptr(unsafe.Pointer(&buf[0])),
				uintptr(len(buf)),
			)
			if n > 0 {
				class := syscall.UTF16ToString(buf[:n])
				switch class {
				case "Chrome_RenderWidgetHostHWND":
					if f.render == 0 {
						f.render = child
					}
				case "Chrome_WidgetWin_0", "Chrome_WidgetWin_1":
					if f.host == 0 {
						f.host = child
					}
				}
			}
			// 递归向下
			enumRec(child)
			return 1 // 继续枚举同级
		})
		procEnumChildWindows.Call(parent, cb, 0)
	}
	enumRec(hwnd)

	target := f.render
	if target == 0 {
		target = f.host
	}
	if target == 0 {
		return
	}

	// 用 SendMessage 投递 WM_ACTIVATE+WM_SETFOCUS，让 WebView2 host 在它自己
	// 的 UI 线程里跑激活逻辑（这是 OS 自己模拟用户点击窗口时走的相同路径）。
	//
	// 为什么不用 SetFocus + AttachThreadInput？
	// 实测 AttachThreadInput 在我们这个时机点常返回 0（attach 失败），原因是
	// Go runtime worker 线程没创建 message queue —— AttachThreadInput 要求
	// 两个线程都有 input queue 才合法。即使加了 LockOSThread 也救不了已经
	// 失败的 attach。
	//
	// SendMessage 走的是另一套机制：它是**跨线程同步消息**，由 OS 把消息塞进
	// 目标线程的队列并阻塞等待目标线程处理完。完全不需要调用方有 message
	// queue，也不需要 attach。WebView2 host 的 wndproc 收到 WM_ACTIVATE 时
	// 会执行内部的"激活流程"——包括把 keyboard focus 下放到 RenderWidget。
	//
	// 同时发 WM_SETFOCUS 是双保险（某些 Chromium 版本只看 WM_SETFOCUS）。
	// WPARAM = MAKEWPARAM(WA_CLICKACTIVE, 0)，模拟"用户点击激活"
	procSendMessageW.Call(target, wmActivate, waClickactive, 0)
	procSendMessageW.Call(target, wmSetfocus, 0, 0)
}

// ResignMainKey 在 Windows 上是 no-op（等价概念是把前台让给其他窗口，
// 由 HideMain + RestorePreviousWindow 实现）。
func ResignMainKey(ctx context.Context) {
	_ = ctx
}
