//go:build windows

package cursor

import (
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

// WorkArea 返回主显示器工作区域（排除任务栏），单位：**物理像素**。
//
// 与 cursor.Position 保持同一坐标系，方便 app.go 里做边界夹紧后直接
// 把结果传给 Wails 的 WindowSetPosition（后者在 Windows 下也是物理像素）。
// 返回值：x, y, width, height。失败返回 0,0,0,0。
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
	return int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top)
}
