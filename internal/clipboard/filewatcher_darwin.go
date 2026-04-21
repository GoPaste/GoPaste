//go:build darwin

package clipboard

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// getFileURLs 返回剪切板中的文件路径（换行分隔），无文件返回 NULL。
// 调用方负责调用 free() 释放返回的非 NULL 指针。
const char* getFileURLs() {
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
            [paths addObject:url.path];
        }
    }
    if (paths.count == 0) {
        return NULL;
    }
    NSString *joined = [paths componentsJoinedByString:@"\n"];
    return strdup([joined UTF8String]);
}
*/
import "C"
import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

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
