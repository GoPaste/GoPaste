//go:build darwin

package paste

// macOS 侧的"粘贴前恢复焦点"已整体废弃。
//
// 背景：
//   我们把主窗口改造成了 NSPanel + NSWindowStyleMaskNonactivatingPanel
//   （见 internal/window/panel_darwin.m）。面板显示时**不会夺走前台应用
//   的 active 状态**，所以根本不存在"前一个应用"这回事——用户原先在编
//   辑的那个 app 自始至终就是 active。PasteItem 的 mac 分支里 orderOut
//   面板后直接 CGEventPost Cmd+V，目标就是正确的应用。
//
// 为什么这里还保留类型 + 函数：
//   保持 internal/paste 包在三端的 API 一致（CapturePreviousWindow /
//   RestorePreviousWindow / PreviousWindow.IsValid），让 app.go 里 mac
//   分支短路即可，不用加额外的 build tag。实际不会被调用——见
//   app.captureFocusBeforeShow() 里的 darwin 早退。
//
// 为什么不再动用 AppKit：
//   此前实现走过三条都会崩的路径：
//     1) 自建串行队列上跑 AppKit API → __builtin_trap
//     2) cgo 线程 dispatch_sync(main) → 与 WindowHide 组合成死锁 → 看门狗杀
//     3) cgo 线程直接调 activateWithOptions → 同 1
//   既然新架构下根本不需要 activate，索性把 cgo 全部移除，消除整条
//   风险链。

import "fmt"

// PreviousWindow 跨平台句柄。Mac 上始终为零值。
type PreviousWindow struct{}

// IsValid 恒为 false —— mac 上没有需要恢复的"前一个窗口"。
func (PreviousWindow) IsValid() bool { return false }

// CapturePreviousWindow 在 mac 下是 no-op，返回空句柄与固定错误。
// 调用方（app.captureFocusBeforeShow）已经在 mac 下短路，不会实际调到这里。
// 保留错误返回只是给任何意外调用一个明确的信号。
func CapturePreviousWindow() (PreviousWindow, error) {
	return PreviousWindow{}, fmt.Errorf("focus: not applicable on macOS (NSPanel NonactivatingPanel)")
}

// RestorePreviousWindow 在 mac 下是 no-op。
func RestorePreviousWindow(PreviousWindow) error {
	return nil
}
