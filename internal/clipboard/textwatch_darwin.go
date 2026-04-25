//go:build darwin

package clipboard

import (
	"context"
	"sync/atomic"
	"time"
)

// startTextWatch 在 macOS 上启动自研文本监听。
//
// 【为什么不用 golang.design/x/clipboard.Watch(FmtText)】
// 见 filewatcher_darwin.go 顶部注释（崩溃排查 2026-04-24）：
//   golang.design 的 Watch 是后台 goroutine 裸调 NSPasteboard.dataForType:，
//   不在我们的 pasteboardQueue 上。它和 file/image watcher / CopyToClipboard
//   并发访问 [NSPasteboard generalPasteboard]（单例），AppKit 内部
//   _updateTypeCacheIfNeeded 枚举 _typeArray 时被另一线程 setData: 改动 →
//     NSGenericException "Collection mutated while being enumerated"
//     → terminate handler → abort() → SIGABRT，进程整个消失。
//   表现：用户点击粘贴，gopaste 进程"闪退" 1-2 次内必现，无 panic 栈。
//
// 本实现轮询 changeCount，仅在变化时通过 pasteboardQueue 串行读字符串，
// 与 file/image watcher / CopyToClipboard 完全互斥，根除竞态。
//
// 轮询间隔与 golang.design 原版保持一致（1s），既不增加 CPU 也不影响 UX
// （file watcher 500ms 已经足够覆盖大部分用户感知）。本通道只发"变化后的内容"，
// 不做去重——上层 watcher.handle 已基于 hash 去重，这里保持简单。
func startTextWatch(ctx context.Context) <-chan []byte {
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
				// 仅文本：图片/文件由各自 watcher 处理；
				// 这里如果 pasteboard 里只有图/文件，readString 返回 nil，
				// 直接 skip 不会误派发。
				b := readClipboardStringGo()
				if len(b) == 0 {
					continue
				}
				select {
				case out <- b:
				default:
					// 极端堆积保护，丢弃；上层 hash 去重已能容忍漏推。
				}
			}
		}
	}()
	return out
}
