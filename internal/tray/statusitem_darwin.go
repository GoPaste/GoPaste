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

// statusItemCallbacks 由 installStatusItem 注册，被下面的 export 函数调用。
var statusItemCallbacks struct {
	onShow    func()
	onAbout   func()
	onWebsite func()
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

//export goStatusItemOnWebsite
func goStatusItemOnWebsite() {
	if statusItemCallbacks.onWebsite != nil {
		go statusItemCallbacks.onWebsite()
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
	statusItemCallbacks.onWebsite = cb.OnWebsite
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

// currentIconStyle 记录当前图标风格，showStatusItem 重建时使用。
var currentIconStyle = "color"

// showStatusItem 重新创建并显示菜单栏图标（幂等），使用当前图标风格。
func showStatusItem() {
	icon := iconColorPNG
	if currentIconStyle == "gray" {
		icon = iconGrayPNG
	}
	ptr := (*C.uchar)(unsafe.Pointer(&icon[0]))
	C.GoPasteStatusItemInstall(ptr, C.int(len(icon)))
}

// hideStatusItem 从菜单栏移除图标（幂等）。
func hideStatusItem() {
	C.GoPasteStatusItemUninstall()
}
