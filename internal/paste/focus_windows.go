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

	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentTID  = kernel32.NewProc("GetCurrentThreadId")
)

// PreviousWindow 跨平台句柄。Windows 上是 HWND。
type PreviousWindow struct {
	hwnd uintptr
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
func CapturePreviousWindow() (PreviousWindow, error) {
	r, _, _ := procGetForegroundWnd.Call()
	if r == 0 {
		return PreviousWindow{}, fmt.Errorf("focus: GetForegroundWindow returned 0")
	}
	return PreviousWindow{hwnd: r}, nil
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
	r, _, err := procSetForegroundWnd.Call(p.hwnd)
	if r == 0 {
		return fmt.Errorf("focus: SetForegroundWindow failed: %v", err)
	}

	// 再小睡一下让前台真正切完，提高 SendInput 命中率
	time.Sleep(50 * time.Millisecond)
	return nil
}

// 防止"未使用"告警
var _ = unsafe.Sizeof(PreviousWindow{})
