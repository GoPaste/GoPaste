//go:build windows

package clipboard

import (
	"syscall"
	"unsafe"
)

// Windows DLL 和 API 声明。
// 将所有剪贴板相关的系统调用集中在这里，供 filewatcher_windows.go、
// imagewatch_windows.go、textwatch_windows.go 共同使用。
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	// user32.dll
	openClipboard       = user32.NewProc("OpenClipboard")
	closeClipboard      = user32.NewProc("CloseClipboard")
	emptyClipboard      = user32.NewProc("EmptyClipboard")
	getClipboardData    = user32.NewProc("GetClipboardData")
	setClipboardData    = user32.NewProc("SetClipboardData")
	isClipboardFmtAvail = user32.NewProc("IsClipboardFormatAvailable")
	getClipboardSeqNum  = user32.NewProc("GetClipboardSequenceNumber")

	// kernel32.dll
	globalLock   = kernel32.NewProc("GlobalLock")
	globalUnlock = kernel32.NewProc("GlobalUnlock")
	globalSize   = kernel32.NewProc("GlobalSize")
	globalAlloc  = kernel32.NewProc("GlobalAlloc")
	globalFree   = kernel32.NewProc("GlobalFree")

	// shell32.dll
	dragQueryFileW = shell32.NewProc("DragQueryFileW")
)

// 剪贴板格式常量
const (
	cfHDROP       = 15 // CF_HDROP — 文件列表
	cfUnicodeText = 13 // CF_UNICODETEXT — UTF-16 文本
	cfDIBV5       = 17 // CF_DIBV5 — BITMAPV5 图像
	cfDIB         = 8  // CF_DIB — BMP 图像
)

// globalMemSize 获取全局内存块的大小（字节数）。
func globalMemSize(hMem uintptr) uintptr {
	size, _, _ := globalSize.Call(hMem)
	return size
}

// clipboardSequenceNumber 获取剪贴板序列号（每次剪贴板内容变更 +1）。
func clipboardSequenceNumber() uint32 {
	ret, _, _ := getClipboardSeqNum.Call()
	return uint32(ret)
}

// copyFromUintptr 将 uintptr 指向的内存安全拷贝到 dst 切片。
// 使用 RtlMoveMemory 进行拷贝，避免 go vet 对 uintptr→unsafe.Pointer 转换的警告。
func copyFromUintptr(dst []byte, src uintptr, size uintptr) {
	if len(dst) == 0 || size == 0 {
		return
	}
	// 使用 RtlMoveMemory（kernel32）做内存拷贝，无需 uintptr→unsafe.Pointer 转换
	rtlMoveMemory.Call(
		uintptr(unsafe.Pointer(&dst[0])),
		src,
		size,
	)
}

var rtlMoveMemory = kernel32.NewProc("RtlMoveMemory")
