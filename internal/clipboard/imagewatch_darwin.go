//go:build darwin

package clipboard

import (
	"context"
	"sync/atomic"
	"time"
)

// startImageWatch 在 macOS 上启动自研图片监听。
//
// 【为什么不用 golang.design/x/clipboard.Watch(FmtImage)】
// 该库的 read_image 只查 NSPasteboardTypePNG（public.png）；但 macOS 系统
// 截图（Cmd+Shift+3/4/5）、Preview「复制」、绝大多数 app 的「复制图片」
// 都写入 NSPasteboardTypeTIFF（public.tiff），于是截图后面板里压根没
// 新增记录。symptom 就是用户报的"不支持图片"。
//
// 本实现自己轮询 NSPasteboard.changeCount，并优先读 PNG、退化到 TIFF
// 并转 PNG（在 Obj-C 侧用 NSBitmapImageRep 完成，避免 Go 侧引入 tiff 解码依赖）。
//
// 轮询间隔与 golang.design 原实现保持一致（1 秒），避免显著增加 CPU。
// FileWatcher 已经在做 500ms 的 changeCount 轮询——两者独立、都走同一个
// pasteboardQueue 串行化，不会相互踩踏。
//
// 关于 Watch context 生命周期：返回的 channel 在 ctx.Done() 后会关闭，
// watcher.go 的 select 会把 imageCh 设为 nil 然后退出 loop（和原 golang.design
// watch 行为完全一致）。
func startImageWatch(ctx context.Context) <-chan []byte {
	out := make(chan []byte, 1)
	var lastCC int64 = -1
	go func() {
		defer close(out)
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cc := pasteboardChangeCountGo()
				prev := atomic.LoadInt64(&lastCC)
				if cc == prev {
					continue
				}
				atomic.StoreInt64(&lastCC, cc)
				// 只有当前 pasteboard 没有文件才当图片看：
				// "复制文件" 在有些 app（如 Finder）会同时产生 icon tiff——我们
				// 已经把它当文件处理了，别再重复落盘为图片。
				if pasteboardHasFile() {
					continue
				}
				b := pollClipboardImagePNG()
				if b == nil {
					continue
				}
				// 阻塞写入不会发生——上游 loop 是 select 快速消费。
				// 但为防御极端情况（watcher 未启动 / 堆积）还是用 default 丢弃。
				select {
				case out <- b:
				default:
				}
			}
		}
	}()
	return out
}
