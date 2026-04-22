package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
}

// NewApp 创建 App 实例。依赖在 startup 中初始化。
func NewApp() *App {
	return &App{
		log: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})),
	}
}

// startup 初始化各子系统。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	paths, err := config.ResolvePaths()
	if err != nil {
		a.log.Error("resolve paths", "err", err)
		return
	}
	a.paths = paths

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
	w := clipboard.New()
	if err := w.Start(ctx); err != nil {
		a.log.Error("start clipboard watcher", "err", err)
	} else {
		a.watcher = w
		go a.consumeEvents(ctx)
	}

	// 4b) 文件剪切板监听
	fw := clipboard.NewFileWatcher()
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

// positionWindow 根据设置决定窗口出现位置。
func (a *App) positionWindow() {
	s := settings.Default()
	if a.settings != nil {
		s = a.settings.Get()
	}
	switch s.WindowPosition {
	case "follow", "remember":
		// 跟随鼠标和记住位置都使用上次保存的坐标
		if a.posInited {
			wailsruntime.WindowSetPosition(a.ctx, a.lastX, a.lastY)
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

// SetMousePosition 前端在快捷键触发时传入鼠标坐标（用于 follow 模式）。
func (a *App) SetMousePosition(x, y int) {
	s := settings.Default()
	if a.settings != nil {
		s = a.settings.Get()
	}
	if s.WindowPosition == "follow" {
		a.lastX = x
		a.lastY = y
		a.posInited = true
	}
}

// togglePanel 切换主窗口的可见状态：若已显示则隐藏，否则显示并置顶。
// 由托盘左键点击和全局快捷键共享。
func (a *App) togglePanel() {
	if a.ctx == nil {
		return
	}
	if wailsruntime.WindowIsMinimised(a.ctx) {
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
	wailsruntime.WindowShow(a.ctx)
	a.positionWindow()
	a.setVisible(true)
}

func (a *App) setVisible(v bool) {
	a.visMu.Lock()
	a.windowVisible = v
	a.visMu.Unlock()
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

// PasteItem 写回剪切板并尝试自动粘贴到前台窗口。
func (a *App) PasteItem(id int64) error {
	if err := a.CopyToClipboard(id); err != nil {
		return err
	}
	s := settings.Default()
	if a.settings != nil {
		s = a.settings.Get()
	}
	if s.HideOnPaste && a.ctx != nil {
		wailsruntime.WindowHide(a.ctx)
		// 让焦点回到上一个窗口后再发送按键
		time.Sleep(80 * time.Millisecond)
	}
	if s.AutoPaste {
		if err := paste.SendPaste(); err != nil {
			a.log.Warn("send paste", "err", err)
		}
	}
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
