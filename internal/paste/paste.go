// Package paste 负责把选中条目写回系统剪切板，并尽力模拟粘贴动作。
//
// 各平台真正"发 Ctrl/Cmd+V"的能力差异很大，因此主流程仅保证写回剪切板；
// 若平台支持模拟按键则在对应 paste_<os>.go 中实现 SendPaste()。
package paste

import (
	"errors"

	"golang.design/x/clipboard"

	"gopaste/internal/types"
)

// ErrUnsupported 平台不支持自动粘贴时返回。
var ErrUnsupported = errors.New("paste: auto-paste unsupported on this platform")

// WriteClipboard 把内容写回系统剪切板。
func WriteClipboard(t types.ItemType, content []byte) error {
	if err := clipboard.Init(); err != nil {
		return err
	}
	switch t {
	case types.TypeImage:
		clipboard.Write(clipboard.FmtImage, content)
	default:
		clipboard.Write(clipboard.FmtText, content)
	}
	return nil
}

// SendPaste 模拟发送 Ctrl/Cmd+V。实现在 paste_<os>.go。
// 默认实现返回 ErrUnsupported（平台文件可覆盖 sendPasteImpl）。
func SendPaste() error { return sendPasteImpl() }
