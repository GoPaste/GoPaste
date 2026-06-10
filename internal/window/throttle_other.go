//go:build !windows

package window

// DisablePowerThrottling 在非 Windows 平台是空操作。
// macOS / Linux 没有等价的"按进程关闭 OS 节流"接口；macOS 的
// AppNap 由 NSProcessInfo.beginActivity 控制，但 GoPaste 是 NSPanel
// 常驻并且在 startup 时就已经活跃，AppNap 不会针对它启动。
func DisablePowerThrottling() {}
