//go:build darwin

package cursor

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// getVisibleArea 返回主屏幕可用工作区，坐标系与 getMousePos 一致（Wails 参数空间）。
// x=0 对应 visibleFrame 左边界，y=0 对应 visibleFrame 顶部。
static void getVisibleArea(int *ox, int *oy, int *ow, int *oh) {
    NSScreen *screen = [NSScreen mainScreen];
    NSRect vf = screen.visibleFrame;
    // 在 Wails 坐标系里，x 和 y 已经相对 visibleFrame 原点做了偏移：
    //   cursor x = p.x - vf.origin.x  → 工作区从 x=0 开始
    //   cursor y = visibleTop - p.y   → 工作区从 y=0 开始（visibleFrame 顶部）
    *ox = 0;
    *oy = 0;
    *ow = (int)vf.size.width;
    *oh = (int)vf.size.height;
}
*/
import "C"

// WorkArea 返回主屏幕可用工作区（排除菜单栏和 Dock），单位：逻辑像素。
// 坐标原点与 Wails WindowSetPosition 一致（主屏幕左上角，不含菜单栏）。
func WorkArea() (int, int, int, int) {
	var ox, oy, ow, oh C.int
	C.getVisibleArea(&ox, &oy, &ow, &oh)
	return int(ox), int(oy), int(ow), int(oh)
}

// ScaleForPoint 在 macOS 上恒为 1.0（Wails 坐标 API 已是逻辑像素）。
func ScaleForPoint(x, y int) float64 {
	return 1.0
}
