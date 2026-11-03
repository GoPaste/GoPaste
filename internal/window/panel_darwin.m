// panel_darwin.m —— macOS 主面板（NSPanel + NonactivatingPanel）的辅助能力。
//
// 设计前提：GoPaste 的面板显示时**不能抢走前台应用的 active 状态**，否则粘贴的
// Cmd+V 会打回 GoPaste 自己。Wails v3 通过 WebviewWindowOptions.Mac 原生创建了
// 带 NSWindowStyleMaskNonactivatingPanel 的 WebviewPanel，并且它的 Show/Focus 也遵循
// 这个约定（见 wails 的 windowShow / windowFocus：非激活面板只 orderFrontRegardless +
// makeKeyWindow，绝不 activateIgnoringOtherApps:）。
//
// 因此本文件只做两件 wails 没覆盖的事，除此之外不再碰窗口：
//
//   1. 一次性外观配置（ConfigurePanel）：隐藏红绿灯、禁止 AppKit 的“按住背景拖窗口”。
//      - 隐藏红绿灯：wails 提供了 SetCloseButtonState 等，这里用 AppKit 兜底，因为
//        TitleBarHiddenInset 的 toolbar 重新布局后按钮偶尔会被还原。
//      - movableByWindowBackground=NO：wails 的 WebviewPanel 初始化时强制设为 YES，
//        会让 AppKit 在鼠标按下时进入窗口拖动跟踪。GoPaste 的窗口拖动走前端
//        drag-region（wails:drag → performWindowDragWithEvent），不需要 AppKit 代劳。
//
//   2. 系统对话框需要 active app（ActivateForDialog / GoPasteSaveFileDialog）：
//      NonactivatingPanel 下 NSAlert / NSSavePanel 需要 active app 才能显示并接收输入。
//
//   3. 点击穿透（install_first_mouse）：让 WKWebView 的 acceptsFirstMouse: 返回 YES，
//      修掉“窗口未获焦点时第一次点击被吃掉”。acceptsFirstMouse: 是 NSView 的公开 API，
//      不是私有接口；同类做法见 Electron 的 acceptFirstMouse 选项。
//
// 刻意不做的：
//   - 不 isa swap（会破坏 wails WebviewPanel 的 sendEvent:/keyDown: 重写）。
//   - 不在每次显示时重设 collectionBehavior / level：Space 行为由 wails 的
//     Mac.CollectionBehavior 选项统一负责，重复设置只会引入两套互相打架的逻辑。
//   - 不再保存窗口指针：show/hide 由 wails 的 Show()/Hide() 负责，本文件不参与。

#import <Cocoa/Cocoa.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>
#include <string.h>  // for strdup

// ------------------------------------------------------------------
// 按 title 在 [NSApp windows] 中查找主窗口。
// GoPaste 只有一个主窗口；title 来自 WebviewWindowOptions.Title。
// ------------------------------------------------------------------
static NSWindow *find_window(NSString *title) {
    for (NSWindow *w in [NSApp windows]) {
        if ([w isKindOfClass:[NSPanel class]] && [[w title] isEqualToString:title]) {
            return w;
        }
    }
    return nil;
}

// ------------------------------------------------------------------
// 同步 / 异步调度到主线程（若已在主线程则直接执行），确保 AppKit 调用安全。
// ------------------------------------------------------------------
static void run_on_main_sync(void (^block)(void)) {
    if ([NSThread isMainThread]) {
        block();
    } else {
        dispatch_sync(dispatch_get_main_queue(), block);
    }
}

static void run_on_main_async(void (^block)(void)) {
    if ([NSThread isMainThread]) {
        block();
    } else {
        dispatch_async(dispatch_get_main_queue(), block);
    }
}

// ------------------------------------------------------------------
// 对外 C 接口
// ------------------------------------------------------------------

static void install_occlusion_watch(void);
static void install_first_mouse(void);

// GoPasteConfigurePanel 做一次性的面板外观配置，返回：
//    1  找到窗口，且确实是 NonactivatingPanel
//    0  找不到窗口（原生窗口尚未创建，调用方可稍后重试）
//   -1  找到窗口但不是 NonactivatingPanel —— wails 的 Panel 选项没生效，
//       面板会抢前台、粘贴会打回自己。只告警，不尝试修复（isa swap 会破坏 wails）。
//
// 幂等，可在启动阶段重试调用。
int GoPasteConfigurePanel(const char *ctitle) {
    if (!ctitle) return 0;
    install_occlusion_watch();
    install_first_mouse();
    NSString *title = [NSString stringWithUTF8String:ctitle];
    __block int status = 0;
    run_on_main_sync(^{
        NSWindow *win = find_window(title);
        if (!win) {
            status = 0;
            return;
        }

        // 隐藏左上角 traffic lights（关闭/最小化/缩放）：剪贴板面板是轻量浮层，
        // 关闭由 ESC / 失焦触发，最小化/缩放语义不适用。
        NSButton *closeBtn = [win standardWindowButton:NSWindowCloseButton];
        NSButton *miniBtn  = [win standardWindowButton:NSWindowMiniaturizeButton];
        NSButton *zoomBtn  = [win standardWindowButton:NSWindowZoomButton];
        if (closeBtn) closeBtn.hidden = YES;
        if (miniBtn)  miniBtn.hidden  = YES;
        if (zoomBtn)  zoomBtn.hidden  = YES;

        // 窗口拖动由前端 drag-region 负责，不让 AppKit 在按下时抢走鼠标序列。
        [win setMovableByWindowBackground:NO];

        BOOL ok = [win isKindOfClass:[NSPanel class]] &&
            ([win styleMask] & NSWindowStyleMaskNonactivatingPanel) != 0;
        if (ok) {
            status = 1;
        } else {
            NSLog(@"gopaste: WARN window '%@' (class=%@) is NOT a NonactivatingPanel; "
                  @"check WebviewWindowOptions.Mac.WindowClass/PanelPreferences",
                  title, NSStringFromClass([win class]));
            status = -1;
        }
    });
    return status;
}

// ------------------------------------------------------------------
// 遮挡状态自愈：偶发“列表区域变空白”的兜底。
//
// 依据（实测）：诊断日志里出现过 isVisible=YES 却 occlusionState 缺少
// NSWindowOcclusionStateVisible 的矛盾状态。WebKit 依据遮挡状态决定是否绘制，
// 状态错乱时列表内容不被重绘。
//
// 处理：监听遮挡状态变化，发现“可见但被判为遮挡”且窗口是 key（用户正在使用）
// 时，延迟 0.5s 复查，仍未恢复则 orderFront 一次，强制 AppKit 重新评估遮挡。
// 非 key 不处理（用户在别的应用里，被盖住是正常的，自动前置会打扰）。
// ------------------------------------------------------------------
static void install_occlusion_watch(void) {
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        NSNotificationCenter *nc = [NSNotificationCenter defaultCenter];
        [nc addObserverForName:NSWindowDidChangeOcclusionStateNotification
                        object:nil
                         queue:NSOperationQueue.mainQueue
                    usingBlock:^(NSNotification *n) {
            NSWindow *win = find_window(@"GoPaste");
            if (win == nil || n.object != win) return;
            if (![win isVisible]) return;
            if ([win occlusionState] & NSWindowOcclusionStateVisible) return;
            if (![win isKeyWindow]) {
                NSLog(@"gopaste: [diag] occlusion missing Visible while panel visible (not key, skip self-heal)");
                return;
            }
            dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.5 * NSEC_PER_SEC)),
                           dispatch_get_main_queue(), ^{
                if (![win isVisible]) return;
                if ([win occlusionState] & NSWindowOcclusionStateVisible) return;
                NSLog(@"gopaste: [diag] panel visible but occluded while key for 0.5s, forcing orderFront to resume painting");
                [win orderFront:nil];
            });
        }];
    });
}

// ------------------------------------------------------------------
// 让网页内容接受“首次点击”（点击穿透）。
//
// 问题（已在 minrepro 最小复现里确认）：当窗口不是 key window 时，macOS 默认
// 只把第一次点击用来激活窗口，不派发给内容 —— 用户看到“第一次点击没反应，
// 要点第二次”。AppKit 会先问命中视图的 acceptsFirstMouse:，
// 而 WKWebView 对网页内容默认返回 NO。
//
// 修复：用 class_replaceMethod 把 WKWebView 的 acceptsFirstMouse: 换成返回 YES。
//   - 只作用于 WKWebView 这个类，不会改到父类 NSView；
//   - 不改变面板的非激活语义：点击仍然只是让面板成为 key window，
//     不会 activateIgnoringOtherApps:，粘贴的 Cmd+V 依旧落到原应用。
// 同类做法：Electron 的 acceptFirstMouse 选项。
// ------------------------------------------------------------------
static BOOL gopaste_acceptsFirstMouse(id self, SEL _cmd, NSEvent *event) {
    return YES;
}

static void install_first_mouse(void) {
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        Class cls = [WKWebView class];
        SEL sel = @selector(acceptsFirstMouse:);
        Method m = class_getInstanceMethod(cls, sel);
        class_replaceMethod(cls, sel, (IMP)gopaste_acceptsFirstMouse,
                            m ? method_getTypeEncoding(m) : "c@:@");
    });
}

// 弹出系统对话框前临时激活 GoPaste。
void GoPasteActivateForDialog(void) {
    run_on_main_sync(^{
        [[NSApplication sharedApplication] activateIgnoringOtherApps:YES];
    });
}

// 对话框关闭后将激活状态交还给上一个前台应用。
void GoPasteDeactivateAfterDialog(void) {
    run_on_main_async(^{
        [[NSApplication sharedApplication] deactivate];
    });
}

// GoPasteSaveFileDialog 在主线程内原子执行：激活 → NSSavePanel → 恢复。
// allowedExt 为不带点的扩展名（如 "json" / "zip" / "png"），为空则不限制类型。
// 返回用户选择的路径（C 字符串，调用方需 free），取消返回 NULL。
char *GoPasteSaveFileDialog(const char *title, const char *defaultName, const char *allowedExt) {
    __block char *result = NULL;
    run_on_main_sync(^{
        [[NSApplication sharedApplication] activateIgnoringOtherApps:YES];

        NSSavePanel *panel = [NSSavePanel savePanel];
        if (title) {
            panel.title = [NSString stringWithUTF8String:title];
        }
        if (defaultName) {
            panel.nameFieldStringValue = [NSString stringWithUTF8String:defaultName];
        }
        if (allowedExt && strlen(allowedExt) > 0) {
            NSString *ext = [NSString stringWithUTF8String:allowedExt];
            if (@available(macOS 11.0, *)) {
                UTType *type = [UTType typeWithFilenameExtension:ext];
                if (type) {
                    panel.allowedContentTypes = @[type];
                }
            } else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
                panel.allowedFileTypes = @[ext];
#pragma clang diagnostic pop
            }
        }
        panel.canCreateDirectories = YES;

        NSModalResponse resp = [panel runModal];

        // 恢复非激活状态
        [[NSApplication sharedApplication] deactivate];

        if (resp == NSModalResponseOK && panel.URL) {
            const char *path = [[panel.URL path] UTF8String];
            result = strdup(path);
        }
    });
    return result;
}
