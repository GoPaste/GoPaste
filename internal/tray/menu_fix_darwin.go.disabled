//go:build darwin

package tray

// menu_fix_darwin.m 里用 __attribute__((constructor)) 在 dylib 加载阶段
// 替换 fyne.io/systray 的 show_menu IMP，修正菜单位置错位 + ⌃ 箭头问题。
// 此文件仅负责把 .m 拉进编译 + 引入必要 framework。

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -lobjc
*/
import "C"
