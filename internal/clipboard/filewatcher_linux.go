//go:build linux

package clipboard

import (
	"bytes"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// hasFilesOnClipboard 在 Linux 上通过查询剪切板支持的 targets 判断是否有文件。
// 这样比实际 poll 一次文件要轻。xclip 的 TARGETS 列表里若包含 text/uri-list
// 且不是纯文本（比如浏览器里复制的 URL 也会报这个类型），就视为"有文件"。
func hasFilesOnClipboard() bool {
	var out bytes.Buffer
	var cmd *exec.Cmd
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd = exec.Command("xclip", "-selection", "clipboard", "-t", "TARGETS", "-o")
	} else if _, err := exec.LookPath("wl-paste"); err == nil {
		cmd = exec.Command("wl-paste", "--list-types")
	} else {
		return false
	}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false
	}
	targets := out.String()
	if !strings.Contains(targets, "text/uri-list") {
		return false
	}
	// 进一步确认 uri-list 的第一条是 file:// 而不是 http(s):// 等（浏览器复制的链接也会写 uri-list）。
	return firstURIListIsFile()
}

func firstURIListIsFile() bool {
	var cmd *exec.Cmd
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd = exec.Command("xclip", "-selection", "clipboard", "-target", "text/uri-list", "-o")
	} else if _, err := exec.LookPath("wl-paste"); err == nil {
		cmd = exec.Command("wl-paste", "--type", "text/uri-list")
	} else {
		return false
	}
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return false
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return strings.HasPrefix(line, "file://")
	}
	return false
}

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
