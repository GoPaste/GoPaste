// statusitem_darwin.m
// 纯 ObjC 实现 macOS 菜单栏图标（NSStatusItem + NSMenu）。
//
// 设计要点：
//   - 所有 ObjC 对象（NSStatusItem、NSMenu、delegate）都由 ObjC ARC 持有，
//     不经过 Go GC，彻底避免 fyne/systray 的"Go GC 回收 ObjC target → 野指针"问题。
//   - 回调通过 goStatusItemOnShow / goStatusItemOnAbout / goStatusItemOnRestart /
//     goStatusItemOnQuit 四个 Go export 函数转回 Go 层，这些函数本身不持有任何
//     ObjC 对象，GC 安全。
//   - 整个生命周期（install → click → menu select → uninstall）全部在主线程运行，
//     dispatch_async 保证调用方无需关心线程。

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

// Go 侧导出的回调，由 statusitem_darwin.go 实现
extern void goStatusItemOnShow(void);
extern void goStatusItemOnAbout(void);
extern void goStatusItemOnRestart(void);
extern void goStatusItemOnQuit(void);

// --------------------------------------------------------------------------
// GoPasteStatusItemDelegate：NSStatusItem 点击 + NSMenu 事件处理
// --------------------------------------------------------------------------
@interface GoPasteStatusItemDelegate : NSObject
@end

@implementation GoPasteStatusItemDelegate

- (void)onIconClicked:(id)sender {
    goStatusItemOnShow();
}

- (void)onMenuShow:(id)sender {
    goStatusItemOnShow();
}

- (void)onMenuAbout:(id)sender {
    goStatusItemOnAbout();
}

- (void)onMenuRestart:(id)sender {
    goStatusItemOnRestart();
}

- (void)onMenuQuit:(id)sender {
    goStatusItemOnQuit();
}

@end

// --------------------------------------------------------------------------
// 模块级静态变量（全部由 ARC 持有）
// --------------------------------------------------------------------------
static NSStatusItem          *gStatusItem  = nil;
static GoPasteStatusItemDelegate *gDelegate = nil;
static BOOL                   gInstalled   = NO;

// --------------------------------------------------------------------------
// 对外 C 接口（由 statusitem_darwin.go 调用）
// --------------------------------------------------------------------------

// GoPasteStatusItemInstall 在主线程创建 NSStatusItem + NSMenu。
// icon_png / icon_len：PNG 数据；传 NULL 则用系统默认文字"P"。
// 多次调用幂等。
void GoPasteStatusItemInstall(const unsigned char *icon_png, int icon_len) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (gInstalled) return;

        gDelegate = [[GoPasteStatusItemDelegate alloc] init];

        // 1) 创建 NSStatusItem，固定宽度让系统自动排版
        gStatusItem = [[NSStatusBar systemStatusBar]
                        statusItemWithLength:NSSquareStatusItemLength];

        // 2) 设置图标
        if (icon_png && icon_len > 0) {
            NSData *data = [NSData dataWithBytes:icon_png length:icon_len];
            NSImage *img = [[NSImage alloc] initWithData:data];
            if (img) {
                // 菜单栏标准尺寸 18pt（@2x → 36px），但 macOS 会自动缩放
                [img setSize:NSMakeSize(18, 18)];
                // template=YES：系统按深浅主题自动染色（黑/白）
                // 如果用彩色图标，改为 NO 即可
                [img setTemplate:YES];
                gStatusItem.button.image = img;
            }
        } else {
            gStatusItem.button.title = @"P";
        }

        // 3) 左键点击直接触发 OnShow（不弹菜单）
        //    右键弹菜单：通过 NSStatusItem 的 menu 属性实现
        // 注：设置 menu 后，左键/右键点击都会弹出菜单，button.action 不再触发。
        // 这是 macOS 菜单栏 app 的标准交互，"显示主面板"放在菜单第一项即可。

        // 4) 构造右键菜单
        NSMenu *menu = [[NSMenu alloc] initWithTitle:@"GoPaste"];
        [menu setAutoenablesItems:NO];

        NSMenuItem *mShow = [[NSMenuItem alloc]
            initWithTitle:@"显示主面板"
                   action:@selector(onMenuShow:)
            keyEquivalent:@""];
        mShow.target = gDelegate;
        [menu addItem:mShow];

        [menu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *mAbout = [[NSMenuItem alloc]
            initWithTitle:@"关于"
                   action:@selector(onMenuAbout:)
            keyEquivalent:@""];
        mAbout.target = gDelegate;
        [menu addItem:mAbout];

        [menu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *mRestart = [[NSMenuItem alloc]
            initWithTitle:@"重启"
                   action:@selector(onMenuRestart:)
            keyEquivalent:@""];
        mRestart.target = gDelegate;
        [menu addItem:mRestart];

        NSMenuItem *mQuit = [[NSMenuItem alloc]
            initWithTitle:@"退出"
                   action:@selector(onMenuQuit:)
            keyEquivalent:@"q"];
        mQuit.target = gDelegate;
        [menu addItem:mQuit];

        // 把右键菜单挂上去；左键点击会弹菜单（macOS 菜单栏 app 标准交互）。
        gStatusItem.menu = menu;

        gInstalled = YES;
        NSLog(@"gopaste: NSStatusItem installed");
    });
}

// GoPasteStatusItemUninstall 从菜单栏移除图标（主线程异步）。
void GoPasteStatusItemUninstall(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!gInstalled) return;
        [[NSStatusBar systemStatusBar] removeStatusItem:gStatusItem];
        gStatusItem = nil;
        gDelegate   = nil;
        gInstalled  = NO;
        NSLog(@"gopaste: NSStatusItem uninstalled");
    });
}

// GoPasteStatusItemSetIcon 动态更新图标（主线程异步）。
void GoPasteStatusItemSetIcon(const unsigned char *icon_png, int icon_len, BOOL isTemplate) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!gStatusItem) return;
        if (!icon_png || icon_len <= 0) return;
        NSData *data = [NSData dataWithBytes:icon_png length:icon_len];
        NSImage *img = [[NSImage alloc] initWithData:data];
        if (!img) return;
        [img setSize:NSMakeSize(18, 18)];
        [img setTemplate:isTemplate];
        gStatusItem.button.image = img;
    });
}
