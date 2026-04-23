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

	"gopaste/internal/appguard"
	"gopaste/internal/config"
	"gopaste/internal/settings"
	"gopaste/internal/window"
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

	// 单实例保护：已有实例在运行时直接退出。
	// 后续可扩展：通过 IPC 把启动参数转发给已运行实例并请求它激活窗口。
	if !appguard.AcquireSingleInstance() {
		bootProbe("main: another instance running, exit")
		return
	}
	defer appguard.Release()

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
		// Mac 下会被 Mac.WindowIsTranslucent=true 覆盖成透明；
		// Windows/Linux 下保留原有不透明背景色。
		BackgroundColour: &options.RGBA{R: 20, G: 22, B: 28, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	}
	window.ApplyOptions(appOpts)
	err := wails.Run(appOpts)

	if err != nil {
		bootProbe("main: wails.Run returned err=" + err.Error())
		println("Error:", err.Error())
	} else {
		bootProbe("main: wails.Run returned ok")
	}
}
