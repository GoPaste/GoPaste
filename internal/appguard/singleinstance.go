// Package appguard 提供应用启动阶段的守护能力：
// 目前包含单实例保护。
package appguard

import (
	"os"
	"path/filepath"

	singleinstance "github.com/allan-simon/go-singleinstance"
)

// lockFile 在进程生命周期内持有，避免被 GC 关闭导致锁释放。
var lockFile *os.File

// AcquireSingleInstance 尝试获取单实例锁。
// 如果已有实例在运行，返回 false；调用方应当 exit。
// 如果本进程是第一个实例，返回 true，并持有锁到进程结束。
//
// 锁文件位于用户临时目录下的 gopaste.lock。
// 原理（跨平台，由 allan-simon/go-singleinstance 提供）：
//   - Windows: 创建独占文件（后台互斥体语义）
//   - macOS/Linux: flock 系统调用
func AcquireSingleInstance() bool {
	path := filepath.Join(os.TempDir(), "gopaste.lock")
	f, err := singleinstance.CreateLockFile(path)
	if err != nil {
		// 锁已被占用，说明另一实例在运行
		return false
	}
	lockFile = f
	return true
}

// Release 显式释放锁（通常进程退出时系统自动回收，无需主动调用）。
func Release() {
	if lockFile != nil {
		_ = lockFile.Close()
		lockFile = nil
	}
}
