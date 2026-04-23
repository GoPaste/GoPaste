// Package clipboard 提供跨平台剪切板监听能力。
//
// 监听文本与图片（PNG）的变更事件，并通过 channel 向上游推送捕获到的 Item（未持久化、未加密）。
package clipboard

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.design/x/clipboard"

	"gopaste/internal/storage"
	"gopaste/internal/types"
)

// previewLimit 文本预览最大长度（字符数）。
const previewLimit = 200

// Watcher 剪切板监听器。
type Watcher struct {
	out        chan types.Item
	lastSig    string
	mu         sync.Mutex
	suppressor *Suppressor // 与 FileWatcher 共享，用于跳过文件事件附带的路径文本
}

// New 创建一个 Watcher。
func New() *Watcher {
	return &Watcher{out: make(chan types.Item, 32)}
}

// SetSuppressor 绑定共享的抑制器（可选）。若未设置，文本文件路径不会被去重。
func (w *Watcher) SetSuppressor(s *Suppressor) { w.suppressor = s }

// Events 返回事件通道。每次剪切板变化（去重后）会推送一个 Item。
func (w *Watcher) Events() <-chan types.Item { return w.out }

// Start 启动监听直到 ctx 结束。
//
// 初始化失败会返回错误（例如 Linux 缺少 xclip/xsel 或 Wayland 限制）。
func (w *Watcher) Start(ctx context.Context) error {
	if err := clipboard.Init(); err != nil {
		return fmt.Errorf("clipboard: init: %w", err)
	}

	textCh := clipboard.Watch(ctx, clipboard.FmtText)
	imageCh := clipboard.Watch(ctx, clipboard.FmtImage)

	go w.loop(ctx, textCh, imageCh)
	return nil
}

func (w *Watcher) loop(ctx context.Context, textCh, imageCh <-chan []byte) {
	defer close(w.out)
	for {
		select {
		case <-ctx.Done():
			return
		case b, ok := <-textCh:
			if !ok {
				textCh = nil
				continue
			}
			// 复制文件/文件夹时，系统会同时写入文件 URL 和文件名文本。
			// 文件 URL 由 FileWatcher 处理；这里必须忽略那个伴随文本，
			// 否则同一次复制会既入"文件"又入"文本"两条记录。
			if hasFilesOnClipboard() {
				continue
			}
			w.handle(typeOfText(b), b)
		case b, ok := <-imageCh:
			if !ok {
				imageCh = nil
				continue
			}
			w.handle(types.TypeImage, b)
		}
		if textCh == nil && imageCh == nil {
			return
		}
	}
}

func (w *Watcher) handle(t types.ItemType, raw []byte) {
	if len(raw) == 0 {
		return
	}
	// 文件复制去重：
	//  1) 若当前系统剪切板同时含 fileURL，说明这次变更是一个"文件复制"，
	//     文本槽位只是 Finder 附带的 basename/path —— 直接丢弃。
	//     pasteboardHasFile 内部按 changeCount 做缓存，高频调用也廉价。
	//  2) 若 FileWatcher 已经登记了路径/文件名集合（文件先到、文本后到），
	//     Suppressor 再兜底匹配一次。
	// 注意：这里故意不做 time.Sleep 等"延迟重试"，因为阻塞 watcher goroutine
	// 会导致 golang.design/x/clipboard 的事件堆积以及 CGo 重入频率升高，
	// 进而在 macOS 上触发 AppKit 异常循环，被系统判定为 CPU 失控。
	if t != types.TypeImage {
		if pasteboardHasFile() {
			return
		}
		if w.suppressor != nil && w.suppressor.ShouldSuppressText(raw) {
			return
		}
	}
	hash := storage.HashBytes(raw)

	w.mu.Lock()
	if hash == w.lastSig {
		w.mu.Unlock()
		return
	}
	w.lastSig = hash
	w.mu.Unlock()

	item := types.Item{
		Hash:      hash,
		Type:      t,
		Content:   raw,
		Size:      int64(len(raw)),
		Preview:   previewOf(t, raw),
		CharCount: charCount(t, raw),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	select {
	case w.out <- item:
	default:
		// 缓冲满则丢弃（极端场景下保护主线程）
	}
}

// previewOf 生成用于展示和搜索的明文摘要。
func previewOf(t types.ItemType, raw []byte) string {
	switch t {
	case types.TypeImage:
		if cfg, err := png.DecodeConfig(bytes.NewReader(raw)); err == nil {
			return fmt.Sprintf("[image] %dx%d · %.1f KB", cfg.Width, cfg.Height, float64(len(raw))/1024)
		}
		return fmt.Sprintf("[image] %.1f KB", float64(len(raw))/1024)
	default:
		s := string(raw)
		if !utf8.ValidString(s) {
			return fmt.Sprintf("[binary] %d bytes", len(raw))
		}
		s = strings.TrimSpace(s)
		if r := []rune(s); len(r) > previewLimit {
			s = string(r[:previewLimit])
		}
		return s
	}
}

// typeOfText 粗略判别链接/代码/纯文本。
func typeOfText(b []byte) types.ItemType {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return types.TypeText
	}
	// URL
	if u, err := url.ParseRequestURI(s); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return types.TypeLink
	}
	// 代码启发：包含常见符号 + 换行
	if strings.Contains(s, "\n") && strings.ContainsAny(s, "{};=") {
		return types.TypeCode
	}
	return types.TypeText
}

// charCount 返回文本字符数（图片返回 0）。
func charCount(t types.ItemType, raw []byte) int {
	if t == types.TypeImage {
		return 0
	}
	return utf8.RuneCount(raw)
}
