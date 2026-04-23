// panel_darwin.m —— 把 Wails 创建的普通 NSWindow 改造成
// NSPanel + NSWindowStyleMaskNonactivatingPanel。
//
// 背景：
//   参见 docs/ecopaste-analysis.md。剪贴板面板类 app 必须解决"显示时不
//   抢前台应用的 active 状态"。普通 NSWindow 做不到；macOS 给这类场景
//   专门设计了 NSPanel 的 NonactivatingPanel 样式位。
//
//   Wails v2 没有暴露让你指定窗口类型，它走 [NSWindow alloc] init… 的标
//   准路径。我们只能在窗口已经创建好以后，用 Obj-C runtime 原位替换它
//   的 isa 指针到 NSPanel 子类。这是 tauri-nspanel / 很多 Cocoa 老项目
//   都在用的合法手法。
//
//   NSPanel 默认在带 NonactivatingPanel 样式时 canBecomeKeyWindow 返
//   回 NO（这也是"不抢 active app"的关键机制之一）。但我们的面板里有
//   搜索框 / 方向键导航 / 回车，必须能接收键盘。所以定义一个子类
//   GoPasteNSPanel 显式把 canBecomeKeyWindow 覆盖成 YES：只要面板出现
//   就成为 key window（拿到键盘输入），但**因为有 NonactivatingPanel
//   样式位，它并不成为 active application** —— 用户原本在编辑的那个
//   app 仍然是 [NSApplication sharedApplication].active，Cmd+V 最终会
//   打到它身上。这正是我们想要的。

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

// ------------------------------------------------------------------
// GoPasteNSPanel —— 空壳子类，仅重写 canBecomeKeyWindow / CanBecomeMain
// ------------------------------------------------------------------
@interface GoPasteNSPanel : NSPanel
@end

@implementation GoPasteNSPanel
- (BOOL)canBecomeKeyWindow  { return YES; }
- (BOOL)canBecomeMainWindow { return NO;  }  // 不做 main，避免干扰 AppKit 的 main-window 逻辑
@end

// ------------------------------------------------------------------
// 根据 title 在 [NSApp windows] 中查找目标窗口。
// Wails 每个 App 只有一个主窗口；title 来自 options.App.Title。
// ------------------------------------------------------------------
static NSWindow *find_window(NSString *title) {
    for (NSWindow *w in [NSApp windows]) {
        if ([[w title] isEqualToString:title]) {
            return w;
        }
    }
    // Fallback：Wails 在 TitleBarHiddenInset 下可能短暂没 title，
    // 用 className 过滤掉系统辅助窗口后挑第一个可见的。
    for (NSWindow *w in [NSApp windows]) {
        NSString *cls = NSStringFromClass([w class]);
        if ([cls hasPrefix:@"NSStatusBar"]) continue;
        if ([cls hasPrefix:@"NSToolbar"])   continue;
        if ([cls hasPrefix:@"NSMenu"])      continue;
        return w;
    }
    return nil;
}

// ------------------------------------------------------------------
// 把窗口转成 NonactivatingPanel。多次调用等价于一次（用 associated
// object 打标记幂等）。必须在主线程执行。
// ------------------------------------------------------------------
static const void *kPanelConvertedKey = &kPanelConvertedKey;

static void convert_to_panel_on_main(NSString *title) {
    NSWindow *win = find_window(title);
    if (!win) {
        NSLog(@"gopaste: convert_to_panel: window with title '%@' not found", title);
        return;
    }

    NSNumber *done = objc_getAssociatedObject(win, kPanelConvertedKey);
    if ([done boolValue]) return;

    // 1) 改 class：NSWindow → GoPasteNSPanel（GoPasteNSPanel 继承自 NSPanel）。
    //    object_setClass 仅修改 isa，对象内存布局保持兼容。NSPanel 没有比
    //    NSWindow 多加 ivar，所以这是安全的。
    object_setClass(win, [GoPasteNSPanel class]);

    // 2) 加上 NonactivatingPanel 样式位。这一位就是"显示时不抢 active app"
    //    的关键。注意 setStyleMask 要保留原有的 Titled/Closable/Miniaturizable/Resizable。
    NSWindowStyleMask mask = [win styleMask] | NSWindowStyleMaskNonactivatingPanel;
    [win setStyleMask:mask];

    // 3) 浮动层级：让面板始终在普通应用窗口之上（和 Alfred/Raycast 一致）。
    //    NSFloatingWindowLevel = NSNormalWindowLevel + 3。
    [win setLevel:NSFloatingWindowLevel];

    // 4) 其他面板化行为：
    //    - HidesOnDeactivate=NO：我们自己控制显隐，不要 AppKit 代劳
    //    - BecomesKeyOnlyIfNeeded=NO：让面板能正常成为 key，接收键盘
    //    - CollectionBehavior：跨 Space / 在全屏应用上可见 / 不被 Expose 动
    [win setHidesOnDeactivate:NO];
    if ([win isKindOfClass:[NSPanel class]]) {
        [(NSPanel *)win setBecomesKeyOnlyIfNeeded:NO];
        [(NSPanel *)win setFloatingPanel:YES];
    }
    [win setCollectionBehavior:
        NSWindowCollectionBehaviorCanJoinAllSpaces |
        NSWindowCollectionBehaviorFullScreenAuxiliary |
        NSWindowCollectionBehaviorStationary];

    // 5) 隐藏左上角 traffic lights（关闭/最小化/缩放）。
    //    剪贴板面板是轻量浮层，不需要系统按钮——关闭由 ESC / 失焦触发，
    //    最小化/缩放语义不适用。把三个 NSButton hidden=YES 即可，不影响
    //    窗口内容区的可拖拽 titlebar 区域（Wails 的 drag region 不变）。
    //    参考：Alfred/Raycast/Eco 都是同样处理方式。
    NSButton *closeBtn = [win standardWindowButton:NSWindowCloseButton];
    NSButton *miniBtn  = [win standardWindowButton:NSWindowMiniaturizeButton];
    NSButton *zoomBtn  = [win standardWindowButton:NSWindowZoomButton];
    if (closeBtn) closeBtn.hidden = YES;
    if (miniBtn)  miniBtn.hidden  = YES;
    if (zoomBtn)  zoomBtn.hidden  = YES;

    objc_setAssociatedObject(win, kPanelConvertedKey, @YES, OBJC_ASSOCIATION_RETAIN);

    NSLog(@"gopaste: window '%@' converted to NonactivatingPanel", title);
}

// ------------------------------------------------------------------
// 对外 C 接口
// ------------------------------------------------------------------

// 同步调度到主线程（若已在主线程则直接执行），确保 AppKit 调用安全。
static void run_on_main_sync(void (^block)(void)) {
    if ([NSThread isMainThread]) {
        block();
    } else {
        dispatch_sync(dispatch_get_main_queue(), block);
    }
}

// 同 run_on_main_sync 但异步，不阻塞调用线程。
static void run_on_main_async(void (^block)(void)) {
    if ([NSThread isMainThread]) {
        block();
    } else {
        dispatch_async(dispatch_get_main_queue(), block);
    }
}

void GoPasteConvertToNonactivatingPanel(const char *ctitle) {
    if (!ctitle) return;
    NSString *title = [NSString stringWithUTF8String:ctitle];
    // 转换涉及读取 NSApp.windows，AppKit 要求主线程。
    // 用 sync 让 Go 侧调用完成后立刻能依赖面板状态。
    run_on_main_sync(^{
        convert_to_panel_on_main(title);
    });
}

// 隐藏面板：用 orderOut: 而非 [NSApp hide] / setIsVisible:NO。
// orderOut 只把窗口从屏幕上移除，不牵动 active application 状态；
// 对 NonactivatingPanel 而言是正确配对。
void GoPasteOrderOut(const char *ctitle) {
    if (!ctitle) return;
    NSString *title = [NSString stringWithUTF8String:ctitle];
    run_on_main_async(^{
        NSWindow *win = find_window(title);
        if (win) {
            [win orderOut:nil];
        }
    });
}

// 显示面板：orderFrontRegardless 让面板前置但不激活 app。
// 配合 makeKeyWindow 让我们的 JS/输入框拿到键盘焦点。
void GoPasteOrderFront(const char *ctitle) {
    if (!ctitle) return;
    NSString *title = [NSString stringWithUTF8String:ctitle];
    run_on_main_async(^{
        NSWindow *win = find_window(title);
        if (win) {
            [win orderFrontRegardless];
            [win makeKeyWindow];
        }
    });
}

// 交还键盘焦点给下面的应用：粘贴前调用一次，让目标应用成为 key window。
// 不隐藏面板（Eco 的做法：resign 即可，hide 交给后续动作或让用户感知）。
void GoPasteResignKey(const char *ctitle) {
    if (!ctitle) return;
    NSString *title = [NSString stringWithUTF8String:ctitle];
    run_on_main_async(^{
        NSWindow *win = find_window(title);
        if (win) {
            [win resignKeyWindow];
        }
    });
}
