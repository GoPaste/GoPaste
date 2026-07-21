//go:build windows

package clipboard

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.design/x/clipboard"
)

// WriteText Windows 平台沿用 golang.design/x/clipboard。
func WriteText(b []byte) error {
	if err := clipboard.Init(); err != nil {
		return fmt.Errorf("clipboard: init: %w", err)
	}
	clipboard.Write(clipboard.FmtText, b)
	return nil
}

// WriteImage Windows 平台沿用 golang.design/x/clipboard。
func WriteImage(b []byte) error {
	if err := clipboard.Init(); err != nil {
		return fmt.Errorf("clipboard: init: %w", err)
	}
	clipboard.Write(clipboard.FmtImage, b)
	return nil
}

// WriteFiles 把文件路径列表以 CF_HDROP 格式写入 Windows 剪贴板，
// 使目标程序（资源管理器等）能够识别并粘贴实际文件，而非纯文本。
//
// CF_HDROP 内存布局（DROPFILES 结构 + 双 NUL 终止的 UTF-16 路径列表）：
//
//	struct DROPFILES {
//	    DWORD pFiles;  // 路径列表相对于结构体起始的偏移（字节），固定 sizeof(DROPFILES) = 20
//	    POINT pt;      // 拖放起点，填 (0,0) 即可
//	    BOOL  fNC;     // 非客户区标志，填 0
//	    BOOL  fWide;   // TRUE(1) 表示路径为 UTF-16LE
//	}
//	<UTF-16LE 路径 1>\0
//	<UTF-16LE 路径 2>\0
//	...
//	\0  ← 额外的 NUL 终止整个列表
func WriteFiles(paths []string) error {
	const dropfilesSize = 20 // sizeof(DROPFILES)

	// Win32 要求 OpenClipboard 和 CloseClipboard 必须在同一个 OS 线程调用。
	// Go goroutine 默认在任意线程运行；若 Open/Close 落到不同线程，
	// Windows 会静默拒绝后续的 OpenClipboard，导致第二次及之后的调用失败。
	// LockOSThread 确保整个函数在同一 OS 线程执行，与 golang.design/x/clipboard
	// 内部的实现策略一致。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// 1. 把所有路径转为 UTF-16LE，计算路径区总字节数
	encoded := make([][]uint16, 0, len(paths))
	totalPathBytes := 0
	for _, p := range paths {
		u16, err := syscall.UTF16FromString(p) // 自动追加 NUL
		if err != nil {
			return fmt.Errorf("clipboard: utf16 encode %q: %w", p, err)
		}
		encoded = append(encoded, u16)
		totalPathBytes += len(u16) * 2
	}
	totalPathBytes += 2 // 列表终止 NUL uint16

	totalSize := dropfilesSize + totalPathBytes

	// 2. 在 Go 侧构造完整字节缓冲
	gobuf := make([]byte, totalSize)

	// DROPFILES.pFiles = 20
	binary.LittleEndian.PutUint32(gobuf[0:4], uint32(dropfilesSize))
	// DROPFILES.pt.x / pt.y = 0（make 已清零）
	// DROPFILES.fNC = 0（make 已清零）
	// DROPFILES.fWide = 1（TRUE，UTF-16）
	binary.LittleEndian.PutUint32(gobuf[16:20], 1)

	// 写入路径列表（UTF-16LE，每条含结尾 NUL）
	off := dropfilesSize
	for _, u16 := range encoded {
		for _, ch := range u16 {
			gobuf[off] = byte(ch)
			gobuf[off+1] = byte(ch >> 8)
			off += 2
		}
	}
	// 列表终止 NUL（gobuf 已零值，off 处无需再写）

	// 3. 分配可移动全局内存并写入
	const gMemMoveable = 0x0002
	hGlobal, _, err := globalAlloc.Call(gMemMoveable, uintptr(totalSize))
	if hGlobal == 0 {
		return fmt.Errorf("clipboard: GlobalAlloc failed: %w", err)
	}

	ptr, _, err := globalLock.Call(hGlobal)
	if ptr == 0 {
		globalFree.Call(hGlobal)
		return fmt.Errorf("clipboard: GlobalLock failed: %w", err)
	}

	// 把 Go 侧缓冲拷入全局内存。
	// RtlMoveMemory(dst, src, size)：dst=全局内存地址(uintptr)，src=&gobuf[0]
	rtlMoveMemory.Call(ptr, uintptr(unsafe.Pointer(&gobuf[0])), uintptr(totalSize))
	_ = gobuf[0] // 防止 GC 在 RtlMoveMemory 完成前回收 gobuf

	globalUnlock.Call(hGlobal)

	// 4. 写入剪贴板（在已 LockOSThread 的线程上执行，保证 Open/Close 同线程）
	//
	// 其他程序（包括 golang.design goroutine）可能短暂持有剪贴板，
	// 重试几次直到成功，与 golang.design/x/clipboard 的策略一致。
	for {
		r, _, _ := openClipboard.Call(0)
		if r != 0 {
			break
		}
	}

	ret, _, err := emptyClipboard.Call()
	if ret == 0 {
		closeClipboard.Call()
		globalFree.Call(hGlobal)
		return fmt.Errorf("clipboard: EmptyClipboard failed: %w", err)
	}

	// SetClipboardData 成功后所有权转移给系统，不能再 GlobalFree
	ret, _, err = setClipboardData.Call(uintptr(cfHDROP), hGlobal)
	if ret == 0 {
		closeClipboard.Call()
		globalFree.Call(hGlobal)
		return fmt.Errorf("clipboard: SetClipboardData failed: %w", err)
	}

	closeClipboard.Call()
	return nil
}
