package main

import (
	"embed"
	"errors"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"gopaste/internal/config"
	"gopaste/internal/settings"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
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

	err := wails.Run(&options.App{
		Title:             "GoPaste",
		Width:             480,
		Height:            680,
		MinWidth:          380,
		MinHeight:         480,
		DisableResize:     false,
		Frameless:         true,
		StartHidden:       startHidden,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 20, G: 22, B: 28, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    true,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
