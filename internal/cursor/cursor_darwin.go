//go:build darwin

package cursor

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void getMousePos(int *x, int *y) {
    NSPoint p = [NSEvent mouseLocation];
    // NSEvent.mouseLocation 是左下角原点，转为左上角原点
    NSScreen *screen = [NSScreen mainScreen];
    *x = (int)p.x;
    *y = (int)(screen.frame.size.height - p.y);
}
*/
import "C"

func Position() (int, int) {
	var x, y C.int
	C.getMousePos(&x, &y)
	return int(x), int(y)
}
