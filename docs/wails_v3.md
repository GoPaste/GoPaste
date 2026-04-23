# Wails v3 调研与 GoPaste 升级评估

> 最后更新：2026-04-23
> 调研来源：[v3alpha.wails.io](https://v3alpha.wails.io)、[wails GitHub v3-alpha](https://github.com/wailsapp/wails/tree/v3-alpha) 分支
> 本文用途：评估 Wails v3 现状、v3 相对 v2 的关键能力提升，以及 GoPaste 升级到 v3 后能获得的改造收益与代价。

---

## 一、当前进度（TL;DR）

| 指标 | 当前状态 |
|------|----------|
| **版本** | `v3.0.0-alpha.77`（2026-04-18 发布） |
| **稳定度** | Alpha，但官方说明"API 相对稳定，有应用已在生产使用" |
| **Beta 时间表** | 无明确日期，官方"正在完善文档和工具" |
| **Stable** | 无预期时间 |
| **CLI 命令** | `wails3` 替代 `wails` |
| **模块路径** | `github.com/wailsapp/wails/v3/...` |

**结论**：**v3 已经相对可用，但仍属 alpha 阶段**。升级到 v3 是**中高风险中长期投资**，不适合立刻切主分支。建议：**新建 `feat/wails-v3` 分支预研 + 试跑**，观察 2~3 个 alpha 版本再决定。

---

## 二、Wails v3 核心能力提升

### 2.1 架构层面

#### ① Services 架构（替代 v2 Bind list）

**v2 写法**：
```go
// main.go
app := NewApp()
wails.Run(&options.App{
    Bind: []interface{}{app},
    OnStartup: app.startup,
    OnShutdown: app.shutdown,
})
```

**v3 写法**：
```go
app := application.New(application.Options{
    Services: []application.Service{
        application.NewService(&ClipboardService{}),
        application.NewService(&SettingsService{}),
        application.NewService(&HotkeyService{}),
    },
})

type ClipboardService struct{ db *sql.DB }

func (s *ClipboardService) ServiceStartup(ctx context.Context, opts application.ServiceOptions) error {
    s.db, _ = sql.Open(...)
    return nil
}
func (s *ClipboardService) ServiceShutdown() error { return s.db.Close() }
// 导出方法自动变成前端可调用
func (s *ClipboardService) ListItems() (...) { ... }
```

**好处**：
- 每个服务有**独立生命周期**，可单独调试
- **注册顺序 = 启动顺序**，依赖明确（reverse 顺序 shutdown）
- `ServiceStartup` 返回 `error` 会**中止启动**，比 v2 的 panic 更优雅
- `ctx` 会在 shutdown 时 cancel，后台 goroutine 自动退出

对我们 GoPaste 的价值：
- 当前 `app.go` 840+ 行、`App` struct 里混杂了 clipboard/storage/hotkey/paste/window 等多重职责
- v3 能自然拆成 `ClipboardService` / `SettingsService` / `HotkeyService` / `WindowService`，职责清晰

#### ② 程序化 API（替代 v2 声明式）

**v2 声明式**：`wails.Run(&options.App{...})` 一坨配置塞进去
**v3 程序化**：
```go
app := application.New(application.Options{...})
mainWindow := app.Window.New(&application.WebviewWindowOptions{...})
mainWindow.Show()
settingsWindow := app.Window.New(&application.WebviewWindowOptions{...})
app.Run()
```

- 窗口是**一等公民**（first-class citizen）
- 可随时创建/销毁窗口、监听窗口事件

#### ③ 多窗口原生支持 ⭐️

v2 里多窗口是靠 hack 实现的（第二个 webview 或浏览器弹窗）。v3 **原生支持多窗口**：
```go
mainWindow := app.Window.New(&application.WebviewWindowOptions{URL: "/"})
settingsWindow := app.Window.New(&application.WebviewWindowOptions{
    URL: "/settings",
    Hidden: true,  // 默认隐藏，点设置按钮才 Show()
})
```

对我们价值：**设置页可以从"主窗口内嵌覆盖 view"改成真正的独立窗口**，体验更接近原生应用。

#### ④ 静态分析绑定（替代反射）

- v2：运行时反射 `reflect.TypeOf(app)` 找方法，生成前端 binding
- v3：编译期用 Go AST 分析，**生成类型更精确的 TS/JS 绑定**
- 构建更快、更能在编译期捕获类型错误
- 支持 Go 泛型、枚举、复杂嵌套类型

### 2.2 跨平台能力层面（最关键！）

以下是 **v3 内置、v2 需要自己写**的能力对照：

| 能力 | v2 | v3 |
|------|-----|-----|
| **单实例保护** | ❌ 无（我们 TODO 未做） | ✅ `SingleInstanceOptions` 内置，跨平台：mac 用 `NSDistributedNotificationCenter`、win 用 `SendMessage`、linux 用 `D-Bus`，**支持传递命令行参数 + AES 加密** |
| **系统托盘** | ❌ 要用 `fyne.io/systray` + `_darwin.m` 胶水 | ✅ 原生 `app.SystemTray.New(...)`，跨平台，菜单事件、tooltip 更新都内置 |
| **原生菜单** | ❌ 基本没有 | ✅ Application Menu、Context Menu 全内置 |
| **全局快捷键** | ❌ 用 `golang.design/x/hotkey` | ✅ `Keyboard Shortcuts` 系统内置（注意：v3 说明是否 system-wide 需核实，但至少 App 级完善） |
| **剪贴板** | ❌ 自己 `golang.design/x/clipboard` | ✅ `app.Clipboard` 跨平台 API |
| **Dock & Taskbar** | ❌ 我们 110 行手写 `SetSkipTaskbar` | ✅ 内置 `GetBadge` / `Show()` / `Hide()` 统一 API |
| **自动更新** | ❌ 未做 | ✅ 内置 Auto-Updates 系统 |
| **文件关联** | ❌ 要自己注册 | ✅ File Associations 内置 |
| **自定义协议（myapp://）** | ❌ | ✅ Custom Protocols 内置 |
| **macOS Universal Links** | ❌ | ✅ alpha.42+ 内置 |
| **拖放文件** | ⚠️ 部分 | ✅ `EnableFileDrop` + 事件携带元数据 |
| **macOS 模态 Sheets** | ❌ | ✅ alpha.74+ 新增 |
| **macOS Liquid Glass** | ❌ | ✅ alpha.65+（macOS 15+） |
| **CollectionBehavior**（跨 Space/Fullscreen） | ❌（我们在 TitleBarHiddenInset 里只做了基础） | ✅ `MacWindow.CollectionBehavior` 完整暴露 |
| **Panic Handling** | ❌ 自己恢复 | ✅ 内置全局 panic 捕获 |

### 2.3 打包 / 分发层面

| 能力 | v2 | v3 |
|------|-----|-----|
| Windows NSIS | ⚠️ 需配置 | ✅ 内置模板 |
| **Windows MSIX** | ❌ | ✅ 支持 MSIX 打包 + 自定义协议 |
| macOS 签名公证 | ⚠️ 手动 | ✅ `wails3 package` 内置 sign + notarize |
| Linux deb/rpm/AppImage | ⚠️ 需写脚本 | ✅ Linux Packaging 内置 |
| 代码签名 | ⚠️ | ✅ Code Signing 作为一等公民章节 |

### 2.4 性能 / 开发体验

- ✅ JSON 处理切换到 `goccy/go-json`，性能 **21~63%** 提升、内存分配减少 **40~60%**
- ✅ 移除部分依赖，二进制减小约 **1.5MB**
- ✅ **In-memory IPC**（不再用本地 HTTP 端口）
- ✅ 更快的 hot reload
- ✅ 更精简的 Application API

### 2.5 实验性 / 前瞻

| 特性 | 状态 |
|------|------|
| **Server Build** (`-tags server`) | 🧪 Experimental — 作为 HTTP 服务跑，无原生 GUI 依赖 |
| **iOS 支持** | 🚧 路线图中（`IOS_ARCHITECTURE.md` 已在仓库） |
| **Android** | 路线图"Mobile coming soon" |
| **WebKitGTK 6.0 / GTK4** | 🧪 `-tags gtk4` 实验 |

---

## 三、GoPaste 升级 v3 的改造清单

基于当前项目结构（见 `ecopaste-analysis.md` 和 `todo.md`），升级 v3 能带来以下具体改造：

### 3.1 可删除的代码（v3 内置 → 我们手写变多余）

| 现有代码 | 可删除 | 替代方案 |
|----------|--------|----------|
| `internal/window/taskbar_{windows,other}.go` (~110 行 Win32 syscall) | ✅ 完全删除 | `window.SetSkipTaskbar(bool)` 官方 API |
| `internal/window/corners_{windows,other}.go` (~47 行 DWM 调用) | ⚠️ 部分保留 | v3 有 `CollectionBehavior` 但 Win11 圆角仍需调 DWM（待验证） |
| `internal/tray/dispatch_darwin.{go,m}` + `dock_darwin.{go,m}` | ✅ 完全删除 | `app.SystemTray.New(...)` 跨平台封装 |
| `internal/hotkey/*.go` 依赖 `golang.design/x/hotkey` | ⚠️ 评估 | v3 `Keyboard Shortcuts` 是否 system-wide 待核实；可能仍需自写 |
| `main.go` 里 `BackgroundColour` 硬编码 | ✅ 用 v3 MacWindow Frameless + 透明度配置替代 |
| **新功能可直接获得**： 单实例保护、自动更新、文件关联 | ✅ 无需代码 | `SingleInstanceOptions` / `AutoUpdateOptions` 官方内置 |

### 3.2 需要重构的代码（API 破坏性变更）

#### `app.go` 重构最重要！

**当前** 840+ 行的 `type App struct{...}` + 一堆 Wails RPC 方法。

**v3 拆分**：
```
services/
├─ clipboard_service.go   # ListItems/GetContent/DeleteItem/ToggleFavorite/TogglePin/SetNote/CopyToClipboard
├─ paste_service.go       # PasteItem + focus 捕获（内部仍用 internal/paste）
├─ settings_service.go    # GetSettings/UpdateSettings/ExportData/ClearHistory/DataDir
├─ window_service.go      # HideWindow/ShowWindow/WindowSetAlwaysOnTop（v3 API 已是 first-class，可能整个服务都不需要）
├─ url_service.go         # OpenURL/RevealInExplorer/SaveImageToFile/GetFileThumbnail
└─ about_service.go       # About / Restart / TrayNeedsRestart
```

每个 Service 实现 `ServiceStartup(ctx, opts)` / `ServiceShutdown()`。

#### `main.go` 重构
```go
// v3
app := application.New(application.Options{
    Name: "GoPaste",
    SingleInstance: &application.SingleInstanceOptions{
        UniqueID: "com.gopaste.app",
        OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
            mainWindow.Restore()
            mainWindow.Focus()
        },
    },
    Services: []application.Service{
        application.NewService(&ClipboardService{}),
        application.NewService(&SettingsService{}),
        // ...
    },
})

mainWindow := app.Window.New(&application.WebviewWindowOptions{
    Name:   "main",
    URL:    "/",
    Width:  480,
    Height: 680,
    Mac: application.MacWindow{
        TitleBar: application.MacTitleBarHiddenInset,
        Backdrop: application.MacBackdropTranslucent,
        CollectionBehavior: application.NSWindowCollectionBehaviorMoveToActiveSpace |
                           application.NSWindowCollectionBehaviorFullScreenAuxiliary,
    },
})

// 设置窗口独立
settingsWindow := app.Window.New(&application.WebviewWindowOptions{
    Name:   "settings",
    URL:    "/settings",
    Hidden: true,
})

app.Run()
```

#### 前端绑定路径变化
- v2：`../wailsjs/go/main/App` → v3：`../bindings/services/ClipboardService`
- import 路径全部要改
- 配合 `wails3 generate bindings` 重跑生成

### 3.3 可直接新增的功能（v3 带来的免费午餐）

| 新功能 | v3 接入成本 |
|--------|------------|
| **单实例保护**（TODO P0） | 10 行 `SingleInstanceOptions` 配置 |
| **自动更新**（TODO P0） | 配 updater server + 几行 options |
| **独立设置窗口**（目前是覆盖式） | 替换成 `app.Window.New()` 一个新窗口即可 |
| **macOS 模态 Sheets** | 删除弹窗时可用原生 sheet 替代自绘 modal |
| **macOS Liquid Glass**（macOS 15+） | 几行配置，立刻获得原生毛玻璃效果 |
| **panic 全局恢复** | 无需自己写 defer recover |
| **Windows MSIX 打包** | 商店分发支持 |

### 3.4 不受影响的代码

- `internal/clipboard/` 剪贴板监听（不依赖 Wails）
- `internal/storage/` SQLite + gorm
- `internal/crypto/` 加密
- `internal/types/` 数据结构
- `internal/config/` 配置路径
- `internal/paste/` 粘贴执行（仅被 `paste_service.go` 调用，不需要改）
- `frontend/` Vue 组件业务逻辑（只改 binding import）

---

## 四、升级代价（风险评估）

### 4.1 工程成本

| 项 | 估算 |
|---|------|
| 阅读 v3 文档 + 跑通 hello world | 0.5 人天 |
| `main.go` + `app.go` 拆成 Services | **2~3 人天** |
| 窗口/托盘代码迁移到 v3 API | 1~2 人天 |
| 前端 binding 迁移 + 回归测试 | 1 人天 |
| 三端构建 & 回归测试 | 1 人天 |
| **总计** | **~6~8 人天** |

### 4.2 风险点

1. **Alpha 稳定性**：版本每 1~2 周迭代一次 alpha，**API 可能小幅调整**。需跟进 changelog。
2. **生态滞后**：`golang.design/x/hotkey`、`fyne.io/systray` 等第三方库可能与 v3 不完全兼容（需要重写 glue 层）。
3. **文档还不全**：路线图页基本是占位，迁移指南也还在完善。
4. **v2 LTS 不明确**：官方没明说 v2 维护多久，但 2.12 还是 2026-03-26 发布，活跃度 OK。
5. **我们 DWM 圆角代码**可能需要适配：v3 的 `MacWindow.Backdrop` 等新配置可能和我们 `applyWin11RoundCorners` 冲突或重复。

### 4.3 收益回报

| 收益 | 价值 |
|------|------|
| 代码量减少 **~200 行**（taskbar、tray、部分 paste） | ⭐⭐⭐ |
| 免费获得单实例/自动更新/独立设置窗 | ⭐⭐⭐⭐ |
| app.go 拆成 Services 自然解决 P1 todo | ⭐⭐⭐ |
| 性能提升 20~60% JSON 处理 + 二进制减小 1.5MB | ⭐⭐ |
| 未来 iOS/Android 支持 | ⭐⭐（路线图，长远） |

---

## 五、建议路线

### 方案 A：激进升级（不推荐当前阶段）
立刻开 `feat/wails-v3` 分支全量迁移，一周内出可用版本。
**风险**：踩 alpha 坑，PR 长期挂着，v2 修复同步困难。

### 方案 B：平行预研（✅ 推荐）
1. **立即**：新建 `spike/wails-v3` 分支，跑通 `wails3 init` hello world，评估 CLI/Taskfile 体验
2. **短期（1~2 周内）**：把 `internal/clipboard` + `storage` 作为 Service 移植过去试水，验证核心功能
3. **中期**：等 v3 官方进入 **Beta**（时间未定，可能 2026 下半年），再做全量迁移决策
4. **长期**：Beta 发布 + 3 个版本稳定后切主分支

### 方案 C：保守不升（保留选项）
- 继续用 v2.12 稳定版
- 仅通过关注 v3 changelog 跟进，适时抄 Tauri/v3 的设计模式优化我们的 v2 代码（例如 Service 化拆分 app.go 完全可以在 v2 上先做）
- 等 v3 出 stable 再说

**个人建议**：**方案 B**。理由：
- 我们已经有正常工作的 v2 产品，没必要冒险
- Service 化拆分的思想**可以提前应用在 v2**（拆成小结构体，每个暴露一组方法，在 `Bind` 里注册多个对象而不是一个大 `App`）
- 单实例/自动更新等 TODO 可以**先手写轻量实现**，等 v3 稳定后再替换

---

## 六、扩展：在 v2 上先落地 v3 风格

即使不升级框架，也可以**借鉴 v3 的思想**改造现有代码：

### 6.1 Service 化拆分（v2 也能做）

`main.go` 改为：
```go
clipboardSvc := &ClipboardService{}
settingsSvc := &SettingsService{}
// ...
wails.Run(&options.App{
    Bind: []interface{}{
        clipboardSvc,
        settingsSvc,
        hotkeySvc,
        windowSvc,
    },
    OnStartup: func(ctx context.Context) {
        clipboardSvc.Startup(ctx)
        settingsSvc.Startup(ctx)
        // ...
    },
})
```

这样 **app.go 拆分重构**可以**独立于 v3 升级**先做，解决 TODO P1 项。

### 6.2 单实例先手写

参考 Tauri/v3 的方案：
- Windows：`CreateMutexW` 创建命名互斥体
- macOS：bundle ID + `NSDistributedNotificationCenter`
- Linux：D-Bus 唯一名或 socket 文件

~100 行代码（每平台 30+ 行），比等 v3 升级成本低。

### 6.3 自动更新先手写

- 启动时 GET `https://api.github.com/repos/.../releases/latest`
- 比对 tag → 有新版本就弹 toast "发现新版本 x.y.z, 前往下载"
- 用户点 → `OpenURL(downloadURL)` 交给浏览器

~50 行代码搞定。

---

## 七、结论

| 问题 | 答案 |
|------|------|
| **v3 现在能用吗？** | ✅ 能用，alpha.77 API 已稳定，有生产应用。但**仍是 alpha，不建议立即生产切换** |
| **v3 相比 v2 提升大吗？** | ⭐⭐⭐⭐ 很大。**Services 架构、多窗口、单实例、自动更新、跨平台托盘、自定义协议**等都是质变 |
| **GoPaste 现在就升吗？** | ❌ **不建议**。继续用 v2.12，等 v3 Beta 再评估 |
| **有什么能先做？** | ✅ 在 v2 上按 v3 思想拆分 app.go 为多个 Service；手写实现单实例和自动更新 |
| **升级到 v3 能省多少代码？** | 约 200~300 行（taskbar/tray/部分 glue）+ 免费拿到 单实例/自动更新/MSIX 打包 |

**最核心 take-away**：
> Wails v3 正在补齐 Tauri 的生态优势（系统托盘、单实例、自动更新、多窗口、统一 API），**等它 Stable 以后 Wails 的竞争力会显著提升**。对 GoPaste 来说，升级是**"什么时候升"而非"要不要升"的问题**。

---

## 附录 A：参考资源

- [Wails v3 Alpha 首页](https://v3alpha.wails.io/)
- [Wails v3 Changelog](https://v3alpha.wails.io/changelog)
- [Application Lifecycle 文档](https://v3alpha.wails.io/concepts/lifecycle/)
- [Single Instance 文档](https://v3alpha.wails.io/guides/single-instance/)
- [GitHub v3-alpha 分支](https://github.com/wailsapp/wails/tree/v3-alpha)

## 附录 B：近期 alpha 重要里程碑

| 版本 | 日期 | 关键变更 |
|------|------|----------|
| alpha.77 | 2026-04-18 | ScreenManager 数据竞争修复 |
| alpha.74 | 2026-03-01 | **macOS 模态 Sheets 支持** |
| alpha.71 | - | Dock service 新增 `GetBadge` |
| alpha.68 | - | **实验 WebKitGTK 6.0 / GTK4 支持** |
| alpha.66 | - | **`UseApplicationMenu` 跨平台选项** |
| alpha.65 | - | **macOS Liquid Glass effect** |
| alpha.55 | - | **JSON 性能 +21~63%**、二进制 -1.5MB |
| alpha.54 | - | **`CollectionBehavior` macOS 选项** |
| alpha.48 | - | 移除包级 dialog 函数（破坏性）|
| alpha.44 | - | 生产构建成为默认（破坏性）|
| alpha.42 | - | **Universal Links (macOS)** |
