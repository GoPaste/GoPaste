//go:build !darwin

package paste

import "errors"

// ErrNoAccessibility 在非 darwin 平台不会真实发生——这些平台的粘贴能力
// （SendInput / xdotool）没有"辅助功能"这一权限前置条件。保留符号只是
// 为了让上层 app 代码可以用 errors.Is(err, paste.ErrNoAccessibility)
// 无 build tag 判断。
var ErrNoAccessibility = errors.New("paste: accessibility permission not applicable")

// HasAccessibility 在非 darwin 恒为 true——表示"环境就绪，可以尝试粘贴"。
// 非 darwin 平台即使 SendPaste 失败也不走权限引导分支，而是直接报错。
func HasAccessibility() bool { return true }

// PromptAccessibility 在非 darwin 是 no-op，直接返回 true。
func PromptAccessibility() bool { return true }
