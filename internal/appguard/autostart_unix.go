//go:build !windows

package appguard

import (
	"os"

	"github.com/emersion/go-autostart"
)

// macOS / Linux 沿用 emersion/go-autostart（在非 windows 平台为纯 Go 实现）：
//   - macOS: ~/Library/LaunchAgents/gopaste.plist
//   - Linux: ~/.config/autostart/gopaste.desktop

func autostartApp() (*autostart.App, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &autostart.App{
		Name:        autostartName,
		DisplayName: autostartDisplayName,
		Exec:        []string{exe, autostartSilentArg},
	}, nil
}

func platformIsAutoStartEnabled() bool {
	app, err := autostartApp()
	if err != nil {
		return false
	}
	return app.IsEnabled()
}

func platformEnableAutoStart() error {
	app, err := autostartApp()
	if err != nil {
		return err
	}
	return app.Enable()
}

func platformDisableAutoStart() error {
	app, err := autostartApp()
	if err != nil {
		return err
	}
	return app.Disable()
}
