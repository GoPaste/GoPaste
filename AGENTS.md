# CLAUDE.md

> 本文件提供给 Claude Code / CodeBuddy 等 AI 协作工具，用于快速理解 GoPaste 仓库结构与开发约束。
> **修改本仓库代码前，请先阅读本文件以及 `.codebuddy/rules/` 下的硬性规范。**

---

## 1. 项目定位

**GoPaste** — 轻量、快速、安全的跨平台剪贴板管理工具。

- 技术栈：**Wails v2** (Go 1.25 + WebView) + **Vue 3 + TypeScript + Vite**
- 目标平台：Windows (x64/x86/arm64) / macOS (Universal/arm64/Intel) / Linux (x64)
- 数据存储：本地 SQLite（`glebarez/sqlite`，纯 Go 无 CGO）+ AES-256-GCM 加密 + 系统 Keychain 托管密钥
- 全局热键：默认 `Alt + \``（`golang.design/x/hotkey`）
- 单实例：`allan-simon/go-singleinstance`
- 自动更新：`creativeprojects/go-selfupdate`（GitHub Releases 源）

### 当前版本来源
唯一权威：`wails.json` → `info.productVersion`（Makefile 与 CI 都从此读取）。

---

## 2. 仓库结构速查

```
gopaste/
├── main.go                 # 入口：boot probe / stderr 重定向 / 信号处理 / 单实例 / wails.Run
├── app.go                  # Wails 绑定结构体 App，30+ RPC 方法（前端可调用）
├── wails.json              # Wails 配置（版本号唯一权威）
├── Makefile                # 构建/开发/打包入口（见 §4）
├── go.mod / go.sum         # Go 1.25，模块名 gopaste
│
├── frontend/               # Vue 3 前端
│   ├── src/
│   │   ├── App.vue         # 主面板（注意：当前单文件较大，待按域拆分）
│   │   ├── views/          # Settings.vue 等
│   │   ├── components/     # 组件目录（目前为空，期望逐步迁入）
│   │   ├── i18n.ts         # 手写 i18n（zh-CN / zh-TW / en）
│   │   ├── main.ts
│   │   └── style.css
│   ├── wailsjs/            # Wails 自动生成的 Go 绑定（不要手改）
│   └── package.json        # 依赖：vue + lucide-vue-next（图标库）
│
├── internal/               # Go 后端，按领域拆包
│   ├── appguard/           # 单实例守卫
│   ├── clipboard/          # 剪贴板监听/写入（按 OS 拆分文件，后缀 _darwin/_linux/_windows）
│   ├── config/             # 路径解析（用户配置目录）
│   ├── crypto/             # AES-GCM cipher + keyring 集成
│   ├── cursor/             # 鼠标位置（用于面板定位）
│   ├── hotkey/             # 全局快捷键（mods/keys 按 OS 拆分）
│   ├── logger/             # 文件日志 + RedirectStderr（捕获 panic 栈）
│   ├── paste/              # 模拟粘贴 Cmd+V/Ctrl+V（按 OS 拆分）+ 焦点跟踪
│   ├── settings/           # 用户设置持久化
│   ├── storage/            # GORM Repo（SQLite）
│   ├── tray/               # 系统托盘（macOS 用 Objective-C 桥实现 NSStatusItem）
│   ├── types/              # 共享类型 (Item / SearchQuery / ListResult)
│   ├── updater/            # 自更新（GitHub Releases）+ Version 注入
│   └── window/             # Wails 窗口选项 + macOS NSPanel 改造（关键差异化）
│
├── scripts/                # 图标生成（Python + Go）
├── build/                  # Wails 资源（图标、Info.plist 模板）
├── docs/                   # 设计/分析文档（中英混合）
└── .github/workflows/      # CI：build.yml（5 矩阵全平台打包，tag 触发）
```

### `internal/` 关键命名约定
- **`*_darwin.go` / `*_linux.go` / `*_windows.go`** — Go build tag 自动按 OS 编译
- **`*_darwin.m`** — Objective-C 文件，由 cgo 编译进 darwin 二进制（NSPanel/NSStatusItem 涉及）
- **`*_other.go`** — 非 darwin 平台的桩实现
- **`*.disabled`** — 历史方案保留参考，不参与编译

---

## 3. 重要架构约定

### 3.1 后端
- `App` 结构体在 `app.go`，**所有前端可调用方法都在这里**（GoDoc 注释必填）
- 添加新 RPC 方法后，前端 TS 绑定需 `make generate` 重新生成（输出到 `frontend/wailsjs/`）
- 跨平台逻辑统一通过 build tag 拆分文件，**不要在同一文件用 `runtime.GOOS` 分支**（`PasteItem` 是历史例外，待重构）
- 任何 macOS Cocoa/AppKit 调用必须保证在主线程，参考 `internal/tray/dispatch_darwin.m`
- 粘贴流水线串行化：`pasteMu` 不可省（重入会触发 macOS 硬崩，详见 `app.go` 注释）

### 3.2 前端
- 入口 `main.ts` → `App.vue`（当前是大文件，新功能优先拆到 `components/` 或 `views/`）
- **图标必须用 Lucide**（见 §5），禁止 emoji 作为 UI 图标
- i18n 走自写的 `i18n.ts`（短期内不引入 vue-i18n，避免破坏现有 key）
- 与后端通信只能通过 `frontend/wailsjs/go/main/App` 暴露的方法，不要 fetch/axios

### 3.3 日志与崩溃诊断
GoPaste 在崩溃诊断上比同类项目更下功夫，修改时请保留这些机制：
- `main.go::bootProbe` — 不依赖任何抽象层的 boot 日志，写到 `~/UserConfigDir/gopaste/gopaste.boot.log`
- `logger.RedirectStderr()` — 通过 dup2 把 fd2 重定向到文件，确保 Go runtime panic 栈不丢
- `debug.SetTraceback("crash")` — 等价 `GOTRACEBACK=crash`，崩溃时 dump 所有 goroutine

### 3.4 macOS 特殊性（不可回退）
- `internal/window/panel_darwin.m` 把 main window 改造为 **NSPanel**，实现"呼出后不抢主窗口焦点"
- 模拟粘贴需要"辅助功能"权限，每次升级用户需要在系统设置中先**删除旧授权再重新授权**（升级提示已在 README 中）
- macOS 版本未签名（ad-hoc），用户首次启动会被 Gatekeeper 拦截，由 README 提供绕过指引
- **系统对话框（SaveFileDialog 等）** 需要 active app 才能弹出。NonactivatingPanel 不是 active app，必须先调 `window.SaveFileDialog()`（原生 NSSavePanel，内部原子执行激活→弹框→恢复），不能用 `wailsruntime.SaveFileDialog`
- **Alt+数字切 Tab 热键**：macOS 上 Option+数字被系统拦截（输入特殊字符），需改用 `Cmd+数字`；Windows/Linux 继续用 `Alt+数字`（见 `app.go registerHotkey()`）
- **粘贴流程**：`orderOut` 后 sleep 80ms 等 WindowServer 更新 frontmost，再用 `osascript` 投递 Cmd+V。启动时调 `paste.WarmupOsascript()` 预热，消除首次粘贴冷启动延迟
- **冷启动焦点**：`domReady` 时若非静默启动，需调 `ActivateForDialog` 临时激活窗口后立即 `DeactivateAfterDialog`，让搜索框获得焦点且不长期抢占前台

---

## 4. 常用命令（Makefile）

```bash
# 开发
make dev            # wails dev，热重载
make debug          # wails dev -devtools
make doctor         # 检查 Wails 环境

# 构建（当前平台）
make build          # 自动判断 macOS/Linux

# 构建（指定平台）
make build-win              # Windows amd64
make build-win-arm          # Windows arm64
make build-mac              # macOS Universal（仅 macOS 主机）
make build-mac-arm          # macOS Apple Silicon
make build-mac-intel        # macOS Intel
make build-linux            # Linux amd64（仅 Linux 主机）
make build-all              # Windows + Linux（macOS 必须在 Mac 上单独构建）

# 代码生成
make generate               # 重新生成前端 wailsjs/ TS 绑定
make gen-icons              # 重新生成 appicon + 灰色托盘图标
make gen-icon-template      # 重新生成 macOS 模板图标 tray-template.png

# Go 工具链
make tidy                   # go mod tidy
make test                   # 单元测试（带 -race）
make test-cover             # 测试 + HTML 覆盖率报告
make lint                   # go vet
make bench                  # 性能测试

# 前端
make fe-install / fe-build / fe-dev

# 清理
make clean                  # 清理 build/bin 与 frontend/dist
make clean-all              # 含 node_modules

# 信息
make info                   # 显示版本/Go/Node/Wails
```

> **平台限制**：Wails 不支持 CGO 跨平台编译。
> - macOS 包必须在 macOS 上构建；Linux 包必须在 Linux 上构建
> - 推荐做法：本地只跑 `make dev` / `make build-win`，正式发版走 `git tag vX.Y.Z && git push --tags`，由 GitHub Actions 完成全平台打包

---

## 5. 硬性项目规范

### 5.1 图标规范（强制）
**详见 `.codebuddy/rules/gopaste图标规范.md`**，要点：
- 唯一图标库：**`lucide-vue-next`**
- **禁止** emoji 作为 UI 图标（`📋 🔍 ⭐ 📌 ⚙` 等都不允许出现在按钮/列表/标题栏）
- **禁止** 混用其他图标库（Font Awesome / Material Icons 等）
- 默认尺寸：列表/按钮 16px、标题栏 14px、空状态 48px
- 颜色通过 CSS `color` 继承，不要写死 `stroke` / `fill`
- 文案/日志/Git commit 中保留 emoji 不受此约束

#### 常用映射
| 用途 | Lucide |
|------|--------|
| 搜索 | `Search` |
| 收藏 | `Star` |
| 置顶 | `Pin` |
| 删除 | `Trash2` |
| 查看 | `Eye` |
| 设置 | `Settings` |
| 关闭/最小化/最大化 | `X` / `Minus` / `Square` |
| 返回 | `ArrowLeft` |
| 导出 | `Download` |
| 类型图标 | `FileText` / `Image` / `Link` / `Code2` |
| 空状态 | `ClipboardList` |

### 5.2 文案与注释
- 所有前端 UI 提示文字保持**中文**（其他语言由 `i18n.ts` 翻译）
- 新增 RPC 方法（`app.go` 内的 App 方法）必须写 GoDoc 注释
- `internal/` 下每个包必须有 package-level 注释
- 复杂的跨平台分支或 macOS Objective-C 桥接，**必须写注释解释「为什么」**（参考 `app.go` 中 `pasteMu` 注释风格）

### 5.3 wailsjs 绑定
- `frontend/wailsjs/` 由 Wails 构建时自动生成，**理论上不要手改**
- 添加新 RPC 方法或修改暴露结构体后，需运行 `make generate` 重新生成（需确保 `PATH` / `GOROOT` 指向 Go 1.25）
- `wailsjs/` 在 `.gitignore` 中被忽略；若 `wails generate module` 不生效，可从 `scripts/wailsjs-fallback/` 恢复备份并手动更新

### 5.4 双仓库同步
- **a 仓库**（私有）：`git@github.com:larkwins/GoPaste.git`，保存所有代码
- **b 仓库**（公开）：`git@github.com:GoPaste/GoPaste.git`，过滤掉 `docs/`、`scripts/`、`Makefile`、`.codebuddy/`
- 同步命令：`/sync-public`（见 `.codebuddy/commands/sync-public.md`），执行后还需单独 `git push public v{X.Y.Z}` 推送 tag 到 b 仓库

---

## 6. CI/CD

唯一 workflow：`.github/workflows/build.yml`

- 触发条件：push tag `v*` 或手动 `workflow_dispatch`
- 矩阵：6 个 job（mac arm64 / mac x64 / mac universal / win x64 含 NSIS / win x86 portable / linux x64）
- 产物命名规范：`GoPaste_{VERSION}_{OS_ARCH}{-portable|-setup}.{ext}`
- tag 触发会自动上传到 GitHub Release（`softprops/action-gh-release@v2`）
- Release Notes 分类规则：`.github/release.yml`

发布流程：
```bash
# 1. 改 wails.json 的 productVersion
# 2. 提交并打 tag
git tag v0.2.1
git push origin v0.2.1
# 3. CI 自动跑完 → Release 自动生成
# 4. 同步到公开仓库
# 执行 /sync-public，然后：
git push public v0.2.1
```

---

## 7. 修改代码时的检查清单

提交前自查：

### 后端
- [ ] 跨平台代码用 build tag 拆文件，不在同一文件用 `runtime.GOOS`
- [ ] 新增 `App` 方法有 GoDoc 注释
- [ ] 修改了 `App` 公开方法 → 跑 `make generate` 同步前端绑定
- [ ] macOS Cocoa 调用确认在主线程
- [ ] `make tidy && make lint && make test` 全部通过

### 前端
- [ ] 没有引入新的图标库依赖
- [ ] 没有用 emoji 充当 UI 图标
- [ ] Lucide 图标尺寸符合 16/14/48 分级
- [ ] 没有手改 `frontend/wailsjs/`（自动生成目录，手改会被下次构建覆盖）
- [ ] `cd frontend && npm run build` 通过（`vue-tsc --noEmit`）
- [ ] macOS 平台特有逻辑（对话框、热键修饰键等）已做平台判断

### 通用
- [ ] 不修改 `wails.json` 的 `productVersion` 除非在发版
- [ ] 不删除 `bootProbe` / `RedirectStderr` / `pasteMu` 等关键诊断或同步机制
- [ ] 文档 `.md` 用 LF 换行；不要无意中提交 `.bak` / `.disabled` 之外的临时文件

---

## 8. 文档索引（`docs/`）

- `DESIGN.md` — 总体设计
- `CONTRIBUTING.md` — 贡献指引
- `clipboard-comparison.md` — 同类工具对比
- `ecopaste-analysis.md` — Ecopaste 对比分析
- `macos-accessibility.md` — macOS 辅助功能权限说明
- `wails_v3.md` — Wails v3 升级评估
- `win-release.md` — Windows 发布注意事项
- `plugins.md` — 插件机制规划
- `github-release-notes.md` — Release Notes 配置与 PR 工作流说明
- `product-website-plan.md` — 产品官网搭建方案（Rspress / Cloudflare Pages）
- `todo.md` — 路线图

---

## 9. 已知技术债（按优先级）

参考 `docs/tiny-rdm-analysis.md`，当前主要待办：

1. **P1 前端组件化**：`App.vue` 与 `views/Settings.vue` 单文件较大，应按域拆分到 `components/`
2. **P1 状态管理**：考虑引入 Pinia（替代 props/emit 透传）
3. **P2 后端 api 分层**：把 `app.go` 中 30+ RPC 按域拆到 `internal/api/` + `internal/service/`
4. **P2 Linux 分发**：补 deb / rpm / AppImage（当前只有 tar.gz）
5. **P3 i18n 工程化**：`i18n.ts` → 评估迁移到 `vue-i18n`（保持 key 兼容）

> 重构时请保留 §3.3、§3.4 列出的优势机制，不要"为了统一"删掉。
