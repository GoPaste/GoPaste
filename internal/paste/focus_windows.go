//go:build windows

package paste

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

// 在 Windows 上，"自动粘贴"必须解决一个核心问题：
//
//   Wails 窗口 WindowShow 时夺走前台焦点；WindowHide 时 Windows
//   不会自动把焦点切回上一个窗口。结果 SendInput(Ctrl+V) 发到了
//   桌面/无焦点窗口，永远不会真正粘贴。
//
// 因此面板每次"被显示"前，应当记录当时的前台窗口句柄；
// PasteItem 隐藏面板后，先 SetForegroundWindow 切回去，再发 Ctrl+V。

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	procGetForegroundWnd   = user32.NewProc("GetForegroundWindow")
	procSetForegroundWnd   = user32.NewProc("SetForegroundWindow")
	procAttachThreadInput  = user32.NewProc("AttachThreadInput")
	procGetWindowThreadPID = user32.NewProc("GetWindowThreadProcessId")
	procIsWindow           = user32.NewProc("IsWindow")
	procShowWindow         = user32.NewProc("ShowWindow")
	procBringWinToTop      = user32.NewProc("BringWindowToTop")
	procSetFocus           = user32.NewProc("SetFocus")
	procGetGUIThreadInfo   = user32.NewProc("GetGUIThreadInfo")
	procSendMessage        = user32.NewProc("SendMessageW")

	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentTID  = kernel32.NewProc("GetCurrentThreadId")
)

// 键盘焦点提示（focus rectangle）UI 状态控制。
//
// 目标应用被 SetForegroundWindow 激活的瞬间，会给"记忆的默认控件"（资源
// 管理器上是工具栏"新建"按钮）画出键盘焦点框。这发生在我们把焦点纠正到
// 文件列表之前，于是用户会看到"新建蓝框闪一下"。
//
// WM_CHANGEUISTATE 由应用主动发出，系统据此更新窗口层级的 UI 状态并向下
// 广播 WM_UPDATEUISTATE 给所有子控件。发送 UIS_SET | UISF_HIDEFOCUS 让目标
// 窗口进入"不显示键盘焦点提示"状态——这正是用户用鼠标操作时的默认状态，
// 因此不会带来突兀观感，却能消除激活瞬间的焦点框绘制。
const (
	wmChangeUIState = 0x0127
	uisSet          = 1
	uisfHideFocus   = 0x1
	uisfHideAccel   = 0x2
)

// guiThreadInfo 对应 Win32 GUITHREADINFO。用于取某个 GUI 线程当前真正拥有
// 键盘焦点的子控件句柄（hwndFocus），而不仅仅是顶层窗口。
//
// 64 位布局：cbSize(4)+flags(4) + 6*HWND(8) + RECT(16) = 72 字节。
type guiThreadInfo struct {
	cbSize        uint32
	flags         uint32
	hwndActive    uintptr
	hwndFocus     uintptr
	hwndCapture   uintptr
	hwndMenuOwner uintptr
	hwndMoveSize  uintptr
	hwndCaret     uintptr
	rcCaret       [4]int32
}

// PreviousWindow 跨平台句柄。Windows 上记录顶层窗口 HWND，以及呼出面板前
// 真正拥有键盘焦点的子控件 HWND。
//
// 为什么要额外记 focusHwnd：
// GetForegroundWindow 返回的是顶层窗口（如资源管理器的 CabinetWClass）。
// 仅用 SetForegroundWindow 激活顶层窗口时，键盘焦点会交给该窗口"记忆的默认
// 控件"。GoPaste 反复抢焦点/还焦点后，这个记忆会漂移到工具栏按钮（如
// 资源管理器"新建"），导致粘贴键 Shift+Insert 落到按钮上而非文件列表，
// 表现为"第一次能粘、后面粘不出"。记录并精确恢复真实焦点子控件可根治。
type PreviousWindow struct {
	hwnd      uintptr
	focusHwnd uintptr
}

// IsValid 句柄是否有效。
func (p PreviousWindow) IsValid() bool {
	if p.hwnd == 0 {
		return false
	}
	r, _, _ := procIsWindow.Call(p.hwnd)
	return r != 0
}

// CapturePreviousWindow 记录当前前台窗口（即"被遮挡的"应用），便于稍后还原焦点。
// 同时抓取该窗口线程当前真正的焦点子控件，供 RestorePreviousWindow 精确恢复。
func CapturePreviousWindow() (PreviousWindow, error) {
	r, _, _ := procGetForegroundWnd.Call()
	if r == 0 {
		return PreviousWindow{}, fmt.Errorf("focus: GetForegroundWindow returned 0")
	}
	pw := PreviousWindow{hwnd: r}

	// 抢占式抑制焦点框（根治"新建蓝框闪一下"）：
	// 此刻目标窗口仍是前台、正常处理消息，先把它的 UI 状态设为"不显示键盘
	// 焦点提示"。后面粘贴流程 WindowHide(GoPaste) 会触发系统自动激活该窗口，
	// 因为它此时已是 hide-focus 状态，激活瞬间根本不会绘制默认控件（资源管理器
	// "新建"）的焦点框——从根本上消除竞态。放在这里比放在 RestorePreviousWindow
	// 更可靠：后者发生在 WindowHide 之后，已有 20ms 空窗期可能让焦点框先被画出来。
	suppressFocusRect(pw.hwnd)

	// 取前台窗口所属 GUI 线程当前聚焦的子控件（文件列表 / 编辑框等）。
	// 这一步此时 GoPaste 还没抢焦点，前台仍是目标应用，拿到的就是用户
	// 上一次操作停留的控件——正是我们希望粘贴键作用的地方。
	if tid, _, _ := procGetWindowThreadPID.Call(r, 0); tid != 0 {
		var gui guiThreadInfo
		gui.cbSize = uint32(unsafe.Sizeof(gui))
		if ret, _, _ := procGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gui))); ret != 0 {
			pw.focusHwnd = gui.hwndFocus
		}
	}
	return pw, nil
}

// RestorePreviousWindow 把焦点切回给定窗口。
//
// 直接调用 SetForegroundWindow 在「前台被别的进程持有」时会被 Windows 拒绝
// （从 Win98+SR1 起为防止"焦点抢夺"加了限制）。可靠做法是先 AttachThreadInput
// 把当前线程附加到目标窗口所属线程，再 SetForegroundWindow，最后 Detach。
func RestorePreviousWindow(p PreviousWindow) error {
	if !p.IsValid() {
		return fmt.Errorf("focus: invalid hwnd")
	}

	curTID, _, _ := procGetCurrentTID.Call()
	targetTID, _, _ := procGetWindowThreadPID.Call(p.hwnd, 0)
	if targetTID == 0 {
		return fmt.Errorf("focus: GetWindowThreadProcessId failed")
	}

	if curTID != targetTID {
		_, _, _ = procAttachThreadInput.Call(curTID, targetTID, 1)
		defer procAttachThreadInput.Call(curTID, targetTID, 0)
	}

	// SW_SHOW = 5；万一窗口被最小化/隐藏，先确保可见
	_, _, _ = procShowWindow.Call(p.hwnd, 5)
	_, _, _ = procBringWinToTop.Call(p.hwnd)

	// 关键：先把键盘焦点精确设到"呼出面板前真正聚焦的子控件"（文件列表），
	// 再激活顶层窗口。Windows 只在「目标线程还没有任何子控件持有焦点」时，
	// 才会把焦点交给默认控件（资源管理器"新建"按钮，BS_DEFPUSHBUTTON 会画
	// 出蓝色高亮）。我们提前把焦点放在文件列表上，激活时便沿用该焦点，
	// 默认按钮永远拿不到焦点 → 从根源消除"新建蓝框闪一下"。
	// 必须在 AttachThreadInput 生效期间调用，SetFocus 才能跨线程作用于目标控件。
	if p.focusHwnd != 0 {
		if ok, _, _ := procIsWindow.Call(p.focusHwnd); ok != 0 {
			_, _, _ = procSetFocus.Call(p.focusHwnd)
		}
	}

	r, _, err := procSetForegroundWnd.Call(p.hwnd)
	if r == 0 {
		return fmt.Errorf("focus: SetForegroundWindow failed: %v", err)
	}

	// 再保险地发一次 hide-focus（CapturePreviousWindow 已抢占式发过，
	// 这里幂等兜底，确保连文件列表的虚线焦点框都不绘制）。
	suppressFocusRect(p.hwnd)

	// 再小睡一下让前台真正切完，提高 SendInput 命中率
	time.Sleep(50 * time.Millisecond)
	return nil
}

// PreRestore 在隐藏本窗口「之前」调用：把目标线程的键盘焦点预先"预约"到我们
// 记录的真实子控件（文件列表）。
//
// 为什么必须抢在 WindowHide 之前：本窗口隐藏后，系统会把前台交还给上一个窗口
// （目标）。若那一刻目标线程没有任何子控件持有焦点，Windows 会聚焦其默认控件
// （Explorer 的"新建"按钮），于是画出蓝色高亮——这正是"新建蓝框闪一下"。
// 提前把焦点设到文件列表，随后系统自动激活目标窗口时便沿用该焦点，不再触发
// 默认按钮高亮，从根本上消除那一帧闪烁。
//
// 必须在 AttachThreadInput 生效期间调用，才能把焦点跨线程设到目标控件。
func PreRestore(p PreviousWindow) {
	if p.hwnd == 0 || p.focusHwnd == 0 {
		return
	}
	if ok, _, _ := procIsWindow.Call(p.focusHwnd); ok == 0 {
		return
	}
	curTID, _, _ := procGetCurrentTID.Call()
	targetTID, _, _ := procGetWindowThreadPID.Call(p.hwnd, 0)
	if targetTID == 0 || curTID == targetTID {
		return
	}
	_, _, _ = procAttachThreadInput.Call(curTID, targetTID, 1)
	defer procAttachThreadInput.Call(curTID, targetTID, 0)
	_, _, _ = procSetFocus.Call(p.focusHwnd)
}

// 防止"未使用"告警
var _ = unsafe.Sizeof(PreviousWindow{})

// suppressFocusRect 让目标窗口进入"不显示键盘焦点提示"状态（等同鼠标操作时的
// 默认外观）。通过 WM_CHANGEUISTATE 由顶层窗口广播到所有子控件，可抑制激活
// 瞬间默认控件（资源管理器"新建"按钮）绘制焦点框。对窗口是同步且幂等的。
func suppressFocusRect(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	if ok, _, _ := procIsWindow.Call(hwnd); ok == 0 {
		return
	}
	wParam := uintptr(uisSet | ((uisfHideFocus | uisfHideAccel) << 16))
	_, _, _ = procSendMessage.Call(hwnd, wmChangeUIState, wParam, 0)
}
