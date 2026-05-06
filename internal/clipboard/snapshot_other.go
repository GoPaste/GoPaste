//go:build !darwin

package clipboard

import "golang.design/x/clipboard"

// snapshotCurrentClipboard 在非 darwin 平台读取当前剪贴板的文本 / PNG，
// 供 Watcher.bootstrapFromClipboard 建立初始 hash baseline。
//
// Windows / Linux 不存在 darwin 上的 NSPasteboard 并发崩溃风险，直接走
// golang.design/x/clipboard 提供的 Read 接口即可。
func snapshotCurrentClipboard() (text, image []byte) {
	text = clipboard.Read(clipboard.FmtText)
	image = clipboard.Read(clipboard.FmtImage)
	return
}
