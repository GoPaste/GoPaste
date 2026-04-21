// dock_darwin.m
// 拦截 Dock 图标点击（applicationShouldHandleReopen），将事件转发给 Go 层。
//
// 通过 objc_runtime 向 NSApp 当前 delegate 的 class 动态添加
// applicationShouldHandleReopen:hasVisibleWindows: 方法，
// 而不是替换 delegate 对象本身（避免影响 Wails 自身的 lifecycle 回调）。

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

extern void goDockClicked(void);

// 向已有 class 动态添加 applicationShouldHandleReopen:hasVisibleWindows: 方法
// （Wails 的 AppDelegate 没有实现该方法，因此可以安全添加）。
static BOOL dockHandlerInstalled = NO;

void InstallDockDelegate(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (dockHandlerInstalled) return;

        id appDelegate = [NSApp delegate];
        if (appDelegate == nil) {
            NSLog(@"gopaste: NSApp delegate is nil, cannot install dock handler");
            return;
        }
        Class cls = object_getClass(appDelegate);

        IMP reopenIMP = imp_implementationWithBlock(^BOOL(id _self, NSApplication *app, BOOL flag) {
            goDockClicked();
            return NO; // 返回 NO 阻止系统默认行为
        });

        // 用 class_addMethod 添加；若已存在则用 class_replaceMethod
        SEL sel = @selector(applicationShouldHandleReopen:hasVisibleWindows:);
        if (!class_addMethod(cls, sel, reopenIMP, "B@:@B")) {
            class_replaceMethod(cls, sel, reopenIMP, "B@:@B");
        }

        dockHandlerInstalled = YES;
    });
}
