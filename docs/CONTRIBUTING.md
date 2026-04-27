# GoPaste · 开发者文档

> 基于 **Wails v2 + Go + Vue 3 + TypeScript** 构建的跨平台剪贴板管理工具。

---

## 目录

- [技术栈](#技术栈)
- [环境搭建](#环境搭建)
- [项目结构](#项目结构)
- [架构概览](#架构概览)
- [开发工作流](#开发工作流)
- [构建 & 发布](#构建--发布)
- [图标管理](#图标管理)
- [版本管理](#版本管理)
- [关键模块说明](#关键模块说明)
- [平台特殊处理](#平台特殊处理)
- [代码规范](#代码规范)
- [已知问题 & Todo](#已知问题--todo)

---

## 技术栈

### 后端

| 分类 | 选型 | 说明 |
|------|------|------|
| 桌面框架 | `wailsapp/wails v2` | Go ↔ JS 双向 RPC，原生 WebView |
| 剪贴板监听 | `golang.design/x/clipboard` | 跨平台文本 + 图片监听 |
| 全局快捷键 | `golang.design/x/hotkey` | 三端统一 API |
| 数据库 | `modernc.org/sqlite` | 纯 Go，无 CGO，可交叉编译 |
| ORM | `gorm.io/gorm` | SQLite CRUD 简化 |
| 加密 | 标准库 `crypto/aes` + `crypto/cipher` | AES-256-GCM |
| 密钥管理 | `zalando/go-keyring` | 系统 Keychain（Win/Mac/Linux） |
| 系统托盘 | `fyne.io/systray` | 跨平台托盘；macOS 走纯 CGO NSStatusItem |
| 日志 | `log/slog`（Go 1.21+） | 结构化日志 |
| 自动更新 | `creativeprojects/go-selfupdate` | GitHub Releases 检测 |

### 前端

| 分类 | 选型 |
|------|------|
| 框架 | Vue 3 Composition API |
| 语言 | TypeScript |
| 构建 | Vite（Wails v2 集成） |
| 图标 | `lucide-vue-next` |
| 国际化 | 自实现 `i18n.ts`（简中 / 繁中 / English） |

---

## 环境搭建

### 前置依赖

| 工具 | 最低版本 | 安装方式 |
|------|---------|---------|
| Go | 1.22 | https://golang.org/dl |
| Node.js | 20 | https://nodejs.org |
| Wails CLI | v2.12 | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |

### 平台系统依赖

**Linux (Debian/Ubuntu)：**
```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev libxtst-dev
# 或
make install-deps
```

**Linux (RHEL/TencentOS)：**
```bash
sudo yum install gtk3-devel webkit2gtk4.0-devel libX11-devel libXtst-devel libXi-devel libXrandr-devel
```

**macOS：**
```bash
xcode-select --install
```

**Windows：**
- WebView2 Runtime（Windows 11 自带，Windows 10 需手动安装）

### 克隆 & 初始化

```bash
git clone https://github.com/larkwins/GoPaste.git
cd GoPaste
go mod tidy
cd frontend && npm install
```

### 验证环境

```bash
make doctor   # 检查 Wails 环境
make info     # 显示项目 & 环境信息
```

---

## 项目结构

```
GoPaste/
├── main.go                     # 程序入口，Wails app 初始化
├── app.go                      # Wails RPC 绑定层（暴露给前端的所有方法）
├── wails.json                  # Wails 配置（outputfilename / productVersion 等）
├── go.mod / go.sum
├── Makefile                    # 所有常用命令入口
│
├── internal/
│   ├── appguard/               # 单实例保护 & 开机自启
│   │   ├── autostart.go        # 跨平台自启接口
│   │   ├── autostart_unix.go   # macOS plist / Linux .desktop
│   │   ├── autostart_windows.go# Windows 注册表
│   │   └── singleinstance.go   # 文件锁单实例
│   ├── clipboard/              # 剪贴板监听
│   │   ├── watcher.go          # 主循环：事件驱动 + 防抖
│   │   ├── textwatch_*.go      # 平台差异：文本监听
│   │   ├── imagewatch_*.go     # 平台差异：图片监听
│   │   ├── filewatcher_*.go    # 平台差异：文件监听
│   │   └── write_*.go          # 写回剪贴板（多格式）
│   ├── config/
│   │   └── config.go           # 数据目录（AppName = "GoPaste"）
│   ├── crypto/
│   │   ├── keyring.go          # 系统 Keychain 读写
│   │   └── （加密相关）
│   ├── cursor/                 # 鼠标位置（窗口跟随鼠标模式用）
│   ├── hotkey/                 # 全局快捷键注册 & 重注册
│   ├── logger/                 # 日志重定向（stderr → 文件）
│   ├── paste/                  # 模拟粘贴
│   │   ├── paste.go            # 公共入口
│   │   ├── paste_darwin.go     # macOS: ResignKey + osascript
│   │   ├── paste_windows.go    # Windows: Shift+Insert (SendInput)
│   │   └── paste_linux.go      # Linux: xdotool (X11)
│   ├── settings/
│   │   └── settings.go         # 用户设置结构体 & JSON 持久化
│   ├── storage/                # SQLite 数据层
│   ├── tray/                   # 系统托盘
│   │   ├── tray.go             # 跨平台入口（embed 图标 / Start / SetIconStyle）
│   │   ├── icons/              # 所有托盘图标（由脚本生成）
│   │   │   ├── tray.ico        # Windows 彩色
│   │   │   ├── tray-gray.ico   # Windows 灰色
│   │   │   ├── tray.png        # Linux
│   │   │   ├── tray-color.png  # macOS 彩色
│   │   │   ├── tray-gray.png   # macOS 灰色
│   │   │   └── tray-template.png # macOS 模板（自适应深浅模式备用）
│   │   ├── start_darwin.go     # macOS: 纯 CGO NSStatusItem
│   │   ├── start_other.go      # Windows/Linux: fyne.io/systray
│   │   ├── statusitem_darwin.{go,m}  # macOS NSStatusItem 实现
│   │   ├── visible_windows.go  # Shell_NotifyIcon NIM_DELETE/NIM_ADD
│   │   ├── visible_darwin.go   # NSStatusItem install/uninstall
│   │   └── visible_other.go    # Linux: no-op
│   ├── types/                  # 共享数据结构（Item / SearchQuery 等）
│   ├── updater/
│   │   ├── version.go          # 运行时版本（ldflags 注入）
│   │   └── updater.go          # GitHub Releases 检测
│   └── window/                 # 窗口管理
│       ├── panel_darwin.{go,m} # macOS: NSWindow → NSPanel 转换
│       ├── corners_windows.go  # Windows 11 DWM 圆角
│       ├── taskbar_windows.go  # 任务栏图标显隐
│       ├── showhide_*.go       # 跨平台 ShowWindow / HideWindow
│       └── options_*.go        # 平台差异窗口选项
│
├── frontend/
│   ├── src/
│   │   ├── App.vue             # 主组件（列表 / 搜索 / 详情 / 弹窗）
│   │   ├── views/
│   │   │   └── Settings.vue    # 设置页（通用 / 剪贴板 / 快捷键 / 数据 / 关于）
│   │   ├── i18n.ts             # 国际化（zh / zh-TW / en）
│   │   └── style.css
│   └── wailsjs/                # Wails 自动生成（勿手动修改）
│       ├── go/main/App.{js,d.ts}
│       └── runtime/runtime.{js,d.ts}
│
├── build/
│   ├── appicon.src.png         # 设计源稿（唯一需要手工维护的图标）
│   ├── appicon.png             # Wails 构建主图标（脚本生成）
│   └── windows/icon.ico        # Windows exe 图标（Wails 硬编码路径）
│
├── scripts/
│   ├── gen_appicon.py          # 生成 build/appicon.png + tray-color.png
│   ├── gen_tray_icon_gray.py   # 生成 tray-gray.png + tray-gray.ico
│   └── gen_tray_icon.go        # 生成 tray-template.png（go:build ignore）
│
└── docs/                       # 设计 & 开发文档
    ├── CONTRIBUTING.md         # 本文档
    ├── DESIGN.md               # 架构设计方案
    ├── todo.md                 # 改进事项清单
    ├── macos-accessibility.md  # macOS 辅助功能权限说明
    └── win-release.md          # Windows 发布说明
```

---

## 架构概览

```
┌─────────────────────────────────────────────┐
│              Frontend (Vue 3)               │
│   App.vue          Settings.vue             │
│   ┌──────────────────────────────────────┐  │
│   │  搜索 · 列表 · 类型过滤 · 详情 · 弹窗│  │
│   └──────────────────────────────────────┘  │
│        ↑ Wails RPC       ↑ EventsOn         │
└────────┼─────────────────┼───────────────────┘
         │                 │ runtime.EventsEmit
┌────────▼─────────────────▼───────────────────┐
│              Go Backend (app.go)              │
│  ┌───────────┐ ┌────────┐ ┌───────┐ ┌──────┐ │
│  │ clipboard │ │storage │ │hotkey │ │ tray │ │
│  │  watcher  │ │SQLite+ │ │global │ │      │ │
│  │           │ │crypto  │ │shortcut│ │      │ │
│  └─────┬─────┘ └───┬────┘ └───┬───┘ └──┬───┘ │
│        └───────────┴──────────┴─────────┘     │
│                   OS System APIs              │
└───────────────────────────────────────────────┘
```

### 前后端通信

**RPC 调用**（前端 → 后端）：

```typescript
// Wails 自动生成 TS 类型，直接调用 Go 方法
import { ListItems, PasteItem, UpdateSettings } from '../wailsjs/go/main/App'
const result = await ListItems({ keyword: '', type: '', page: 1, pageSize: 20 })
```

**事件推送**（后端 → 前端）：

```go
// Go 端
runtime.EventsEmit(ctx, "clipboard:new")
runtime.EventsEmit(ctx, "window:show")
```

```typescript
// 前端
EventsOn('clipboard:new', () => refresh())
EventsOn('window:show', () => onWindowShow())
```

---

## 开发工作流

### 常用命令

```bash
make dev          # 热重载开发（前后端同时）
make debug        # 开发模式 + DevTools
make test         # 运行单元测试
make test-cover   # 测试 + HTML 覆盖率报告
make lint         # go vet 静态检查
make generate     # 重新生成前端 TS 绑定（修改 app.go 后执行）
make tidy         # go mod tidy
make info         # 显示当前版本、Go、Node 等信息
make help         # 查看所有可用命令
```

### 修改后端 API 后

每次在 `app.go` 增减公开方法后，需重新生成前端 TS 绑定：

```bash
make generate
# 或
wails generate module
```

生成文件位于 `frontend/wailsjs/go/main/App.{js,d.ts}`，**不要手动修改**。

### 前端独立开发

```bash
make fe-dev     # 启动 Vite 开发服务器（仅前端，无 Go 后端）
make fe-build   # 仅构建前端
```

---

## 构建 & 发布

### 构建命令

| 命令 | 产物 | 备注 |
|------|------|------|
| `make build` | 当前平台 | 自动判断宿主 |
| `make build-win` | `GoPaste_x.x.x.exe` | Linux/Windows 均可构建 |
| `make build-win-arm` | `GoPaste_x.x.x_arm64.exe` | |
| `make build-mac` | `GoPaste_x.x.x.app` | **必须在 macOS 上运行** |
| `make build-mac-arm` | `GoPaste_x.x.x_arm64.app` | Apple Silicon |
| `make build-mac-intel` | `GoPaste_x.x.x_intel.app` | Intel |
| `make build-linux` | `GoPaste_x.x.x_linux_amd64` | **必须在 Linux 上运行** |
| `make build-all` | Win + Linux | 可在 Linux 上一次构建 |
| `make release` | 清理 + build-all | 发布前使用 |

产物统一输出到 `build/bin/`，命名规则：`GoPaste_<VERSION>[_variant].<ext>`。

### 跨平台说明

- **Windows**：所有 `//go:build windows` 代码均为纯 Go（无 CGO），Linux 和 macOS 可直接交叉编译。
- **macOS**：依赖 Objective-C CGO（`internal/tray/`、`internal/window/`、`internal/paste/`），**必须在 macOS 上构建**。
- **Linux**：依赖 gtk/webkit CGO，**必须在 Linux 上构建**（或使用 GitHub Actions）。

### 版本号管理

版本号的**唯一来源**是 `wails.json` 的 `productVersion`：

```json
"info": {
  "productVersion": "0.2.0"
}
```

构建时 `Makefile` 自动读取并注入到 `internal/updater.Version`：

```makefile
VERSION := $(shell node -e "console.log(require('./wails.json').info.productVersion)")
LDFLAGS := -s -w -X gopaste/internal/updater.Version=$(VERSION)
```

**发版步骤：**
1. 修改 `wails.json` 中的 `productVersion`
2. `make release` 构建全平台
3. `git tag vX.X.X && git push --tags`

---

## 图标管理

所有托盘图标统一存放于 `internal/tray/icons/`，由脚本从源图 `build/appicon.src.png` 生成：

| 文件 | 用途 | 生成方式 |
|------|------|---------|
| `tray-color.png` | macOS 菜单栏彩色 | `make gen-icons` → `gen_appicon.py` |
| `tray-gray.png` | macOS 菜单栏灰色 | `make gen-icons` → `gen_tray_icon_gray.py` |
| `tray-gray.ico` | Windows 托盘灰色 | `make gen-icons` → `gen_tray_icon_gray.py` |
| `tray-template.png` | macOS 自适应模板（备用） | `make gen-icon-template` |
| `tray.png` | Linux 托盘 | 手工维护（与源图同源） |
| `tray.ico` | Windows 托盘彩色 | 手工维护（与 `build/windows/icon.ico` 同源） |

**更换 App 图标：**

1. 替换 `build/appicon.src.png`（建议 1024×1024 正方形透明背景）
2. `make gen-icons` 重新生成所有托盘图标
3. 同步替换 `build/windows/icon.ico`（Wails 硬编码路径）
4. 重新构建

---

## 关键模块说明

### 剪贴板监听（`internal/clipboard`）

- 基于事件驱动（`golang.design/x/clipboard` Watch API），不可用时降级 500ms 轮询
- 300ms 防抖避免连续写入风暴
- 文本、图片、链接、代码（代码通过特征识别）、文件路径均支持
- 图片统一转 PNG 字节存储；原始文件写入 `~/<AppName>/images/`

### 加密存储（`internal/crypto`）

```
首次启动：
  生成随机 32 字节 DEK → keyring.Set("GoPaste", "dek", hex) → 系统 Keychain

后续启动：
  keyring.Get 取回 DEK → 解密每条 Content

算法：AES-256-GCM，每条记录独立 12 字节 nonce（附在密文头部）
```

### 模拟粘贴（`internal/paste`）

| 平台 | 实现 | 关键点 |
|------|------|--------|
| macOS | ResignKey → sleep 50ms → osascript `keystroke v cmd` | 必须先 ResignKey，否则键事件回落到自身导致死循环 |
| Windows | `SendInput` Shift↓ + Insert↓↑ + Shift↑ | Insert 需 `KEYEVENTF_EXTENDEDKEY` + 正确 `wScan`，否则 NumLock 影响 |
| Linux | `xdotool key ctrl+v`（X11） | Wayland 下不支持，降级为仅复制到剪贴板 |

### 系统托盘（`internal/tray`）

macOS 使用纯 CGO NSStatusItem（绕开 `fyne.io/systray`），原因：fyne/systray 在 macOS 上将 Go GC 管理的对象设为 ObjC target，GC 回收后触发野指针 SIGSEGV。

Windows 通过 `Shell_NotifyIcon NIM_DELETE/NIM_ADD` 平滑切换图标显隐，无需重启 systray 消息循环。

彩色/灰色图标风格切换通过 `tray.SetIconStyle("color"|"gray")` 实现，macOS 和 Windows 均支持。

### 窗口管理（`internal/window`）

- macOS：`NSWindow → GoPasteNSPanel`（NSPanel 子类），允许非激活状态下响应键盘事件
- Windows：DWM 圆角（`DwmSetWindowAttribute DWMWCP_ROUND`）、任务栏图标显隐（`SetWindowLong GWL_EXSTYLE`）
- 窗口位置策略：居中 / 记住上次 / 跟随鼠标（可在设置中切换）

---

## 平台特殊处理

### macOS

- **CGO 依赖**：`internal/tray/`、`internal/window/`、`internal/paste/`、`internal/clipboard/` 均有 `.m` 文件，必须在 macOS 上编译
- **辅助功能权限**：模拟粘贴需要 TCC `Accessibility` 权限，ad-hoc 签名下每次升级后 CDHash 变化，需用户重新授权
- **Gatekeeper**：未公证签名，需用户手动允许；详见 `docs/macos-accessibility.md`

### Windows

- **无 CGO**：所有 `//go:build windows` 文件均为纯 Go，可在 Linux/macOS 上交叉编译
- **WebView2**：Win11 自带，Win10 需 [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)
- **窗口圆角**：`internal/window/corners_windows.go` 通过 DWM API 实现 Win11 原生圆角

### Linux

- **X11 only**：全局快捷键和模拟粘贴依赖 X11，Wayland 下功能降级
- **系统托盘**：需要桌面环境支持 `StatusNotifierItem`（KDE/GNOME 均支持）
- **构建依赖**：需 `libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev libxtst-dev`

---

## 代码规范

### Go

- 平台差异代码用 `//go:build <platform>` 拆分到独立文件，不用 `runtime.GOOS` 判断
- 公开方法必须有注释（`// FuncName 描述...`）
- 使用 `log/slog` 结构化日志，禁止 `fmt.Println` 调试输出

### 前端

- 图标**必须**使用 `lucide-vue-next`，禁止使用 emoji 或其他图标库作为 UI 图标
- 图标默认 16px，标题栏 14px，空状态 48px
- 国际化字符串统一在 `i18n.ts` 中管理，不硬编码中文字符串（除组件自身名称）
- 新增设置项必须同时更新：`settings.go`（结构体+默认值）、`Settings.vue`（UI）、`i18n.ts`（翻译）

### Commit 规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
feat: 添加新功能
fix: 修复 bug
refactor: 代码重构（无功能变化）
docs: 文档更新
chore: 构建/工具/依赖变更
perf: 性能优化
```

---

## 已知问题 & Todo

详见 [todo.md](todo.md)，以下为重点待办：

**P0（影响核心体验）：**
- [ ] `BackgroundColour` 深色模式下拉伸窗口闪浅灰（需动态跟随主题）

**P1（代码质量）：**
- [ ] `app.go` 超 1300 行，建议拆分为 `app_clipboard.go` / `app_window.go` / `app_settings.go`
- [ ] `App.vue` 超 1000 行，建议拆分 `ItemList` / `ContextMenu` / `ConfirmDialog` 等组件
- [ ] Linux Wayland 粘贴支持（`wtype` 或 D-Bus）

**P2（工程化）：**
- [ ] GitHub Actions CI（PR lint/test + tag 自动构建三端 + 发 Release）
- [ ] CHANGELOG.md
- [ ] 英文版 README

---

## 相关文档

- [DESIGN.md](DESIGN.md) — 架构设计方案（早期，部分内容已过时）
- [todo.md](todo.md) — 改进事项清单（持续更新）
- [macos-accessibility.md](macos-accessibility.md) — macOS 辅助功能权限详解
- [win-release.md](win-release.md) — Windows 发布 & 代码签名说明
