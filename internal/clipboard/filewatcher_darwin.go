//go:build darwin

package clipboard

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <dispatch/dispatch.h>

// -----------------------------------------------------------------------------
// 线程安全说明
// AppKit/NSPasteboard 的访问必须限定在特定线程——最稳妥是主线程，或者至少
// 确保访问被串行化。Go 的 goroutine 会被调度到任意 OS 线程，若多个 goroutine
// 并发触发 CGo 调用 AppKit API，会导致：
//   1. 非主线程调用 AppKit 内部断言失败 → NSException
//   2. NSException 被 AppKit 的主 runloop 捕获 → reportException: 进入
//      backtrace_symbols 死循环 → 系统判定 CPU 失控（cpu_resource.diag）
// 因此所有 NSPasteboard 访问都通过 dispatch_sync 派发到一个专用串行队列执行。
// 用专用串行队列而不是主队列，是因为：
//   - 主队列在 wails/AppKit 主 runloop 里跑，会竞争 UI 事件
//   - 串行队列可以保证所有访问顺序化、线程稳定，避免跨线程并发
//   - 只要队列上**永远是同一个线程**（dispatch 串行队列的保证），AppKit
//     的对象所有权/引用计数就不会错乱
// -----------------------------------------------------------------------------

// pasteboardQueue 专用串行队列，所有 pasteboard 访问都通过它串行化。
static dispatch_queue_t pasteboardQueue(void) {
    static dispatch_once_t once;
    static dispatch_queue_t q;
    dispatch_once(&once, ^{
        q = dispatch_queue_create("gopaste.pasteboard", DISPATCH_QUEUE_SERIAL);
    });
    return q;
}

// hasFileURLs 仅检查当前剪切板是否存在文件 URL，不分配任何返回内存。
// 多次调用相当频繁（500ms 轮询 + text 事件探测），必须极度轻量。
//
// 【崩溃根因修复 / 2026-04-23】
// 之前这里是裸 @autoreleasepool + 直接访问 [NSPasteboard generalPasteboard]，
// 没有走 pasteboardQueue 串行化。其他三个函数（getFileURLs / pasteboardChangeCount
// / pasteboardHasFileURL）都在 queue 里，唯独这个漏了。
// 触发链：text watcher 在新文本到达时高频调用本函数（从任意 Go goroutine 线程），
// 同时 500ms FileWatcher tick 在 pasteboardQueue 里跑 readObjectsForClasses —
// NSPasteboard 不是线程安全的，其内部缓存的 NSArray 被并发 retain/release 后
// refcount 归零、另一线程拿到野指针 → EXC_BAD_ACCESS 静默闪退。
// crash report 里 threadTriggered.queue = "gopaste.pasteboard" 完全印证。
// 现在把本函数也收进同一个串行 queue，保证所有 NSPasteboard 访问单线程化。
int hasFileURLs() {
    __block int result = 0;
    dispatch_sync(pasteboardQueue(), ^{
        @autoreleasepool {
            NSPasteboard *pb = [NSPasteboard generalPasteboard];
            NSArray *classes = @[[NSURL class]];
            NSDictionary *options = @{NSPasteboardURLReadingFileURLsOnlyKey: @YES};
            result = [pb canReadObjectForClasses:classes options:options] ? 1 : 0;
        }
    });
    return result;
}

// getFileURLs 返回剪切板中的文件路径（换行分隔），无文件返回 NULL。
// 调用方负责调用 free() 释放返回的非 NULL 指针。
// 访问 NSPasteboard 在专用串行队列上执行，避免跨线程并发触发 AppKit 异常。
// block 内部的 @autoreleasepool 负责释放 Cocoa 自动释放对象（Go goroutine
// 默认没有系统 main loop 提供的 autorelease pool，若缺失会出现长时间运行后
// 内存/引用计数错乱导致的崩溃）。
const char* getFileURLs(void) {
    __block char *result = NULL;
    dispatch_sync(pasteboardQueue(), ^{
        @autoreleasepool {
            NSPasteboard *pb = [NSPasteboard generalPasteboard];
            NSArray *classes = @[[NSURL class]];
            NSDictionary *options = @{NSPasteboardURLReadingFileURLsOnlyKey: @YES};

            if (![pb canReadObjectForClasses:classes options:options]) {
                return;
            }
            NSArray *urls = [pb readObjectsForClasses:classes options:options];
            if (!urls || urls.count == 0) {
                return;
            }
            NSMutableArray *paths = [NSMutableArray arrayWithCapacity:urls.count];
            for (NSURL *url in urls) {
                if (url.isFileURL) {
                    NSString *p = url.path;
                    if (p.length > 0) {
                        [paths addObject:p];
                    }
                }
            }
            if (paths.count == 0) {
                return;
            }
            NSString *joined = [paths componentsJoinedByString:@"\n"];
            const char *utf8 = [joined UTF8String];
            if (utf8 != NULL) {
                // strdup 出的字节由 Go 侧 C.free 释放；autorelease pool 销毁时
                // joined/paths 会被释放，但字节已复制，不会悬垂。
                result = strdup(utf8);
            }
        }
    });
    return result;
}

// pasteboardChangeCount 返回 NSPasteboard 的 changeCount。
// Go 侧可据此跳过未变化的轮询，降低内存/CPU 开销。
long pasteboardChangeCount(void) {
    __block long result = 0;
    dispatch_sync(pasteboardQueue(), ^{
        @autoreleasepool {
            result = (long)[[NSPasteboard generalPasteboard] changeCount];
        }
    });
    return result;
}

// readClipboardImagePNG 读取当前 NSPasteboard 中的图片并以 PNG 字节序列返回。
// 调用方负责 free() 返回的指针（非 NULL 时）。读不到图片返回 NULL 且 *outLen=0。
//
// 【为什么要自己写这个】
// golang.design/x/clipboard 的 read_image 只查 NSPasteboardTypePNG（public.png）。
// 但 macOS 系统截图（Cmd+Shift+4/3/5、Preview「复制」、多数 app 的复制图片）
// 实际写入的是 NSPasteboardTypeTIFF（public.tiff），少数才是 PNG。
// 于是截图后 golang.design 的 image watcher 收不到事件——表现就是"截图没记录"。
//
// 解决策略：先尝试 public.png 直取（最快路径、零拷贝转码）；
// 没有则尝试 public.tiff，用 NSBitmapImageRep 直接 in-memory 转 PNG。
// 两者都没有就返回 NULL。
//
// 统一走 pasteboardQueue 串行化，避免和 FileWatcher/文本过滤的 NSPasteboard
// 访问并发（见文件顶部注释的崩溃根因）。
const void* readClipboardImagePNG(unsigned long *outLen) {
    __block void *bytes = NULL;
    __block unsigned long len = 0;
    dispatch_sync(pasteboardQueue(), ^{
        @autoreleasepool {
            NSPasteboard *pb = [NSPasteboard generalPasteboard];

            // 1) 直接拿 PNG
            NSData *png = [pb dataForType:NSPasteboardTypePNG];
            if (png != nil && png.length > 0) {
                len = (unsigned long)png.length;
                bytes = malloc(len);
                if (bytes != NULL) {
                    memcpy(bytes, png.bytes, len);
                }
                return;
            }

            // 2) 退化到 TIFF（截图/Preview 默认格式）并转 PNG
            NSData *tiff = [pb dataForType:NSPasteboardTypeTIFF];
            if (tiff == nil || tiff.length == 0) {
                return;
            }
            NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:tiff];
            if (rep == nil) {
                return;
            }
            NSData *conv = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
            if (conv == nil || conv.length == 0) {
                return;
            }
            len = (unsigned long)conv.length;
            bytes = malloc(len);
            if (bytes != NULL) {
                memcpy(bytes, conv.bytes, len);
            }
        }
    });
    if (outLen != NULL) {
        *outLen = len;
    }
    return bytes;
}

// pasteboardHasFileURL 判断当前 pasteboard 是否含有 fileURL。
// 用于文本 Watcher 在处理新到达的文本时过滤 Finder 附带的 basename。
// 返回 1 表示有，0 表示无。
int pasteboardHasFileURL(void) {
    __block int result = 0;
    dispatch_sync(pasteboardQueue(), ^{
        @autoreleasepool {
            NSPasteboard *pb = [NSPasteboard generalPasteboard];
            NSArray *classes = @[[NSURL class]];
            NSDictionary *options = @{NSPasteboardURLReadingFileURLsOnlyKey: @YES};
            result = [pb canReadObjectForClasses:classes options:options] ? 1 : 0;
        }
    });
    return result;
}

// -----------------------------------------------------------------------------
// 通用 NSPasteboard 读写（替代 golang.design/x/clipboard 在 darwin 上的实现）
// -----------------------------------------------------------------------------
// 【为什么必须自己写】
// 见 docs/macos-accessibility.md / 崩溃排查记录（2026-04-24）：
//   golang.design/x/clipboard 的 darwin 实现 (clipboard_read_string /
//   clipboard_write_string / clipboard_read_image / clipboard_write_image)
//   在任意 goroutine 直接裸调 [NSPasteboard generalPasteboard] dataForType:/
//   setData:，不走任何串行化。我们自己 image/file watcher 又在 pasteboardQueue
//   里访问同一个 NSPasteboard 单例。
//   并发触发：watcher.dataForType 内部走 _updateTypeCacheIfNeeded 枚举
//   _typeArray，与另一线程 setData: 的 mutate 撞车 →
//     NSGenericException "Collection was mutated while being enumerated"
//     → terminate handler → abort() → SIGABRT，进程整个消失。
//   崩溃 backtrace 完美定位到 clipboard_read_string，铁证。
//
// 修复：所有 NSPasteboard 访问全部走 pasteboardQueue 串行化（同一线程顺序执行）。
// 调用方在 darwin 上不再使用 golang.design/x/clipboard 的 read/write/watch。

// pasteboardReadString 串行化读取 NSPasteboardTypeString。
// 返回 strdup 后的字节，由 Go 侧 free。NULL 表示空 / 不可读。
const char* pasteboardReadString(void) {
    __block char *result = NULL;
    dispatch_sync(pasteboardQueue(), ^{
        @autoreleasepool {
            NSPasteboard *pb = [NSPasteboard generalPasteboard];
            NSData *data = [pb dataForType:NSPasteboardTypeString];
            if (data == nil || data.length == 0) {
                return;
            }
            // 复制成 C 字符串。NSData 内部缓冲在 autorelease pool 销毁后失效，
            // 这里 strndup 拷贝一份给 Go 侧持有。
            result = (char *)malloc(data.length + 1);
            if (result != NULL) {
                memcpy(result, data.bytes, data.length);
                result[data.length] = '\0';
            }
        }
    });
    return result;
}

// pasteboardWriteString 串行化写入字符串到 NSPasteboardTypeString。
// 写之前 clearContents（与 golang.design 行为一致）。返回 0 成功 / -1 失败。
int pasteboardWriteString(const void *bytes, unsigned long n) {
    __block int rc = -1;
    dispatch_sync(pasteboardQueue(), ^{
        @autoreleasepool {
            NSPasteboard *pb = [NSPasteboard generalPasteboard];
            NSData *data = (n > 0 && bytes != NULL)
                ? [NSData dataWithBytes:bytes length:(NSUInteger)n]
                : [NSData data];
            [pb clearContents];
            BOOL ok = [pb setData:data forType:NSPasteboardTypeString];
            rc = ok ? 0 : -1;
        }
    });
    return rc;
}

// pasteboardWriteImagePNG 串行化写入 PNG 图片到 NSPasteboardTypePNG。
// 写之前 clearContents。返回 0 成功 / -1 失败。
int pasteboardWriteImagePNG(const void *bytes, unsigned long n) {
    __block int rc = -1;
    dispatch_sync(pasteboardQueue(), ^{
        @autoreleasepool {
            NSPasteboard *pb = [NSPasteboard generalPasteboard];
            NSData *data = (n > 0 && bytes != NULL)
                ? [NSData dataWithBytes:bytes length:(NSUInteger)n]
                : [NSData data];
            [pb clearContents];
            BOOL ok = [pb setData:data forType:NSPasteboardTypePNG];
            rc = ok ? 0 : -1;
        }
    });
    return rc;
}

// pasteboardWriteFileURLs 串行化把 POSIX 路径列表写为 NSPasteboard 文件 URL 列表。
// paths 为换行（\n）分隔的 UTF-8 路径字符串，n 为字节数。
// 使用 writeObjects: 写入 NSURL 数组，使文件管理器/支持拖放的应用能直接粘贴文件。
// 写之前 clearContents。返回 0 成功 / -1 失败。
int pasteboardWriteFileURLs(const char *paths, unsigned long n) {
    __block int rc = -1;
    dispatch_sync(pasteboardQueue(), ^{
        @autoreleasepool {
            if (paths == NULL || n == 0) {
                rc = -1;
                return;
            }
            NSString *joined = [[NSString alloc] initWithBytes:paths
                                                        length:(NSUInteger)n
                                                      encoding:NSUTF8StringEncoding];
            if (!joined) {
                rc = -1;
                return;
            }
            NSArray<NSString *> *lines = [joined componentsSeparatedByString:@"\n"];
            NSMutableArray<NSURL *> *urls = [NSMutableArray arrayWithCapacity:lines.count];
            for (NSString *line in lines) {
                NSString *trimmed = [line stringByTrimmingCharactersInSet:
                                     [NSCharacterSet whitespaceAndNewlineCharacterSet]];
                if (trimmed.length == 0) continue;
                NSURL *url = [NSURL fileURLWithPath:trimmed];
                if (url) [urls addObject:url];
            }
            if (urls.count == 0) {
                rc = -1;
                return;
            }
            NSPasteboard *pb = [NSPasteboard generalPasteboard];
            [pb clearContents];
            BOOL ok = [pb writeObjects:urls];
            rc = ok ? 0 : -1;
        }
    });
    return rc;
}
*/
import "C"
import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"unsafe"
)

// errClipboardWrite 写 NSPasteboard 失败。底层 setData: 返回 NO 时抛出。
var errClipboardWrite = errors.New("clipboard: write to NSPasteboard failed")

// lastChangeCount 上次观察到的 pasteboard changeCount，用于跳过无变化轮询。
var lastChangeCount int64 = -1

// hasFileCache 缓存「某个 changeCount 下 pasteboard 是否含 fileURL」。
// 高 32 位存 changeCount（限幅），低位存结果（2 表示 true, 1 表示 false, 0 表示未知）。
// 这样 pasteboardHasFile 在高频调用（每次文本到达）时不必反复进 CGo/dispatch_sync。
var hasFileCache atomic.Int64

// encodeHasFile 简易编码：changeCount * 4 + (0 未知 | 1 false | 2 true)。
func encodeHasFile(cc int64, has bool) int64 {
	v := cc << 2
	if has {
		v |= 2
	} else {
		v |= 1
	}
	return v
}

// hasFilesOnClipboard 返回当前剪切板是否含有文件 URL。
// Mac 在 Finder 里复制文件/文件夹时，NSPasteboard 会同时写入文件 URL 与文件名文本，
// text watcher 读到的是文件名 —— 这会导致「文件 + 文本」双重入库。
// 上层 Watcher 会用本函数过滤掉「明明剪切板里是文件，却以文本形式上报」的情况。
// 走轻量 hasFileURLs() 路径（不过 dispatch_sync），够用；
// 对于文本路径的快速过滤，请使用 pasteboardHasFile（带缓存）。
func hasFilesOnClipboard() bool {
	return C.hasFileURLs() != 0
}

// pollFiles 读取 macOS 剪切板中的文件 URL 列表。
// 若 pasteboard 未变化（changeCount 相同）则直接返回 nil，避免反复 alloc NS 对象。
func pollFiles() []FileInfo {
	cc := int64(C.pasteboardChangeCount())
	prev := atomic.LoadInt64(&lastChangeCount)
	if cc == prev {
		return nil
	}
	atomic.StoreInt64(&lastChangeCount, cc)

	cstr := C.getFileURLs()
	if cstr == nil {
		// 更新缓存：该 changeCount 下无文件
		hasFileCache.Store(encodeHasFile(cc, false))
		return nil
	}
	gostr := C.GoString(cstr)
	C.free(unsafe.Pointer(cstr))

	if gostr == "" {
		hasFileCache.Store(encodeHasFile(cc, false))
		return nil
	}
	// 更新缓存：该 changeCount 下有文件
	hasFileCache.Store(encodeHasFile(cc, true))

	paths := strings.Split(gostr, "\n")
	files := make([]FileInfo, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
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

// pasteboardHasFile 判断当前系统剪切板是否含有 fileURL（带缓存）。
// 文本 Watcher 在处理到达的文本时据此过滤：若 pasteboard 同时含文件，
// 则文本槽位只是 Finder 附带的 basename/path，应丢弃。
// 若缓存的 changeCount 与当前相同则直接返回，否则查一次并更新缓存。
func pasteboardHasFile() bool {
	cc := int64(C.pasteboardChangeCount())
	cached := hasFileCache.Load()
	// 高位是 changeCount（cached >> 2），低位是结果
	if cached != 0 && (cached>>2) == cc {
		return cached&3 == 2
	}
	has := C.pasteboardHasFileURL() != 0
	hasFileCache.Store(encodeHasFile(cc, has))
	return has
}

// pollClipboardImagePNG 读取当前剪切板中的图片并返回 PNG 字节。
// 若当前剪切板没有图片（或非图片 / 非可识别格式）返回 nil。
// 优先读 PNG，退化到 TIFF → PNG 转换，覆盖 macOS 系统截图场景。
//
// 上层 Watcher 会结合 changeCount 去重，只在"changeCount 变化 且
// 不是文件事件"时才调用本函数，避免不必要的 TIFF→PNG 转码开销。
func pollClipboardImagePNG() []byte {
	var n C.ulong
	p := C.readClipboardImagePNG(&n)
	if p == nil || n == 0 {
		return nil
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoBytes(p, C.int(n))
}

// pasteboardChangeCountGo 暴露 changeCount 给 Go 侧 watcher loop 使用。
// 纯 int64 返回，避免多处 import "C"。
func pasteboardChangeCountGo() int64 {
	return int64(C.pasteboardChangeCount())
}

// readClipboardStringGo 串行化读取 NSPasteboard 中的字符串内容。
// 取代 golang.design/x/clipboard.Read(FmtText)，避免裸调 NSPasteboard
// 与 file/image watcher 并发触发 NSGenericException（见 filewatcher 顶部注释）。
func readClipboardStringGo() []byte {
	cstr := C.pasteboardReadString()
	if cstr == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(cstr))
	s := C.GoString(cstr)
	if s == "" {
		return nil
	}
	return []byte(s)
}

// writeClipboardStringGo 串行化写入字符串到 NSPasteboard。
// 取代 golang.design/x/clipboard.Write(FmtText)。返回 nil 表示成功。
func writeClipboardStringGo(b []byte) error {
	var p unsafe.Pointer
	if len(b) > 0 {
		p = unsafe.Pointer(&b[0])
	}
	if rc := C.pasteboardWriteString(p, C.ulong(len(b))); rc != 0 {
		return errClipboardWrite
	}
	return nil
}

// writeClipboardImagePNGGo 串行化写入 PNG 字节到 NSPasteboard。
// 取代 golang.design/x/clipboard.Write(FmtImage)。返回 nil 表示成功。
func writeClipboardImagePNGGo(b []byte) error {
	var p unsafe.Pointer
	if len(b) > 0 {
		p = unsafe.Pointer(&b[0])
	}
	if rc := C.pasteboardWriteImagePNG(p, C.ulong(len(b))); rc != 0 {
		return errClipboardWrite
	}
	return nil
}

// writeClipboardFileURLsGo 串行化把 POSIX 路径列表写为 NSPasteboard 文件 URL 列表。
// 走 pasteboardQueue 串行化，与其他 watcher 访问不冲突。
func writeClipboardFileURLsGo(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	joined := strings.Join(paths, "\n")
	b := []byte(joined)
	var p unsafe.Pointer
	if len(b) > 0 {
		p = unsafe.Pointer(&b[0])
	}
	if rc := C.pasteboardWriteFileURLs((*C.char)(p), C.ulong(len(b))); rc != 0 {
		return errClipboardWrite
	}
	return nil
}
