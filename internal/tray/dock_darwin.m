// dock_darwin.m
// - 拦截 Dock 图标点击（applicationShouldHandleReopen），将事件转发给 Go 层。
// - 注入 applicationShouldTerminateAfterLastWindowClosed:NO，防止主窗口被
//   WindowHide 关闭后 NSApp 自动终止整个进程（我们是托盘常驻应用）。
//
// 通过 objc_runtime 向 NSApp 当前 delegate 的 class 动态添加方法，
// 而不是替换 delegate 对象本身（避免影响 Wails 自身的 lifecycle 回调）。

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

extern void goDockClicked(void);

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

        // 1) applicationShouldHandleReopen:hasVisibleWindows:
        //    Dock 图标被再次点击时转发给 Go。
        IMP reopenIMP = imp_implementationWithBlock(^BOOL(id _self, NSApplication *app, BOOL flag) {
            goDockClicked();
            return NO;
        });
        SEL reopenSel = @selector(applicationShouldHandleReopen:hasVisibleWindows:);
        if (!class_addMethod(cls, reopenSel, reopenIMP, "B@:@B")) {
            class_replaceMethod(cls, reopenSel, reopenIMP, "B@:@B");
        }

        // 2) applicationShouldTerminateAfterLastWindowClosed:
        //    GoPaste 是托盘常驻应用。WindowHide 会关闭唯一的主窗口，
        //    默认行为会导致 NSApp 终止；这里显式返回 NO 阻止之。
        IMP keepAliveIMP = imp_implementationWithBlock(^BOOL(id _self, NSApplication *app) {
            return NO;
        });
        SEL keepAliveSel = @selector(applicationShouldTerminateAfterLastWindowClosed:);
        if (!class_addMethod(cls, keepAliveSel, keepAliveIMP, "B@:@")) {
            class_replaceMethod(cls, keepAliveSel, keepAliveIMP, "B@:@");
        }

        dockHandlerInstalled = YES;
    });
}
