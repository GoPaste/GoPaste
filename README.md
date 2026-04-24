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
