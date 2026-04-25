package clipboard

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Suppressor 跨 Watcher 共享的抑制器：
// 当 FileWatcher 从 pasteboard 上检出文件 URL 时，系统通常会同时暴露
// 对应的文本（macOS 上一般是 basename，也可能是完整路径），
// 普通的 text watcher 会把它当成一条文本记录。为了避免同一次复制同时产生
// "文件"和"文本"两条历史，本结构体记录最近一次文件事件的路径集合与时间戳，
// 文本 Watcher 在入库前据此跳过。
type Suppressor struct {
	mu        sync.Mutex
	paths     map[string]struct{} // 完整路径
	basenames map[string]struct{} // 文件名（basename），用于匹配 pasteboard 上的 display name
	until     time.Time
}

// defaultSuppressWindow 抑制窗口：文件事件触发后这段时间内到来的匹配文本视为重复。
const defaultSuppressWindow = 3 * time.Second

// NewSuppressor 构造一个抑制器。
func NewSuppressor() *Suppressor {
	return &Suppressor{
		paths:     make(map[string]struct{}),
		basenames: make(map[string]struct{}),
	}
}

// MarkFiles 登记一批文件路径，进入抑制窗口。
// 多次调用会合并路径集合并刷新过期时间。
func (s *Suppressor) MarkFiles(paths []string) {
	if s == nil || len(paths) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 过期则先清空
	if time.Now().After(s.until) {
		s.paths = make(map[string]struct{}, len(paths))
		s.basenames = make(map[string]struct{}, len(paths))
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		s.paths[p] = struct{}{}
		base := filepath.Base(strings.TrimRight(p, "/"))
		if base != "" && base != "." && base != "/" {
			s.basenames[base] = struct{}{}
		}
	}
	s.until = time.Now().Add(defaultSuppressWindow)
}

// ShouldSuppressText 判断这段文本是否是刚刚的文件事件附带的路径或文件名，
// 若命中（在抑制窗口内）返回 true，调用方应丢弃。
//
// 匹配规则：文本逐行拆分，每行必须满足下列任一条件方可视为"属于文件事件"：
//  1. 与某个登记的完整路径相等；
//  2. 与某个登记的 basename 相等（macOS Finder 复制时 pasteboard 附带的文本是文件名）。
//
// 全部行都命中才抑制；有任一行不匹配即视为普通文本。
func (s *Suppressor) ShouldSuppressText(raw []byte) bool {
	if s == nil || len(raw) == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().After(s.until) || (len(s.paths) == 0 && len(s.basenames) == 0) {
		return false
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 0 {
		return false
	}
	matched := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := s.paths[line]; ok {
			matched++
			continue
		}
		if _, ok := s.basenames[line]; ok {
			matched++
			continue
		}
		// 有任何非空行既不是完整路径也不是 basename → 非文件事件附带的文本。
		return false
	}
	return matched > 0
}
