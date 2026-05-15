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

	"gopaste/internal/lang"
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
//
// 【darwin 与其他平台的差异】
// 非 darwin：直接用 golang.design/x/clipboard.Watch(FmtText) 监听文本。
// darwin：必须用自家 startTextWatch — golang.design 的 Watch 在后台 goroutine
// 裸调 NSPasteboard.dataForType:，与我们 file/image watcher 在 pasteboardQueue
// 里的访问并发，会触发 NSGenericException 被 abort()，进程整个消失。
// 详细排查见 filewatcher_darwin.go 顶部注释。
func (w *Watcher) Start(ctx context.Context) error {
	if err := clipboard.Init(); err != nil {
		return fmt.Errorf("clipboard: init: %w", err)
	}

	// 先同步读一次当前剪贴板内容，把内容 hash 作为 lastSig 初值。
	// 目的：防止启动后首个 tick 把"启动前就已存在于系统剪贴板里的内容"
	// 当成一次新复制入库（症状：清空数据 + 重启后，凭空复现最后一条记录）。
	//
	// 为什么不在 textwatch/imagewatch 里用 pasteboardChangeCount 作 baseline：
	// 那样会与"App 启动期间用户刚好复制"形成 race —— baseline 采到的
	// changeCount 已经包含了用户这次复制，tick 阶段 cc == prev 就不会派发，
	// 表现为"偶现首次复制丢失"。用内容 hash 做 baseline 没有这个窗口：
	// 用户复制的新内容 hash 与历史残留 hash 不同，会正常派发。
	//
	// 只在 Start 最开始调用这一次，此时 FileWatcher/ImageWatch 都还没跑，
	// 不存在并发竞态。
	w.bootstrapFromClipboard()

	textCh := startTextWatch(ctx)
	// 图片监听走平台特化实现（见 imagewatch_{darwin,other}.go）。
	// darwin 上 golang.design/x/clipboard 只识别 public.png，漏掉系统截图
	// 这种 public.tiff 场景——那里改由我们自己轮询并做 TIFF→PNG 转换。
	imageCh := startImageWatch(ctx)

	go w.loop(ctx, textCh, imageCh)
	return nil
}

// bootstrapFromClipboard 把当前系统剪贴板内容的 hash 预先塞进 lastSig，
// 使得启动后首次读到的若是"启动前残留"会被 hash 去重，不会入库。
// 取剪贴板里"最新"那份内容的 hash：优先图片（图片 + 文件路径文本通常
// 伴生出现，文件路径文本并不稳定；而图片内容是稳定指纹）。
func (w *Watcher) bootstrapFromClipboard() {
	text, image := snapshotCurrentClipboard()
	var baseline []byte
	switch {
	case len(image) > 0:
		baseline = image
	case len(text) > 0:
		baseline = text
	default:
		return
	}
	w.mu.Lock()
	w.lastSig = storage.HashBytes(baseline)
	w.mu.Unlock()
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

	// 不在这里做"吞掉首次事件"的 bootstrap 兜底：
	//   - darwin：startTextWatch / startImageWatch 已用 pasteboardChangeCount
	//     作为 baseline，textCh/imageCh 天然只派发"启动之后"的变更；
	//   - 非 darwin：golang.design/x/clipboard.Watch 内部同样只比较 changeCount
	//     变化，不会重放启动前的残留。
	// 若在此再加 bootstrapped 吞掉首帧，会误伤用户启动后的第一次真正复制
	// （表现为首次复制不同步到 GoPaste）。

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
	// 仅 code 类型预先识别语言并落库，避免每次列表渲染都在前端跑 hljs 检测。
	// 详见 internal/lang/detect.go：对中文笔记类内容会主动放弃识别避免误判。
	if t == types.TypeCode {
		item.Language = lang.Detect(string(raw))
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
	// 中文为主的内容（笔记 / 聊天 / 文档），即使含 = 和换行也不应判为 code。
	// 与 lang.Detect 第一道自然语言保护同构，避免出现"分类是 Code 但识别不出语言"
	// 而最终展示通用 Code 标签的体验问题（详见 docs/todo.md）。
	if lang.IsMostlyCJK(s) {
		return types.TypeText
	}
	// 结构化数据 + Chroma 识别：覆盖单行 JSON/XML 等场景
	if lang.LooksLikeCode(s) {
		return types.TypeCode
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
