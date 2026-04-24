// Package paste 负责把选中条目写回系统剪切板，并尽力模拟粘贴动作。
//
// 各平台真正"发 Ctrl/Cmd+V"的能力差异很大，因此主流程仅保证写回剪切板；
// 若平台支持模拟按键则在对应 paste_<os>.go 中实现 SendPaste()。
package paste

import (
	"errors"

	"gopaste/internal/clipboard"
	"gopaste/internal/types"
)

// ErrUnsupported 平台不支持自动粘贴时返回。
var ErrUnsupported = errors.New("paste: auto-paste unsupported on this platform")

// WriteClipboard 把内容写回系统剪切板。
//
// 【darwin 必须走 internal/clipboard.Write*Clipboard*】
// 不再使用 golang.design/x/clipboard.Write —— 那条路径裸调 NSPasteboard，
// 与 file/image/text watcher 并发会触发 NSGenericException → abort()。
// 详细排查见 internal/clipboard/filewatcher_darwin.go 顶部注释。
//
// 非 darwin 平台 internal/clipboard 包内部仍可委托给 golang.design/x/clipboard，
// 但本入口统一对外暴露的就是 internal/clipboard 的封装。
func WriteClipboard(t types.ItemType, content []byte) error {
	switch t {
	case types.TypeImage:
		return clipboard.WriteImage(content)
	default:
		return clipboard.WriteText(content)
	}
}

// SendPaste 模拟发送 Ctrl/Cmd+V。实现在 paste_<os>.go。
// 默认实现返回 ErrUnsupported（平台文件可覆盖 sendPasteImpl）。
func SendPaste() error { return sendPasteImpl() }

// -----------------------------------------------------------------------------
// macOS Accessibility 权限接口
// -----------------------------------------------------------------------------
// 真实实现分两处：
//   - darwin：paste_darwin.go 里用 AXIsProcessTrustedWithOptions
//   - 非 darwin：paste_nondarwin.go 提供 no-op 兜底
//
// 对外契约（跨平台）：
//   - HasAccessibility() bool         — 静默查询授权状态
//   - PromptAccessibility() bool       — 首次调用触发系统弹框
//   - ErrNoAccessibility               — SendPaste 因未授权返回的哨兵错误
//
// 这么拆是为了让 app 层不写 build tag —— 无脑调 `paste.HasAccessibility()`
// 即可，其他平台永远返回 true 不走引导分支。
// -----------------------------------------------------------------------------
