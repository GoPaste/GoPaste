//go:build !windows

package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// RedirectStderr 把进程的 stderr（fd 2）和 stdout（fd 1）永久重定向到
// ~/Library/Application Support/gopaste/gopaste.stderr.log（macOS）或
// ~/.config/gopaste/gopaste.stderr.log（Linux）。
//
// 为什么必须做：
//   - .app 双击启动时 stderr 没人接收 → Go runtime 的 panic 栈、fatal error
//     文本、cgo abort 信息全部丢失，崩溃报告里只剩 SIGQUIT/SIGABRT 信号。
//   - macOS DiagnosticReports 因为 -ldflags "-s -w" strip 过符号表，
//     Go 函数全是 0x0，更读不出。
//
// 用 syscall.Dup2 而非 os.Stderr = f：
//   Go runtime 的 throw/fatalpanic 走的是底层 write(2,...) 即 fd 2，
//   只覆盖 os.Stderr *File 不影响 fd 2，panic 栈仍然丢失。
//
// 错误时静默跳过——不能因为日志重定向失败影响主流程。
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

	if err := syscall.Dup2(int(f.Fd()), int(os.Stderr.Fd())); err != nil {
		return
	}
	_ = syscall.Dup2(int(f.Fd()), int(os.Stdout.Fd()))

	os.Stderr = os.NewFile(uintptr(syscall.Stderr), "/dev/stderr")
	os.Stdout = os.NewFile(uintptr(syscall.Stdout), "/dev/stdout")
}
