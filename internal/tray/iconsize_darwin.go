//go:build darwin

package tray

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>

void SetTrayIconSize(double pt);
*/
import "C"

// setTrayIconSizePt 把菜单栏图标逻辑尺寸设为 pt（默认 22pt = 菜单栏内容高度）。
// 必须在 systray.SetIcon 之后调用，否则会被 systray 内部硬编码的 16pt 覆盖。
func setTrayIconSizePt(pt float64) {
	C.SetTrayIconSize(C.double(pt))
}
