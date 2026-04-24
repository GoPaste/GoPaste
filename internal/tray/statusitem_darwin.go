//go:build darwin

package tray

/*
#cgo LDFLAGS: -framework Cocoa

extern void GoPasteStatusItemInstall(const unsigned char *icon_png, int icon_len);
extern void GoPasteStatusItemUninstall(void);
extern void GoPasteStatusItemSetIcon(const unsigned char *icon_png, int icon_len, int isTemplate);
*/
import "C"
import "unsafe"

// statusItemCallbacks 由 installStatusItem 注册，被下面四个 export 函数调用。
var statusItemCallbacks struct {
	onShow    func()
	onAbout   func()
	onRestart func()
	onQuit    func()
}

//export goStatusItemOnShow
func goStatusItemOnShow() {
	if statusItemCallbacks.onShow != nil {
		go statusItemCallbacks.onShow()
	}
}

//export goStatusItemOnAbout
func goStatusItemOnAbout() {
	if statusItemCallbacks.onAbout != nil {
		go statusItemCallbacks.onAbout()
	}
}

//export goStatusItemOnRestart
func goStatusItemOnRestart() {
	if statusItemCallbacks.onRestart != nil {
		go statusItemCallbacks.onRestart()
	}
}

//export goStatusItemOnQuit
func goStatusItemOnQuit() {
	if statusItemCallbacks.onQuit != nil {
		go statusItemCallbacks.onQuit()
	}
}

// installStatusItem 注册回调并调用 ObjC 侧创建 NSStatusItem。
// iconPNG 为 PNG 字节数据；传 nil 则 ObjC 侧降级用文字 "P"。
func installStatusItem(cb Callbacks, iconPNG []byte) func() {
	statusItemCallbacks.onShow = cb.OnShow
	statusItemCallbacks.onAbout = cb.OnAbout
	statusItemCallbacks.onRestart = cb.OnRestart
	statusItemCallbacks.onQuit = cb.OnQuit

	if len(iconPNG) > 0 {
		ptr := (*C.uchar)(unsafe.Pointer(&iconPNG[0]))
		C.GoPasteStatusItemInstall(ptr, C.int(len(iconPNG)))
	} else {
		C.GoPasteStatusItemInstall(nil, 0)
	}

	return func() {
		C.GoPasteStatusItemUninstall()
	}
}

// setStatusItemIcon 动态替换菜单栏图标。
// isTemplate=true 时系统按深浅主题自动染色。
func setStatusItemIcon(iconPNG []byte, isTemplate bool) {
	if len(iconPNG) == 0 {
		return
	}
	t := C.int(0)
	if isTemplate {
		t = 1
	}
	ptr := (*C.uchar)(unsafe.Pointer(&iconPNG[0]))
	C.GoPasteStatusItemSetIcon(ptr, C.int(len(iconPNG)), t)
}
