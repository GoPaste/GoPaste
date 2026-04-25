// iconsize_darwin.m —— 修正 fyne.io/systray v1.12.0 在 macOS 下把状态栏图标
// 强制设成 16×16pt 的问题。
//
// 现象：
//   不论传入多大的 PNG，菜单栏图标看起来都很小。
//
// 原因：
//   systray 的 setIcon 实现里硬编码：
//     [image setSize:NSMakeSize(16, 16)];
//   这把 NSImage 的逻辑尺寸固定为 16pt。macOS 菜单栏内容区高度约 22pt，
//   16pt 的图标会比同栏其它 app 图标"小一圈"。
//
// 修复（v2，避免 v1 的崩溃）：
//   v1 版本是直接 [statusItem.button.image setSize:NSMakeSize(22,22)]，
//   会导致 AppKit 在重绘 status bar button 时偶发 SIGSEGV(addr=0x20)，
//   原因是 AppKit 内部缓存了旧 size 对应的 NSImageRep，原对象的 size
//   被外部改写后，重绘命中测试访问了已失效的字段。
//
//   v2 不再修改原 image：
//     1) 取出 statusItem.button.image
//     2) **复制**一份 image
//     3) 在副本上 setSize:22pt
//     4) 把副本装回 button.image
//   这样 AppKit 拿到的是一个全新对象，不会和它内部对原对象的缓存产生
//   不一致，原对象保持 16pt 不被外部触碰。
//
// 调用时机：每次 applyIcon() 之后调一次。systray 后续如再次 setIcon，
// 会再次把 button.image 替换为 16pt 版本，需重新 SetTrayIconSize。

#import <AppKit/AppKit.h>

// 设置状态栏图标的逻辑尺寸（单位：pt）。
// pt 取 0 或负值时回退到默认 22。
void SetTrayIconSize(double pt) {
    if (pt <= 0) {
        pt = 22.0;
    }

    // 所有 NSStatusItem / button 操作必须主线程。
    dispatch_block_t work = ^{
        @autoreleasepool {
            Class cls = NSClassFromString(@"SystrayAppDelegate");
            if (!cls) {
                return;
            }
            id delegate = [NSApp delegate];
            if (![delegate isKindOfClass:cls]) {
                return;
            }
            NSStatusItem *statusItem = nil;
            @try {
                statusItem = [delegate valueForKey:@"statusItem"];
            } @catch (NSException *e) {
                return;
            }
            if (!statusItem) {
                return;
            }
            NSStatusBarButton *button = statusItem.button;
            if (!button) {
                return;
            }
            NSImage *original = button.image;
            if (!original) {
                return;
            }

            // 复制一份再改，避免触碰 systray 内部持有的原 image 对象。
            // [original copy] 走 NSCopying，会复制 NSImageRep 数组（浅拷贝
            // representation 引用，但 NSImage 自身字段是新的）。
            NSImage *resized = [original copy];
            if (!resized) {
                return;
            }
            [resized setSize:NSMakeSize(pt, pt)];

            // 装回 button：AppKit 会以新对象为准重新建立缓存。
            button.image = resized;
        }
    };

    if ([NSThread isMainThread]) {
        work();
    } else {
        dispatch_async(dispatch_get_main_queue(), work);
    }
}
