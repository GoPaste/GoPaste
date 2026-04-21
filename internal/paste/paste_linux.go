//go:build linux

package paste

import (
	"os/exec"
)

// sendPasteImpl 在 X11 上优先使用 xdotool；Wayland 下一般无权限，调用失败即返回 ErrUnsupported。
func sendPasteImpl() error {
	if _, err := exec.LookPath("xdotool"); err == nil {
		return exec.Command("xdotool", "key", "ctrl+v").Run()
	}
	return ErrUnsupported
}
