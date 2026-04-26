# gopaste

> 基于 **Wails v2 + Go + Vue 3 + TypeScript** 的轻量跨平台剪切板管理工具。

## 特性

- 🕐 自动记录剪切板历史（文本 + 图片）
- 🔍 全文搜索 · 类型过滤（文本 / 图片 / 链接 / 代码）
- ⭐ 收藏、📌 置顶
- ⌨️  全局快捷键 `Ctrl/Cmd + Shift + V` 呼出
- 🔐 本地 AES-256-GCM 加密存储，密钥存于系统 Keychain
- 🖥  Windows / macOS / Linux 三端支持

详细设计见 [DESIGN.md](./DESIGN.md)。

## 安装与升级须知

### macOS

GoPaste 当前发行版**未使用 Apple Developer 证书签名**（采用 ad-hoc 签名），
因此 macOS 下用户可能遇到以下三种现象，按出现顺序说明。

#### 现象 1：首次打开提示"无法打开，来自身份不明的开发者"

最常见的情况。从 GitHub Releases 下载 `.dmg` 后双击 `GoPaste.app` 时弹出：

> 无法打开 "GoPaste"，因为它来自身份不明的开发者。

**解决方法：**

1. 把 `GoPaste.app` 从 dmg 拖到 `应用程序`（Applications）
2. 双击运行（被拒绝是预期，先关掉提示框）
3. 打开 **系统设置 → 隐私与安全性**
4. 滚动到底部"安全性"区域，会看到一行 *"已阻止使用 GoPaste，因为来自身份不明的开发者"*，
   点右边的 **"仍要打开"**
5. 在弹出的二次确认框中点 **"打开"**

之后双击即可正常启动，系统会记住这次授权。

> 也可以用更快的方式：在 Finder 中**右键（Control + 单击）GoPaste → 打开**，
> 弹框里点"打开"。效果一样。

#### 现象 2：提示"GoPaste.app 已损坏，无法打开"

少数情况会直接显示：

> "GoPaste.app" 已损坏，无法打开。你应该将它移到废纸篓。

这**并非文件真的损坏**，而是 macOS 14+ 在某些场景下对 ad-hoc 签名 + quarantine
属性的组合更严格，连"仍要打开"按钮都不给。

**解决方法：** 打开 **终端（Terminal）**，执行：

```bash
xattr -cr /Applications/GoPaste.app
```

该命令递归清除 .app 的扩展属性（含触发 Gatekeeper 的 `com.apple.quarantine`），
再双击即可正常打开。

#### 现象 3：粘贴失效，"辅助功能"开关看着却是开的（首次安装 / 每次升级后）

GoPaste 通过模拟 `Cmd+V` 把内容粘贴到目标应用，需要在
**系统设置 → 隐私与安全性 → 辅助功能** 中授权 GoPaste。

首次安装时，第一次触发粘贴会弹出系统授权框，按引导授权即可。

但**每次升级到新版本后，原有授权会失效**：

- 系统设置里"辅助功能"列表中 GoPaste 开关看起来仍是开着的 ✅
- 但点击粘贴时面板消失、内容没粘出来 ❌

原因：ad-hoc 签名下 macOS TCC 按二进制哈希（CDHash）追踪授权，
新版二进制 CDHash 变了，旧授权失效——但 UI 仍按 bundle id 显示，
导致看起来"还在授权列表里、却不生效"。

**解决方法：**

1. 打开 **系统设置 → 隐私与安全性 → 辅助功能**
2. **选中 GoPaste，点 −（减号）删除这条记录**
3. 回到 GoPaste 再触发一次粘贴，系统会重新弹框请求授权
4. 勾选后即恢复正常

> 详细原理与开发者侧的彻底解决方案（Developer ID 签名）见
> [`docs/macos-accessibility.md`](./docs/macos-accessibility.md)。

### Windows / Linux

无上述限制，正常安装升级即可。

## 开发环境

- Go 1.22+
- Node.js 20+
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### 各平台系统依赖

| 平台 | 依赖 |
|------|------|
| Linux (Debian/Ubuntu) | `libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev libxtst-dev` |
| Linux (RHEL/TencentOS) | `gtk3-devel webkit2gtk4.0-devel libX11-devel libXtst-devel` |
| macOS | Xcode Command Line Tools |
| Windows | WebView2 Runtime（Win11 自带） |

## 常用命令

项目提供了 `Makefile`，运行 `make help` 查看所有可用命令：

### 开发

| 命令 | 说明 |
|------|------|
| `make dev` | 热重载开发模式 |
| `make debug` | 开发模式 + DevTools |

### 构建

| 命令 | 说明 |
|------|------|
| `make build` | 构建当前平台 |
| `make build-win` | 构建 Windows x64 |
| `make build-win-arm` | 构建 Windows ARM64 |
| `make build-mac` | 构建 macOS Universal |
| `make build-mac-arm` | 构建 macOS Apple Silicon |
| `make build-mac-intel` | 构建 macOS Intel |
| `make build-linux` | 构建 Linux x64 |
| `make build-all` | 全平台一次构建 |
| `make release` | 清理 + 全平台构建（发布用） |

### 测试 & 检查

| 命令 | 说明 |
|------|------|
| `make test` | 运行单元测试 |
| `make test-cover` | 测试 + 覆盖率 HTML 报告 |
| `make bench` | 性能测试 |
| `make lint` | Go vet 静态检查 |

### 前端

| 命令 | 说明 |
|------|------|
| `make fe-install` | 安装前端依赖 |
| `make fe-build` | 仅构建前端 |
| `make fe-dev` | 前端独立开发服务器 |

### 工具 & 环境

| 命令 | 说明 |
|------|------|
| `make generate` | 重新生成前端 TS 绑定 |
| `make tidy` | go mod tidy |
| `make doctor` | 检查 Wails 环境 |
| `make install-deps` | 安装 Linux 系统依赖（自动识别 apt/yum） |
| `make install-wails` | 安装 Wails CLI |

### 清理 & 信息

| 命令 | 说明 |
|------|------|
| `make clean` | 清理构建产物 |
| `make clean-all` | 深度清理（含 node_modules） |
| `make info` | 显示项目/环境信息 |
| `make help` | 显示所有命令 |

## 目录结构

```
gopaste/
├── app.go                 # Wails RPC 绑定层
├── main.go                # 入口
├── internal/
│   ├── types/             # 共享数据结构
│   ├── clipboard/         # 剪切板监听
│   ├── storage/           # SQLite 持久化
│   ├── crypto/            # AES-256-GCM + Keyring
│   ├── hotkey/            # 全局快捷键
│   ├── paste/             # 写回剪切板 & 模拟粘贴（平台分文件）
│   └── config/            # 数据目录
└── frontend/              # Vue 3 前端
```

## License

MIT
