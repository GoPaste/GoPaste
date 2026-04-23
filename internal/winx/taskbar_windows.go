//go:build windows

// Package winx 封装 Windows 特有的 Win32 辅助能力（窗口风格、任务栏显隐等）。
package winx

import (
	"syscall"
	"unsafe"
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	procEnumWindows        = user32.NewProc("EnumWindows")
	procGetWindowThreadPID = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextW     = user32.NewProc("GetWindowTextW")
	procGetWindowLongPtrW  = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW  = user32.NewProc("SetWindowLongPtrW")
	procShowWindow         = user32.NewProc("ShowWindow")
	procSetWindowPos       = user32.NewProc("SetWindowPos")
	procIsWindowVisible    = user32.NewProc("IsWindowVisible")

	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentPID = kernel32.NewProc("GetCurrentProcessId")
)

// GWL_EXSTYLE = -20，作为 uintptr 使用需要符号扩展。
// 直接写常量 uintptr(-20) 会被编译器判为 overflow，因此用运行时计算。
var gwlExStyle = ^uintptr(0) - 19 // = 0xFFFFFFFFFFFFFFEC on 64-bit

const (
	wsExToolWindow = 0x00000080
	wsExAppWindow  = 0x00040000

	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020

	swHide           = 0
	swShowNoActivate = 4
)

// FindMainWindow 按"属于当前进程 + 标题匹配"找到主窗口 HWND。返回 0 表示未找到。
func FindMainWindow(title string) uintptr {
	curPID, _, _ := procGetCurrentPID.Call()
	titleUTF16, _ := syscall.UTF16FromString(title)
	want := titleUTF16
	if len(want) > 0 && want[len(want)-1] == 0 {
		want = want[:len(want)-1]
	}

	var found uintptr
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		var pid uint32
		procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if uintptr(pid) != curPID {
			return 1 // 继续
		}
		buf := make([]uint16, 256)
		n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 256)
		if n == 0 || int(n) < len(want) {
			return 1
		}
		got := buf[:n]
		for i, c := range want {
			if got[i] != c {
				return 1
			}
		}
		found = hwnd
		return 0
	})
	procEnumWindows.Call(cb, 0)
	return found
}

// SetTaskbarVisible 控制主窗口在任务栏的显隐。
//
// 原理：
//   - 隐藏：加 WS_EX_TOOLWINDOW、移除 WS_EX_APPWINDOW
//   - 显示：反之
//   - 扩展风格改变只有在窗口从 hide→show 过程中才会被重算，
//     所以内部做 Hide → SetStyle → Show。
func SetTaskbarVisible(hwnd uintptr, visible bool) {
	if hwnd == 0 {
		return
	}
	ex, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExStyle)
	var newEx uintptr
	if visible {
		newEx = (ex | wsExAppWindow) &^ wsExToolWindow
	} else {
		newEx = (ex | wsExToolWindow) &^ wsExAppWindow
	}
	if newEx == ex {
		return
	}

	wasVisible, _, _ := procIsWindowVisible.Call(hwnd)
	if wasVisible != 0 {
		procShowWindow.Call(hwnd, swHide)
	}
	procSetWindowLongPtrW.Call(hwnd, gwlExStyle, newEx)
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
		uintptr(swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged))
	if wasVisible != 0 {
		procShowWindow.Call(hwnd, swShowNoActivate)
	}
}
