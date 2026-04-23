//go:build darwin

package clipboard

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// hasFileURLs 仅检查当前剪切板是否存在文件 URL，不分配任何返回内存。
// 多次调用相当频繁（500ms 轮询 + text 事件探测），必须极度轻量。
int hasFileURLs() {
    @autoreleasepool {
        NSPasteboard *pb = [NSPasteboard generalPasteboard];
        NSArray *classes = @[[NSURL class]];
        NSDictionary *options = @{NSPasteboardURLReadingFileURLsOnlyKey: @YES};
        return [pb canReadObjectForClasses:classes options:options] ? 1 : 0;
    }
}

// getFileURLs 返回剪切板中的文件路径（换行分隔），无文件返回 NULL。
// 调用方负责调用 free() 释放返回的非 NULL 指针。
//
// 关键：所有 Cocoa 调用都必须包裹在 @autoreleasepool 里。
// 本函数运行在 Go 的 goroutine 上，没有系统自带的 main loop autorelease pool；
// 缺少该池会导致 NSPasteboard 等 API 内部创建的 autorelease 对象不断累积，
// 最终触发进程崩溃（表现为"多次复制后闪退"）。
const char* getFileURLs() {
    const char *result = NULL;
    @autoreleasepool {
        NSPasteboard *pb = [NSPasteboard generalPasteboard];
        NSArray *classes = @[[NSURL class]];
        NSDictionary *options = @{NSPasteboardURLReadingFileURLsOnlyKey: @YES};

        if (![pb canReadObjectForClasses:classes options:options]) {
            return NULL;
        }

        NSArray *urls = [pb readObjectsForClasses:classes options:options];
        if (!urls || urls.count == 0) {
            return NULL;
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
            return NULL;
        }
        NSString *joined = [paths componentsJoinedByString:@"\n"];
        const char *utf = [joined UTF8String];
        if (utf) {
            // strdup 分配的内存由 Go 侧 C.free 释放，autorelease pool 销毁时
            // joined/paths 会被释放，但我们已经复制出字节，不会悬垂。
            result = strdup(utf);
        }
    }
    return result;
}
*/
import "C"
import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

// hasFilesOnClipboard 返回当前剪切板是否含有文件 URL。
// Mac 在 Finder 里复制文件/文件夹时，NSPasteboard 会同时写入文件 URL 与文件名文本，
// text watcher 读到的是文件名 —— 这会导致「文件 + 文本」双重入库。
// 上层 Watcher 会用本函数过滤掉「明明剪切板里是文件，却以文本形式上报」的情况。
func hasFilesOnClipboard() bool {
	return C.hasFileURLs() != 0
}

// pollFiles 读取 macOS 剪切板中的文件 URL 列表。
func pollFiles() []FileInfo {
	cstr := C.getFileURLs()
	if cstr == nil {
		return nil
	}
	gostr := C.GoString(cstr)
	C.free(unsafe.Pointer(cstr))

	if gostr == "" {
		return nil
	}

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
