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
	if s.ShowTrayIcon {
		a.startTray()
	}
	// 即使不显示托盘也要设置 Dock 回调
	tray.SetDockClickCallback(a.showPanel)

	// 7) 启动时按策略 prune
	go a.runPrune()

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
// 输入：绝对屏幕坐标 (x, y)。
// 返回：(absX, absY, workLeft, workTop)
//   - absX/absY 为夹紧后的绝对屏幕坐标；
//   - workLeft/workTop 为工作区原点。Wails v2 在 Windows 下 WindowSetPosition 的入参
//     是「相对工作区」的坐标（Wails 内部会再加上 workArea.Left/Top），因此调用方
//     需要传入 absX-workLeft, absY-workTop。其他平台 workLeft/workTop 为 0。
// margin 为四周保留的安全边距，避免阴影/边框被屏幕或任务栏切到。
func (a *App) clampToScreen(x, y int) (int, int, int, int) {
	w, h := wailsruntime.WindowGetSize(a.ctx)

	// 优先使用系统工作区（Windows 会排除任务栏）
	var sx, sy, sw, sh int
	wx, wy, ww, wh := cursor.WorkArea()
	if ww > 0 && wh > 0 {
		sx, sy, sw, sh = wx, wy, ww, wh
	} else {
		// fallback：用 Wails 返回的屏幕总尺寸
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

	const margin = 8
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

// setWindowPosAbsolute 以「绝对屏幕坐标」方式设置窗口位置，自动适配 Wails 在
// Windows 下 WindowSetPosition 使用「工作区相对坐标」的差异。
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
		wailsruntime.WindowShow(a.ctx)
		a.positionWindow()
		a.setVisible(true)
		return
	}
	a.visMu.Lock()
	visible := a.windowVisible
	a.visMu.Unlock()

	if visible {
		a.saveWindowPosition()
		wailsruntime.WindowHide(a.ctx)
		a.setVisible(false)
	} else {
		a.captureFocusBeforeShow()
		wailsruntime.WindowShow(a.ctx)
		a.positionWindow()
		a.setVisible(true)
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
	wailsruntime.WindowShow(a.ctx)
	a.positionWindow()
	a.setVisible(true)
}

// captureFocusBeforeShow 在窗口显示前抓住当前前台窗口，便于稍后粘贴时还原焦点。
// 必须在 WindowShow 之前调用，否则抓到的就是本应用自己。
func (a *App) captureFocusBeforeShow() {
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
func (a *App) startTray() {
	if a.trayEnd != nil {
		return // 已启动
	}
	a.trayEnd = tray.Start(tray.Callbacks{
		OnShow:    a.togglePanel,
		OnAbout:   a.showAbout,
		OnRestart: a.restartApp,
		OnQuit:    a.quitApp,
	})
}

// stopTray 关闭系统托盘。
func (a *App) stopTray() {
	if a.trayEnd != nil {
		a.trayEnd()
		a.trayEnd = nil
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
// 流程：写剪贴板 → 隐藏窗口 → 等焦点切换 → 发送 Ctrl/Cmd+V
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

	// 取出展示前抓到的"前一个窗口"
	prev := a.takePrevFocus()
	prevValid := prev.IsValid()
	bootProbeApp(fmt.Sprintf("PasteItem: prevFocus valid=%v", prevValid))

	// 隐藏面板，让出前台。
	// 注意：只在"当前确实可见"时才调用 WindowHide。否则在 macOS 下连续调用两次 Hide
	// （比如上一次粘贴后用户又很快触发了一次）有概率让 Wails 的窗口状态机走进
	// "最后一个窗口关闭 → applicationShouldTerminateAfterLastWindowClosed:"
	// 路径，AppKit 直接 [NSApp terminate:] ——进程没走我们的 shutdown、也没
	// 落 crash report，表现就是"闪退"。
	if a.ctx != nil && a.isVisible() {
		a.saveWindowPosition()
		bootProbeApp("PasteItem: before WindowHide")
		wailsruntime.WindowHide(a.ctx)
		bootProbeApp("PasteItem: after WindowHide")
		a.setVisible(false)
	} else {
		bootProbeApp("PasteItem: skip WindowHide (already hidden)")
	}

	// 把焦点切回前一个应用——Windows/Linux 在 WindowHide 后焦点不会自动回到上一个窗口，
	// 必须显式 SetForegroundWindow / activate；否则 Ctrl+V 发到桌面，永远粘不出来。
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

	bootProbeApp("PasteItem: before SendPaste")
	if err := paste.SendPaste(); err != nil {
		bootProbeApp("PasteItem: SendPaste err=" + err.Error())
		a.log.Warn("paste: send paste failed", "err", err)
		return err
	}
	bootProbeApp(fmt.Sprintf("PasteItem: sent id=%d", id))
	a.log.Info("paste: sent", "id", id)
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
		wailsruntime.WindowHide(a.ctx)
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
	if err := a.settings.Set(ns); err != nil {
		return err
	}
	a.registerHotkey()
	go a.runPrune()

	return nil
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
