// dispatch_darwin.m
// 将 Go 回调调度到 macOS 主线程。

#import <Foundation/Foundation.h>

extern void goRunPendingMainFn(void);

void DispatchOnMain(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        goRunPendingMainFn();
    });
}
