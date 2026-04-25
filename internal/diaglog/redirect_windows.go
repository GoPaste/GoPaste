//go:build windows

package diaglog

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	setStdHandle   = kernel32.NewProc("SetStdHandle")
)

const (
	stdOutputHandle = ^uintptr(10) // STD_OUTPUT_HANDLE = -11
	stdErrorHandle  = ^uintptr(11) // STD_ERROR_HANDLE  = -12
)

// RedirectStderr 把 stderr/stdout 重定向到
// %APPDATA%\gopaste\gopaste.stderr.log。
// Windows GUI 子系统（-H windowsgui）下无控制台，panic 栈会静默丢失；
// 重定向到文件后可离线分析。
func RedirectStderr() {
	defer func() { _ = recover() }()

	root, err := os.UserConfigDir()
	if err != nil {
		root = os.TempDir()
	}
	dir := filepath.Join(root, "gopaste")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	logPath := filepath.Join(dir, "gopaste.stderr.log")

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}

	fmt.Fprintf(f, "\n========== gopaste pid=%d started at %s ==========\n",
		os.Getpid(), time.Now().Format("2006-01-02 15:04:05.000"))

	h := syscall.Handle(f.Fd())
	setStdHandle.Call(stdErrorHandle, uintptr(h))
	setStdHandle.Call(stdOutputHandle, uintptr(h))

	os.Stderr = f
	os.Stdout = f
}
