//go:build windows

package window

import (
	"sync"
	"syscall"
	"time"
)

// DisableAltSpaceSysMenu 屏蔽 Windows 窗口的 "Alt+空格" 系统菜单
// （还原/移动/大小/最小化/最大化/关闭）。
//
// 演化历程（每次都被复现现象证伪后升级）：
//
//	v1: 子类化拦 WM_SYSCOMMAND(SC_KEYMENU)
//	    → 漏：菜单实际走 WM_SYSKEYDOWN 路径直接弹出
//	v2: 子类化拦 WM_SYSKEYDOWN(VK_SPACE/UP/DOWN/...) + WM_SYSCHAR
//	    → 漏：偶现，因为子类化在启动初期 5s 轮询期可能还没装上
//	v3: 加 EnsureSysMenuSubclass + ClearAltMenuState (keybd_event 模拟 Alt)
//	    → 漏：WindowShow 异步，keybd_event 调用时窗口可能还不是前台，
//	         模拟的 Alt 派发到了别的窗口
//	v4: GetSystemMenu(hwnd, TRUE) 销毁系统菜单副本
//	    → 漏：TRUE 仅销毁当前副本，下次 OS 按窗口风格自动重建一份新的，
//	         按 Alt+Space / Alt+N 唤起后按 Space 仍能弹出
//	v5 (本版)：直接清掉窗口的 WS_SYSMENU 风格位 + SWP_FRAMECHANGED 刷新。
//	    OS 看到窗口已无 WS_SYSMENU，根本不会再去构造或派发系统菜单。
//	    其余拦截器全部保留作多道防线。
//
// 根因（已最终定位）：
//
//	使用 RegisterHotKey 注册的全局热键 Alt+<n> 触发时，OS 把 Alt 的
//	keydown/keyup 直接消费，窗口侧 Win32 menubar 状态机收不到 Alt 释放，
//	"菜单激活态"残留。此后窗口收到任意按键，DefWindowProc 都按 SYSKEY
//	路径解读：Space → 弹系统菜单；Up/Down → 菜单导航；字母 → 助记符匹配。
//	这一切都发生在 OS 层，根本不进 WebView2，更不到 JS。
//
// 为什么 v5 一定有效：
//
//	WS_SYSMENU 是 Windows 决定 "这个窗口是否拥有系统菜单" 的开关。
//	位被清掉之后：
//	  - DefWindowProc 处理 WM_SYSCOMMAND(SC_KEYMENU) 时，第一步检查窗口
//	    风格，无 WS_SYSMENU 直接 return，菜单根本不会被构造、不会被派发；
//	  - GetSystemMenu(hwnd, FALSE) 返回 NULL，无法重建；
//	  - 窗口本来 frameless 无标题栏，无任何视觉影响；
//	  - Alt+F4 走 SC_CLOSE，不依赖系统菜单的 "关闭" 项，仍能正常关窗。
func DisableAltSpaceSysMenu(title string) {
	go func() {
		defer func() { _ = recover() }()
		installSubclassOnce(title)
	}()
}

// EnsureSysMenuSubclass 在窗口被唤起时再触发一次安装。
// 内部用 sync.Once 保护，多次调用安全；用于覆盖应用启动初期 5s 安装窗口期。
func EnsureSysMenuSubclass(title string) {
	go func() {
		defer func() { _ = recover() }()
		installSubclassOnce(title)
	}()
}

var (
	installOnce sync.Once
	installOK   bool
)

func installSubclassOnce(title string) bool {
	installOnce.Do(func() {
		user32 := syscall.NewLazyDLL("user32.dll")
		comctl32 := syscall.NewLazyDLL("comctl32.dll")
		setSubclass := comctl32.NewProc("SetWindowSubclass")
		defSubclassProc = comctl32.NewProc("DefSubclassProc")
		getWindowLongPtr := user32.NewProc("GetWindowLongPtrW")
		setWindowLongPtr := user32.NewProc("SetWindowLongPtrW")
		setWindowPos := user32.NewProc("SetWindowPos")

		// 等窗口出现，最长 ~5s。
		var hwnd uintptr
		for i := 0; i < 50; i++ {
			hwnd = FindMainWindow(title)
			if hwnd != 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if hwnd == 0 {
			return
		}

		// === 终极一招：清掉窗口的 WS_SYSMENU 风格位 ===
		// 之前 v4 用 GetSystemMenu(hwnd, TRUE) 删菜单不奏效——因为 TRUE 只是
		// 销毁当前菜单副本，下次 OS 还会按窗口风格自动重建一份。要根除必须
		// 直接关掉 "这个窗口需要系统菜单" 这个开关，即 WS_SYSMENU 风格位。
		// 关掉之后：
		//   - Alt+Space 不再弹菜单
		//   - 菜单不会被自动重建
		//   - frameless 窗口本来就没标题栏，无任何视觉影响
		//   - Alt+F4 走 SC_CLOSE 路径，不依赖系统菜单的 "关闭" 项，正常关窗
		clearSysMenuStyle(hwnd, getWindowLongPtr, setWindowLongPtr, setWindowPos)

		// === 双保险：装子类化拦截器 ===
		const subclassID = 0x47504153 // 'GPAS'
		ret, _, _ := setSubclass.Call(
			hwnd,
			syscall.NewCallback(subclassWndProc),
			uintptr(subclassID),
			0,
		)
		installOK = ret != 0
	})
	return installOK
}

const (
	wsSysMenu = 0x00080000
)

// gwlStyle 是 GetWindowLongPtr/SetWindowLongPtr 的 nIndex 参数，
// 在 Win32 SDK 里定义为 -16。直接 uintptr(-16) 在常量上下文会被
// 编译器判为 overflow，因此参考 gwlExStyle 的写法用运行时计算。
var gwlStyle = ^uintptr(0) - 15 // = 0xFFFFFFFFFFFFFFF0 on 64-bit (-16)

// clearSysMenuStyle 关闭 hwnd 的 WS_SYSMENU 风格位，并强制刷新非客户区。
// 必须在风格位变更后调用 SetWindowPos(SWP_FRAMECHANGED) 让 OS 重新计算
// 非客户区，新风格才真正生效。
func clearSysMenuStyle(hwnd uintptr, getWLP, setWLP, setWP *syscall.LazyProc) {
	cur, _, _ := getWLP.Call(hwnd, gwlStyle)
	newStyle := cur &^ wsSysMenu
	if newStyle == cur {
		return
	}
	setWLP.Call(hwnd, gwlStyle, newStyle)
	setWP.Call(hwnd, 0, 0, 0, 0, 0,
		uintptr(swpNoSize|swpNoMove|swpNoZOrder|swpNoActivate|swpFrameChanged))
}

// defSubclassProc：未处理消息交还给原 WndProc 链。
var defSubclassProc *syscall.LazyProc

const (
	wmSysKeyDown    uintptr = 0x0104
	wmSysKeyUp      uintptr = 0x0105
	wmSysChar       uintptr = 0x0106
	wmSysCommand    uintptr = 0x0112
	wmInitMenu      uintptr = 0x0116
	wmInitMenuPopup uintptr = 0x0117

	scKeyMenu   uintptr = 0xF100
	scMouseMenu uintptr = 0xF090 // 鼠标右键标题栏触发的系统菜单（无标题栏窗口理论上不会有）
	// SC_xxx 的低 4 位由系统保留，比较时必须 & 0xFFF0。
	scMask uintptr = 0xFFF0

	// 需要拦截的虚拟键（在 SYSKEY 状态下会触发菜单行为）。
	vkBack   = 0x08
	vkTab    = 0x09
	vkReturn = 0x0D
	vkEscape = 0x1B
	vkSpace  = 0x20
	vkLeft   = 0x25
	vkUp     = 0x26
	vkRight  = 0x27
	vkDown   = 0x28

	// 显式放行：Alt+F4（关窗）。
	vkF4 = 0x73
)

func shouldSwallowSysKey(vk uintptr) bool {
	switch vk {
	case vkSpace, vkUp, vkDown, vkLeft, vkRight, vkReturn, vkEscape, vkBack, vkTab:
		return true
	}
	return false
}

func subclassWndProc(hwnd, msg, wParam, lParam, _, _ uintptr) uintptr {
	switch msg {
	case wmSysKeyDown:
		vk := wParam & 0xFF
		if vk == vkF4 {
			break
		}
		if shouldSwallowSysKey(vk) {
			return 0
		}
	case wmSysKeyUp:
		vk := wParam & 0xFF
		if vk == vkF4 {
			break
		}
		if shouldSwallowSysKey(vk) {
			return 0
		}
	case wmSysChar:
		// 吞掉避免 "叮" 提示音。
		return 0
	case wmSysCommand:
		sc := wParam & scMask
		if sc == scKeyMenu || sc == scMouseMenu {
			return 0
		}
	case wmInitMenu, wmInitMenuPopup:
		// 极端兜底：万一系统菜单还是被某种方式重建出来，
		// 在初始化阶段直接拒掉。返回 0 表示已处理。
		return 0
	}
	if defSubclassProc == nil {
		return 0
	}
	r, _, _ := defSubclassProc.Call(hwnd, msg, wParam, lParam)
	return r
}


