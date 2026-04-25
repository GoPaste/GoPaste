//go:build darwin

package clipboard

// WriteText 把文本写入系统剪切板（darwin：走 pasteboardQueue 串行化）。
//
// 不能用 golang.design/x/clipboard.Write(FmtText)：那条路径裸调
// [NSPasteboard generalPasteboard] setData:，与 file/image/text watcher
// 在 pasteboardQueue 上的访问跨线程并发，会让 AppKit 内部
// _updateTypeCacheIfNeeded 在枚举 _typeArray 时撞上 mutate，抛
// NSGenericException 触发 abort() —— 进程整个消失（SIGABRT）。
// 见 filewatcher_darwin.go 顶部注释。
func WriteText(b []byte) error { return writeClipboardStringGo(b) }

// WriteImage 把 PNG 图片写入系统剪切板（darwin：走 pasteboardQueue 串行化）。
// 同上：不能再用 golang.design/x/clipboard.Write(FmtImage)。
func WriteImage(b []byte) error { return writeClipboardImagePNGGo(b) }
