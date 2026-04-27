//go:build windows

package appguard

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// Windows 开机自启实现：向 HKCU\Software\Microsoft\Windows\CurrentVersion\Run
// 写入字符串值。相比快捷方式 / 任务计划方案：
//   - 不需要 cgo，可纯 Go 交叉编译到 windows/amd64、windows/386、windows/arm64
//   - 不需要管理员权限（HKCU 归当前用户）
//   - 登录后由 explorer 自动执行
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// autostartCommand 生成写入注册表的命令字符串，形如：
//
//	"C:\Users\xxx\AppData\Local\Programs\GoPaste\gopaste.exe" --silent-start
//
// 可执行路径两侧加双引号，避免路径含空格时 shell 解析出错。
func autostartCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return `"` + exe + `" ` + autostartSilentArg, nil
}

func platformIsAutoStartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	if _, _, err := k.GetStringValue(autostartName); err != nil {
		return false
	}
	return true
}

func platformEnableAutoStart() error {
	cmd, err := autostartCommand()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	return k.SetStringValue(autostartName, cmd)
}

func platformDisableAutoStart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		// 键不存在视为已禁用
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	defer k.Close()

	if err := k.DeleteValue(autostartName); err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	return nil
}
