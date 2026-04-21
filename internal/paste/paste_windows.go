//go:build windows

package paste

import (
	"syscall"
	"unsafe"
)

// sendPasteImpl 使用 Win32 SendInput 模拟 Ctrl+V。
// 实现参考 MSDN：https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-sendinput
func sendPasteImpl() error {
	user32 := syscall.NewLazyDLL("user32.dll")
	sendInput := user32.NewProc("SendInput")

	const (
		inputKeyboard   = 1
		keyeventfKeyup  = 0x0002
		vkControl       = 0x11
		vkV             = 0x56
	)

	type keybdInput struct {
		wVk         uint16
		wScan       uint16
		dwFlags     uint32
		time        uint32
		dwExtraInfo uintptr
	}
	type input struct {
		inputType uint32
		_         uint32 // padding on 64-bit
		ki        keybdInput
		_         [8]byte
	}

	inputs := []input{
		{inputType: inputKeyboard, ki: keybdInput{wVk: vkControl}},
		{inputType: inputKeyboard, ki: keybdInput{wVk: vkV}},
		{inputType: inputKeyboard, ki: keybdInput{wVk: vkV, dwFlags: keyeventfKeyup}},
		{inputType: inputKeyboard, ki: keybdInput{wVk: vkControl, dwFlags: keyeventfKeyup}},
	}

	ret, _, err := sendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if ret == 0 {
		return err
	}
	return nil
}
