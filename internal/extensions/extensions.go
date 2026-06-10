// Package extensions 提供"扩展功能"所需的系统级能力。
//
// 目前仅包含 macOS 上 Cmd+Q 防误触功能。
// 前端通过设置面板（Settings.vue 的"扩展功能" tab）选择：
//   - "default" : 保持 macOS 原生行为（立即退出）
//   - "confirm" : 第一次按 Cmd+Q 提示，窗口内再按一次确认，或等待自动清除
//   - "disable" : 完全禁用 Cmd+Q 退出（可通过托盘菜单/退出按钮退出）
//
// 该包在非 macOS 平台上的实现为 no-op，以便调用方无需做平台判断。
package extensions

// CmdQBehavior 表示 Cmd+Q 的处理策略。
type CmdQBehavior string

const (
	// CmdQDefault 原生行为：立即退出。
	CmdQDefault CmdQBehavior = "default"
	// CmdQConfirm 二次确认：按下后 N 秒内再按一次才真的退出。
	CmdQConfirm CmdQBehavior = "confirm"
	// CmdQDisable 完全禁用：Cmd+Q 不会退出应用。
	CmdQDisable CmdQBehavior = "disable"
)

// NormalizeCmdQBehavior 把前端传入的字符串归一化为合法的策略值。
// 未知值按 "default" 处理。
func NormalizeCmdQBehavior(v string) CmdQBehavior {
	switch CmdQBehavior(v) {
	case CmdQConfirm:
		return CmdQConfirm
	case CmdQDisable:
		return CmdQDisable
	default:
		return CmdQDefault
	}
}

// OnCmdQHandled 回调类型：当 Cmd+Q 被拦截时通知调用方。
// reason 取值：
//   - "confirm-first"          L1 首次按下（仅 GoPaste 自身前台）
//   - "confirm-first-global"   L0 首次按下（全局，可能前台是其他 App）
//   - "confirm-timeout"        确认窗口超时失效
//   - "disabled"               L1 disable 被按下
//   - "disabled-global"        L0 disable 被按下
type OnCmdQHandled func(reason string)

// TapAuthStatus 表示「输入监控」权限的状态。
// 仅 macOS 下有实际含义；全局 Cmd+Q 拦截（CGEventTap）需要此权限。
type TapAuthStatus int

const (
	// TapAuthUnknown 用户尚未做出选择。
	TapAuthUnknown TapAuthStatus = 0
	// TapAuthGranted 已授权。
	TapAuthGranted TapAuthStatus = 1
	// TapAuthDenied 已拒绝；需用户去系统设置手动勾选。
	TapAuthDenied TapAuthStatus = 2
)
