//go:build linux

package paste

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// PreviousWindow 跨平台句柄。Linux/X11 上记录 xdotool 抓到的窗口 ID（十进制字符串）。
type PreviousWindow struct {
	wid string
}

func (p PreviousWindow) IsValid() bool { return p.wid != "" }

// CapturePreviousWindow 用 xdotool getactivewindow 抓当前活动窗口 ID。
func CapturePreviousWindow() (PreviousWindow, error) {
	if _, err := exec.LookPath("xdotool"); err != nil {
		return PreviousWindow{}, fmt.Errorf("focus: xdotool not found")
	}
	out, err := exec.Command("xdotool", "getactivewindow").Output()
	if err != nil {
		return PreviousWindow{}, fmt.Errorf("focus: getactivewindow: %v", err)
	}
	return PreviousWindow{wid: strings.TrimSpace(string(out))}, nil
}

// RestorePreviousWindow 把焦点切回。
func RestorePreviousWindow(p PreviousWindow) error {
	if !p.IsValid() {
		return fmt.Errorf("focus: invalid wid")
	}
	if err := exec.Command("xdotool", "windowactivate", "--sync", p.wid).Run(); err != nil {
		return fmt.Errorf("focus: windowactivate %s: %v", p.wid, err)
	}
	time.Sleep(50 * time.Millisecond)
	return nil
}
