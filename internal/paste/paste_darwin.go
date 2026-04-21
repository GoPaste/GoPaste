//go:build darwin

package paste

import "os/exec"

// sendPasteImpl 使用 AppleScript 触发前台应用的 Cmd+V。
// 需要用户授予"辅助功能"权限。
func sendPasteImpl() error {
	script := `tell application "System Events" to keystroke "v" using command down`
	return exec.Command("osascript", "-e", script).Run()
}
