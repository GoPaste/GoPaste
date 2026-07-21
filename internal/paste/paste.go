// Package paste 负责把选中条目写回系统剪切板，并尽力模拟粘贴动作。
//
// 各平台真正"发 Ctrl/Cmd+V"的能力差异很大，因此主流程仅保证写回剪切板；
// 若平台支持模拟按键则在对应 paste_<os>.go 中实现 SendPaste()。
package paste

import (
	"errors"
	"strings"

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
	case types.TypeFile:
		// 文件类型：把路径列表写回为系统文件剪贴板格式（Windows CF_HDROP、
		// macOS NSPasteboard file URL、Linux text/uri-list），
		// 而非纯文本——这样目标程序（资源管理器/Finder 等）才能识别并粘贴实际文件。
		paths := splitPaths(content)
		if len(paths) == 0 {
			return nil
		}
		return clipboard.WriteFiles(paths)
	default:
		return clipboard.WriteText(content)
	}
}

// splitPaths 把换行分隔的路径字节切片拆成路径列表，过滤空行。
func splitPaths(b []byte) []string {
	parts := strings.Split(string(b), "\n")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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
