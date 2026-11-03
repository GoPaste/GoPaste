//go:build !windows

package main

// position_other.go —— macOS / Linux 下的主窗口定位（Wails v3）。
//
// 坐标单位约定：
//   - Wails v2 的 WindowSetPosition(x, y) 在 macOS 上是"相对窗口所在屏幕 visibleFrame
//     的逻辑点（左上角为原点）"。internal/cursor 的 Position()/WorkArea() 正是按这个
//     坐标系返回的（见 cursor_darwin.go / workarea_darwin.go 的注释）。
//   - Wails v3 的 SetRelativePosition / RelativePosition 与 v2 的 SetPosition **逐行
//     等价**（windowSetRelativePosition：origin = visibleFrame.origin + (x, top - h - y)），
//     Size() 也同样是逻辑点。所以这里的算法与 v2 完全一致，只把 runtime 调用换成 v3。
//   - 不要换成 v3 的 SetPosition：它是以主屏为基准的绝对坐标，与 internal/cursor
//     的坐标系不一致，会导致"跟随鼠标"偏移。

import "gopaste/internal/cursor"

// windowMargin 是窗口离工作区边缘的安全距离（逻辑像素）。
const windowMargin = 8

// placeAtCursor 让窗口左上角对齐鼠标位置（mx/my 为 internal/cursor 坐标系）。
func (a *App) placeAtCursor(mx, my int) {
	a.setWindowPosAbsolute(mx, my)
}

// placeRemembered 恢复上次保存的窗口位置（RelativePosition 坐标系）。
func (a *App) placeRemembered(x, y int) {
	a.setWindowPosAbsolute(x, y)
}

// currentWindowPos 返回当前窗口位置（RelativePosition 坐标系），供"记住位置"保存。
func (a *App) currentWindowPos() (int, int) {
	if a.win == nil {
		return 0, 0
	}
	return a.win.RelativePosition()
}

// setWindowPosAbsolute 把窗口左上角放到 (absX, absY)，先约束在工作区内，
// 再减去工作区原点得到 SetRelativePosition 所需的相对坐标。
func (a *App) setWindowPosAbsolute(absX, absY int) {
	if !a.uiReady() {
		return
	}
	x, y, workLeft, workTop := a.clampToScreen(absX, absY)
	a.win.SetRelativePosition(x-workLeft, y-workTop)
}

// clampToScreen 约束窗口坐标在屏幕可见工作区内（排除菜单栏 / Dock 等系统 UI）。
// 返回 (x, y, workLeft, workTop)。
func (a *App) clampToScreen(x, y int) (int, int, int, int) {
	w, h := a.win.Size()

	var sx, sy, sw, sh int
	wx, wy, ww, wh := cursor.WorkArea()
	if ww > 0 && wh > 0 {
		sx, sy, sw, sh = wx, wy, ww, wh
	} else {
		// fallback：用窗口当前所在屏幕的尺寸（对应 v2 ScreenGetAll 里 IsCurrent 的那块屏）
		scr, err := a.win.GetScreen()
		if err != nil || scr == nil {
			scr = a.wapp.Screen.GetPrimary()
		}
		if scr == nil {
			return x, y, 0, 0
		}
		sw, sh = scr.Size.Width, scr.Size.Height
	}

	x, y = clampRect(x, y, w, h, sx, sy, sw, sh, windowMargin)
	return x, y, sx, sy
}
