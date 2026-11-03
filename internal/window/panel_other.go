//go:build !darwin

package window

// 非 macOS 平台无需 NSPanel 相关处理，提供空实现保证跨平台代码一致。
// PanelStatus 定义见 panel_status.go。

// ConfigurePanel 在非 macOS 上是 no-op。
func ConfigurePanel(title string) PanelStatus { return PanelOK }

// ActivateForDialog 在非 macOS 上是 no-op。
func ActivateForDialog() {}

// DeactivateAfterDialog 在非 macOS 上是 no-op。
func DeactivateAfterDialog() {}

// SaveFileDialog 在非 macOS 上返回空，调用方应使用 v3 的 app.Dialog.SaveFile()。
func SaveFileDialog(title, defaultName, ext string) string { return "" }

// HasNativeSaveDialog 在非 macOS 上为 false：直接用 v3 的保存对话框即可。
func HasNativeSaveDialog() bool { return false }
