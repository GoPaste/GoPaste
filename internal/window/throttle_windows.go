//go:build windows

package window

import (
	"syscall"
	"unsafe"
)

// ===========================================================================
// Power throttling 关闭：避免长时间空闲后整个进程被 OS 冻结
// ===========================================================================
//
// 现象（已观察到多次）：
//   - GoPaste 长时间不显示窗口（30 分钟～几小时）后，热键唤起无响应
//   - 进程还在、托盘图标还在、boot.log 里能看到 hotkey goroutine 跑了
//     一两行 syscall（focus: captured prev window），但进入 togglePanel
//     执行到中间 (positionWindow / setVisible 之间) 就停住
//   - watchdog goroutine 的 5 秒超时 dump 不触发 → time.After 也不走 →
//     说明整个 Go runtime 调度器被冻结，不是单个 mutex/channel 死锁
//   - 用任务管理器强杀进程后，重启一切正常
//
// 根因：
//   Windows 11 的 Power Throttling / EcoQoS 默认会对"看起来不重要的
//   后台进程"做激进节流，长时间空闲且没有 foreground window 时，可能
//   把进程的所有线程进入 suspended 状态。GoPaste 是托盘常驻应用，
//   90% 时间没有可见窗口，正好是 OS 判断的高节流候选。
//
// 修复：
//   调用 Win32 SetProcessInformation + ProcessPowerThrottling，明确标记
//   "ExecutionSpeed 不要被节流"，等价于告诉 OS "我是一个常驻服务，请
//   保留正常调度优先级，不要因为我没 foreground 就限制我"。
//
//   只关 ExecutionSpeed (位 0)，保留 IgnoreTimerResolution 等其它策略
//   走系统默认（避免无谓提高功耗）。
//
// API 参考:
//   https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-setprocessinformation
//   https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/ns-processthreadsapi-process_power_throttling_state
//
// 兼容性：
//   - SetProcessInformation 自 Windows 8 引入；ProcessPowerThrottling
//     枚举值自 Windows 10 1709 引入。
//   - 调用失败（旧系统、被组策略禁用、被防病毒拦截等）时静默忽略，
//     不影响主流程——这只是性能优化，不是功能依赖。

const (
	// ProcessPowerThrottling 是 PROCESS_INFORMATION_CLASS 的枚举值，
	// 对应 SetProcessInformation 的第 2 个参数。
	// winnt.h: ProcessPowerThrottling = 4
	processPowerThrottling = 4

	// PROCESS_POWER_THROTTLING_CURRENT_VERSION
	processPowerThrottlingCurrentVersion = 1

	// PROCESS_POWER_THROTTLING_EXECUTION_SPEED 位 0：控制 CPU 节流。
	// 在 ControlMask 里置位表示"我要管这一项"，在 StateMask 里置位
	// 表示"启用节流"，清零表示"关闭节流"。
	// 我们要的是 (ControlMask=0x1, StateMask=0x0) → "我管 ExecutionSpeed
	// 这一项，并且关掉它（不要节流）"。
	processPowerThrottlingExecutionSpeed = 0x1
)

// processPowerThrottlingState 对应 Win32 PROCESS_POWER_THROTTLING_STATE 结构。
type processPowerThrottlingState struct {
	Version     uint32
	ControlMask uint32
	StateMask   uint32
}

// DisablePowerThrottling 关闭当前进程的 Power Throttling。
// 在 startup 阶段调用一次即可——设置随进程生命周期持续，无需重复施加。
//
// 失败时静默忽略：旧版 Windows 不支持这个 API，但也不会因此崩溃。
func DisablePowerThrottling() {
	defer func() { _ = recover() }()

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getCurrentProcess := kernel32.NewProc("GetCurrentProcess")
	setProcessInformation := kernel32.NewProc("SetProcessInformation")

	// GetCurrentProcess 返回伪句柄 -1，调用零成本。
	hProc, _, _ := getCurrentProcess.Call()

	state := processPowerThrottlingState{
		Version:     processPowerThrottlingCurrentVersion,
		ControlMask: processPowerThrottlingExecutionSpeed, // 我管 ExecutionSpeed
		StateMask:   0,                                    // 关掉节流（位 0 清零）
	}

	// SetProcessInformation(handle, ProcessInformationClass, info, size)
	setProcessInformation.Call(
		hProc,
		uintptr(processPowerThrottling),
		uintptr(unsafe.Pointer(&state)),
		unsafe.Sizeof(state),
	)
	// 返回值忽略：成功失败都不影响主流程。失败常见原因：
	//   - 系统版本太老（Win10 1709 之前）→ 忽略
	//   - 组策略禁用 → 忽略
	//   - 已被外部工具设置过 → 忽略
}
