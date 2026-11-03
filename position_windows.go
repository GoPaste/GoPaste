//go:build windows

package main

// position_windows.go —— Windows 下的主窗口定位（Wails v3）。
//
// 坐标单位约定（这是本项目反复翻车的地方，务必读清楚）：
//
//   - Wails v2：WindowSetPosition(x, y) 内部是
//     SetWindowPos(hwnd, …, workRect.Left+x, workRect.Top+y, …)，即"相对工作区的
//     **物理像素**"；而 WindowGetSize 返回**逻辑像素**。v2 时代 clampToScreen 必须
//     把尺寸乘以 DPI scale 才能和 cursor / 工作区同单位比较。
//   - Wails v3：Position / SetPosition / Size 全部是**虚拟桌面上的 DIP 绝对坐标**
//     （内部 PhysicalToDipRect / DipToPhysicalRect 按显示器 DPI 换算），屏幕信息
//     （Screen.WorkArea / Bounds）也是 DIP。
//
// 因此 v3 下统一在 DIP 空间里计算：
//   - 跟随鼠标：cursor.Position() 返回 Win32 物理像素 → 先用 v3 的
//     ScreenManager.PhysicalToDipPoint 换成 DIP；
//   - 记住位置：保存的就是 v3 Position() 的 DIP 值，原样恢复；
//   - clamp：用 ScreenNearestDipPoint 找到目标点所在显示器的 WorkArea（排除任务栏）。
// 混合 DPI 多显示器下也不需要手动乘除 scale。

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// windowMarginDIP 是窗口离工作区边缘的安全距离，避免阴影 / 边框被屏幕或任务栏切到。
const windowMarginDIP = 8

// placeAtCursor 让窗口左上角对齐鼠标位置（mx/my 为 Win32 物理像素）。
func (a *App) placeAtCursor(mx, my int) {
	if !a.uiReady() {
		return
	}
	phys := application.Point{X: mx, Y: my}
	// ScreenNearestPhysicalPoint 在没有任何屏幕信息时返回 nil；此时
	// PhysicalToDipPoint 会对 nil 解引用，所以先判空再换算。
	if a.wapp.Screen.ScreenNearestPhysicalPoint(phys) == nil {
		a.centerWindow()
		return
	}
	dip := a.wapp.Screen.PhysicalToDipPoint(phys)
	a.placeClampedDIP(dip.X, dip.Y)
}

// placeRemembered 恢复上次保存的窗口位置（DIP 绝对坐标）。
func (a *App) placeRemembered(x, y int) {
	if !a.uiReady() {
		return
	}
	a.placeClampedDIP(x, y)
}

// currentWindowPos 返回当前窗口位置（DIP 绝对坐标），供"记住位置"保存。
func (a *App) currentWindowPos() (int, int) {
	if a.win == nil {
		return 0, 0
	}
	return a.win.Position()
}

// placeClampedDIP 把窗口左上角放到 (x, y)，并约束在该点所在显示器的工作区内。
func (a *App) placeClampedDIP(x, y int) {
	scr := a.wapp.Screen.ScreenNearestDipPoint(application.Point{X: x, Y: y})
	if scr == nil {
		a.win.SetPosition(x, y)
		return
	}
	w, h := a.win.Size()
	wa := scr.WorkArea
	if wa.Width <= 0 || wa.Height <= 0 {
		wa = scr.Bounds
	}
	x, y = clampRect(x, y, w, h, wa.X, wa.Y, wa.Width, wa.Height, windowMarginDIP)
	a.win.SetPosition(x, y)
}
