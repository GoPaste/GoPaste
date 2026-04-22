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
//
// 之前实现把 KEYBDINPUT 视为 20 字节并外补 [8]byte，
// 导致整个 input 结构体实际只有 36/40 字节，sizeof 不等于 40，SendInput 失败。

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// input 必须严格保持 40 字节（cbSize 参数）。
// 通过 [16]byte 联合体占位，再用 unsafe.Pointer 写 KEYBDINPUT。
type input struct {
	inputType uint32
	_         uint32   // 8 字节对齐的 padding
	union     [32]byte // MOUSEINPUT 大小，足以容纳 KEYBDINPUT
}

// sendPasteImpl 使用 Win32 SendInput 模拟 Ctrl+V。
// 实现参考 MSDN：https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-sendinput
func sendPasteImpl() error {
	user32 := syscall.NewLazyDLL("user32.dll")
	sendInput := user32.NewProc("SendInput")
	getLastError := syscall.NewLazyDLL("kernel32.dll").NewProc("GetLastError")

	const (
		inputKeyboard  = 1
		keyeventfKeyup = 0x0002
		vkControl      = 0x11
		vkV            = 0x56
	)

	mkInput := func(vk uint16, up bool) input {
		ki := keybdInput{wVk: vk}
		if up {
			ki.dwFlags = keyeventfKeyup
		}
		var in input
		in.inputType = inputKeyboard
		// 把 KEYBDINPUT 写入 union 起始处
		*(*keybdInput)(unsafe.Pointer(&in.union[0])) = ki
		return in
	}

	inputs := []input{
		mkInput(vkControl, false),
		mkInput(vkV, false),
		mkInput(vkV, true),
		mkInput(vkControl, true),
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
