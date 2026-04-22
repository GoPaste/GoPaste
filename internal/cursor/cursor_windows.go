//go:build windows

package cursor

import (
	"syscall"
	"unsafe"
)

var (
	user32         = syscall.NewLazyDLL("user32.dll")
	getCursorPos   = user32.NewProc("GetCursorPos")
)

type point struct {
	X, Y int32
}

func Position() (int, int) {
	var pt point
	getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}
