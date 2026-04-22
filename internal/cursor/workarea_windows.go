//go:build windows

package cursor

import (
	"syscall"
	"unsafe"
)

var (
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
)

type rect struct {
	Left, Top, Right, Bottom int32
}

const (
	spiGetWorkArea = 0x0030
)

// WorkArea 返回屏幕工作区域（排除任务栏），以逻辑像素为单位。
// 返回值：x, y, width, height。err != nil 时返回 0,0,0,0。
func WorkArea() (int, int, int, int) {
	var r rect
	ret, _, _ := procSystemParametersInfo.Call(
		spiGetWorkArea,
		0,
		uintptr(unsafe.Pointer(&r)),
		0,
	)
	if ret == 0 {
		return 0, 0, 0, 0
	}
	_ = syscall.Errno(0)
	return int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top)
}
