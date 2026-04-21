//go:build linux

package clipboard

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pollFiles 在 Linux 上读取剪切板中的 text/uri-list 格式（文件管理器复制文件时写入）。
//
// 使用 xclip -selection clipboard -target text/uri-list -o 读取。
// Wayland 下使用 wl-paste --type text/uri-list 替代。
func pollFiles() []FileInfo {
	var output []byte
	var err error

	// 优先 xclip（X11）
	if _, e := exec.LookPath("xclip"); e == nil {
		output, err = exec.Command("xclip", "-selection", "clipboard", "-target", "text/uri-list", "-o").Output()
	} else if _, e := exec.LookPath("wl-paste"); e == nil {
		// Wayland
		output, err = exec.Command("wl-paste", "--type", "text/uri-list").Output()
	}

	if err != nil || len(output) == 0 {
		return nil
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return nil
	}

	lines := strings.Split(raw, "\n")
	files := make([]FileInfo, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // uri-list 允许 # 注释行
		}
		// file:///path/to/file → /path/to/file
		u, err := url.Parse(line)
		if err != nil || u.Scheme != "file" {
			continue
		}
		p := u.Path
		fi := FileInfo{
			Path: p,
			Name: filepath.Base(p),
		}
		if info, err := os.Stat(p); err == nil {
			fi.Size = info.Size()
			fi.IsDir = info.IsDir()
		}
		files = append(files, fi)
	}
	return files
}
