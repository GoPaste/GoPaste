//go:build windows

package tray

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows 上通过 Shell_NotifyIcon 的 NIM_DELETE / NIM_ADD 来隐藏/显示
// 通知区域图标，而非 systray.Quit()/Start()——后者受 sync.Once 限制。
//
// 思路：systray 的消息循环窗口始终存活，我们只操作通知区域图标本身。
// 这和 Tauri 的 tray_icon.set_visible(false/true) 是同等效果。

var (
	shell32           = windows.NewLazySystemDLL("Shell32.dll")
	pShell_NotifyIcon = shell32.NewProc("Shell_NotifyIconW")

	user32        = windows.NewLazySystemDLL("User32.dll")
	pFindWindowEx = user32.NewProc("FindWindowExW")

	visMu   sync.Mutex
	visible = true // systray 启动后图标默认已添加
)

const (
	nimAdd    = 0x00000000
	nimDelete = 0x00000002

	nifMessage = 0x00000001

	wmUser = 0x0400
)

// nidCompat 和 fyne.io/systray 内部 notifyIconData 结构一致。
// 我们只需要 Wnd + ID 字段做 NIM_DELETE，NIM_ADD 时还需要 Flags + CallbackMessage。
type nidCompat struct {
	Size                       uint32
	Wnd                        windows.Handle
	ID, Flags, CallbackMessage uint32
	Icon                       windows.Handle
	Tip                        [128]uint16
	State, StateMask           uint32
	Info                       [256]uint16
	Timeout, Version           uint32
	InfoTitle                  [64]uint16
	InfoFlags                  uint32
	GuidItem                   windows.GUID
	BalloonIcon                windows.Handle
}

// findSystrayHwnd 通过窗口类名找到 fyne.io/systray 创建的隐藏消息窗口。
// systray_windows.go 注册的类名是 "SystrayClass"。
func findSystrayHwnd() windows.Handle {
	cn, _ := windows.UTF16PtrFromString("SystrayClass")
	h, _, _ := pFindWindowEx.Call(0, 0, uintptr(unsafe.Pointer(cn)), 0)
	return windows.Handle(h)
}

func setIconVisible(show bool) {
	visMu.Lock()
	defer visMu.Unlock()

	if show == visible {
		return // 幂等
	}

	hwnd := findSystrayHwnd()
	if hwnd == 0 {
		return // systray 尚未初始化
	}

	nid := nidCompat{Wnd: hwnd, ID: 100} // systray 固定使用 ID=100
	nid.Size = uint32(unsafe.Sizeof(nid))

	if !show {
		// 从通知区域删除图标，但不销毁 systray 的消息循环窗口
		pShell_NotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
		visible = false
	} else {
		// 重新添加图标到通知区域
		// 必须设置 NIF_MESSAGE + CallbackMessage，这样 systray 的 wndProc 才能收到点击事件
		nid.Flags = nifMessage
		nid.CallbackMessage = wmUser + 1 // systray 固定的回调消息号
		pShell_NotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
		visible = true
		// 调用方（tray.SetVisible）会紧接着重新 SetIcon/SetTooltip，
		// 因为 NIM_ADD 创建的图标不携带这些属性。
	}
}
