//go:build windows

package paste

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Win32 INPUT 结构（union）：
//   typedef struct tagINPUT {
//     DWORD type;
//     union { MOUSEINPUT mi; KEYBDINPUT ki; HARDWAREINPUT hi; } DUMMYUNIONNAME;
//   } INPUT;
// 64 位下 sizeof(INPUT)=40：DWORD(4)+pad(4)+union(32)。
// MOUSEINPUT 是最大成员（32 字节），KEYBDINPUT 仅 24 字节，因此需要补 padding。
//
// KEYBDINPUT:
//   WORD wVk; WORD wScan; DWORD dwFlags; DWORD time; ULONG_PTR dwExtraInfo;
//   = 2 + 2 + 4 + 4 + 8 = 20 字节，按 8 字节对齐后实际 24 字节。

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// input 必须严格保持 40 字节（cbSize 参数）。
// 通过 [32]byte 联合体占位（MOUSEINPUT 大小），再用 unsafe.Pointer 写 KEYBDINPUT。
type input struct {
	inputType uint32
	_         uint32   // 8 字节对齐的 padding
	union     [32]byte // MOUSEINPUT 大小，足以容纳 KEYBDINPUT
}

// Win32 常量
const (
	inputKeyboard = 1

	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
	keyeventfScancode    = 0x0008

	// Virtual Keys
	vkShift  = 0x10
	vkInsert = 0x2D

	// MapVirtualKey map type
	mapvkVkToVsc = 0x00 // VK -> scan code（不含扩展前缀）
)

// sendPasteImpl 使用 Win32 SendInput 模拟 Shift+Insert 触发系统级粘贴。
//
// 为什么选 Shift+Insert 而不是 Ctrl+V？
//   - Shift+Insert 是 Windows 自 DOS 时代起就支持的通用粘贴快捷键。
//   - 兼容 cmd、PowerShell、Git Bash、WSL、各类终端（Ctrl+V 在它们里通常不触发粘贴）。
//   - 兼容 RDP / VNC 客户端、部分 VMware Console（Ctrl+V 会被吞）。
//   - 和 Ctrl+V 走一样的系统剪贴板路径，不改变粘贴语义。
//   - EcoPaste (Rust + enigo) 也是这个选择。
//
// 实现要点（之前踩过的坑，务必保留）：
//
//  1. Insert 是"扩展键"（extended key）。它与小键盘 Numpad0 共享 VK_INSERT=0x2D。
//     如果 dwFlags 不带 KEYEVENTF_EXTENDEDKEY：
//       - NumLock 开 → 被解释为 Numpad0，应用里输入 "0" 而不是粘贴；
//       - NumLock 关 → 多数应用直接忽略该事件；
//       - 部分 Chromium / Electron / RDP 客户端只识别带扩展标志的 Insert。
//     参考：
//     https://learn.microsoft.com/en-us/windows/win32/inputdev/about-keyboard-input#extended-key-flag
//     enigo-rs 的做法见 src/win/win_impl.rs 中 is_extended_key / translate_key。
//
//  2. wScan 必须填。很多低级钩子（RDP/VMware、某些游戏反作弊层）只看 scan code，
//     忽略纯 VK 事件。用 MapVirtualKeyW(VK, MAPVK_VK_TO_VSC) 取当前布局下的 scan。
//
//  3. cbSize 必须严格等于 sizeof(INPUT)=40。之前版本因为 KEYBDINPUT 后没补 padding
//     导致 SendInput 直接失败——这里保留 [32]byte 联合体占位的实现。
//
// 参考：
//   - https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-sendinput
//   - https://learn.microsoft.com/windows/win32/api/winuser/ns-winuser-keybdinput
func sendPasteImpl() error {
	user32 := syscall.NewLazyDLL("user32.dll")
	sendInput := user32.NewProc("SendInput")
	mapVirtualKey := user32.NewProc("MapVirtualKeyW")
	getLastError := syscall.NewLazyDLL("kernel32.dll").NewProc("GetLastError")

	// 为每个键拿到当前键盘布局下的硬件 scan code。
	// MapVirtualKeyW 失败返回 0 也没关系——部分应用只看 VK，scan=0 不致命，
	// 但对要求 scan 的场景（RDP 等）依然退化到旧路径，不会比 Ctrl+V 更差。
	scan := func(vk uint16) uint16 {
		r, _, _ := mapVirtualKey.Call(uintptr(vk), uintptr(mapvkVkToVsc))
		return uint16(r)
	}

	shiftScan := scan(vkShift)
	insertScan := scan(vkInsert)

	// Insert 是扩展键。Shift 不是。
	mkInput := func(vk, sc uint16, extended, up bool) input {
		var flags uint32
		if extended {
			flags |= keyeventfExtendedKey
		}
		if up {
			flags |= keyeventfKeyUp
		}
		ki := keybdInput{
			wVk:     vk,
			wScan:   sc,
			dwFlags: flags,
		}
		var in input
		in.inputType = inputKeyboard
		*(*keybdInput)(unsafe.Pointer(&in.union[0])) = ki
		return in
	}

	// 顺序：Shift↓ → Insert↓ → Insert↑ → Shift↑
	// 和 enigo 的 Key::Shift(Press) + Key::Other(0x2D)(Click) + Key::Shift(Release) 等价。
	inputs := []input{
		mkInput(vkShift, shiftScan, false, false),  // Shift down
		mkInput(vkInsert, insertScan, true, false), // Insert down（扩展键）
		mkInput(vkInsert, insertScan, true, true),  // Insert up（扩展键）
		mkInput(vkShift, shiftScan, false, true),   // Shift up
	}

	// 防御：sizeof(input) 必须为 40，否则 SendInput 一定失败
	cb := unsafe.Sizeof(inputs[0])
	if cb != 40 {
		return fmt.Errorf("paste: unexpected INPUT size %d (want 40)", cb)
	}

	ret, _, callErr := sendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		cb,
	)
	if int(ret) != len(inputs) {
		// SendInput 返回成功注入的事件数；少于预期视为失败
		le, _, _ := getLastError.Call()
		return fmt.Errorf("paste: SendInput injected %d/%d events, lastErr=%d, syscallErr=%v",
			int(ret), len(inputs), uint32(le), callErr)
	}
	return nil
}
