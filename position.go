package main

// position.go —— 主窗口定位的跨平台公共逻辑。
// 平台相关的坐标换算在 position_windows.go（v3 DIP 绝对坐标）与
// position_other.go（macOS/Linux，相对工作区，与 v2 等价）中实现，二者提供：
//
//	placeAtCursor(mx, my)   跟随鼠标
//	placeRemembered(x, y)   恢复上次位置
//	currentWindowPos()      读取当前位置（与 placeRemembered 同一坐标系）

import (
	"gopaste/internal/cursor"
	"gopaste/internal/settings"
)

// clampRect 把左上角为 (x, y)、尺寸为 w×h 的窗口约束在区域 (ax, ay, aw, ah) 内，
// 四周保留 margin。区域比窗口还小时贴左上角。
func clampRect(x, y, w, h, ax, ay, aw, ah, margin int) (int, int) {
	minX := ax + margin
	minY := ay + margin
	maxX := ax + aw - w - margin
	maxY := ay + ah - h - margin
	if maxX < minX {
		maxX = minX
	}
	if maxY < minY {
		maxY = minY
	}
	if x < minX {
		x = minX
	}
	if y < minY {
		y = minY
	}
	if x > maxX {
		x = maxX
	}
	if y > maxY {
		y = maxY
	}
	return x, y
}

// positionWindow 根据设置决定窗口出现位置。
func (a *App) positionWindow() {
	if !a.uiReady() {
		return
	}
	s := settings.Default()
	if a.settings != nil {
		s = a.settings.Get()
	}
	switch s.WindowPosition {
	case "follow":
		// 跟随鼠标：窗口左上角对齐鼠标位置
		mx, my := cursor.Position()
		if mx > 0 || my > 0 {
			a.placeAtCursor(mx, my)
		} else {
			a.centerWindow()
		}
	case "remember":
		// 记住位置：恢复上次保存的坐标
		if a.posInited {
			a.placeRemembered(a.lastX, a.lastY)
		} else {
			a.centerWindow()
		}
	default: // "center"
		a.centerWindow()
	}
}

// saveWindowPosition 保存当前窗口位置（供"记住位置"模式下次恢复）。
func (a *App) saveWindowPosition() {
	if !a.uiReady() {
		return
	}
	x, y := a.currentWindowPos()
	a.lastX = x
	a.lastY = y
	a.posInited = true
}
