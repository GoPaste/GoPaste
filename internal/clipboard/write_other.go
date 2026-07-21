//go:build linux

package clipboard

import (
	"fmt"

	"golang.design/x/clipboard"
)

// WriteText 非 darwin 平台沿用 golang.design/x/clipboard。
func WriteText(b []byte) error {
	if err := clipboard.Init(); err != nil {
		return fmt.Errorf("clipboard: init: %w", err)
	}
	clipboard.Write(clipboard.FmtText, b)
	return nil
}

// WriteImage 非 darwin 平台沿用 golang.design/x/clipboard。
func WriteImage(b []byte) error {
	if err := clipboard.Init(); err != nil {
		return fmt.Errorf("clipboard: init: %w", err)
	}
	clipboard.Write(clipboard.FmtImage, b)
	return nil
}

// WriteFiles Linux 平台：把路径列表写为纯文本（file:// URI 列表）。
// 真正的 text/uri-list 写回依赖 xclip/wl-copy，当前作为降级方案写文本。
// TODO: 若需要完整 Linux 文件粘贴支持，可通过 xclip -target text/uri-list 实现。
func WriteFiles(paths []string) error {
	uris := make([]byte, 0, 128)
	for i, p := range paths {
		if i > 0 {
			uris = append(uris, '\n')
		}
		uris = append(uris, []byte("file://"+p)...)
	}
	return WriteText(uris)
}
