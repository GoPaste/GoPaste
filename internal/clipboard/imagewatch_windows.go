//go:build windows

package clipboard

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"sync/atomic"
	"time"
)

// startImageWatch 在 Windows 上启动自研图片监听。
//
// 【为什么不用 golang.design/x/clipboard.Watch(FmtImage)】
// 该库的 readImage 函数使用 reflect.SliceHeader + unsafe.Pointer 直接在
// 全局内存上构建 Go slice，并按 Width*Height 逐像素遍历。当剪贴板中的
// BITMAPV5HEADER 的 Height 为负数（top-down DIB）被 uint32() 转换后溢出为
// 巨大值，或者剪贴板在读取过程中被其他程序修改/清空时，都会触发 access
// violation（0xc0000005），这是 runtime fatal 级别的错误，无法 recover。
//
// 本实现：
// 1. 使用 GetClipboardSequenceNumber 轮询变化（与原库一致）
// 2. 安全读取 CF_DIBV5/CF_DIB 数据——先用 GlobalSize 验证内存大小、检查
//    Width/Height 的合法性后再拷贝到 Go 管理的 []byte 中操作
// 3. 将 DIB 像素数据转为 PNG 格式输出
func startImageWatch(ctx context.Context) <-chan []byte {
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
				// 如果剪贴板有文件，不当图片处理
				if hasFilesOnClipboard() {
					continue
				}
				b := safeReadClipboardImage()
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

// safeReadClipboardImage 安全地从 Windows 剪贴板读取图片数据并转换为 PNG。
// 与 golang.design/x/clipboard 不同，这里先将全局内存拷贝到 Go 管理的 []byte
// 中再进行解析，避免直接在全局内存上构建 slice 导致的 access violation。
func safeReadClipboardImage() []byte {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// 检查是否有图片格式可用（优先 CF_DIBV5，退化到 CF_DIB）
	format := uintptr(cfDIBV5)
	ret, _, _ := isClipboardFmtAvail.Call(format)
	if ret == 0 {
		format = uintptr(cfDIB)
		ret, _, _ = isClipboardFmtAvail.Call(format)
		if ret == 0 {
			return nil
		}
	}

	// 打开剪贴板（最多重试 5 次，每次间隔 10ms）
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

	hMem, _, _ := getClipboardData.Call(format)
	if hMem == 0 {
		return nil
	}

	// 获取全局内存块大小
	size := globalMemSize(hMem)
	if size == 0 || size < 40 { // 至少需要 BITMAPINFOHEADER (40 bytes)
		return nil
	}

	ptr, _, _ := globalLock.Call(hMem)
	if ptr == 0 {
		return nil
	}

	// 关键安全措施：将全局内存完整拷贝到 Go 管理的 []byte 中
	// 这样即使剪贴板在后续被其他程序修改，我们操作的是自己的副本
	data := make([]byte, size)
	copyFromUintptr(data, ptr, size)

	globalUnlock.Call(hMem)

	// 根据格式解析
	if format == uintptr(cfDIBV5) {
		return parseDIBV5ToPNG(data)
	}
	return parseDIBToPNG(data)
}

// bitmapV5Info 从 CF_DIBV5 数据中提取的关键字段。
type bitmapV5Info struct {
	Size     uint32
	Width    int32
	Height   int32
	BitCount uint16
}

// parseDIBV5ToPNG 安全解析 CF_DIBV5 格式数据并转为 PNG。
func parseDIBV5ToPNG(data []byte) []byte {
	if len(data) < 124 { // BITMAPV5HEADER 最小为 124 字节
		return nil
	}

	// 安全读取头信息
	headerSize := binary.LittleEndian.Uint32(data[0:4])
	width := int32(binary.LittleEndian.Uint32(data[4:8]))
	height := int32(binary.LittleEndian.Uint32(data[8:12]))
	bitCount := binary.LittleEndian.Uint16(data[14:16])

	// 校验合法性
	if width <= 0 || width > 32768 {
		return nil
	}
	absHeight := height
	topDown := false
	if height < 0 {
		absHeight = -height
		topDown = true
	}
	if absHeight <= 0 || absHeight > 32768 {
		return nil
	}
	if bitCount != 32 && bitCount != 24 {
		return nil
	}

	bytesPerPixel := int(bitCount) / 8
	stride := ((int(width)*int(bitCount) + 31) / 32) * 4 // 行对齐到 4 字节
	imageDataSize := stride * int(absHeight)
	expectedSize := int(headerSize) + imageDataSize

	if len(data) < expectedSize {
		return nil
	}

	offset := int(headerSize)
	img := image.NewRGBA(image.Rect(0, 0, int(width), int(absHeight)))

	for y := 0; y < int(absHeight); y++ {
		var srcY int
		if topDown {
			srcY = y
		} else {
			srcY = int(absHeight) - 1 - y
		}
		rowOffset := offset + srcY*stride
		for x := 0; x < int(width); x++ {
			idx := rowOffset + x*bytesPerPixel
			if idx+bytesPerPixel > len(data) {
				return nil // 安全边界检查
			}
			b := data[idx+0]
			g := data[idx+1]
			r := data[idx+2]
			var a uint8 = 255
			if bitCount == 32 {
				a = data[idx+3]
				// 如果所有像素的 alpha 都是 0，说明 alpha 通道未使用
				// 这种情况先不处理，后面统一修正
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}

	// 修正 alpha 通道：如果所有像素 alpha 都为 0，设为不透明
	if bitCount == 32 {
		fixAlphaIfNeeded(img)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// parseDIBToPNG 安全解析 CF_DIB (BITMAPINFOHEADER) 格式数据并转为 PNG。
func parseDIBToPNG(data []byte) []byte {
	if len(data) < 40 { // BITMAPINFOHEADER 为 40 字节
		return nil
	}

	headerSize := binary.LittleEndian.Uint32(data[0:4])
	width := int32(binary.LittleEndian.Uint32(data[4:8]))
	height := int32(binary.LittleEndian.Uint32(data[8:12]))
	bitCount := binary.LittleEndian.Uint16(data[14:16])

	if width <= 0 || width > 32768 {
		return nil
	}
	absHeight := height
	topDown := false
	if height < 0 {
		absHeight = -height
		topDown = true
	}
	if absHeight <= 0 || absHeight > 32768 {
		return nil
	}
	if bitCount != 32 && bitCount != 24 {
		return nil
	}

	bytesPerPixel := int(bitCount) / 8
	stride := ((int(width)*int(bitCount) + 31) / 32) * 4
	imageDataSize := stride * int(absHeight)
	expectedSize := int(headerSize) + imageDataSize

	if len(data) < expectedSize {
		return nil
	}

	offset := int(headerSize)
	img := image.NewRGBA(image.Rect(0, 0, int(width), int(absHeight)))

	for y := 0; y < int(absHeight); y++ {
		var srcY int
		if topDown {
			srcY = y
		} else {
			srcY = int(absHeight) - 1 - y
		}
		rowOffset := offset + srcY*stride
		for x := 0; x < int(width); x++ {
			idx := rowOffset + x*bytesPerPixel
			if idx+bytesPerPixel > len(data) {
				return nil
			}
			b := data[idx+0]
			g := data[idx+1]
			r := data[idx+2]
			var a uint8 = 255
			if bitCount == 32 {
				a = data[idx+3]
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}

	if bitCount == 32 {
		fixAlphaIfNeeded(img)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// fixAlphaIfNeeded 修正全零 alpha 通道的情况。
// 很多 Windows 程序（如截图工具）写入 CF_DIBV5 时 alpha 通道全为 0，
// 实际含义是"不透明"。如果检测到全零则统一设为 255。
func fixAlphaIfNeeded(img *image.RGBA) {
	pix := img.Pix
	hasNonZeroAlpha := false
	for i := 3; i < len(pix); i += 4 {
		if pix[i] != 0 {
			hasNonZeroAlpha = true
			break
		}
	}
	if !hasNonZeroAlpha {
		for i := 3; i < len(pix); i += 4 {
			pix[i] = 255
		}
	}
}
