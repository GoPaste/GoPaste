package main

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"gopaste/internal/config"
	"gopaste/internal/platform"
	"gopaste/internal/settings"
)

//go:embed all:frontend/dist
var assets embed.FS

// bootProbe 在不依赖任何抽象层的前提下，向 ~/AppData/Roaming/gopaste/gopaste.boot.log
// 直接 append 一行。用于诊断：到底进程跑到了哪一步、是否被异常退出。
func bootProbe(stage string) {
	defer func() { _ = recover() }()
	root, err := os.UserConfigDir()
	if err != nil {
		root = os.TempDir()
	}
	dir := filepath.Join(root, "gopaste")
	_ = os.MkdirAll(dir, 0o700)
	f, err := os.OpenFile(filepath.Join(dir, "gopaste.boot.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s pid=%d\n",
		time.Now().Format("2006-01-02 15:04:05.000"), stage, os.Getpid())
}

func main() {
	bootProbe("main: enter")
	defer bootProbe("main: exit")
	app := NewApp()

	// 预加载设置以支持静默启动
	startHidden := false
	if paths, err := config.ResolvePaths(); err == nil {
		if data, err := os.ReadFile(paths.Settings); err == nil || errors.Is(err, os.ErrNotExist) {
			s := settings.Default()
			if data != nil {
				if ss, err := settings.Open(paths.Settings); err == nil {
					s = ss.Get()
				}
			}
			startHidden = s.SilentStart
		}
	}

	bootProbe(fmt.Sprintf("main: before wails.Run startHidden=%v", startHidden))
	appOpts := &options.App{
		Title:             "GoPaste",
		Width:             480,
		Height:            680,
		MinWidth:          480,
		MinHeight:         600,
		DisableResize:     false,
		StartHidden:       startHidden,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 245, G: 245, B: 245, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	}
	platform.ApplyOptions(appOpts)
	err := wails.Run(appOpts)

	if err != nil {
		bootProbe("main: wails.Run returned err=" + err.Error())
		println("Error:", err.Error())
	} else {
		bootProbe("main: wails.Run returned ok")
	}
}
