//go:build darwin

package cursor

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void getMousePos(int *x, int *y) {
    NSPoint p = [NSEvent mouseLocation];
    // Wails SetPosition(x, y) 内部实现（WailsContext.m）：
    //   window.origin.x = visibleFrame.origin.x + x
    //   window.origin.y = (visibleFrame.origin.y + visibleFrame.size.height) - windowH - y
    //
    // 要让窗口左上角对齐鼠标，需要：
    //   x_wails = p.x - visibleFrame.origin.x
    //   y_wails = (visibleFrame.origin.y + visibleFrame.size.height) - p.y
    //
    // 用鼠标所在屏幕（而非 mainScreen）匹配 Wails 的 getCurrentScreen 逻辑。
    NSScreen *targetScreen = [NSScreen mainScreen];
    for (NSScreen *s in [NSScreen screens]) {
        if (NSPointInRect(p, s.frame)) {
            targetScreen = s;
            break;
        }
    }
    NSRect vf = targetScreen.visibleFrame;
    CGFloat visibleTop = vf.origin.y + vf.size.height;
    *x = (int)(p.x - vf.origin.x);
    *y = (int)(visibleTop - p.y);
}
*/
import "C"

func Position() (int, int) {
	var x, y C.int
	C.getMousePos(&x, &y)
	return int(x), int(y)
}
