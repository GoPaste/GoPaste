// cmdq_darwin.m —— macOS 下"键盘 Cmd+Q 拦截"实现。
//
// 边界（重要）：
//   本模块 *只* 拦截"键盘 Cmd+Q"这一条键盘事件通道。
//   菜单栏 Quit、Dock 右键 Quit、业务主动调 terminate:/wailsruntime.Quit
//   等非键盘路径，一律不干预，保持系统默认行为。
//
// 策略：
//   - default : 不拦截
//   - confirm : 第一次按下记录时间戳并通知前端 toast；窗口内再次按下才放行
//               真正的 Cmd+Q 事件
//   - disable : 直接吞掉 Cmd+Q keyDown；通知前端一次 "disabled"
//
// 实现（三层，按收敛度排序）：
//
//   L0) CGEventTap（kCGSessionEventTap, HeadInsert）
//       *全局* 拦截——无论前台是哪个 App，Cmd+Q 都会先进入本 tap。
//       需要用户在「系统设置 → 隐私与安全性 → 输入监控」授权 GoPaste。
//       未授权时 tap 创建失败，自动降级到 L1/L2。
//
//   L1) Swizzle -[NSApplication sendEvent:]
//       所有键盘事件都会经过 sendEvent:（Wails / WKWebView 也不例外）。
//       仅对 *GoPaste 自身前台* 的 Cmd+Q 生效。作为 L0 未授权时的降级方案。
//
//   L2) NSEvent addLocalMonitorForEventsMatchingMask:
//       第二保险兜底。
//
// 关键点：confirm 模式"第二次按下要真正退出"——必须由我们自己显式调用
// [NSApp terminate:nil]，而不是依赖菜单 keyEquivalent。
// 原因：Wails 应用没有自带带 ⌘Q keyEquivalent 的 "Quit" 菜单项，
// 因此把事件原样放行后没有任何 responder 会处理它，结果就是"按两次也不退出"。
// 显式 terminate 同时把事件吞掉（return YES / return NULL），避免事件继续传播。

#import <Cocoa/Cocoa.h>
#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <IOKit/hidsystem/IOHIDLib.h>
#import <Carbon/Carbon.h>  // kVK_ANSI_Q
#import <objc/runtime.h>
#import <objc/message.h>
#include <string.h>

// 由 cmdq_darwin.go 提供。
extern void GoPasteCmdQNotify(char *reason);

// ------------------------------------------------------------------
// 策略单例
// ------------------------------------------------------------------
@interface GoPasteCmdQDelegate : NSObject
@property (nonatomic, copy)   NSString *behavior;           // default / confirm / disable
@property (nonatomic, assign) NSInteger confirmWindowMs;    // confirm 模式的时间窗，默认 1500
@property (nonatomic, assign) NSTimeInterval lastPressTs;   // confirm 模式：上次按下时间戳（秒）
@property (nonatomic, strong) id localMonitor;              // L2 monitor 句柄

// L0 Tap 相关
@property (nonatomic, assign) CFMachPortRef tapPort;
@property (nonatomic, assign) CFRunLoopSourceRef tapRLSrc;
@property (nonatomic, assign) BOOL tapEnabled;

// L0 Toast 浮层
@property (nonatomic, strong) NSPanel *toastPanel;
@end

@implementation GoPasteCmdQDelegate

+ (instancetype)shared {
    static GoPasteCmdQDelegate *s = nil;
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        s = [GoPasteCmdQDelegate new];
        s.behavior = @"default";
        s.confirmWindowMs = 1500;
        s.lastPressTs = 0;
        s.localMonitor = nil;
        s.tapPort = NULL;
        s.tapRLSrc = NULL;
        s.tapEnabled = NO;
        s.toastPanel = nil;
    });
    return s;
}

// 对一个已确认是"纯 Cmd+Q keyDown"的事件做策略判定。
// 返回 YES = 吞掉（不向下传播）；NO = 放行（原样继续派发）。
// fromTap=YES 表示来自 L0 全局 tap，通知 reason 会带 -global 后缀。
- (BOOL)shouldSwallowCmdQFromTap:(BOOL)fromTap {
    NSString *b = self.behavior ?: @"default";

    if ([b isEqualToString:@"disable"]) {
        GoPasteCmdQNotify((char *)(fromTap ? "disabled-global" : "disabled"));
        if (fromTap) {
            [self showToastText:@"Command+Q 已禁用，可在「扩展功能」中修改"];
        }
        return YES;
    }

    if ([b isEqualToString:@"confirm"]) {
        NSTimeInterval now = [[NSDate date] timeIntervalSince1970];
        NSTimeInterval windowSec = (NSTimeInterval)self.confirmWindowMs / 1000.0;
        if (self.lastPressTs > 0 && (now - self.lastPressTs) <= windowSec) {
            // 第二次按下且仍在窗口内 → 主动 terminate。
            // 不能依赖"放行让菜单 keyEquivalent 处理"——Wails App 没有带
            // ⌘Q 的 Quit 菜单项，原样放行不会触发任何退出。
            self.lastPressTs = 0;
            GoPasteCmdQNotify((char *)(fromTap ? "confirm-second-global" : "confirm-second"));
            // 切回主线程异步 terminate，避免在 tap callback / sendEvent 调用栈里
            // 直接退出导致的清理时序问题。
            dispatch_async(dispatch_get_main_queue(), ^{
                [NSApp terminate:nil];
            });
            return YES;  // 吞掉这次按键事件本身（terminate 由上面的 block 触发）
        }
        // 首次按下 → 记录时间戳，通知 toast，然后吞掉这次按键。
        self.lastPressTs = now;
        GoPasteCmdQNotify((char *)(fromTap ? "confirm-first-global" : "confirm-first"));
        if (fromTap) {
            [self showToastText:@"再按一次 Command+Q 以退出"];
        }
        NSTimeInterval windowSec2 = windowSec;
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(windowSec2 * NSEC_PER_SEC)),
                       dispatch_get_main_queue(), ^{
            NSTimeInterval cur = [[NSDate date] timeIntervalSince1970];
            if (self.lastPressTs > 0 && (cur - self.lastPressTs) >= windowSec2 - 0.05) {
                self.lastPressTs = 0;
                GoPasteCmdQNotify((char *)"confirm-timeout");
            }
        });
        return YES;
    }

    // default：放行。
    return NO;
}

// ------------------------------------------------------------------
// NSPanel Toast —— L0 模式下不依赖 GoPaste 前端即可显示
// ------------------------------------------------------------------
- (void)showToastText:(NSString *)text {
    dispatch_async(dispatch_get_main_queue(), ^{
        [self ensureToastPanel];
        NSTextField *label = (NSTextField *)self.toastPanel.contentView.subviews.firstObject;
        label.stringValue = text;
        [label sizeToFit];

        NSRect tf = label.frame;
        CGFloat padX = 24, padY = 14;
        NSSize panelSize = NSMakeSize(tf.size.width + padX * 2, tf.size.height + padY * 2);
        label.frame = NSMakeRect(padX, padY, tf.size.width, tf.size.height);

        NSScreen *screen = [NSScreen mainScreen];
        NSRect sf = screen.visibleFrame;
        NSRect panelFrame = NSMakeRect(sf.origin.x + (sf.size.width - panelSize.width) / 2.0,
                                        sf.origin.y + sf.size.height - panelSize.height - 120,
                                        panelSize.width, panelSize.height);
        [self.toastPanel setFrame:panelFrame display:YES];
        [self.toastPanel orderFrontRegardless];

        // 2s 后淡出
        static int32_t token = 0;
        int32_t myToken = ++token;
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(1.8 * NSEC_PER_SEC)),
                       dispatch_get_main_queue(), ^{
            if (myToken != token) return;  // 有新 toast，跳过这次隐藏
            [self.toastPanel orderOut:nil];
        });
    });
}

- (void)ensureToastPanel {
    if (self.toastPanel) return;
    NSPanel *p = [[NSPanel alloc] initWithContentRect:NSMakeRect(0, 0, 360, 44)
                                            styleMask:(NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel)
                                              backing:NSBackingStoreBuffered
                                                defer:NO];
    p.level = NSFloatingWindowLevel;
    p.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces
                         | NSWindowCollectionBehaviorStationary
                         | NSWindowCollectionBehaviorFullScreenAuxiliary
                         | NSWindowCollectionBehaviorIgnoresCycle;
    p.hidesOnDeactivate = NO;
    p.opaque = NO;
    p.backgroundColor = [NSColor clearColor];
    p.hasShadow = YES;
    p.ignoresMouseEvents = YES;
    p.movableByWindowBackground = NO;

    NSView *content = [[NSView alloc] initWithFrame:p.contentView.bounds];
    content.wantsLayer = YES;
    content.layer.cornerRadius = 12.0;
    content.layer.masksToBounds = YES;

    NSVisualEffectView *bg = [[NSVisualEffectView alloc] initWithFrame:content.bounds];
    bg.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    bg.blendingMode = NSVisualEffectBlendingModeBehindWindow;
    bg.material = NSVisualEffectMaterialHUDWindow;
    bg.state = NSVisualEffectStateActive;
    bg.wantsLayer = YES;
    bg.layer.cornerRadius = 12.0;
    bg.layer.masksToBounds = YES;
    [content addSubview:bg];

    NSTextField *label = [[NSTextField alloc] initWithFrame:NSMakeRect(24, 14, 300, 18)];
    label.bezeled = NO;
    label.drawsBackground = NO;
    label.editable = NO;
    label.selectable = NO;
    label.stringValue = @"";
    label.font = [NSFont systemFontOfSize:14 weight:NSFontWeightMedium];
    label.textColor = [NSColor labelColor];
    label.alignment = NSTextAlignmentCenter;
    [content addSubview:label];

    p.contentView = content;
    self.toastPanel = p;
}

@end

// ------------------------------------------------------------------
// 判断 NSEvent 是不是"纯 Cmd+Q" 的 keyDown（不含 Shift/Option/Ctrl）。
// ------------------------------------------------------------------
static BOOL event_is_cmd_q_keydown(NSEvent *e) {
    if ([e type] != NSEventTypeKeyDown) return NO;
    NSEventModifierFlags flags = [e modifierFlags] & NSEventModifierFlagDeviceIndependentFlagsMask;
    if (!(flags & NSEventModifierFlagCommand)) return NO;
    if (flags & NSEventModifierFlagShift)     return NO;
    if (flags & NSEventModifierFlagOption)    return NO;
    if (flags & NSEventModifierFlagControl)   return NO;

    NSString *chars = [e charactersIgnoringModifiers];
    if ([chars length] == 0) chars = [e characters];
    return [[chars lowercaseString] isEqualToString:@"q"];
}

// 同上，但作用于 CGEvent（L0 tap 层）。
static BOOL cg_event_is_cmd_q_keydown(CGEventType type, CGEventRef ev) {
    if (type != kCGEventKeyDown) return NO;
    CGEventFlags flags = CGEventGetFlags(ev);
    if (!(flags & kCGEventFlagMaskCommand)) return NO;
    if (flags & kCGEventFlagMaskShift)      return NO;
    if (flags & kCGEventFlagMaskAlternate)  return NO;
    if (flags & kCGEventFlagMaskControl)    return NO;
    int64_t keycode = CGEventGetIntegerValueField(ev, kCGKeyboardEventKeycode);
    return keycode == kVK_ANSI_Q;
}

// ------------------------------------------------------------------
// L0) CGEventTap 全局拦截
// ------------------------------------------------------------------
static CGEventRef gopaste_tap_callback(CGEventTapProxy proxy, CGEventType type,
                                        CGEventRef event, void *refcon) {
    // tap 被系统禁用时要重新启用。
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        GoPasteCmdQDelegate *d = [GoPasteCmdQDelegate shared];
        if (d.tapPort) {
            CGEventTapEnable(d.tapPort, true);
            NSLog(@"gopaste: cmdq: L0 tap re-enabled after disable (type=%d)", type);
        }
        return event;
    }

    // 诊断日志：只在 Cmd 键按下时打（避免刷屏），能确认 callback 活着。
    if (type == kCGEventKeyDown) {
        CGEventFlags flags = CGEventGetFlags(event);
        if (flags & kCGEventFlagMaskCommand) {
            int64_t kc = CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
            NSLog(@"gopaste: cmdq: L0 tap saw Cmd+keycode=%lld (kVK_ANSI_Q=%d)", kc, kVK_ANSI_Q);
        }
    }

    if (!cg_event_is_cmd_q_keydown(type, event)) {
        return event;
    }

    NSLog(@"gopaste: cmdq: L0 tap matched Cmd+Q");
    GoPasteCmdQDelegate *d = [GoPasteCmdQDelegate shared];
    BOOL swallow = [d shouldSwallowCmdQFromTap:YES];
    NSLog(@"gopaste: cmdq: L0 swallow=%@ behavior=%@", swallow ? @"YES" : @"NO", d.behavior);
    if (swallow) {
        return NULL;  // 吞掉：事件不再向下派发
    }
    return event;     // 放行：照常送到前台 App，系统走 Quit 菜单
}

static void install_event_tap(void) {
    GoPasteCmdQDelegate *d = [GoPasteCmdQDelegate shared];
    if (d.tapPort != NULL) {
        NSLog(@"gopaste: cmdq: L0 tap already installed, skip");
        return;
    }

    // 诊断：打印当前权限状态
    if (@available(macOS 10.15, *)) {
        IOHIDAccessType at = IOHIDCheckAccess(kIOHIDRequestTypeListenEvent);
        NSLog(@"gopaste: cmdq: IOHIDCheckAccess(ListenEvent)=%d (0=unknown,1=denied,2=granted)", at);
    }
    NSLog(@"gopaste: cmdq: bundleID=%@ bundlePath=%@",
          [[NSBundle mainBundle] bundleIdentifier],
          [[NSBundle mainBundle] bundlePath]);

    CGEventMask mask = CGEventMaskBit(kCGEventKeyDown);
    CFMachPortRef tap = CGEventTapCreate(kCGSessionEventTap,
                                          kCGHeadInsertEventTap,
                                          kCGEventTapOptionDefault,
                                          mask,
                                          gopaste_tap_callback,
                                          NULL);
    if (!tap) {
        NSLog(@"gopaste: cmdq: L0 tap create FAILED (input monitoring not authorized or wrong binary path?)");
        d.tapEnabled = NO;
        return;
    }
    CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
    CFRunLoopAddSource(CFRunLoopGetMain(), src, kCFRunLoopCommonModes);
    CGEventTapEnable(tap, true);

    d.tapPort = tap;
    d.tapRLSrc = src;
    d.tapEnabled = YES;
    NSLog(@"gopaste: cmdq: L0 global event tap installed successfully");
}

static void uninstall_event_tap(void) {
    GoPasteCmdQDelegate *d = [GoPasteCmdQDelegate shared];
    if (!d.tapPort) return;
    CGEventTapEnable(d.tapPort, false);
    if (d.tapRLSrc) {
        CFRunLoopRemoveSource(CFRunLoopGetMain(), d.tapRLSrc, kCFRunLoopCommonModes);
        CFRelease(d.tapRLSrc);
        d.tapRLSrc = NULL;
    }
    CFRelease(d.tapPort);
    d.tapPort = NULL;
    d.tapEnabled = NO;
    NSLog(@"gopaste: cmdq: L0 global event tap uninstalled");
}

// ------------------------------------------------------------------
// L1) Swizzle -[NSApplication sendEvent:]
// ------------------------------------------------------------------
static void (*g_orig_sendEvent)(id, SEL, NSEvent *) = NULL;

static void gopaste_swizzled_sendEvent(id self, SEL _cmd, NSEvent *event) {
    if (event_is_cmd_q_keydown(event)) {
        // 如果 L0 已启用，L1 直接不介入（避免同一事件被双拦——
        // 实际上 L0 已经吞了就到不了这里，但 confirm 放行路径会进来，
        // 这里必须放行避免重复记账）。
        GoPasteCmdQDelegate *d = [GoPasteCmdQDelegate shared];
        if (!d.tapEnabled) {
            if ([d shouldSwallowCmdQFromTap:NO]) {
                return;  // 吞掉
            }
        }
    }
    if (g_orig_sendEvent) {
        g_orig_sendEvent(self, _cmd, event);
    }
}

static void install_sendEvent_swizzle(void) {
    static BOOL installed = NO;
    if (installed) return;

    Class cls = [NSApplication class];
    Method m = class_getInstanceMethod(cls, @selector(sendEvent:));
    if (!m) {
        NSLog(@"gopaste: cmdq: WARN -[NSApplication sendEvent:] not found");
        return;
    }
    g_orig_sendEvent = (void (*)(id, SEL, NSEvent *))method_getImplementation(m);
    method_setImplementation(m, (IMP)gopaste_swizzled_sendEvent);
    installed = YES;
    NSLog(@"gopaste: cmdq: -[NSApplication sendEvent:] swizzled (L1)");
}

// ------------------------------------------------------------------
// L2) NSEvent local keyDown monitor（兜底）
// ------------------------------------------------------------------
static void install_local_monitor(void) {
    GoPasteCmdQDelegate *d = [GoPasteCmdQDelegate shared];
    if (d.localMonitor != nil) return;

    id monitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown
                                                       handler:^NSEvent * _Nullable(NSEvent * _Nonnull event) {
        if (!event_is_cmd_q_keydown(event)) return event;
        return event;
    }];
    d.localMonitor = monitor;
    NSLog(@"gopaste: cmdq: local key monitor installed (L2)");
}

// ------------------------------------------------------------------
// 安装入口
// ------------------------------------------------------------------
static void install_on_main(void) {
    install_sendEvent_swizzle();
    install_local_monitor();
}

// ------------------------------------------------------------------
// 对外 C 接口
// ------------------------------------------------------------------

void GoPasteInstallCmdQGuard(void) {
    if ([NSThread isMainThread]) {
        install_on_main();
    } else {
        dispatch_async(dispatch_get_main_queue(), ^{
            install_on_main();
        });
    }
}

void GoPasteSetCmdQBehavior(const char *behavior) {
    if (!behavior) return;
    NSString *b = [NSString stringWithUTF8String:behavior];
    [GoPasteCmdQDelegate shared].behavior = b;
    // 切换策略时重置确认窗口，避免残留 lastPressTs 导致误放行。
    [GoPasteCmdQDelegate shared].lastPressTs = 0;
}

void GoPasteSetCmdQConfirmWindowMs(int ms) {
    [GoPasteCmdQDelegate shared].confirmWindowMs = (NSInteger)ms;
}

// ------------------------------------------------------------------
// L0 全局拦截（CGEventTap）—— 对外接口
// ------------------------------------------------------------------

// 查询输入监控授权状态。
// 返回：0 未决 / 1 已授权 / 2 被拒
int GoPasteCmdQTapAuthStatus(void) {
    if (@available(macOS 10.15, *)) {
        IOHIDAccessType t = IOHIDCheckAccess(kIOHIDRequestTypeListenEvent);
        switch (t) {
            case kIOHIDAccessTypeGranted: return 1;
            case kIOHIDAccessTypeDenied:  return 2;
            default:                      return 0;
        }
    }
    // < 10.15 无此 API，默认当作已授权（老系统上 CGEventTap 无需显式授权）
    return 1;
}

// 触发系统授权弹窗（仅首次有效；后续用户要去设置里手动勾）。
// 返回：1 调用后已授权 / 0 未授权（等用户操作）
int GoPasteCmdQTapRequestAccess(void) {
    if (@available(macOS 10.15, *)) {
        return IOHIDRequestAccess(kIOHIDRequestTypeListenEvent) ? 1 : 0;
    }
    return 1;
}

// 启用 L0 全局 Cmd+Q 拦截。
// 返回：1 成功 / 0 失败（通常是未授权）
int GoPasteCmdQTapStart(void) {
    __block int result = 0;
    void (^work)(void) = ^{
        install_event_tap();
        result = [GoPasteCmdQDelegate shared].tapEnabled ? 1 : 0;
    };
    if ([NSThread isMainThread]) {
        work();
    } else {
        dispatch_sync(dispatch_get_main_queue(), work);
    }
    return result;
}

// 停用 L0 全局拦截。之后行为回退到 L1/L2（仅 GoPaste 自身前台生效）。
void GoPasteCmdQTapStop(void) {
    if ([NSThread isMainThread]) {
        uninstall_event_tap();
    } else {
        dispatch_async(dispatch_get_main_queue(), ^{
            uninstall_event_tap();
        });
    }
}

// 打开系统设置 → 输入监控面板。
void GoPasteCmdQOpenInputMonitoringPrefs(void) {
    NSURL *u = [NSURL URLWithString:@"x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent"];
    [[NSWorkspace sharedWorkspace] openURL:u];
}

// 调试：让 Go 侧能直接打 NSLog（绕过 slog 文件 sink）。
void GoPasteDebugLog(const char *msg) {
    if (!msg) return;
    NSLog(@"gopaste: %s", msg);
}
