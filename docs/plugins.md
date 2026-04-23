# GoPaste 可用的 Go 库 / Wails 插件调研

> 最后更新：2026-04-23
> 调研范围：Wails v2 缺失的能力，是否有现成 Go 库 / 插件可直接用，而不是自己写原生代码

---

## TL;DR

| 能力 | 我们现状 | 有无现成库 | 推荐库 |
|------|--------|-----------|--------|
| 系统托盘 | `fyne.io/systray` + `_darwin.m` 胶水 | ✅ 已用 | `fyne.io/systray`（当前方案还是最好的） |
| 全局快捷键 | `golang.design/x/hotkey` | ✅ 已用 | `golang.design/x/hotkey`（当前方案就是推荐） |
| 剪贴板监听 | `golang.design/x/clipboard` | ✅ 已用 | 当前 OK，无更好替代 |
| **单实例保护** | ❌ 未做 | ✅ 有库 | `allan-simon/go-singleinstance` 或自写 |
| **开机自启** | ❌ 未做 | ✅ 有库 | **`emersion/go-autostart`**（跨平台，单文件封装） |
| **自动更新** | ❌ 未做 | ✅ 有库 | **`creativeprojects/go-selfupdate`**（支持 GitHub Releases） |
| 窗口圆角（Win11） | 自写 DWM syscall | ⚠️ 基本没有 | 继续自写或接受 WebView2 默认 |
| 自动粘贴模拟 | `internal/paste/*_{os}.go` | ⚠️ 有跨平台库但重 | `robotgo`（重度 cgo）/ 当前方案更轻 |

**核心结论**：**单实例、开机自启、自动更新**这三个 TODO P0 项**都有成熟 Go 库可用**，合计 < 200 行代码就能接入。不用自己写原生代码，也不用等 Wails v3。

---

## 一、已用库的评估

### 1.1 `fyne.io/systray` ✅ 当前最佳

我们 `internal/tray/` 里用的就是它。

- **跨平台支持**：Windows / macOS / Linux
- **Stars**：~800+
- **社区活跃**：有维护
- **缺点**：macOS 侧需要 Objective-C glue（我们有 `dispatch_darwin.m` / `dock_darwin.m` 两个 .m 文件）—— 但这是 Cocoa API 的必然，任何库都避不开

**替代候选对比**：

| 库 | 评估 |
|----|------|
| **`getlantern/systray`** | fyne fork 自它；getlantern 版本已停更，**fyne 版是现在最活跃的** |
| `energye/systray` | fyne 的另一 fork，功能差不多 |
| Wails v3 内置 `SystemTray` | 未 stable，v2 下不可用 |

**结论**：**继续用 `fyne.io/systray`**，没必要换。

### 1.2 `golang.design/x/hotkey` ✅ 当前最佳

我们 `internal/hotkey/` 里就用它。

- **跨平台**：Windows (`RegisterHotKey`) / macOS (`RegisterEventHotKey`) / Linux (X11 `XGrabKey`)
- **设计优雅**：`hk.New(mods, key)` 然后 `hk.Register()`，API 很 Go 风格
- **仅支持 X11**（Wayland 需另想办法，这是 EcoPaste / Tauri 都面临的问题）

**替代候选**：

| 库 | 评估 |
|----|------|
| `MakeNowJust/hotkey` | 不维护 |
| `robotgo` | 功能强但 cgo 依赖重（OpenCV 等），只为 hotkey 不值得 |

**结论**：**继续用**。Wayland 问题是全 Go 生态的共同短板，不怪库。

### 1.3 `golang.design/x/clipboard` ✅ 已用

监听粘贴板变化。

- 作者同 `golang.design/x/hotkey`，同一生态
- 跨平台
- API 清晰 `clipboard.Watch(ctx, clipboard.FmtText)`

**结论**：**继续用**。

### 1.4 `zalando/go-keyring` ✅ 已用

存储 AES 密钥。

- Windows：Credential Manager
- macOS：Keychain
- Linux：D-Bus Secret Service / libsecret
- 非常稳定，广泛使用

**结论**：**继续用**。

---

## 二、缺失能力的现成库推荐

### 2.1 单实例保护 ⭐️ 推荐 `allan-simon/go-singleinstance`

**仓库**：[github.com/allan-simon/go-singleinstance](https://github.com/allan-simon/go-singleinstance)

- **跨平台**：Windows (Named Mutex) / macOS (file lock) / Linux (file lock)
- **代码量**：极小，1 个文件即可
- **用法**：
  ```go
  import "github.com/allan-simon/go-singleinstance"

  lockFile, err := singleinstance.CreateLockFile("/tmp/gopaste.lock")
  if err != nil {
      // 已经有实例在跑，退出
      os.Exit(0)
  }
  defer lockFile.Close()
  ```
- **稳定性**：版本 1.0，作者说"stable，不用频繁更新"
- **有 Debian 官方包**（说明在 Linux 发行版里也被认可）

**对比自写**：
- 自写：Windows CreateMutexW + macOS + Linux 各 30 行 ≈ 100 行
- 用库：**3 行搞定**

#### 限制
- 不支持**把启动参数转发给已运行实例**（Wails v3 内置的 `SingleInstanceOptions.OnSecondInstanceLaunch` 能做到）
- 如果你需要"第二次启动时把命令行参数传给第一个实例并激活窗口"，需要在库之上加一层：socket/pipe 通信

**我们的场景**：不需要转发参数，GoPaste 启动参数很少。**直接用 `allan-simon/go-singleinstance` 就够**。

### 2.2 开机自启 ⭐️ 推荐 `emersion/go-autostart`

**仓库**：[github.com/emersion/go-autostart](https://github.com/emersion/go-autostart)

- **作者**：emersion（`go-imap` 等知名库作者）
- **跨平台**：
  - Windows：写注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
  - macOS：生成 `~/Library/LaunchAgents/<name>.plist`
  - Linux：生成 `~/.config/autostart/<name>.desktop`
- **用法**：
  ```go
  import "github.com/emersion/go-autostart"

  app := &autostart.App{
      Name:        "gopaste",
      DisplayName: "GoPaste",
      Exec:        []string{exePath, "--silent-start"},
  }

  // 启用
  app.Enable()

  // 禁用
  app.Disable()

  // 检查
  if app.IsEnabled() { ... }
  ```
- **代码量**：整个库也就几百行，API 极简

**对比自写**：三平台各自的注册表/plist/desktop 格式是坑点（尤其 macOS plist 的 xml schema），**用库省事**。

### 2.3 自动更新 ⭐️ 推荐 `creativeprojects/go-selfupdate`

**仓库**：[github.com/creativeprojects/go-selfupdate](https://github.com/creativeprojects/go-selfupdate)

- **前身**：fork 自已停更的 `rhysd/go-github-selfupdate`
- **特性**：
  - 从 **GitHub Releases** API 拉取最新版本
  - 比对当前版本（semver）
  - 自动下载对应平台的 asset
  - 替换当前 binary（利用 Go 自更新 trick：新文件写到临时位置，旧文件被系统"持有"但可以 rename）
  - 支持签名校验
- **用法**：
  ```go
  import "github.com/creativeprojects/go-selfupdate"

  latest, found, err := selfupdate.DetectLatest(
      context.Background(),
      selfupdate.ParseSlug("larkwins/gopaste"),
  )
  if found && latest.GreaterThan(currentVersion) {
      // 前端弹 toast 提示
      // 用户确认后：
      exe, _ := os.Executable()
      selfupdate.UpdateTo(ctx, latest.AssetURL, latest.AssetName, exe)
      // 重启程序
  }
  ```
- **跨平台**：Windows/macOS/Linux 都有 "替换当前 binary" 的实现
- **维护中**：2025 年仍有更新

**替代候选**：

| 库 | 评估 |
|----|------|
| `minio/selfupdate` | 更底层，只做"替换 binary"不管版本检测。需自写 GitHub API 调用 |
| `rhysd/go-github-selfupdate` | 停更，被 creativeprojects fork 接手 |
| 自写 | 也就 50 行：fetch + compare + DownloadAndReplace，简单场景可接受 |

**注意**：macOS 如果 binary 被 codesign，更新后签名会失效需要重新签名。企业/个人开发场景下这块要小心，用户可能需要手动拖进 Applications 重新授权。

### 2.4 自动粘贴跨平台库 ⚠️ 不推荐替换

候选：
- **`go-vgo/robotgo`** — 功能最强（键盘、鼠标、图像识别），但 **cgo 依赖重**（OpenCV 等），会让我们的二进制和编译复杂度暴涨
- **`micmonay/keybd_event`** — 纯 keybd_event，轻量但 macOS 侧是基于 AppleScript

我们当前 `internal/paste/*_{os}.go` 每个平台 ~30 行代码，**自写反而更可控**。**不推荐替换。**

### 2.5 Wayland 粘贴支持 ⚠️ 暂无完美方案

| 方案 | 评估 |
|------|------|
| `wtype` 命令行工具 | 需要用户安装 `wtype` 包 |
| `ydotool` | 需要 root 或 setuid |
| D-Bus `org.freedesktop.portal.Clipboard` | XDG portal 在新版 Wayland 下支持 |

**建议**：在 Linux 下做运行时探测（`$XDG_SESSION_TYPE == "wayland"`），退化到 "复制但不自动粘贴" 的模式，UI 上提示用户手动 `Ctrl+V`。

---

## 三、Wails 特定的"插件"生态

### 3.1 awesome-wails 的定位

官方 `wailsapp/awesome-wails` 列表**主要列应用和模板**，**没有分类组织"插件库"**。这反映了 Wails v2 **没有正式的插件机制**——只要是 Go 库都能用。

### 3.2 社区约定俗成的 Wails 搭档库

| 用途 | 库 | 说明 |
|------|-----|------|
| 系统托盘 | `fyne.io/systray` | 最常见搭配 |
| 全局快捷键 | `golang.design/x/hotkey` | 最常见搭配 |
| 剪贴板 | `golang.design/x/clipboard` | 最常见搭配 |
| HTTP 后端（如果嵌入 server） | `gin-gonic/gin` | 见 awesome-wails 的 "My App" 模板 |
| SQLite | `gorm.io/gorm` + `glebarez/sqlite` | 我们就是这个组合 |
| 日志 | `uber-go/zap` 或 `slog` | 我们用 `log/slog` |

**这些库都不是"Wails 插件"，就是普通的 Go 库。** Wails v2 没有类似 Tauri v2 `tauri-plugin-*` 那种"插件 crate" 机制。

### 3.3 Wails v3 的插件/Service 机制

Wails v3 **引入了 Service 概念**，相当于 Tauri 的 plugin。但：
- v3 仍在 alpha
- Service 机制针对的是**业务代码组织**，不是让第三方发布"拿来即用的跨平台托盘"
- v3 内置了之前需要第三方库的几样：**SystemTray、Clipboard、SingleInstance、AutoUpdate、Shortcuts** 等

所以**升级 v3 主要收益是内置能力，而不是"更丰富的插件市场"**。

---

## 四、具体行动建议

基于以上调研，**不升级 v3 的前提下**，我们可以立刻做的：

### 快速落地 P0 TODO

```go
// 1. 单实例（加在 main.go 最开头）
import "github.com/allan-simon/go-singleinstance"

lockFile, err := singleinstance.CreateLockFile(filepath.Join(os.TempDir(), "gopaste.lock"))
if err != nil {
    // 已有实例在跑，可以通过 IPC 激活它，也可以简单 exit
    os.Exit(0)
}
defer lockFile.Close()
```
⏱ **15 分钟接入**

```go
// 2. 开机自启（加到设置服务里）
import "github.com/emersion/go-autostart"

func (s *SettingsService) SetAutoStart(enabled bool) error {
    exe, _ := os.Executable()
    app := &autostart.App{
        Name: "gopaste", DisplayName: "GoPaste",
        Exec: []string{exe, "--silent-start"},
    }
    if enabled { return app.Enable() }
    return app.Disable()
}
```
⏱ **30 分钟接入 + 10 分钟前端设置页开关**

```go
// 3. 自动更新（启动时异步检查）
import "github.com/creativeprojects/go-selfupdate"

func (a *App) CheckForUpdate() (*UpdateInfo, error) {
    latest, found, _ := selfupdate.DetectLatest(ctx, selfupdate.ParseSlug("larkwins/gopaste"))
    if found && latest.GreaterThan(currentVersion) {
        return &UpdateInfo{Version: latest.Version, URL: latest.URL}, nil
    }
    return nil, nil
}
```
⏱ **1~2 小时接入**

### 投入产出

| 项 | 总工时 | 价值 |
|----|-------|------|
| 3 个 P0 TODO 一次搞定 | ~3 小时 | ⭐⭐⭐⭐⭐ |
| 不依赖 Wails v3 升级 | - | ⭐⭐⭐⭐ |
| 未来迁到 v3 时代码只是重写成 Service | - | 无锁定 |

---

## 五、结论与回答

### 你的问题：「GitHub 上有现成的插件可以使用吗？」

**答：有，而且质量不错。**

1. **全局快捷键 / 托盘**：我们**已经用了最合适的库**（`golang.design/x/hotkey` 和 `fyne.io/systray`），没必要换。关于 macOS 侧的 Obj-C `.m` 文件——那是跨语言 FFI 的必要胶水，任何方案都绕不开。

2. **单实例 / 开机自启 / 自动更新**：这三个我们 TODO 上还没做的能力，**都有成熟 Go 库**：
   - `allan-simon/go-singleinstance`
   - `emersion/go-autostart`
   - `creativeprojects/go-selfupdate`

3. **Wails 没有 Tauri 那种插件市场**。但 Go 生态成熟度够用，绝大多数桌面能力都有现成库。

### 为什么之前"感觉要自己写原生代码"？

因为**圆角、任务栏显隐、DWM API** 这些是**单平台单场景**的冷门需求，没有流行的跨平台 Go 库；而**托盘、快捷键、剪贴板、单实例、自启、更新**这些是**通用需求**，Go 生态早有现成库。

### 下一步建议

1. **立刻动手**把上面 3 个库接进来（P0 TODO 都能解决）
2. **继续用现有的 hotkey/systray 库**
3. Wails v3 升级可以再等等

---

## 附录：库选型对比表

| 库 | 跨平台 | 维护状态 | 大小 | cgo | 推荐 |
|----|--------|---------|------|-----|------|
| `fyne.io/systray` | ✅ | 🟢 活跃 | 小 | Mac 需 | ⭐⭐⭐⭐⭐ |
| `golang.design/x/hotkey` | ✅ | 🟢 活跃 | 小 | 是 | ⭐⭐⭐⭐⭐ |
| `golang.design/x/clipboard` | ✅ | 🟢 活跃 | 小 | 是 | ⭐⭐⭐⭐⭐ |
| `zalando/go-keyring` | ✅ | 🟢 活跃 | 小 | 是 | ⭐⭐⭐⭐⭐ |
| `allan-simon/go-singleinstance` | ✅ | 🟡 稳定 | 极小 | 否 | ⭐⭐⭐⭐ |
| `emersion/go-autostart` | ✅ | 🟢 活跃 | 小 | 否 | ⭐⭐⭐⭐⭐ |
| `creativeprojects/go-selfupdate` | ✅ | 🟢 活跃 | 中 | 否 | ⭐⭐⭐⭐ |
| `go-vgo/robotgo` | ✅ | 🟢 活跃 | 大 | 重度 | ⭐⭐ |
| `minio/selfupdate` | ✅ | 🟢 活跃 | 小 | 否 | ⭐⭐⭐ |
