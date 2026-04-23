//go:build !darwin

package window

// 非 macOS 平台无需 NSPanel 相关处理，提供空实现保证跨平台代码一致。

// ConvertToNonactivatingPanel 在非 macOS 上是 no-op。
func ConvertToNonactivatingPanel(title string) {}

// OrderOut 在非 macOS 上是 no-op；调用方应直接用 wailsruntime.WindowHide。
func OrderOut(title string) {}

// OrderFront 在非 macOS 上是 no-op；调用方应直接用 wailsruntime.WindowShow。
func OrderFront(title string) {}

// ResignKey 在非 macOS 上是 no-op。
func ResignKey(title string) {}
