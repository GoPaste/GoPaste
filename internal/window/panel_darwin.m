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
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include <string.h>  // for strdup

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
    //    - CollectionBehavior：每次 show 主动跟到当前 Space / 在全屏应用上可见 / 不被 Expose 动
    [win setHidesOnDeactivate:NO];
    if ([win isKindOfClass:[NSPanel class]]) {
        [(NSPanel *)win setBecomesKeyOnlyIfNeeded:NO];
        [(NSPanel *)win setFloatingPanel:YES];
    }
    // ─── 跨 Space 显示策略：MoveToActiveSpace 而不是 CanJoinAllSpaces ───
    // 之前用 CanJoinAllSpaces，意图是"窗口同时存在于所有 Space"，但实测在
    // 多个全屏 Space（每个全屏应用自成一个 Space）的环境下，靠后的几个
    // Space（Mission Control 里位置 4/5/6+）按全局快捷键不显示 —— AppKit
    // 对 CanJoinAllSpaces 的实现在全屏 Space 多的场景有可观察的退化。
    //
    // 改用 MoveToActiveSpace：苹果文档明确说"窗口在 show 时移到当前 active
    // Space"，专为非激活面板（NonactivatingPanel）跟随用户切 Space 设计。
    // 两者语义互斥：
    //   - CanJoinAllSpaces  = 面板恒存在于所有 Space（菜单栏下拉那种）
    //   - MoveToActiveSpace = 面板按需"跳过来"，更贴合剪贴板/launcher 的
    //                        "按快捷键弹一下"交互
    // FullScreenAuxiliary 仍然保留：让面板能浮在 *全屏应用* 的窗口之上而
    // 不被全屏窗口遮挡。Stationary 让面板不参与 Mission Control 的动画。
    [win setCollectionBehavior:
        NSWindowCollectionBehaviorMoveToActiveSpace |
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
//
// ─── 跨 Space 显示的可靠性 ───────────────────────────────────────
// 用户在 macOS 上可以通过触摸板四指左右滑切换 Space（桌面 / 全屏空间）。
// 切到非"创建窗口时所在 Space"后按全局快捷键，普通 NSWindow 默认绑定
// 在原 Space，系统会切回去而不是把窗口带到当前 Space —— 表现为"按了
// 快捷键好像没反应"（实际上窗口在另一个 Space 出现了）。
//
// 解决方案：CollectionBehavior = MoveToActiveSpace + FullScreenAuxiliary。
// 之前曾用 CanJoinAllSpaces，在普通桌面 Space 间切换没问题，但用户报告
// 多个全屏 Space（6+ 个，前几个能弹后几个不能）时退化，已切换为
// MoveToActiveSpace（每次 show 主动跳到当前 active Space，更适合按需弹出
// 的剪贴板面板）。详见 convert_to_panel_on_main 里的设置注释。
//
// 这里 GoPasteOrderFront 在每次 show 前**重新强制设置一次** collectionBehavior
// + floating level 作为兜底，原因：AppKit 在 makeKeyWindow / 全屏切换 /
// Mission Control 等路径下会偶发性地清掉非默认的 collectionBehavior，导致
// 面板"卡"在原 Space。幂等操作、无副作用，但能消除这类回归。
//
// 另一个隐形坑：orderFrontRegardless 对 hidden window 才会触发"挪到当前
// Space"的动作（前提是 MoveToActiveSpace 已设）。顺序不能反——必须先
// setCollectionBehavior 再 orderFrontRegardless。
void GoPasteOrderFront(const char *ctitle) {
    if (!ctitle) return;
    NSString *title = [NSString stringWithUTF8String:ctitle];
    run_on_main_async(^{
        NSWindow *win = find_window(title);
        if (!win) return;

        // 兜底重置 collectionBehavior —— 与 convert_to_panel_on_main 保持一致。
        // 即便 AppKit 中途清空过这些位，这里也能恢复，保证面板能跟到当前 Space。
        [win setCollectionBehavior:
            NSWindowCollectionBehaviorMoveToActiveSpace |
            NSWindowCollectionBehaviorFullScreenAuxiliary |
            NSWindowCollectionBehaviorStationary];
        // 浮动层级也一并兜底（同样原因：可能被 AppKit 改回 NSNormalWindowLevel）。
        [win setLevel:NSFloatingWindowLevel];

        [win orderFrontRegardless];
        [win makeKeyWindow];
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

// GoPasteIsWindowKey 检查指定 title 的窗口当前是否是 keyWindow。
// 返回 1 (true) / 0 (false)。
// 供热键回调使用：若面板是 keyWindow，说明用户正在与 GoPaste 交互，
// 此时不应再触发全局热键动作（如 togglePanel），避免与前端输入法组合冲突。
int GoPasteIsWindowKey(const char *ctitle) {
    if (!ctitle) return 0;
    __block int isKey = 0;
    NSString *title = [NSString stringWithUTF8String:ctitle];
    dispatch_sync(dispatch_get_main_queue(), ^{
        NSWindow *win = find_window(title);
        if (win && [win isKeyWindow]) {
            isKey = 1;
        }
    });
    return isKey;
}

// 弹出系统对话框前临时激活 GoPaste。
// NonactivatingPanel 不是 active app，SaveFileDialog / OpenFileDialog 等
// NSSavePanel 需要 active app 才能正常显示并接收输入。
// 使用同步调用确保激活完成后再返回，调用方才能弹出对话框。
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
// 避免 Wails SaveFileDialog 跨线程 dispatch 导致激活状态在弹框前丢失。
// 返回用户选择的路径（C 字符串，调用方需 free），取消返回 NULL。
char *GoPasteSaveFileDialog(const char *title, const char *defaultName) {
    __block char *result = NULL;
    run_on_main_sync(^{
        // 激活应用，让 NSSavePanel 能正常显示
        [[NSApplication sharedApplication] activateIgnoringOtherApps:YES];

        NSSavePanel *panel = [NSSavePanel savePanel];
        if (title) {
            panel.title = [NSString stringWithUTF8String:title];
        }
        if (defaultName) {
            panel.nameFieldStringValue = [NSString stringWithUTF8String:defaultName];
        }
        // 限制文件类型
        if (@available(macOS 11.0, *)) {
            panel.allowedContentTypes = @[UTTypeJSON];
        } else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
            panel.allowedFileTypes = @[@"json"];
#pragma clang diagnostic pop
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
