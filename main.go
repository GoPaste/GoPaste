package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"gopaste/internal/appguard"
	"gopaste/internal/config"
	"gopaste/internal/logger"
	"gopaste/internal/settings"
	"gopaste/internal/window"
)

// maxBootRetries 是开机自启（boot）模式下 WebView2 初始化/运行失败时的
// 自愈重试上限。超过后放弃自动恢复，进程正常退出（用户可手动启动）。
const maxBootRetries = 4

// flagSilentStart 由开机自启注册表项传递（"--silent-start"），用于区分
// "登录后由系统拉起"与"用户手动双击"。两者都应静默隐藏启动，但只有 boot
// 模式需要 WebView2 就绪延迟与崩溃自愈重试。
var flagSilentStart = flag.Bool("silent-start", false, "launched by system auto-start; start hidden and apply WebView2 boot-readiness retry")

// bootRetry 记录 boot 模式下的重试次数，通过 GOPASTE_BOOT_RETRY 环境变量在
// 自愈重拉起的子进程间传递。初始启动（注册表拉起）为 0，每次重拉起 +1。
var bootRetry int

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
	flag.Parse()
	bootProbe("main: enter")
	defer bootProbe("main: exit")

	// 解析 boot 模式：注册表自启携带 --silent-start；自愈重拉起的子进程通过
	// GOPASTE_BOOT_RETRY 环境变量标识（且始终静默启动）。
	silentStart := *flagSilentStart
	if v := os.Getenv("GOPASTE_BOOT_RETRY"); v != "" {
		fmt.Sscanf(v, "%d", &bootRetry)
		silentStart = true
	}
	bootMode := *flagSilentStart || bootRetry > 0

	// stderr 重定向：.app 双击启动时 stderr 默认进 console，无人接收，
	// Go runtime panic / fatal error 的栈打不到任何文件，崩溃排查只能靠
	// system DiagnosticReports（信息严重不足）。这里把 stderr/stdout 直接
	// 接到固定文件，保证以后任何 panic 都能在该文件里看到完整 Go 栈。
	// 注意：用 dup2 替换 fd2，而不是只覆盖 os.Stderr——Go runtime 写 panic
	// 的是底层 fd2（write(2,...)），不是 os.Stderr 这个 *File。
	logger.RedirectStderr()

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
				// 正常退出：v3 默认信号处理会走 Quit → ServiceShutdown；1.5s 后强退兜底。
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
	app.bootMode = bootMode
	app.bootStart = time.Now()

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
	// 开机自启路径无论设置如何都静默启动。
	if silentStart {
		startHidden = true
	}

	// 开机自启（boot）模式：登录后系统/WebView2/GPU 往往尚未就绪，
	// 此时初始化 WebView2 极易触发 "not in the correct state" 之类的失败，
	// 表现为"开机弹出报错弹窗 + 托盘图标，点击即崩"。先等待一段时间让系统
	// 稳定下来再创建窗口；每次自愈重试退避更久，提高成功率。
	if bootMode && bootRetry < maxBootRetries {
		delay := time.Duration(5+bootRetry*3) * time.Second
		bootProbe(fmt.Sprintf("main: boot readiness delay=%s retry=%d", delay, bootRetry))
		time.Sleep(delay)
	}

	// 窗口背景色：必须与 CSS :root/[data-theme] 的 --bg 保持一致，
	// 否则拉伸窗口时 WebView 未及时重绘，会露出窗口底色形成"黑边/白边"。
	// Windows/Linux 使用当前主题对应的 --bg 值；macOS 的 Panel 背景由 WebView 自绘。
	bg := application.NewRGBA(20, 22, 28, 255) // 对应 --bg: #14161c
	if theme == "light" {
		bg = application.NewRGBA(245, 245, 245, 255) // 对应 --bg: #f5f5f5
	}

	bootProbe(fmt.Sprintf("main: before app.Run startHidden=%v theme=%s", startHidden, theme))
	err := runApp(app, startHidden, bg)

	if err != nil {
		bootProbe("main: app.Run returned err=" + err.Error())
		// boot 模式下 WebView2 仍可能在就绪前失败（Run 返回错误）。
		// 触发自愈重拉起，由看门狗/重试在系统更就绪后再次尝试。
		if bootMode {
			relaunchForBoot()
		}
		println("Error:", err.Error())
	} else {
		bootProbe("main: app.Run returned ok")
	}
}

// runApp 创建 Wails v3 应用与主窗口并阻塞运行，对应 v2 的 wails.Run(&options.App{…})。
//
// v2 → v3 选项对照（详见 docs/wails-v3-migration-plan.md §3.3）：
//
//	Bind: []interface{}{app}  → Services: application.NewService(app)
//	OnStartup                 → App.ServiceStartup（数据层）+ ApplicationStarted 后的 App.startUI（UI 层）
//	OnDomReady                → 主窗口 events.Common.WindowRuntimeReady
//	OnShutdown                → App.ServiceShutdown
//	StartHidden               → WebviewWindowOptions.Hidden
//	HideWindowOnClose: true   → WindowClosing hook：取消关闭并隐藏
//	AssetServer.Assets        → Assets.Handler = AssetFileServerFS(dist)
func runApp(app *App, startHidden bool, bg application.RGBA) error {
	// embed 的根是 frontend/dist 的上级，v3 的 AssetFileServerFS 需要直接指向 dist。
	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return fmt.Errorf("frontend assets: %w", err)
	}

	appOpts := application.Options{
		Name:        "GoPaste",
		Description: "跨平台剪切板管理工具",
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(dist),
		},
		// v3 默认的 panic 处理会直接 fatal 退出。GoPaste 的约定是：bound method /
		// 事件回调里的 Go 层 panic 落盘后继续运行（与 v2 绑定调用的 recover 行为一致），
		// 真正的进程级崩溃仍由 RedirectStderr + SetTraceback("crash") 兜底记录完整栈。
		PanicHandler: func(d *application.PanicDetails) {
			bootProbe(fmt.Sprintf("PANIC (recovered by wails): %v\n%s", d.Error, d.FullStackTrace))
		},
	}
	window.ApplyAppOptions(&appOpts)
	wapp := application.New(appOpts)

	winOpts := application.WebviewWindowOptions{
		Name:             "main",
		Title:            window.Title, // panel_darwin.m / Win32 工具按标题查找主窗口
		Width:            680,
		Height:           680,
		MinWidth:         600,
		MinHeight:        600,
		Hidden:           startHidden,
		BackgroundColour: bg,
		URL:              "/",
	}
	window.ApplyOptions(&winOpts)
	win := wapp.Window.NewWithOptions(winOpts)
	app.attach(wapp, win)

	// HideWindowOnClose：Alt+F4 / Cmd+W / 系统关闭都只是隐藏面板，进程常驻托盘。
	// 用 RegisterHook（在默认处理之前执行）并 Cancel，阻止 v3 真正销毁窗口。
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		go app.HideWindow()
	})

	// OnDomReady：前端 runtime 就绪（页面重载会再次触发，domReady 内逻辑可重入）。
	win.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
		app.domReady()
	})

	// UI 层初始化必须等原生事件循环跑起来之后（见 App.ServiceStartup 注释）。
	// v3 在独立 goroutine 中派发应用事件，startUI 内部用 sync.Once 防重入。
	wapp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) {
		app.startUI()
	})

	return wapp.Run()
}

// relaunchForBoot 用于开机自启（boot）模式下的崩溃自愈：释放单实例锁，
// 携带递增的 GOPASTE_BOOT_RETRY 计数重新拉起本进程。父进程退出后由子进程
// 重新走 app.Run，在系统更就绪时（延迟随重试次数退避）完成 WebView2 初始化。
// 超过 maxBootRetries 后放弃，仅写日志，交由用户手动启动。
func relaunchForBoot() {
	if bootRetry >= maxBootRetries {
		bootProbe("main: boot retries exhausted, give up self-heal")
		return
	}
	bootProbe(fmt.Sprintf("main: relaunchForBoot retry=%d", bootRetry))
	// 先释放单实例锁，并稍候确保锁文件句柄真正关闭，避免子进程误判"已在运行"。
	appguard.Release()
	time.Sleep(300 * time.Millisecond)
	exe, err := os.Executable()
	if err != nil {
		bootProbe("main: relaunchForBoot os.Executable failed: " + err.Error())
		return
	}
	// 显式保留 --silent-start（flag.Parse 会将其从 os.Args 中消费掉），
	// 子进程据此识别为开机自启；GOPASTE_BOOT_RETRY 环境变量携带重试计数。
	args := append([]string{"--silent-start"}, os.Args[1:]...)
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("GOPASTE_BOOT_RETRY=%d", bootRetry+1))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		bootProbe("main: relaunchForBoot start failed: " + err.Error())
		return
	}
	// 子进程已拉起，父进程退出。看门狗在 app.Run 阻塞期间也可能调用本函数。
	os.Exit(0)
}
