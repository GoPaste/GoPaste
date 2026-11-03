package main

// app_runtime.go —— GoPaste 调用 Wails v3 runtime 的唯一出口。
//
// v2 时代业务代码直接写 wailsruntime.Xxx(a.ctx, …)，散落在 app.go 的 50 多处。
// 迁移到 v3 后统一收敛到这里：
//   - 业务代码只调 a.emit / a.isMinimised / a.saveFileDialog …，不直接碰 v3 API；
//   - 下次框架升级（v3 每周一个 minor）只需要改这一个文件；
//   - 所有方法在 a.wapp / a.win 尚未就绪时安全降级（no-op 或返回错误），
//     对应 v2 时代到处都有的 `if a.ctx == nil { return }` 判空。
//
// 线程模型：v3 的窗口 / 对话框方法内部都会 InvokeSync 到主线程，可以从任意
// goroutine 调用；但**不要在主线程回调里同步调用**（例如 macOS Dock 点击的
// applicationShouldHandleReopen:），否则会死锁 —— 调用方一律用 `go` 脱离主线程，
// 这与 v2 时代的约束相同（见 startUI 里 Dock 回调的注释）。

import (
	"context"
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"gopaste/internal/window"
)

// errNotReady 在 v3 应用 / 主窗口尚未绑定时返回。
var errNotReady = errors.New("not ready")

// attach 由 main.go 在创建应用和主窗口之后调用，把 v3 句柄交给 App。
// 必须在 app.Run() 之前调用：ServiceStartup / 事件回调里会用到。
func (a *App) attach(wapp *application.App, win *application.WebviewWindow) {
	a.wapp = wapp
	a.win = win
}

// uiReady 报告 v3 应用与主窗口是否已绑定，等价于 v2 时代的 `a.ctx != nil`。
func (a *App) uiReady() bool {
	return a.wapp != nil && a.win != nil
}

// mainWindow 返回主窗口（未就绪时为 nil）。window 包的 ShowMain/HideMain
// 自身会对 nil 判空，所以调用方无需再判断。
func (a *App) mainWindow() application.Window {
	if a.win == nil {
		return nil
	}
	return a.win
}

// emit 向前端广播事件。
//
// 与 v2 的差异：v3 前端回调收到的是 WailsEvent 对象，载荷在 `ev.data`
// （单个参数时就是该值本身），前端已在 App.vue 中同步改造。
// v3 的 Go 侧监听回调按 emit 顺序串行投递；GoPaste 只在前端监听，不受影响。
func (a *App) emit(name string, data ...any) {
	if a.wapp == nil {
		return
	}
	a.wapp.Event.Emit(name, data...)
}

// isMinimised 主窗口是否最小化。
func (a *App) isMinimised() bool {
	if a.win == nil {
		return false
	}
	return a.win.IsMinimised()
}

// unminimise 取消最小化。
func (a *App) unminimise() {
	if a.win == nil {
		return
	}
	a.win.UnMinimise()
}

// centerWindow 把主窗口居中到其所在屏幕。
func (a *App) centerWindow() {
	if a.win == nil {
		return
	}
	a.win.Center()
}

// fileFilter 是保存对话框的文件类型过滤器。
type fileFilter struct {
	DisplayName string // 如 "JSON 文件"
	Pattern     string // 如 "*.json"，多个用分号分隔
}

// saveFileDialog 弹出系统保存对话框，返回用户选择的路径；用户取消时返回 ("", nil)。
//
//   - macOS：主面板是 NonactivatingPanel，GoPaste 不是 active app，v3 的 SaveFile
//     （beginSheetModalForWindow / beginWithCompletionHandler）不会先激活应用，
//     对话框可能弹不出来或无法输入。因此走 window.SaveFileDialog：在主线程原子执行
//     "激活 → NSSavePanel → 恢复"。macExt 是不带点的扩展名，用于限制可选类型。
//   - Windows / Linux：v3 的 SaveFile，附着到主窗口（与 v2 行为一致）。
func (a *App) saveFileDialog(title, defaultName, macExt string, filters []fileFilter) (string, error) {
	if window.HasNativeSaveDialog() {
		return window.SaveFileDialog(title, defaultName, macExt), nil
	}
	if a.wapp == nil {
		return "", errNotReady
	}
	opts := &application.SaveFileDialogOptions{
		Title:                title,
		Filename:             defaultName,
		CanCreateDirectories: true,
	}
	if a.win != nil {
		opts.Window = a.win
	}
	for _, f := range filters {
		opts.Filters = append(opts.Filters, application.FileFilter{
			DisplayName: f.DisplayName,
			Pattern:     f.Pattern,
		})
	}
	return a.wapp.Dialog.SaveFileWithOptions(opts).PromptForSingleSelection()
}

// infoDialog 弹出信息对话框（阻塞直到用户关闭）。
// macOS 下先临时激活应用，否则 NonactivatingPanel 场景里 NSAlert 可能出现在其他 App 后面。
func (a *App) infoDialog(title, message string) {
	if a.wapp == nil {
		return
	}
	window.ActivateForDialog()
	defer window.DeactivateAfterDialog()
	a.wapp.Dialog.Info().SetTitle(title).SetMessage(message).Show()
}

// warningDialog 弹出带自定义按钮的警告对话框，返回用户点击的按钮文案。
//
// v3 的 MessageDialog 通过按钮的 OnClick 回调告知结果（不像 v2 直接返回字符串），
// 这里用带缓冲的 channel 把回调结果转换成返回值。不同平台 Show() 可能阻塞也可能立即
// 返回，所以统一在 Show() 之后等待回调；应用退出时随 ctx 取消返回错误，不会永久阻塞
// 调用方。调用方应在独立 goroutine 中调用（与 v2 时代的约束一致）。
func (a *App) warningDialog(ctx context.Context, title, message string, buttons []string, defaultButton, cancelButton string) (string, error) {
	if a.wapp == nil {
		return "", errNotReady
	}
	if ctx == nil {
		ctx = context.Background()
	}
	clicked := make(chan string, len(buttons)+1)
	d := a.wapp.Dialog.Warning().SetTitle(title).SetMessage(message)
	for _, label := range buttons {
		b := d.AddButton(label)
		b.OnClick(func() { clicked <- label })
		if label == defaultButton {
			d.SetDefaultButton(b)
		}
		if label == cancelButton {
			d.SetCancelButton(b)
		}
	}

	window.ActivateForDialog()
	d.Show()
	window.DeactivateAfterDialog()

	select {
	case sel := <-clicked:
		return sel, nil
	case <-ctx.Done():
		return "", fmt.Errorf("warning dialog: %w", ctx.Err())
	}
}

// openURL 用系统默认浏览器（或对应的 URL scheme 处理程序）打开 URL。
//
// 安全：v2 的 runtime.BrowserOpenURL 在 Go 侧会先做 ValidateAndSanitizeURL
// （拒绝 javascript: / data: / file: / 空 scheme、shell 元字符、危险 Unicode）；
// v3 只在前端 runtime 调用路径上校验，Go 侧的 Browser.OpenURL 直接交给系统打开。
// GoPaste 的 OpenURL RPC 入参来自剪贴板内容（不可信），所以这里显式补上同一套校验，
// 保持与 v2 一致的防护。
func (a *App) openURL(rawURL string) error {
	if a.wapp == nil {
		return errNotReady
	}
	url, err := application.ValidateAndSanitizeURL(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	return a.wapp.Browser.OpenURL(url)
}

// quitRuntime 走 v3 的正常退出流程（触发 ServiceShutdown 等清理）。
func (a *App) quitRuntime() {
	if a.wapp == nil {
		return
	}
	a.wapp.Quit()
}
