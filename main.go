package main

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"gopaste/internal/appguard"
	"gopaste/internal/config"
	"gopaste/internal/crashlog"
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

	// stderr 重定向：.app 双击启动时 stderr 默认进 console，无人接收，
	// Go runtime panic / fatal error 的栈打不到任何文件，崩溃排查只能靠
	// system DiagnosticReports（信息严重不足）。这里把 stderr/stdout 直接
	// 接到固定文件，保证以后任何 panic 都能在该文件里看到完整 Go 栈。
	// 注意：用 dup2 替换 fd2，而不是只覆盖 os.Stderr——Go runtime 写 panic
	// 的是底层 fd2（write(2,...)），不是 os.Stderr 这个 *File。
	crashlog.RedirectStderr()

	// 静默死亡诊断：
	//  - SIGBUS/SIGSEGV/SIGILL/SIGFPE/SIGABRT：CGo / AppKit 内部断言失败、野指针、
	//    非主线程访问 UI 等都会落到这里。Go runtime 默认是 print trace 后 exit(2)。
	//  - SIGKILL：系统强杀（Activity Monitor、jetsamd、CPU wakeup 超阈等），
	//    进程收不到 —— 但如果不是 SIGKILL，以下 handler 会抢先把堆栈写进 boot log。
	//  - SIGTERM/SIGHUP/SIGINT：我们正常响应关机信号，写一行日志再 exit。
	debug.SetTraceback("crash") // 等价于 GOTRACEBACK=crash，崩溃时 dump 所有 goroutine
	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh,
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP,
		syscall.SIGABRT, syscall.SIGPIPE,
	)
	go func() {
		for s := range sigCh {
			bootProbe(fmt.Sprintf("main: SIGNAL %s", s))
			if s == syscall.SIGTERM || s == syscall.SIGINT || s == syscall.SIGHUP {
				// 正常退出，给 Wails 一点时间跑 OnShutdown。1.5s 后强退兜底。
				go func() { time.Sleep(1500 * time.Millisecond); os.Exit(130) }()
				return
			}
		}
	}()

	// 单实例保护：已有实例在运行时直接退出。
	// 后续可扩展：通过 IPC 把启动参数转发给已运行实例并请求它激活窗口。
	if !appguard.AcquireSingleInstance() {
		bootProbe("main: another instance running, exit")
		return
	}
	defer appguard.Release()

	app := NewApp()

	// 预加载设置以支持静默启动 & 窗口背景色（根据主题）
	startHidden := false
	theme := "dark" // 默认跟随 settings.Default() 的 Theme
	if paths, err := config.ResolvePaths(); err == nil {
		if data, err := os.ReadFile(paths.Settings); err == nil || errors.Is(err, os.ErrNotExist) {
			s := settings.Default()
			if data != nil {
				if ss, err := settings.Open(paths.Settings); err == nil {
					s = ss.Get()
				}
			}
			startHidden = s.SilentStart
			if s.Theme == "light" {
				theme = "light"
			}
		}
	}

	// 窗口背景色：必须与 CSS :root/[data-theme] 的 --bg 保持一致，
	// 否则拉伸窗口时 WebView 未及时重绘，会露出窗口底色形成"黑边/白边"。
	// Mac 下会被 Mac.WindowIsTranslucent=true 覆盖成透明；
	// Windows/Linux 使用当前主题对应的 --bg 值。
	var bg *options.RGBA
	if theme == "light" {
		bg = &options.RGBA{R: 245, G: 245, B: 245, A: 255} // 对应 --bg: #f5f5f5
	} else {
		bg = &options.RGBA{R: 20, G: 22, B: 28, A: 255} // 对应 --bg: #14161c
	}

	bootProbe(fmt.Sprintf("main: before wails.Run startHidden=%v theme=%s", startHidden, theme))
	appOpts := &options.App{
		Title:             "GoPaste",
		Width:             680,
		Height:            680,
		MinWidth:          450,
		MinHeight:         600,
		DisableResize:     false,
		StartHidden:       startHidden,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: bg,
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
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
