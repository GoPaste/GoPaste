package window

// PanelStatus 是 ReinforcePanel / AfterWindowCreated 的返回值，便于调用方做诊断日志。
// 放在无 build tag、无 cgo 的文件里，所有平台共用同一份定义。
type PanelStatus int

const (
	// PanelNotFound 原生窗口尚未创建（v3 在 ApplicationStarted 之后异步创建窗口），可稍后重试。
	PanelNotFound PanelStatus = 0
	// PanelOK 窗口状态符合预期（macOS 上即 NonactivatingPanel；其他平台恒为此值）。
	PanelOK PanelStatus = 1
	// PanelNotNonactivating 仅 macOS：窗口存在但不是 NonactivatingPanel，说明 v3 的
	// Panel 选项没生效，面板会抢前台、粘贴可能打回自己。只告警，不做 isa swap
	// （v3 下会破坏 WebviewPanel 的 sendEvent:/keyDown: 重写）。
	PanelNotNonactivating PanelStatus = -1
)
