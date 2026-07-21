package clipboard

import (
	"context"
	"strings"
	"time"

	"gopaste/internal/storage"
	"gopaste/internal/types"
)

// FileInfo 表示剪切板中的一个文件。
type FileInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	IsDir bool  `json:"isDir"`
}

// FileWatcher 监听系统剪切板中的文件复制事件。
// 各平台由 filewatcher_<os>.go 实现 pollFiles()。
type FileWatcher struct {
	out        chan types.Item
	lastSig    string
	suppressor *Suppressor // 与 text Watcher 共享，检测到文件后抑制对应文本入库
}

// NewFileWatcher 创建文件剪切板监听器。
func NewFileWatcher() *FileWatcher {
	return &FileWatcher{out: make(chan types.Item, 16)}
}

// SetSuppressor 绑定共享抑制器。
func (fw *FileWatcher) SetSuppressor(s *Suppressor) { fw.suppressor = s }

// Events 返回文件事件通道。
func (fw *FileWatcher) Events() <-chan types.Item { return fw.out }

// HasFilesOnClipboard 报告系统剪贴板当前是否含文件列表（Windows CF_HDROP /
// macOS file URL / Linux 无）。供上层做粘贴诊断：确认写入后 / 发送粘贴键后
// 文件格式是否仍在剪贴板，用于区分"焦点没切回去"与"剪贴板被 WebView2 清空"。
func HasFilesOnClipboard() bool { return pasteboardHasFile() }

// Start 启动轮询监听（500ms 间隔）。
func (fw *FileWatcher) Start(ctx context.Context) {
	// 先同步读一次当前剪贴板文件列表，把签名作为 lastSig 初值。
	// 目的：若启动时剪贴板里已有文件复制（用户上次复制但未粘贴，或
	// 其它程序占用 CF_HDROP），首次 tick 轮询到同样列表会被 lastSig
	// 去重，不会把这份"启动前残留"当作新事件入库。
	// 用户启动后复制的新文件一定产生不同 sig，会正常入库。
	//
	// 早期实现曾用 `bootstrapped bool` 吞掉第一帧非空结果，但当"启动时
	// 剪贴板为空、用户启动后第一次复制文件" race 到首帧时，这次复制
	// 也会被错误吞掉（偶现现象）。用内容签名做 baseline 没有这个窗口。
	fw.bootstrapFromClipboard()

	go func() {
		defer close(fw.out)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				files := pollFiles()
				if len(files) == 0 {
					continue
				}
				fw.handleFiles(files)
			}
		}
	}()
}

// bootstrapFromClipboard 读一次当前剪贴板文件列表，把路径拼接后的 hash
// 塞进 lastSig。若剪贴板当前没有文件则保持空。
func (fw *FileWatcher) bootstrapFromClipboard() {
	files := pollFiles()
	if len(files) == 0 {
		return
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	fw.lastSig = storage.HashBytes([]byte(strings.Join(paths, "\n")))
}

func (fw *FileWatcher) handleFiles(files []FileInfo) {
	// 用所有路径拼成签名做去重
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	// 尽早登记到共享抑制器：即使本次签名与上次相同（重复复制同一文件），
	// 也要刷新抑制窗口，避免被后续路径文本污染历史。
	if fw.suppressor != nil {
		fw.suppressor.MarkFiles(paths)
	}
	sig := storage.HashBytes([]byte(strings.Join(paths, "\n")))
	if sig == fw.lastSig {
		return
	}
	fw.lastSig = sig

	// 构造预览文本
	var preview strings.Builder
	var totalSize int64
	for i, f := range files {
		if i > 0 {
			preview.WriteString("\n")
		}
		if f.IsDir {
			preview.WriteString("📁 ")
		}
		preview.WriteString(f.Name)
		totalSize += f.Size
	}

	// 完整路径列表存入 Content（序列化为换行分隔的路径）
	content := []byte(strings.Join(paths, "\n"))

	item := types.Item{
		Hash:      sig,
		Type:      types.TypeFile,
		Content:   content,
		Preview:   preview.String(),
		Size:      totalSize,
		CharCount: len(files),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	select {
	case fw.out <- item:
	default:
	}
}
