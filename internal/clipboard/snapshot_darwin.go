//go:build darwin

package clipboard

// snapshotCurrentClipboard 在 darwin 上通过自研 pasteboardQueue 读取
// 当前剪贴板的文本 / PNG，供 Watcher.bootstrapFromClipboard 建立初始 hash
// baseline 使用。
//
// 【为什么不用 golang.design/x/clipboard.Read】
// 见 filewatcher_darwin.go 顶部注释：裸调 NSPasteboard 与其它并发访问
// 会触发 NSGenericException 崩溃。自研路径走串行化队列，可与文件 / 图片
// watcher 共存。本调用在 Watcher.Start 返回前同步执行一次，此时图片 /
// 文件 watcher 都还没启动，实际上并无并发，但沿用同一条安全路径更稳妥。
func snapshotCurrentClipboard() (text, image []byte) {
	text = readClipboardStringGo()
	image = pollClipboardImagePNG()
	return
}
