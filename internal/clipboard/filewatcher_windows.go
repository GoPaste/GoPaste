//go:build windows

package clipboard

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	shell32             = syscall.NewLazyDLL("shell32.dll")
	openClipboard       = user32.NewProc("OpenClipboard")
	closeClipboard      = user32.NewProc("CloseClipboard")
	getClipboardData    = user32.NewProc("GetClipboardData")
	isClipboardFmtAvail = user32.NewProc("IsClipboardFormatAvailable")
	globalLock          = kernel32.NewProc("GlobalLock")
	globalUnlock        = kernel32.NewProc("GlobalUnlock")
	dragQueryFileW      = shell32.NewProc("DragQueryFileW")
)

const cfHDROP = 15

// hasFilesOnClipboard 在 Windows 上仅检查剪切板是否含 CF_HDROP（文件列表），
// 不需要 OpenClipboard，调用极其轻量。
//
// Windows 在资源管理器里复制文件/文件夹时，剪切板会同时写入 CF_HDROP 与
// CF_UNICODETEXT（文件名），text watcher 会把文件名误当文本入库 —— 本函数
// 用来让上层忽略这种情况。
func hasFilesOnClipboard() bool {
	ret, _, _ := isClipboardFmtAvail.Call(uintptr(cfHDROP))
	return ret != 0
}

// pollFiles 读取 Windows 剪切板中的 CF_HDROP 数据（文件路径列表）。
func pollFiles() []FileInfo {
	// 检查是否有 CF_HDROP
	ret, _, _ := isClipboardFmtAvail.Call(uintptr(cfHDROP))
	if ret == 0 {
		return nil
	}

	ret, _, _ = openClipboard.Call(0)
	if ret == 0 {
		return nil
	}
	defer closeClipboard.Call()

	h, _, _ := getClipboardData.Call(uintptr(cfHDROP))
	if h == 0 {
		return nil
	}

	ptr, _, _ := globalLock.Call(h)
	if ptr == 0 {
		return nil
	}
	defer globalUnlock.Call(h)

	// DragQueryFileW(hDrop, 0xFFFFFFFF, nil, 0) 返回文件数量
	count, _, _ := dragQueryFileW.Call(ptr, 0xFFFFFFFF, 0, 0)
	if count == 0 {
		return nil
	}

	files := make([]FileInfo, 0, count)
	buf := make([]uint16, 1024)

	for i := uintptr(0); i < count; i++ {
		// DragQueryFileW(hDrop, i, buf, len(buf)) 获取第 i 个文件路径
		n, _, _ := dragQueryFileW.Call(ptr, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n == 0 {
			continue
		}
		path := syscall.UTF16ToString(buf[:n])
		fi := FileInfo{
			Path: path,
			Name: filepath.Base(path),
		}
		if info, err := os.Stat(path); err == nil {
			fi.Size = info.Size()
			fi.IsDir = info.IsDir()
		}
		files = append(files, fi)
	}
	return files
}
