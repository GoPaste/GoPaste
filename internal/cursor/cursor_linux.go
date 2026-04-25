//go:build linux

package cursor

import (
	"os/exec"
	"fmt"
)

func Position() (int, int) {
	// 使用 xdotool 获取鼠标位置
	out, err := exec.Command("xdotool", "getmouselocation", "--shell").Output()
	if err != nil {
		return 0, 0
	}
	var x, y int
	fmt.Sscanf(string(out), "X=%d\nY=%d\n", &x, &y)
	return x, y
}
