//go:build windows

package clipboard

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"
	"unicode/utf16"
)

// startTextWatch 在 Windows 上启动自研文本监听。
//
// 【为什么不用 golang.design/x/clipboard.Watch(FmtText)】
// 该库的 readText 同样使用 reflect.SliceHeader + unsafe.Pointer 操作全局
// 内存，虽然崩溃概率比 readImage 低（因为文本通常较小且有 NUL 终止符），
// 但为了完整性和一致性，Windows 平台全部使用自研实现，确保不依赖有问题的
// 第三方库进行剪贴板读取。
//
// 本实现使用 GlobalSize 预先验证内存大小，并将数据拷贝到 Go 内存后再解析。
func startTextWatch(ctx context.Context) <-chan []byte {
	out := make(chan []byte, 1)
	var lastSeq int64 = -1
	go func() {
		defer close(out)
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				seq := int64(clipboardSequenceNumber())
				prev := atomic.LoadInt64(&lastSeq)
				if seq == prev {
					continue
				}
				atomic.StoreInt64(&lastSeq, seq)
				b := safeReadClipboardText()
				if b == nil {
					continue
				}
				select {
				case out <- b:
				default:
				}
			}
		}
	}()
	return out
}

// safeReadClipboardText 安全地从 Windows 剪贴板读取 Unicode 文本。
// 先将全局内存拷贝到 Go 管理的 []byte 中，再解析 UTF-16 → UTF-8。
func safeReadClipboardText() []byte {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// 检查是否有 CF_UNICODETEXT 可用
	ret, _, _ := isClipboardFmtAvail.Call(uintptr(cfUnicodeText))
	if ret == 0 {
		return nil
	}

	// 打开剪贴板（最多重试 5 次）
	var opened bool
	for i := 0; i < 5; i++ {
		r, _, _ := openClipboard.Call(0)
		if r != 0 {
			opened = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !opened {
		return nil
	}
	defer closeClipboard.Call()

	hMem, _, _ := getClipboardData.Call(uintptr(cfUnicodeText))
	if hMem == 0 {
		return nil
	}

	size := globalMemSize(hMem)
	if size == 0 || size < 2 { // 至少需要一个 UTF-16 字符（2 字节）
		return nil
	}

	ptr, _, _ := globalLock.Call(hMem)
	if ptr == 0 {
		return nil
	}

	// 拷贝到 Go 管理的内存
	raw := make([]byte, size)
	copyFromUintptr(raw, ptr, size)
	globalUnlock.Call(hMem)

	// 将 []byte 解释为 []uint16 并找到 NUL 终止符
	u16 := bytesToUint16Slice(raw)
	// 找到第一个 0（NUL terminator）
	n := len(u16)
	for i, v := range u16 {
		if v == 0 {
			n = i
			break
		}
	}
	if n == 0 {
		return nil
	}

	// UTF-16 → UTF-8
	s := string(utf16.Decode(u16[:n]))
	if s == "" {
		return nil
	}
	return []byte(s)
}

// bytesToUint16Slice 将 []byte 安全转换为 []uint16（不共享底层内存）。
func bytesToUint16Slice(b []byte) []uint16 {
	n := len(b) / 2
	if n == 0 {
		return nil
	}
	result := make([]uint16, n)
	for i := 0; i < n; i++ {
		result[i] = uint16(b[i*2]) | uint16(b[i*2+1])<<8
	}
	return result
}
