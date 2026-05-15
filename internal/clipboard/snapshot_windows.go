//go:build windows

package clipboard

// snapshotCurrentClipboard 在 Windows 上通过自研安全实现读取当前剪贴板
// 的文本 / PNG，供 Watcher.bootstrapFromClipboard 建立初始 hash baseline。
//
// 【为什么不用 golang.design/x/clipboard.Read】
// 该库的 readImage 在 Windows 上存在 access violation 崩溃风险（fatal error:
// fault），详见 imagewatch_windows.go 顶部注释。自研路径对全局内存先做
// 大小验证再拷贝到 Go 内存后操作，完全避免野指针访问。
func snapshotCurrentClipboard() (text, image []byte) {
	text = safeReadClipboardText()
	image = safeReadClipboardImage()
	return
}
