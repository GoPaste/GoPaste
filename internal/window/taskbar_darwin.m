// taskbar_darwin.m
// 动态切换 macOS Dock 图标显隐。
//
// NSApplicationActivationPolicyRegular  → 正常应用，有 Dock 图标 + 菜单栏
// NSApplicationActivationPolicyAccessory → 附属应用，无 Dock 图标，只有菜单栏（状态栏）
//
// 注意：setActivationPolicy: 必须在主线程调用。

#import <Cocoa/Cocoa.h>

void GoPasteSetDockVisible(int visible) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (visible) {
            [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        } else {
            [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        }
    });
}
