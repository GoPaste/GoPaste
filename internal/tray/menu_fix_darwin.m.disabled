// menu_fix_darwin.m —— 修正 fyne.io/systray v1.12.0 在 macOS 下弹出菜单的两个毛病：
//
// 现象：
//   1) 菜单不在图标正下方；
//   2) 菜单顶部出现一个向上的 ⌃ 滚动箭头，点击才能完整展开。
//
// 原因：
//   systray 的 -[SystrayAppDelegate show_menu] 走的是：
//     [menu popUpMenuPositioningItem:nil
//                         atLocation:NSMakePoint(0, button.bounds.size.height+6)
//                             inView:button];
//   在带刘海的屏 / 非主屏 / 被压缩的菜单栏上，+6 偏移会把菜单原点抬到菜单栏
//   上沿之外，AppKit 认为菜单顶部被裁剪 → 绘制 ⌃ 指示并自动挪位，菜单
//   也就不再贴在图标正下方。
//
// 修复：
//   用 Objective-C runtime 在进程启动阶段替换掉 show_menu 的 IMP，改为
//   调用 AppKit 官方推荐的：
//     [NSMenu popUpContextMenu:menu withEvent:event forView:button];
//   该 API 由系统计算弹出位置，保证：紧贴图标正下方、不被菜单栏/刘海裁剪、
//   永远不出现 ⌃ 滚动箭头。
//
// 为什么用 swizzle 而不是 fork：
//   systray 的那行硬编码坐标没有任何调参入口。要修就得动上游，或者把整
//   个 systray 复制进项目，再改。swizzle 一行就能解决，零维护成本；只要
//   方法名 "show_menu" 还在（自 v1.10 起一直没变），就持续生效。

#import <AppKit/AppKit.h>
#import <objc/runtime.h>

// 新的 show_menu 实现。self 就是 SystrayAppDelegate 实例；我们通过
// KVC 取到它持有的 NSStatusItem 和 NSMenu（这两个 ivar 名字是 systray
// 源码里写死的：statusItem / menu）。
static void gopaste_patched_show_menu(id self, SEL _cmd) {
    NSStatusItem *statusItem = [self valueForKey:@"statusItem"];
    NSMenu       *menu       = [self valueForKey:@"menu"];
    if (!statusItem || !menu) {
        return;
    }
    NSView *button = statusItem.button;
    if (!button) {
        return;
    }
    // 优先用"当前正在处理的那个鼠标事件"作为 popUp 锚点——鼠标落点附近，
    // AppKit 会自动贴到最近的合适位置（对 status bar item 来说就是图标
    // 正下方）。拿不到 currentEvent 时退化回按钮原点。
    NSEvent *ev = [NSApp currentEvent];
    if (ev) {
        [NSMenu popUpContextMenu:menu withEvent:ev forView:button];
        return;
    }
    // Fallback：直接贴按钮底部弹（去掉原实现的 +6 偏移，避免越过菜单栏上沿）。
    [menu popUpMenuPositioningItem:nil
                        atLocation:NSMakePoint(0, 0)
                            inView:button];
}

// 在加载阶段替换 SystrayAppDelegate 的 -show_menu IMP。
// systray 会在 applicationDidFinishLaunching 里才真正用到 show_menu；
// 我们这里发生在更早，只要替换成功，后续所有 show_menu 调用都走新逻辑。
__attribute__((constructor))
static void gopaste_install_menu_fix(void) {
    Class cls = NSClassFromString(@"SystrayAppDelegate");
    if (!cls) {
        // systray 尚未被链接进来（理论上同一 bundle 内不应出现），直接放弃。
        return;
    }
    SEL sel = sel_registerName("show_menu");
    Method m = class_getInstanceMethod(cls, sel);
    if (!m) {
        return;
    }
    // 签名：void (id self, SEL _cmd) → "v@:"
    method_setImplementation(m, (IMP)gopaste_patched_show_menu);
}
