# GoPaste · 跨平台剪切板管理工具 · 实施方案

> 基于 **Wails v2 + Go + Vue 3 + TypeScript** 构建，支持 Windows / macOS / Linux。

---

## 1. 项目愿景

打造一款 **轻量、快速、安全** 的跨平台剪切板管理工具，核心价值：

- **即按即用**：全局快捷键一秒呼出，无需切换窗口
- **不丢历史**：后台自动记录文本与图片，永不丢失重要片段
- **隐私优先**：本地加密存储，数据永不上云（除非用户显式开启）
- **原生体验**：三平台均使用系统原生 WebView，安装包 ≤ 20MB，冷启动 < 500ms

---

## 2. 功能需求（MVP + 增强）

### 2.1 MVP 核心功能（P0）

| 编号 | 功能 | 描述 |
|------|------|------|
| F-01 | 剪切板监听 | 后台监听系统剪切板变化，捕获文本 / 图片 |
| F-02 | 历史记录 | 本地 SQLite 持久化，按时间倒序排列 |
| F-03 | 去重 | 相同内容合并（刷新 updated_at，保留一条） |
| F-04 | 全局快捷键 | 默认 `Ctrl/Cmd + Shift + V` 呼出主面板 |
| F-05 | 搜索过滤 | 关键词实时搜索 + 类型筛选（全部/文本/图片/链接/代码） |
| F-06 | 快速粘贴 | 点击/回车 → 复制回剪切板 → 模拟粘贴到目标应用 |
| F-07 | 系统托盘 | 常驻托盘，支持显示/隐藏、退出、设置入口 |
| F-08 | 收藏 & 置顶 | 标记收藏项永久保留，置顶项始终在列表顶部 |
| F-09 | 加密存储 | 使用 AES-256-GCM 加密敏感字段，密钥由系统 Keychain 管理 |

### 2.2 增强功能（P1，后续迭代）

- 类型自动识别：URL / 邮箱 / 颜色值 / JSON / 代码片段（附语法高亮）
- 条目编辑、分组/标签
- 快捷键自定义
- 主题切换（亮/暗/跟随系统）
- 多语言（中英）
- 清理策略（按时长/条数/大小自动清理）
- 导入导出 JSON

### 2.3 非功能需求

| 维度 | 目标 |
|------|------|
| 冷启动 | < 500 ms |
| 呼出延迟 | < 100 ms |
| 内存占用 | 空闲态 < 60 MB |
| 安装包 | 单平台 < 20 MB |
| 剪切板轮询 | 500 ms / 次（或事件驱动） |
| 历史上限 | 默认 1000 条 + 所有收藏项 |

---

## 3. 技术选型

### 3.1 核心依赖

| 分类 | 选型 | 说明 |
|------|------|------|
| 桌面框架 | `github.com/wailsapp/wails/v2` | Go ↔ JS 双向调用，体积小 |
| 剪切板 | `golang.design/x/clipboard` | 跨平台，支持文本 + 图片（PNG） |
| 全局快捷键 | `golang.design/x/hotkey` | 三端统一 API |
| 数据库 | `modernc.org/sqlite` | **纯 Go，无需 CGO**，便于跨平台编译 |
| ORM | `gorm.io/gorm` + `gorm.io/driver/sqlite` | 简化 CRUD |
| 加密 | 标准库 `crypto/aes`, `crypto/cipher` | AES-256-GCM |
| Keychain | `github.com/zalando/go-keyring` | 系统密钥环（Win Credential / macOS Keychain / Linux Secret Service） |
| 自动粘贴 | `github.com/go-vgo/robotgo`（仅用模拟按键，不打包到主流程） 或 平台原生方案（详见 §5.5） | 模拟 `Ctrl/Cmd+V` |
| 日志 | `log/slog`（Go 1.21+） | 结构化日志 |
| 配置 | `github.com/spf13/viper` | YAML 配置 |

### 3.2 前端技术栈

| 分类 | 选型 |
|------|------|
| 框架 | Vue 3 (Composition API) |
| 语言 | TypeScript |
| 构建 | Vite（Wails v2 默认） |
| 状态 | Pinia |
| 样式 | UnoCSS（原子化，体积小） |
| 工具 | VueUse |
| 虚拟列表 | `vue-virtual-scroller`（历史大量条目时性能保障） |
| 图标 | Iconify (`@iconify/vue`) |
| 代码高亮 | `shiki`（按需加载） |

### 3.3 开发工具链

- Go 1.22+
- Node.js 20+ / pnpm
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- 平台依赖：
  - Linux: `libgtk-3-dev libwebkit2gtk-4.1-dev`
  - macOS: Xcode Command Line Tools
  - Windows: WebView2（Win11 自带；Win10 需安装）

---

## 4. 架构设计

### 4.1 总览

```
┌──────────────────────────────────────────────────┐
│                  Frontend (Vue 3)                │
│  ┌────────┐ ┌──────────┐ ┌────────┐ ┌─────────┐ │
│  │ 搜索栏 │ │ 剪切板列表│ │ 详情页 │ │ 设置页  │ │
│  └────────┘ └──────────┘ └────────┘ └─────────┘ │
│         ▲ Pinia Store ▲   ▲ Event Bus ▲         │
└─────────┼──────────────┼───┼────────────┼───────┘
          │ wails bind   │   │ runtime.EventsOn
          ▼              ▼   │            ▼
┌──────────────────────────────────────────────────┐
│                    Go Backend                    │
│  ┌────────────────────────────────────────────┐  │
│  │  App (Wails 绑定层, 暴露 RPC 方法)          │  │
│  └────────────────────────────────────────────┘  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌─────┐ │
│  │ Watcher  │ │ Storage  │ │ Hotkey   │ │Tray │ │
│  │(剪切板) │ │(SQLite+  │ │(全局快捷 │ │     │ │
│  │          │ │ Crypto)  │ │ 键)      │ │     │ │
│  └──────────┘ └──────────┘ └──────────┘ └─────┘ │
│         │           │            │          │   │
│         ▼           ▼            ▼          ▼   │
│   ┌──────────────────────────────────────────┐  │
│   │           OS System APIs                 │  │
│   │  Clipboard / Keychain / Hotkey / Tray    │  │
│   └──────────────────────────────────────────┘  │
└──────────────────────────────────────────────────┘
```

### 4.2 模块职责

| 模块 | 职责 | 关键接口 |
|------|------|---------|
| `app.go` | Wails 绑定入口，聚合各 Service | `Startup / Shutdown / GetItems / Search / Pin / Delete / ...` |
| `internal/clipboard` | 监听系统剪切板，识别类型 | `Watcher.Start(ctx) <-chan Item` |
| `internal/storage` | SQLite 持久化 + 加密 | `Repo.Save / List / Search / Pin / Delete` |
| `internal/crypto` | AES-256-GCM + 密钥管理 | `Cipher.Encrypt / Decrypt` |
| `internal/hotkey` | 全局快捷键注册 | `Hotkey.Register(key, fn)` |
| `internal/tray` | 系统托盘 | `Tray.Run(menu)` |
| `internal/paste` | 模拟粘贴到目标应用 | `Paste.SendPaste()` |
| `internal/config` | 用户配置读写 | `Config.Load / Save` |
| `internal/types` | 共享数据结构 | `Item`, `ItemType`, `SearchQuery` |

### 4.3 数据模型

```go
type ItemType string

const (
    TypeText  ItemType = "text"
    TypeImage ItemType = "image"
    TypeLink  ItemType = "link"  // 自动识别子类型
    TypeCode  ItemType = "code"
)

type Item struct {
    ID         int64     `gorm:"primaryKey"`
    Hash       string    `gorm:"uniqueIndex;size:64"` // SHA-256，去重
    Type       ItemType  `gorm:"index"`
    Content    []byte    // 文本或图片字节（加密后）
    Preview    string    // 前 200 字符明文摘要（仅用于检索，可选加密）
    Size       int64     // 原始大小
    Pinned     bool      `gorm:"index"`
    Favorite   bool      `gorm:"index"`
    SourceApp  string    // 来源应用（可选，Linux 难获取）
    CreatedAt  time.Time `gorm:"index"`
    UpdatedAt  time.Time
}
```

> **加密策略**：`Content` 全字段加密；`Preview` 默认明文以支持 SQL LIKE 搜索，若用户在设置中开启"严格加密模式"则改为全量内存搜索。

### 4.4 前后端通信

1. **RPC 调用**：前端直接调用 Go 方法（Wails 自动生成 TS 类型）
   ```ts
   import { GetItems, Pin } from '../wailsjs/go/main/App'
   const items = await GetItems({ page: 1, pageSize: 50 })
   ```

2. **事件推送**：Go → 前端（剪切板有新内容时）
   ```go
   runtime.EventsEmit(ctx, "clipboard:new", item)
   ```
   ```ts
   EventsOn('clipboard:new', (item) => store.prepend(item))
   ```

---

## 5. 关键技术难点与方案

### 5.1 剪切板监听

**方案**：使用 `golang.design/x/clipboard` 的 `Watch` API（基于事件），不可用时降级为 500ms 轮询。

```go
ch := clipboard.Watch(ctx, clipboard.FmtText)
for data := range ch { /* 处理文本 */ }
```

**难点**：
- 图片剪切板跨平台行为不一致 → 统一转为 PNG 字节保存
- 剪切板变化风暴（某些软件连续写入）→ 加入 300ms 防抖

### 5.2 全局快捷键

**方案**：`golang.design/x/hotkey`

```go
hk := hotkey.New([]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModShift}, hotkey.KeyV)
hk.Register()
go func() { for range hk.Keydown() { showWindow() } }()
```

**平台注意**：
- macOS：需在 `Info.plist` 声明 `NSAppleEventsUsageDescription`
- Linux：Wayland 下部分桌面环境不支持全局快捷键 → 提供"仅 X11"提示 + 备选方案（托盘点击）

### 5.3 自动粘贴

真正的"粘贴到目标应用"需要两步：
1. 将条目写回系统剪切板
2. 向**当前前台窗口**发送 `Ctrl/Cmd+V`

**方案分级**：

| 平台 | 首选 | 降级 |
|------|------|------|
| Windows | `SendInput` Win32 API（CGO 或 syscall） | 仅复制到剪切板，用户手动 Ctrl+V |
| macOS | CGSEvent / AppleScript `tell app "System Events" to keystroke "v"` | 同上 |
| Linux (X11) | `xdotool key ctrl+v`（外部依赖）或 XTest | 同上 |
| Linux (Wayland) | 无统一方案（`ydotool` 需 root） | 同上 |

**实现建议**：封装 `internal/paste` 模块，各平台独立文件（`paste_windows.go` / `paste_darwin.go` / `paste_linux.go`），失败时优雅降级并通知前端。

### 5.4 系统托盘

Wails v2 的 **Systray API（v2.9+）** 已原生支持，无需第三方库：

```go
options.App{
    Tray: &menu.Menu{ /* ... */ },
}
```

若版本不够则用 `getlantern/systray` 兜底。

### 5.5 加密存储

```
┌──────────────────────────────────────────────┐
│  首次启动：                                   │
│  1. 生成随机 32 字节 DEK（Data Encryption Key）│
│  2. 调用 keyring.Set("gopaste", "dek", hex)  │
│     → 存入系统 Keychain                       │
│                                              │
│  后续启动：                                   │
│  1. keyring.Get 取回 DEK                     │
│  2. 解密每条 Content                         │
└──────────────────────────────────────────────┘
```

- 算法：AES-256-GCM（每条独立 nonce，附加到密文头 12 字节）
- 若 Keychain 不可用（Linux 无 Secret Service）→ 回退到"密码派生密钥"（PBKDF2，首次设置主密码）

### 5.6 性能优化

- 列表虚拟滚动（`vue-virtual-scroller`），千条级无感
- 图片存储：原始字节放磁盘文件 `~/.gopaste/images/{hash}.png`，DB 只存路径；缩略图 200×200 嵌入 DB
- 搜索：Preview 字段 + SQLite FTS5 全文索引
- 窗口**隐藏≠销毁**：首次启动后常驻内存，呼出仅 `Show()`

---

## 6. 目录结构

```
gopaste/
├── DESIGN.md                    # 本文档
├── README.md
├── LICENSE
├── wails.json                   # Wails 项目配置
├── go.mod
├── go.sum
├── main.go                      # 入口
├── app.go                       # App 结构体（RPC 绑定）
├── build/                       # Wails 生成的构建产物
│   ├── appicon.png
│   ├── darwin/Info.plist
│   └── windows/gopaste.manifest
├── internal/
│   ├── types/
│   │   └── item.go
│   ├── clipboard/
│   │   ├── watcher.go
│   │   └── watcher_test.go
│   ├── storage/
│   │   ├── repo.go
│   │   ├── repo_test.go
│   │   └── migration.go
│   ├── crypto/
│   │   ├── cipher.go
│   │   ├── cipher_test.go
│   │   └── keyring.go
│   ├── hotkey/
│   │   └── hotkey.go
│   ├── tray/
│   │   └── tray.go
│   ├── paste/
│   │   ├── paste.go
│   │   ├── paste_windows.go
│   │   ├── paste_darwin.go
│   │   └── paste_linux.go
│   ├── config/
│   │   └── config.go
│   └── logger/
│       └── logger.go
└── frontend/
    ├── package.json
    ├── pnpm-lock.yaml
    ├── vite.config.ts
    ├── tsconfig.json
    ├── uno.config.ts
    ├── index.html
    ├── src/
    │   ├── main.ts
    │   ├── App.vue
    │   ├── router/index.ts
    │   ├── stores/
    │   │   ├── clipboard.ts
    │   │   └── settings.ts
    │   ├── views/
    │   │   ├── MainPanel.vue
    │   │   └── Settings.vue
    │   ├── components/
    │   │   ├── SearchBar.vue
    │   │   ├── ClipList.vue
    │   │   ├── ClipItem.vue
    │   │   ├── TypeFilter.vue
    │   │   └── DetailDrawer.vue
    │   ├── composables/
    │   │   ├── useClipboard.ts
    │   │   └── useHotkey.ts
    │   ├── types/index.ts
    │   ├── utils/format.ts
    │   └── styles/global.css
    └── wailsjs/                 # Wails 自动生成的 JS/TS 绑定
```

---

## 7. 前端 UI/UX 规划

### 7.1 主面板（呼出式）

```
┌──────────────────────────────────────────┐
│ 🔍 搜索...                        ⚙  ─ × │  ← 顶部栏
├──────────────────────────────────────────┤
│ [全部] [文本] [图片] [链接] [代码] ⭐   │  ← 类型过滤
├──────────────────────────────────────────┤
│ 📌 https://wails.io/docs/...     2m 前   │  ← 置顶
│ ───────────────────────────────────────  │
│ 📝 Lorem ipsum dolor sit amet...  5m 前  │
│ 🖼  [图片缩略图]      512 KB      12m 前 │
│ 🔗 github.com/wailsapp/wails     1h 前  │
│ ...                                      │
├──────────────────────────────────────────┤
│ 123 条记录 · ↑↓ 选择 · ↵ 粘贴 · Esc 关闭 │  ← 状态栏
└──────────────────────────────────────────┘
```

**交互**：
- `↑↓` 选中，`Enter` 粘贴并关闭，`Esc` 隐藏
- `Ctrl/Cmd+F` 聚焦搜索，`Ctrl/Cmd+D` 删除选中，`Ctrl/Cmd+P` 置顶切换
- 失焦自动隐藏（可配置）

### 7.2 设置页

- 常规：开机自启、历史上限、保留天数
- 快捷键：呼出/隐藏、删除、置顶
- 安全：严格加密模式、设置主密码
- 外观：主题、语言、窗口位置（居中/跟随鼠标）
- 数据：导出 / 导入 / 清空

---

## 8. 开发里程碑

| 阶段 | 周期 | 交付物 |
|------|------|--------|
| **M0 · 脚手架** | Day 1 | `wails init` 完成，能跑 Hello World，CI 打三端包 |
| **M1 · 监听 + 存储** | Day 2-3 | 剪切板监听 → SQLite 持久化，控制台能 `list` |
| **M2 · 基础 UI** | Day 4-5 | Vue 列表展示、搜索、删除、点击复制回剪切板 |
| **M3 · 快捷键 + 托盘** | Day 6 | 全局快捷键呼出/隐藏，托盘菜单 |
| **M4 · 加密 + 置顶 + 收藏** | Day 7-8 | AES-GCM + Keychain，置顶/收藏 UI |
| **M5 · 图片支持 + 自动粘贴** | Day 9-10 | 图片预览，三平台模拟粘贴 |
| **M6 · 设置页 + 打磨** | Day 11-12 | 设置页、主题、多语言基础 |
| **M7 · 打包发布** | Day 13-14 | GitHub Actions 三端打包，README，v0.1 发布 |

---

## 9. 测试策略

- **单元测试**：`internal/*` 每个包配 `*_test.go`，覆盖加密、去重、搜索
- **集成测试**：`testcontainers` 跑真实 SQLite；剪切板监听用 mock
- **端到端**：手动测试清单（平台矩阵 × 核心功能），后期接入 Playwright
- **性能测试**：benchmark 1w 条历史的搜索延迟

---

## 10. 风险与备选

| 风险 | 影响 | 应对 |
|------|------|------|
| Wayland 全局快捷键受限 | Linux 部分发行版功能降级 | 提示用户 + 托盘点击备选 |
| `robotgo` 依赖 CGO → 打包复杂 | 跨平台编译困难 | 剥离到独立模块，用 build tag，失败时降级 |
| 剪切板图片格式差异 | 丢失 HDR/透明通道 | 统一转 PNG，原始元数据存扩展字段 |
| SQLite 大图片膨胀 | 数据库变慢 | 图片落盘，DB 存路径 |
| Keychain 在无头 Linux 不可用 | 加密降级 | 主密码派生方案兜底 |
| macOS 签名公证 | 分发成本 | 初期提供无签名 zip + 自签名教程 |

---

## 11. 命令速查（开发者）

```bash
# 初始化
wails init -n gopaste -t vue-ts
cd gopaste

# 开发（热重载）
wails dev

# 构建当前平台
wails build -clean

# 跨平台交叉编译
wails build -platform windows/amd64
wails build -platform darwin/universal
wails build -platform linux/amd64

# 运行后端单测
go test ./internal/...
```

---

## 12. 下一步

本方案确认无误后，按 **M0 → M7** 顺序推进，每个里程碑提交一次可运行版本。

**待确认项**（无阻塞也可先开工，使用默认值）：
1. 应用图标是否需要我用 AI 生成？（默认：用剪贴板+闪电的简约图标）
2. 默认快捷键是否接受 `Ctrl/Cmd+Shift+V`？
3. 是否强制开启加密？（默认：开启，首次启动自动生成密钥）
4. License 选 MIT？（默认：MIT）
