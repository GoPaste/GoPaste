package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// redirectStderr 把进程的 stderr（fd 2）和 stdout（fd 1）永久重定向到
// ~/Library/Application Support/gopaste/gopaste.stderr.log。
//
// 为什么必须做：
//   - .app 双击启动时 stderr 没人接收 → Go runtime 的 panic 栈、fatal error
//     文本、cgo abort 信息全部丢失，崩溃报告里只剩 SIGQUIT/SIGABRT 信号，
//     完全没法定位真因。
//   - macOS DiagnosticReports 因为我们 -ldflags "-s -w" strip 过符号表，
//     Go 函数全是 0x0，更读不出。
//
// 为什么用 dup2 而不是 os.Stderr = f：
//   - Go runtime 的 throw / fatalpanic 写 panic 走的是底层 write(2, ...)，
//     即 fd 2，不是 os.Stderr 这个 *File。只覆盖 os.Stderr 不会改变 fd 2。
//   - syscall.Dup2 (linux) / Dup3 / 在 darwin 用 syscall.Dup2 也可，
//     这里用 syscall.Dup2 跨 unix 平台都能编译。
//
// 错误时静默跳过——不能因为日志重定向失败影响主流程。
func redirectStderr() {
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

	f, err := os.OpenFile(logPath,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	// 不 close f：fd 2/1 已经 dup 出新副本指向同一文件，关闭原 *File 不影响。

	// 写一个分隔符方便区分进程
	fmt.Fprintf(f, "\n========== gopaste pid=%d started at %s ==========\n",
		os.Getpid(), time.Now().Format("2006-01-02 15:04:05.000"))

	// dup2(f.Fd(), 2) → fd 2 现在指向日志文件
	if err := syscall.Dup2(int(f.Fd()), int(os.Stderr.Fd())); err != nil {
		return
	}
	// stdout 也接过来，省得有 println 调试信息丢了
	_ = syscall.Dup2(int(f.Fd()), int(os.Stdout.Fd()))

	// 同步 os.Stderr / os.Stdout 这两个 *File 也指向新位置——这样
	// fmt.Fprintln(os.Stderr, ...) 和直接 write(2,...) 都进同一个文件。
	// 注意：Go 内部 panic 走的就是 fd 2 直接 write，不依赖 os.Stderr，
	// 所以即便不重新赋值这两个全局，panic 也能落到日志里。重新赋值
	// 主要是让用户代码里的 fmt.Println / log.Println 也能看到。
	os.Stderr = os.NewFile(uintptr(syscall.Stderr), "/dev/stderr")
	os.Stdout = os.NewFile(uintptr(syscall.Stdout), "/dev/stdout")
}
