//go:build darwin

package clipboard

/*
#cgo CFLAGS: -x objective-c
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

// getFileURLs 返回剪切板中的文件路径（换行分隔），无文件返回 NULL。
// 调用方负责调用 free() 释放返回的非 NULL 指针。
// 访问 NSPasteboard 在专用串行队列上执行，避免跨线程并发触发 AppKit 异常。
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
                    [paths addObject:url.path];
                }
            }
            if (paths.count == 0) {
                return;
            }
            NSString *joined = [paths componentsJoinedByString:@"\n"];
            const char *utf8 = [joined UTF8String];
            if (utf8 != NULL) {
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
*/
import "C"
import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"unsafe"
)

// lastChangeCount 上次观察到的 pasteboard changeCount，用于跳过无变化轮询。
var lastChangeCount int64 = -1

// hasFileCache 缓存「某个 changeCount 下 pasteboard 是否含 fileURL」。
// 高 32 位存 changeCount（限幅），低位存结果（2 表示 true, 1 表示 false, 0 表示未知）。
// 这样 pasteboardHasFile 在高频调用（每次文本到达）时不必反复进 CGo/dispatch_sync。
var hasFileCache atomic.Int64

// encodeCache / decodeCache 简易编码：changeCount * 4 + (0 未知 | 1 false | 2 true)。
func encodeHasFile(cc int64, has bool) int64 {
	v := cc << 2
	if has {
		v |= 2
	} else {
		v |= 1
	}
	return v
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
