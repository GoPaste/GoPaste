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
	out     chan types.Item
	lastSig string
}

// NewFileWatcher 创建文件剪切板监听器。
func NewFileWatcher() *FileWatcher {
	return &FileWatcher{out: make(chan types.Item, 16)}
}

// Events 返回文件事件通道。
func (fw *FileWatcher) Events() <-chan types.Item { return fw.out }

// Start 启动轮询监听（500ms 间隔）。
func (fw *FileWatcher) Start(ctx context.Context) {
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

func (fw *FileWatcher) handleFiles(files []FileInfo) {
	// 用所有路径拼成签名做去重
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
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
