package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"gopaste/internal/appguard"
	"gopaste/internal/clipboard"
	"gopaste/internal/config"
	"gopaste/internal/crypto"
	"gopaste/internal/cursor"
	"gopaste/internal/hotkey"
	"gopaste/internal/paste"
	"gopaste/internal/settings"
	"gopaste/internal/storage"
	"gopaste/internal/tray"
	"gopaste/internal/types"
	"gopaste/internal/updater"
	"gopaste/internal/window"
)

// App 是 Wails 绑定的主结构体。
type App struct {
	ctx      context.Context
	log      *slog.Logger
	paths    *config.Paths
	repo     *storage.Repo
	watcher  *clipboard.Watcher
	fileWatch *clipboard.FileWatcher
	hotkey   *hotkey.Manager
	trayEnd  func()
	trayQuit bool // 本进程内是否已调用过 systray.Quit()；一旦为 true，托盘无法在本进程再启用
	settings *settings.Store

	// 窗口可见状态
	visMu         sync.Mutex
	windowVisible bool

	// 记住窗口位置
	lastX, lastY int
	posInited    bool

	// 记住"显示面板时"的前台窗口，用于粘贴时恢复焦点
	prevFocus   paste.PreviousWindow
	prevFocusMu sync.Mutex

	// 粘贴操作串行化：
	// 用户双击/回车可能触发快速重入，或前端事件抖动连击会并发调用 PasteItem；
	// 同一时刻若有两条粘贴流水线交错，会同时：
	//   1) 触碰 NSWorkspace/NSRunningApplication（AppKit 主线程断言）
	//   2) 并发 fork+exec osascript
	//   3) 并发 WindowHide / 焦点还原
	// 在 macOS 上实测会触发硬崩（进程被系统直接杀掉，不写 panic）。
	// 这里用 TryLock 保证同时只有一次 PasteItem 在跑，重入直接忽略。
	pasteMu sync.Mutex

	// macOS 辅助功能权限引导对话框去重：进程内只弹一次。
	// 详见 showAccessibilityGuide() 的注释。
	accessGuideOnce sync.Once
}

// NewApp 创建 App 实例。依赖在 startup 中初始化。
// 默认 logger 写 io.Discard（Windows GUI 子系统下 stdout 不可用，写它会失败导致日志被吞）。
func NewApp() *App {
	bootProbeApp("NewApp")
	return &App{
		log: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})),
	}
}

// bootProbeApp 转发到 main 包级 bootProbe（同包，不需要导入）。
func bootProbeApp(stage string) { bootProbe(stage) }

// startup 初始化各子系统。
func (a *App) startup(ctx context.Context) {
	bootProbeApp("startup: enter")
	a.ctx = ctx

	paths, err := config.ResolvePaths()
	if err != nil {
		bootProbeApp("startup: ResolvePaths err=" + err.Error())
		a.log.Error("resolve paths", "err", err)
		return
	}
	a.paths = paths
	bootProbeApp("startup: paths.Root=" + paths.Root)

	// 0) 初始化文件日志：~/AppData/Roaming/gopaste/gopaste.log（其它平台同理在 paths.Root 下）
	a.initFileLogger(paths.Root)
	bootProbeApp("startup: initFileLogger done")
	a.log.Info("startup", "root", paths.Root, "os", runtime.GOOS)

	// 1) 加解密
	key, err := crypto.LoadOrCreateKey(paths.Key)
	if err != nil {
		a.log.Error("load key", "err", err)
		return
	}
	cipher, err := crypto.New(key)
	if err != nil {
		a.log.Error("new cipher", "err", err)
		return
	}

	// 2) 存储
	repo, err := storage.Open(paths.DB, paths.Images, cipher)
	if err != nil {
		a.log.Error("open db", "err", err)
		return
	}
	a.repo = repo

	// 3) 设置
	ss, err := settings.Open(paths.Settings)
	if err != nil {
		a.log.Error("open settings", "err", err)
	}
	a.settings = ss

	s := settings.Default()
	if ss != nil {
		s = ss.Get()
	}

	// 若用户配置了开机自启，启动时重新写一遍 OS 自启配置，
	// 保证 Exec 路径始终指向当前 exe（应对用户移动/升级程序后路径变化）。
	if s.AutoStart {
		if err := appguard.SetAutoStart(true); err != nil {
			a.log.Warn("reapply autostart", "err", err)
		}
	}

	// 静默启动：窗口初始隐藏
	if s.SilentStart {
		a.setVisible(false)
	} else {
		a.setVisible(true)
	}

	// 4) 剪切板监听（文本 + 图片）
	// 共享抑制器：FileWatcher 检测到文件后，短时间内让文本 Watcher 跳过对应路径文本，
	// 避免同一次复制同时产生 file / text 两条历史。
	suppressor := clipboard.NewSuppressor()

	w := clipboard.New()
	w.SetSuppressor(suppressor)
	if err := w.Start(ctx); err != nil {
		a.log.Error("start clipboard watcher", "err", err)
	} else {
		a.watcher = w
		go a.consumeEvents(ctx)
	}

	// 4b) 文件剪切板监听
	fw := clipboard.NewFileWatcher()
	fw.SetSuppressor(suppressor)
	fw.Start(ctx)
	a.fileWatch = fw
	go a.consumeFileEvents(ctx)

	// 5) 快捷键
	a.registerHotkey()

	// 6) 托盘
	//
	// 策略分平台：
	//   - Windows（tray.CanToggle() == true）：始终启动 systray（消息循环常驻），
	//     如果 ShowTrayIcon=false 则在 onReady 后立即调 SetVisible(false) 隐藏图标。
	//     这样后续用户在设置页开关时只需 SetVisible(true/false)，无需重启进程。
	//   - macOS/Linux：只在 ShowTrayIcon=true 时启动。关闭后再开启走 restartApp。
	if tray.CanToggle() {
		// Windows：始终启动
		a.startTray()
		if !s.ShowTrayIcon {
			// 给 systray onReady 足够时间完成图标设置，再隐藏
			go func() {
				time.Sleep(500 * time.Millisecond)
				tray.SetVisible(false)
			}()
		}
	} else if s.ShowTrayIcon {
		a.startTray()
	}
	// 即使不显示托盘也要设置 Dock 回调
	tray.SetDockClickCallback(a.showPanel)

	// 7) 启动时按策略 prune
	go a.runPrune()

	// 8) Windows 任务栏图标显隐（需要 HWND 就绪后再应用；轮询 3 秒内生效）
	go a.applyTaskbarIconWithRetry()

	// 9) macOS：把主窗口改造成 NSPanel + NonactivatingPanel。
	// 这样"显示面板"不会抢走前台应用的 active 状态，粘贴时也就不再需要
	// hide + activate 的回环（那才是 mac 上反复闪退的根源）。
	// Wails startup 回调不保证 NSWindow 已挂到 [NSApp windows]，所以轮询。
	go a.convertMainWindowToPanelWithRetry()

	a.log.Info("GoPaste started", "data", paths.Root)
}

// shutdown 清理资源。
func (a *App) shutdown(ctx context.Context) {
	if a.hotkey != nil {
		_ = a.hotkey.Close()
	}
	if a.trayEnd != nil {
		a.trayEnd()
	}
	if a.repo != nil {
		_ = a.repo.Close()
	}
}

func (a *App) registerHotkey() {
	if a.settings == nil {
		return
	}
	s := a.settings.Get()
	if a.hotkey != nil {
		_ = a.hotkey.Close()
		a.hotkey = nil
	}
	hk, err := hotkey.New(a.ctx, s.HotkeyModifiers, s.HotkeyKey, a.togglePanel)
	if err != nil {
		a.log.Warn("hotkey register failed", "err", err, "mods", s.HotkeyModifiers, "key", s.HotkeyKey)
		return
	}
	a.hotkey = hk
}

func (a *App) runPrune() {
	if a.repo == nil || a.settings == nil {
		return
	}
	s := a.settings.Get()
	n, err := a.repo.Prune(s.MaxItems, s.MaxDays)
	if err != nil {
		a.log.Warn("prune", "err", err)
		return
	}
	if n > 0 {
		a.log.Info("pruned old items", "count", n)
	}
}

func (a *App) consumeEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-a.watcher.Events():
			if !ok {
				return
			}
			a.saveAndNotify(ctx, item)
		}
	}
}

func (a *App) consumeFileEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-a.fileWatch.Events():
			if !ok {
				return
			}
			a.saveAndNotify(ctx, item)
		}
	}
}

func (a *App) saveAndNotify(ctx context.Context, item types.Item) {
	cp := item
	if err := a.repo.Save(&cp); err != nil {
		a.log.Error("save item", "err", err)
		return
	}
	notice := cp
	notice.Content = nil
	wailsruntime.EventsEmit(ctx, "clipboard:new", notice)
}

// clampToScreen 约束窗口坐标在屏幕可见工作区内（排除任务栏等系统 UI）。
//
// 坐标单位约定（这是本项目反复翻车的地方，务必读清楚）：
//   - Windows：全部用 **物理像素**。
//     cursor.Position / cursor.WorkArea 均返回 Win32 原值（物理像素）；
//     Wails 的 `WindowSetPosition(x, y)` 内部实际是
//         SetWindowPos(hwnd, HWND_TOP, workRect.Left+x, workRect.Top+y, …)
//     其中 workRect 来自 GetMonitorInfo 也是物理像素——Wails 并未对入参做
//     DPI 缩放。所以我们把 (absX-workLeft, absY-workTop) 直接传给它即可；
//     但 WindowGetSize 内部做了 scaleToDefaultDPI，返回的是**逻辑像素**，
//     这里需要乘以 DPI 缩放才能和 cursor/工作区同单位参与 clamp。
//   - macOS/Linux：Wails / Cocoa 都用逻辑像素，scale==1，下面的逻辑保持不变。
//
// 返回：(absX, absY, workLeft, workTop)，全部是 Windows 物理像素 /
// 非 Windows 的逻辑像素。调用方再减去 work 原点得到 WindowSetPosition 入参。
func (a *App) clampToScreen(x, y int) (int, int, int, int) {
	// 窗口尺寸：Wails 返回逻辑像素，Windows 下乘以鼠标所在显示器的 DPI scale
	// 换算成物理像素，才能和 workArea / cursor 同单位比较。
	w, h := wailsruntime.WindowGetSize(a.ctx)
	scale := cursor.ScaleForPoint(x, y)
	if scale <= 0 {
		scale = 1
	}
	w = int(float64(w) * scale)
	h = int(float64(h) * scale)

	// 优先使用系统工作区（Windows 会排除任务栏）
	var sx, sy, sw, sh int
	wx, wy, ww, wh := cursor.WorkArea()
	if ww > 0 && wh > 0 {
		sx, sy, sw, sh = wx, wy, ww, wh
	} else {
		// fallback：用 Wails 返回的屏幕总尺寸（其它平台）
		screens, err := wailsruntime.ScreenGetAll(a.ctx)
		if err != nil || len(screens) == 0 {
			return x, y, 0, 0
		}
		for _, sc := range screens {
			if sc.IsCurrent {
				sw, sh = sc.Size.Width, sc.Size.Height
				break
			}
		}
		if sw == 0 {
			sw, sh = screens[0].Size.Width, screens[0].Size.Height
		}
	}

	// 留出安全边距（物理像素），避免窗口阴影/边框被屏幕或任务栏切到。
	margin := int(8 * scale)
	minX := sx + margin
	minY := sy + margin
	maxX := sx + sw - w - margin
	maxY := sy + sh - h - margin
	if maxX < minX {
		maxX = minX
	}
	if maxY < minY {
		maxY = minY
	}
	if x < minX {
		x = minX
	}
	if y < minY {
		y = minY
	}
	if x > maxX {
		x = maxX
	}
	if y > maxY {
		y = maxY
	}
	return x, y, sx, sy
}

// setWindowPosAbsolute 以「绝对屏幕坐标」方式设置窗口位置。
// Windows 下 absX/absY 应为物理像素；其它平台为逻辑像素。
// 内部会减去工作区原点得到 Wails WindowSetPosition 所需的「相对工作区」坐标。
func (a *App) setWindowPosAbsolute(absX, absY int) {
	x, y, workLeft, workTop := a.clampToScreen(absX, absY)
	wailsruntime.WindowSetPosition(a.ctx, x-workLeft, y-workTop)
}

// positionWindow 根据设置决定窗口出现位置。
func (a *App) positionWindow() {
	s := settings.Default()
	if a.settings != nil {
		s = a.settings.Get()
	}
	switch s.WindowPosition {
	case "follow":
		// 跟随鼠标：窗口左上角对齐鼠标位置
		mx, my := cursor.Position()
		if mx > 0 || my > 0 {
			a.setWindowPosAbsolute(mx, my)
		} else {
			wailsruntime.WindowCenter(a.ctx)
		}
	case "remember":
		// 记住位置：恢复上次保存的坐标
		if a.posInited {
			a.setWindowPosAbsolute(a.lastX, a.lastY)
		} else {
			wailsruntime.WindowCenter(a.ctx)
		}
	default: // "center"
		wailsruntime.WindowCenter(a.ctx)
	}
}

// saveWindowPosition 保存当前窗口位置。
func (a *App) saveWindowPosition() {
	if a.ctx == nil {
		return
	}
	x, y := wailsruntime.WindowGetPosition(a.ctx)
	a.lastX = x
	a.lastY = y
	a.posInited = true
}

// togglePanel 切换主窗口的可见状态：若已显示则隐藏，否则显示并置顶。
// 由托盘左键点击和全局快捷键共享。
func (a *App) togglePanel() {
	if a.ctx == nil {
		return
	}
	if wailsruntime.WindowIsMinimised(a.ctx) {
		a.captureFocusBeforeShow()
		wailsruntime.WindowUnminimise(a.ctx)
		window.ShowMain(a.ctx)
		a.positionWindow()
		a.setVisible(true)
		wailsruntime.EventsEmit(a.ctx, "window:show")
		return
	}
	a.visMu.Lock()
	visible := a.windowVisible
	a.visMu.Unlock()

	if visible {
		a.saveWindowPosition()
		window.HideMain(a.ctx)
		a.setVisible(false)
	} else {
		a.captureFocusBeforeShow()
		window.ShowMain(a.ctx)
		a.positionWindow()
		a.setVisible(true)
		// 通知前端窗口已显示，触发"激活时回到顶部 / 切换至全部分组"等逻辑。
		// Windows 上 visibilitychange 不一定由 WindowShow 触发，所以需要显式 emit。
		wailsruntime.EventsEmit(a.ctx, "window:show")
	}
}

// showPanel 无条件显示窗口（用于 Dock 图标点击）。
func (a *App) showPanel() {
	if a.ctx == nil {
		return
	}
	if wailsruntime.WindowIsMinimised(a.ctx) {
		wailsruntime.WindowUnminimise(a.ctx)
	}
	a.captureFocusBeforeShow()
	window.ShowMain(a.ctx)
	a.positionWindow()
	a.setVisible(true)
	wailsruntime.EventsEmit(a.ctx, "window:show")
}

// captureFocusBeforeShow 在窗口显示前抓住当前前台窗口，便于稍后粘贴时还原焦点。
// 必须在 WindowShow 之前调用，否则抓到的就是本应用自己。
//
// macOS 下 ****不需要也不应该**** 做 capture/restore：主窗口已被改造成
// NonactivatingPanel，显示面板不会抢走前台应用的 active 状态，所以"前一
// 个窗口"从未丢过焦点，restore 反而是多余的 AppKit 调用、曾是闪退来源。
func (a *App) captureFocusBeforeShow() {
	if runtime.GOOS == "darwin" {
		return
	}
	pw, err := paste.CapturePreviousWindow()
	if err != nil {
		a.log.Warn("focus: capture failed", "err", err)
		return
	}
	a.prevFocusMu.Lock()
	a.prevFocus = pw
	a.prevFocusMu.Unlock()
	bootProbeApp(fmt.Sprintf("focus: captured prev window valid=%v", pw.IsValid()))
}

// takePrevFocus 取出并清空记录的前一个窗口。
func (a *App) takePrevFocus() paste.PreviousWindow {
	a.prevFocusMu.Lock()
	defer a.prevFocusMu.Unlock()
	pw := a.prevFocus
	a.prevFocus = paste.PreviousWindow{}
	return pw
}

func (a *App) setVisible(v bool) {
	a.visMu.Lock()
	a.windowVisible = v
	a.visMu.Unlock()
}

func (a *App) isVisible() bool {
	a.visMu.Lock()
	defer a.visMu.Unlock()
	return a.windowVisible
}

// convertMainWindowToPanelWithRetry 在 macOS 启动阶段把 NSWindow 改造成
// NonactivatingPanel。调用是幂等的，我们只要在 NSWindow 已经被 AppKit
// 注册到 [NSApp windows] 之后做一次即可。
// 其他平台下 window.ConvertToNonactivatingPanel 是 no-op，这里整个函数
// 也是早退，不做任何事。
func (a *App) convertMainWindowToPanelWithRetry() {
	if runtime.GOOS != "darwin" {
		return
	}
	// Wails 创建 NSWindow 的时机略晚于 OnStartup 回调——startup 刚调用时
	// [NSApp windows] 可能还没收录主窗口。给 3 秒轮询足够宽裕，正常情况
	// 下首次尝试就能命中。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		// ConvertToNonactivatingPanel 内部自己找窗口；没找到是 NSLog 警告，
		// 不抛错。这里我们靠"主窗口已经存在"这一外部事实来判断是否成功，
		// 简单起见固定等 300ms 再做一次，幂等不会出错。
		window.ConvertToNonactivatingPanel(window.Title)
		time.Sleep(300 * time.Millisecond)
		// 做第二次兜底（覆盖 Wails 延迟创建窗口的情形），然后退出。
		window.ConvertToNonactivatingPanel(window.Title)
		return
	}
}

// applyTaskbarIconWithRetry 在启动阶段窗口 HWND 就绪前轮询，找到后应用任务栏显隐设置。
// Windows 专用；其他平台 window.FindMainWindow 返回 0，直接退出。
func (a *App) applyTaskbarIconWithRetry() {
	if runtime.GOOS != "windows" {
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if hwnd := window.FindMainWindow("GoPaste"); hwnd != 0 {
			a.applyTaskbarIcon(hwnd)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	a.log.Warn("applyTaskbarIcon: HWND not found within 5s")
}

// applyTaskbarIcon 按当前设置把主窗口从任务栏显/隐。
func (a *App) applyTaskbarIcon(hwnd uintptr) {
	if runtime.GOOS != "windows" {
		return
	}
	if hwnd == 0 {
		hwnd = window.FindMainWindow("GoPaste")
	}
	if hwnd == 0 {
		return
	}
	s := settings.Default()
	if a.settings != nil {
		s = a.settings.Get()
	}
	window.SetTaskbarVisible(hwnd, s.ShowTaskbarIcon)
	bootProbeApp(fmt.Sprintf("taskbar: applied visible=%v", s.ShowTaskbarIcon))
}

// initFileLogger 把日志写到 <root>/gopaste.log。
// Windows GUI 子系统下 os.Stdout 不可用，因此只写文件；如果文件无法打开，则把
// 错误信息回写到一个保底文本，方便定位"日志为何空的"问题。
func (a *App) initFileLogger(root string) {
	if root == "" {
		return
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		// 写保底文件到临时目录，便于排错
		_ = os.WriteFile(filepath.Join(os.TempDir(), "gopaste.boot.log"),
			[]byte(fmt.Sprintf("mkdir %q failed: %v\n", root, err)), 0o600)
		return
	}
	logPath := filepath.Join(root, "gopaste.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0o600)
	if err != nil {
		_ = os.WriteFile(filepath.Join(root, "gopaste.boot.log"),
			[]byte(fmt.Sprintf("open log %q failed: %v\n", logPath, err)), 0o600)
		return
	}
	a.log = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// startTray 启动系统托盘。
//
// 注意：fyne/systray 内部用 sync.Once 保护 Quit，一个进程只能关一次；
// 关闭之后再调用 Start 不会真正弹出图标，此时 a.trayQuit=true，需要重启进程。
//
// Windows 上 systray 在 startup 时无条件启动（消息循环常驻），后续通过
// tray.SetVisible() 平滑切换图标显隐，不会走 stopTray。
func (a *App) startTray() {
	if a.trayEnd != nil {
		return // 已启动
	}
	if a.trayQuit {
		a.log.Warn("tray: cannot re-enable after quit; restart required")
		return
	}
	a.trayEnd = tray.Start(tray.Callbacks{
		OnShow:    a.togglePanel,
		OnAbout:   a.showAbout,
		OnRestart: a.restartApp,
		OnQuit:    a.quitApp,
	})
}

// stopTray 关闭系统托盘。
// 仅在 macOS/Linux 上使用（Windows 上走 tray.SetVisible(false) 路径）。
func (a *App) stopTray() {
	if a.trayEnd != nil {
		a.trayEnd()
		a.trayEnd = nil
		a.trayQuit = true
	}
}

// ---------------- RPC 方法 ----------------

// ListItems 查询剪切板历史。
func (a *App) ListItems(q types.SearchQuery) (*types.ListResult, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("storage not ready")
	}
	return a.repo.List(q)
}

// GetContent 返回指定条目的明文内容（base64 编码）。
func (a *App) GetContent(id int64) (string, error) {
	if a.repo == nil {
		return "", fmt.Errorf("storage not ready")
	}
	b, err := a.repo.GetContent(id)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// CopyToClipboard 把指定条目写回剪切板（不模拟粘贴）。
func (a *App) CopyToClipboard(id int64) error {
	if a.repo == nil {
		return fmt.Errorf("storage not ready")
	}
	t, content, err := a.repo.GetItemWithContent(id)
	if err != nil {
		return err
	}
	return paste.WriteClipboard(t, content)
}

// PasteItem 写回剪切板并粘贴到前台窗口。
// 流程：写剪贴板 → 隐藏窗口 → 等焦点切换 → 发送 Shift+Insert(Win) / Cmd+V(Mac) / Ctrl+V(Linux)
// 是否由单击还是双击触发，由前端根据 PasteTrigger 设置绑定事件决定。
func (a *App) PasteItem(id int64) error {
	// 防重入：用户双击或 UI 抖动可能连触两次 PasteItem。
	// 第二次若在第一次还没走完时挤进来，macOS 上会叠加 fork osascript +
	// AppKit 跨线程调用，触发进程被系统直接 kill（无 crash log）。
	if !a.pasteMu.TryLock() {
		bootProbeApp(fmt.Sprintf("PasteItem: skip reentrant id=%d", id))
		a.log.Info("paste: skip reentrant", "id", id)
		return nil
	}
	defer a.pasteMu.Unlock()

	// 兜底：Go 层任何 panic 都在这里被吞掉并落盘，避免进程直接退出而我们看不到原因。
	defer func() {
		if r := recover(); r != nil {
			bootProbeApp(fmt.Sprintf("PasteItem: PANIC id=%d r=%v", id, r))
			a.log.Error("paste: panic", "id", id, "recover", r)
		}
	}()

	bootProbeApp(fmt.Sprintf("PasteItem: enter id=%d", id))
	a.log.Info("paste: enter", "id", id)

	// macOS：辅助功能权限预检。
	// CGEventPost 注入 Cmd+V 需要 Accessibility 授权。之前的实现把检查放
	// 在 SendPaste 里——那已经太晚了：此时面板已经 HideMain 了，用户看到
	// 的现象是"点击历史项 → 面板消失 → 原应用里什么都没贴上"，不知道
	// 发生了什么。**这才是用户报的"闪退"真相**——面板消失 ≠ 进程退出。
	//
	// 现在改成"进入 PasteItem 就预检"：
	//   1) PromptAccessibility() 首次调用会弹系统权限框（幂等，进程内只弹一次）
	//   2) HasAccessibility() 没通过 → 弹 Wails 对话框引导用户到系统设置
	//      → **不 HideMain、不 SendPaste、不改剪贴板** → 下次用户授权后重试即可
	//
	// 这个检查在所有平台都会跑，但只有 darwin 会真的拦住（其他平台
	// HasAccessibility 恒为 true）。
	if runtime.GOOS == "darwin" {
		paste.PromptAccessibility() // 首次会弹系统框；已授权则是 no-op
		if !paste.HasAccessibility() {
			bootProbeApp("PasteItem: blocked by accessibility")
			a.log.Warn("paste: blocked by accessibility permission")
			go a.showAccessibilityGuide() // 异步，不阻塞 RPC 返回
			return paste.ErrNoAccessibility
		}
	}

	if err := a.CopyToClipboard(id); err != nil {
		bootProbeApp("PasteItem: copy err=" + err.Error())
		a.log.Warn("paste: copy to clipboard failed", "id", id, "err", err)
		return err
	}
	s := settings.Default()
	if a.settings != nil {
		s = a.settings.Get()
	}
	bootProbeApp(fmt.Sprintf("PasteItem: ready trigger=%s", s.PasteTrigger))
	a.log.Info("paste: ready", "id", id, "pasteTrigger", s.PasteTrigger)

	// 取出展示前抓到的"前一个窗口"（macOS 下永远是零值，captureFocusBeforeShow
	// 在 mac 下短路）。
	prev := a.takePrevFocus()
	prevValid := prev.IsValid()
	bootProbeApp(fmt.Sprintf("PasteItem: prevFocus valid=%v", prevValid))

	// 隐藏面板 + 切回焦点，分平台走不同流程。
	if runtime.GOOS == "darwin" {
		// macOS：最终方案（2026-04-23 第 3 次迭代）。
		//
		// 历史踩坑轨迹（务必保留这段，否则后人还会再翻一遍）：
		//
		//   v1（CGEventPost + orderOut）
		//     - orderOut 后面板在 AppKit 里仍是 keyWindow；
		//     - CGEventPost 盲注 Cmd+V，落回自己 WebView；
		//     - 前端被触发 → 递归调 PasteItem → HID flood → SIGKILL。
		//     - 表现：粘几次后无声闪退，无 crash report，无 log show 事件。
		//
		//   v2（osascript + resignKey，面板不隐藏）
		//     - 想照抄 EcoPaste "只 resign 不 hide"。
		//     - 结果：NSPanel+NonactivatingPanel 上 resignKeyWindow 基本是 no-op
		//       ——面板仍在屏幕上、仍是 frontmost process、仍是 key window。
		//     - osascript 的 keystroke 通过 System Events 查 frontmost process
		//       得到的是 GoPaste 自己，Cmd+V 仍然打回 WebView。
		//     - 表现：同 v1，依然闪退。日志止步于 "sent"。
		//
		//   v3（当前）—— osascript + orderOut 先行
		//     关键洞察：System Events 的 `keystroke "v" using command down` 是
		//     查询"谁是当前 frontmost process"后定向投递。只要面板**不是 frontmost**，
		//     就不会打回自己。最干脆的保证办法就是 orderOut: 把面板从屏幕上
		//     移走——此时 AppKit 自动把 frontmost 交给下一个可见 app。
		//
		//     为什么 v3 的 orderOut 不会重蹈 v1 覆辙：
		//       v1 翻车是因为 CGEventPost 是盲注 HID，谁是 key 谁收。
		//       v3 用的 osascript/System Events 走 AppleEvent，System Events
		//       自己的 AXUIElement 逻辑会挑"frontmost process"（面板已 orderOut
		//       → 不算 frontmost），键事件定向投递到目标 app，不会回落到 GoPaste。
		//
		//     流程：orderOut → 120ms sleep → osascript Cmd+V。不需要 resign、
		//     不需要异步再 hide、不需要追踪 prevFocus。
		if a.ctx != nil && a.isVisible() {
			a.saveWindowPosition()
			bootProbeApp("PasteItem: before HideMain (mac/panel)")
			window.HideMain(a.ctx)
			a.setVisible(false)
			bootProbeApp("PasteItem: after HideMain (mac/panel)")
		} else {
			bootProbeApp("PasteItem: skip HideMain (already hidden)")
		}
		// 120ms：给 AppKit 的 orderOut 走完 + WindowServer 更新 frontmost
		// 进程排序 + 目标 app 的 windowDidBecomeKey 回调链。
		// 经验值：40ms 时 osascript 偶尔仍把键发到 Dock/背景；80ms 稳定；
		// 120ms 留余量（对用户不可感知）。
		time.Sleep(120 * time.Millisecond)
	} else {
		// Windows / Linux：WindowHide 后焦点不会自动回到上一个窗口，
		// 必须显式 SetForegroundWindow / xdotool activate，否则
		// Shift+Insert（Windows）/ Ctrl+V（Linux）发到桌面，永远粘不出来。
		if a.ctx != nil && a.isVisible() {
			a.saveWindowPosition()
			bootProbeApp("PasteItem: before WindowHide")
			wailsruntime.WindowHide(a.ctx)
			bootProbeApp("PasteItem: after WindowHide")
			a.setVisible(false)
		} else {
			bootProbeApp("PasteItem: skip WindowHide (already hidden)")
		}
		if prevValid {
			// 给系统一点时间处理 Hide
			time.Sleep(80 * time.Millisecond)
			bootProbeApp("PasteItem: before RestorePreviousWindow")
			err := paste.RestorePreviousWindow(prev)
			bootProbeApp("PasteItem: after RestorePreviousWindow")
			if err != nil {
				bootProbeApp("PasteItem: restore focus err=" + err.Error())
				a.log.Warn("paste: restore focus failed", "err", err)
			} else {
				bootProbeApp("PasteItem: focus restored")
			}
		} else {
			// 没抓到（首次 / 异常）只能盲目等焦点漂回去
			time.Sleep(150 * time.Millisecond)
		}
	}

	bootProbeApp("PasteItem: before SendPaste")
	if err := paste.SendPaste(); err != nil {
		bootProbeApp("PasteItem: SendPaste err=" + err.Error())
		a.log.Warn("paste: send paste failed", "err", err)
		return err
	}
	bootProbeApp(fmt.Sprintf("PasteItem: sent id=%d", id))
	a.log.Info("paste: sent", "id", id)
	// mac 分支面板已在 SendPaste 之前 orderOut，这里无需再 hide。
	// 非 mac 分支原本就是 WindowHide 先行。
	return nil
}

// DeleteItem 删除一条记录。
func (a *App) DeleteItem(id int64) error {
	if a.repo == nil {
		return fmt.Errorf("storage not ready")
	}
	return a.repo.Delete(id)
}

// TogglePin 切换置顶。
func (a *App) TogglePin(id int64, pinned bool) error {
	if a.repo == nil {
		return fmt.Errorf("storage not ready")
	}
	return a.repo.SetPinned(id, pinned)
}

// ToggleFavorite 切换收藏。
func (a *App) ToggleFavorite(id int64, favorite bool) error {
	if a.repo == nil {
		return fmt.Errorf("storage not ready")
	}
	return a.repo.SetFavorite(id, favorite)
}

// SetNote 设置/更新条目的备注。传空字符串清除备注。
func (a *App) SetNote(id int64, note string) error {
	if a.repo == nil {
		return fmt.Errorf("storage not ready")
	}
	return a.repo.SetNote(id, note)
}

// ClearHistory 清空非收藏非置顶的历史。
func (a *App) ClearHistory() error {
	if a.repo == nil {
		return fmt.Errorf("storage not ready")
	}
	return a.repo.Clear()
}

// HideWindow 隐藏窗口。
func (a *App) HideWindow() {
	if a.ctx != nil {
		a.saveWindowPosition()
		window.HideMain(a.ctx)
		a.setVisible(false)
	}
}

// QuitApp 完全退出应用（供前端标题栏退出按钮调用）。
func (a *App) QuitApp() {
	a.quitApp()
}

// GetSettings 返回当前偏好设置。
func (a *App) GetSettings() settings.Settings {
	if a.settings == nil {
		return settings.Default()
	}
	return a.settings.Get()
}

// UpdateSettings 更新偏好并重新注册快捷键。
func (a *App) UpdateSettings(ns settings.Settings) error {
	if a.settings == nil {
		return fmt.Errorf("settings not ready")
	}
	prev := a.settings.Get()
	if err := a.settings.Set(ns); err != nil {
		return err
	}
	a.registerHotkey()
	go a.runPrune()

	// Windows 任务栏图标实时生效
	a.applyTaskbarIcon(0)

	// 托盘图标实时生效：
	//
	// Windows（tray.CanToggle() == true）：
	//   systray 消息循环在 startup 时就已启动并常驻，这里只需调
	//   tray.SetVisible(true/false) 来添加/删除通知区域图标，无需
	//   重启进程。等效于 Tauri 的 tray_icon.set_visible()。
	//
	// macOS/Linux（tray.CanToggle() == false）：
	//   fyne.io/systray 未暴露隐藏 API，且 Quit() 受 sync.Once 限制。
	//   关闭后再开启仍走 restartApp 兜底（和之前一样）。
	if prev.ShowTrayIcon != ns.ShowTrayIcon {
		if tray.CanToggle() {
			// Windows：平滑切换，无需重启
			tray.SetVisible(ns.ShowTrayIcon)
		} else if ns.ShowTrayIcon {
			if a.trayQuit {
				// macOS/Linux：sync.Once 门已消耗，自动重启
				a.log.Info("tray: re-enable after quit, auto-restarting")
				go a.restartApp()
				return nil
			}
			a.startTray()
		} else {
			a.stopTray()
		}
	}

	// 开机自启实时同步：仅在状态变化时写 OS 自启配置，避免重复 I/O。
	if prev.AutoStart != ns.AutoStart {
		if err := appguard.SetAutoStart(ns.AutoStart); err != nil {
			a.log.Error("set autostart", "enabled", ns.AutoStart, "err", err)
			// 不中断 UpdateSettings，只记录日志
		}
	}

	return nil
}

// TrayNeedsRestart 返回是否需要重启才能重新显示托盘图标。
// Deprecated: v0.2+ 中 UpdateSettings 会在需要时自动重启，前端无需再调用此方法。
// 保留此方法以兼容旧前端。
func (a *App) TrayNeedsRestart() bool {
	return a.trayQuit
}

// ExportData 导出所有历史为 JSON 字符串（不含图片二进制；仅元数据）。
func (a *App) ExportData() (string, error) {
	if a.repo == nil {
		return "", fmt.Errorf("storage not ready")
	}
	all, err := a.repo.List(types.SearchQuery{PageSize: 100000})
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(all.Items, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DataDir 返回用户数据目录（前端设置页展示）。
func (a *App) DataDir() string {
	if a.paths == nil {
		return ""
	}
	return a.paths.Root
}

// GetFileThumbnail 读取文件路径（从文件类型条目的 Content 中），
// 如果是图片文件则返回 base64 编码的内容，否则返回空字符串。
func (a *App) GetFileThumbnail(id int64) (string, error) {
	if a.repo == nil {
		return "", fmt.Errorf("storage not ready")
	}
	_, content, err := a.repo.GetItemWithContent(id)
	if err != nil {
		return "", err
	}
	paths := strings.Split(string(content), "\n")
	if len(paths) != 1 {
		return "", nil // 多文件不预览
	}
	p := strings.TrimSpace(paths[0])
	ext := strings.ToLower(filepath.Ext(p))
	imageExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".bmp": true, ".webp": true, ".ico": true,
	}
	if !imageExts[ext] {
		return "", nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", nil // 文件可能已删除
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// openDataDir 在系统文件管理器中打开数据目录（托盘菜单触发）。
func (a *App) openDataDir() {
	if a.paths == nil {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", a.paths.Root)
	case "darwin":
		cmd = exec.Command("open", a.paths.Root)
	default:
		cmd = exec.Command("xdg-open", a.paths.Root)
	}
	if err := cmd.Start(); err != nil {
		a.log.Warn("open data dir", "err", err)
	}
}

// showAbout 弹出关于对话框。
func (a *App) showAbout() {
	if a.ctx == nil {
		return
	}
	_, _ = wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
		Type:    wailsruntime.InfoDialog,
		Title:   "关于 GoPaste",
		Message: "GoPaste · 跨平台剪切板管理工具\n\n基于 Wails + Go + Vue 3 构建。\n数据本地加密存储，永不上云。",
		Buttons: []string{"确定"},
	})
}

// showAccessibilityGuide 在 macOS 未授权辅助功能时弹框引导用户授权。
// 点击"打开系统设置"会直达"隐私与安全 → 辅助功能"面板。
//
// 这个函数被 PasteItem 在 early-exit 时 goroutine 启动——不能在 RPC 调用栈
// 内直接弹（MessageDialog 是 modal，会阻塞 Wails 的 RPC 处理，前端调用看起来
// 就像"hang 住了"）。异步起 goroutine，RPC 立即带 ErrNoAccessibility 返回，
// 前端可以在 toast 里展示错误、用户看到系统弹框去授权即可。
//
// 进程内只弹一次引导框：一旦弹过就靠 promptOnce（系统层 prompt=YES 弹框），
// 以及我们自己的 accessGuideOnce（应用层引导弹框），都只执行一次，避免
// 用户每次按粘贴都被两层弹框轰炸。
func (a *App) showAccessibilityGuide() {
	if runtime.GOOS != "darwin" || a.ctx == nil {
		return
	}
	a.accessGuideOnce.Do(func() {
		// 文案重点：用户最常踩的坑是"明明列表里 GoPaste 的开关是开的，
		// 程序却一直报未授权"。这是因为 GoPaste 当前用的是 **ad-hoc 签名**
		// （`codesign -dvvv` 能看到 `Signature=adhoc` / `TeamIdentifier=not set`），
		// macOS 的 TCC 数据库按代码签名的 CDHash 记录授权——每次重新 `wails
		// build` 产生的二进制 CDHash 和上次可能不一样，于是 TCC 里那条旧记录
		// 的"签名要求"（csreq）就和当前进程对不上，系统内部判定为"未授权"，
		// 但列表 UI 是按 bundle id 显示的，开关仍然"亮"。
		//
		// 唯一稳定的解决办法：让用户**把列表里的 GoPaste 条目删掉再重新授权**，
		// 这样 TCC 会按当前 binary 的 CDHash 重建记录。
		//
		// 长期方案是上 Apple 开发者证书做正式签名（TeamID 稳定 = TCC 永远对得上），
		// 但这不是一次对话能解决的事，先保证用户能用。
		sel, err := wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
			Type:  wailsruntime.WarningDialog,
			Title: "需要重新授权辅助功能",
			Message: "GoPaste 需要「辅助功能」权限才能模拟 Cmd+V 完成自动粘贴。\n\n" +
				"⚠️ 如果你已经在列表里勾选过 GoPaste，仍然看到此提示，\n" +
				"这是因为 GoPaste 每次重新构建后签名会变化，系统之前保存的\n" +
				"授权记录和当前版本对不上了。\n\n" +
				"【解决方法】\n" +
				"1. 点击下方「打开系统设置」\n" +
				"2. 在「辅助功能」列表里找到 GoPaste，选中后点 −（减号）删除\n" +
				"3. 回到 GoPaste 再按一次粘贴，系统会重新弹框请求授权\n" +
				"4. 这次勾选后即可正常使用（直到下次重新构建）",
			Buttons:       []string{"打开系统设置", "稍后"},
			DefaultButton: "打开系统设置",
			CancelButton:  "稍后",
		})
		if err != nil {
			a.log.Warn("accessibility guide: dialog err", "err", err)
			return
		}
		if sel == "打开系统设置" {
			// x-apple.systempreferences URL scheme 直达"辅助功能"面板。
			// 适用 macOS 13+（Ventura 起）和旧版的 Security & Privacy。
			wailsruntime.BrowserOpenURL(a.ctx,
				"x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")
		}
	})
}

// HasPastePermission 前端 RPC：查询当前是否具备模拟粘贴所需权限。
// mac 下反映 Accessibility 授权状态；其他平台恒 true。
// 前端可在设置页显示权限指示灯，或在"自动粘贴"开关旁提示未授权。
func (a *App) HasPastePermission() bool {
	return paste.HasAccessibility()
}

// RequestPastePermission 前端 RPC：主动触发一次系统权限弹框。
// 用户在设置页点击"授予权限"按钮时调用。已授权则 no-op。
func (a *App) RequestPastePermission() bool {
	return paste.PromptAccessibility()
}

// quitApp 托盘点击"退出"时调用：走正常的 Wails 关闭流程，若一段时间内未退出则强制 os.Exit。
func (a *App) quitApp() {
	if a.ctx != nil {
		go wailsruntime.Quit(a.ctx)
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		a.log.Warn("force exit after timeout")
		os.Exit(0)
	}()
}

// restartApp 重启应用：启动自身的新实例，然后退出当前进程。
func (a *App) restartApp() {
	exe, err := os.Executable()
	if err != nil {
		a.log.Error("restart: get executable", "err", err)
		return
	}
	cmd := exec.Command(exe)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		a.log.Error("restart: start new process", "err", err)
		return
	}
	a.log.Info("restarting GoPaste", "new_pid", cmd.Process.Pid)
	a.quitApp()
}

// RevealInExplorer 在系统文件管理器中定位并选中文件。
func (a *App) RevealInExplorer(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", "/select,", path)
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	default:
		// Linux: 打开所在目录
		dir := filepath.Dir(path)
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}

// OpenURL 使用系统默认浏览器打开 URL。
// 直接调用 Wails runtime 的 BrowserOpenURL，屏蔽平台差异。
func (a *App) OpenURL(url string) error {
	if a.ctx == nil {
		return fmt.Errorf("not ready")
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("empty url")
	}
	// 如果缺少协议头，默认补 https://
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}
	wailsruntime.BrowserOpenURL(a.ctx, url)
	return nil
}

// GetAppVersion 返回当前应用版本（semver 字符串，形如 "0.1.0"）。
func (a *App) GetAppVersion() string {
	return updater.Version
}

// CheckForUpdate 检查 GitHub Releases 是否有更新。
// 返回结构体中 HasUpdate 为 true 时，前端可展示"新版本可用"提示，
// 并引导用户点击 ReleaseURL 跳转下载。检测失败（无网络等）返回空结果而非错误，
// 避免弹错误弹窗；真实错误记录到日志。
func (a *App) CheckForUpdate() updater.Result {
	if a.ctx == nil {
		return updater.Result{CurrentVersion: updater.Version}
	}
	res, err := updater.Check(a.ctx, updater.Version)
	if err != nil {
		a.log.Warn("check for update", "err", err)
		return updater.Result{CurrentVersion: updater.Version}
	}
	return res
}

// SaveImageToFile 将图片内容保存到用户选择的位置（通过系统保存对话框）。
func (a *App) SaveImageToFile(id int64) (string, error) {
	if a.repo == nil || a.ctx == nil {
		return "", fmt.Errorf("not ready")
	}
	t, content, err := a.repo.GetItemWithContent(id)
	if err != nil {
		return "", err
	}
	if t != types.TypeImage {
		return "", fmt.Errorf("not an image")
	}

	savePath, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "保存图片",
		DefaultFilename: fmt.Sprintf("gopaste_%d.png", id),
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "PNG 图片", Pattern: "*.png"},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	if savePath == "" {
		return "", nil // 用户取消
	}
	if err := os.WriteFile(savePath, content, 0o644); err != nil {
		return "", err
	}
	return savePath, nil
}

// OpenImageExternal 将图片写入临时文件，用系统默认图片查看器打开（原始尺寸）。
// 支持 image 类型和 file 类型（单个图片文件直接打开源文件）。
func (a *App) OpenImageExternal(id int64) error {
	if a.repo == nil {
		return fmt.Errorf("storage not ready")
	}
	t, content, err := a.repo.GetItemWithContent(id)
	if err != nil {
		return err
	}

	var target string

	if t == types.TypeFile {
		// 文件类型：直接打开源文件路径
		paths := strings.Split(string(content), "\n")
		if len(paths) == 1 {
			p := strings.TrimSpace(paths[0])
			if _, err := os.Stat(p); err == nil {
				target = p
			}
		}
		if target == "" {
			return fmt.Errorf("file not found or multiple files")
		}
	} else if t == types.TypeImage {
		// 图片类型：写入临时文件
		tmp := filepath.Join(os.TempDir(), fmt.Sprintf("gopaste_preview_%d.png", id))
		if err := os.WriteFile(tmp, content, 0o644); err != nil {
			return err
		}
		target = tmp
	} else {
		return fmt.Errorf("not an image type")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "shimgvw.dll,ImageView_Fullscreen", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}
