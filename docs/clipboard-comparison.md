# 剪切板监听方案对比

> gopaste 项目中两种剪切板监听方案的技术对比与选型记录。

---

## 方案概览

| 维度 | 方案 A：`golang.design/x/clipboard` | 方案 B：原生平台 API（自实现） |
|------|------|------|
| 用途 | 文本 + 图片 | 文件（路径列表） |
| 实现方式 | 第三方库，CGO 调用各平台 API | 分平台 Go 文件（build tag 隔离） |
| 适用场景 | 主流剪切板内容（文/图） | 文件管理器复制文件 |
| 当前状态 | ✅ 在用（主链路） | ✅ 在用（feat/file-clipboard 分支） |

gopaste 同时使用两种方案：方案 A 负责文本和图片，方案 B 负责文件。

---

## 方案 A：`golang.design/x/clipboard`

### 简介

- **仓库**：https://github.com/nicefail/clipboard（原 golang.design/x/clipboard）
- **协议**：MIT
- **实现**：CGO，各平台原生 C/ObjC 代码
- **API**：`clipboard.Watch(ctx, FmtText/FmtImage) <-chan []byte`

### 支持格式

| 格式 | 常量 | 说明 |
|------|------|------|
| 纯文本 | `FmtText` | UTF-8 字符串 |
| PNG 图片 | `FmtImage` | PNG 字节流 |

**不支持**：文件（CF_HDROP）、富文本（RTF）、HTML、自定义格式。

### 各平台实现细节

#### Windows
| 项 | 说明 |
|----|------|
| 底层 API | Win32 `OpenClipboard` / `GetClipboardData` |
| 文本格式 | `CF_UNICODETEXT` |
| 图片格式 | `CF_DIB`（设备无关位图）→ 库内部转 PNG |
| Watch 机制 | **事件驱动**：`AddClipboardFormatListener` + `WM_CLIPBOARDUPDATE` |
| 性能 | 最优：零轮询，事件即时通知 |
| 额外依赖 | 无 |

#### macOS
| 项 | 说明 |
|----|------|
| 底层 API | Objective-C `NSPasteboard` |
| 文本格式 | `NSPasteboardTypeString` |
| 图片格式 | `NSPasteboardTypePNG` |
| Watch 机制 | **轮询**：定时检查 `NSPasteboard.changeCount`（macOS 无剪切板变更事件） |
| 性能 | 轻微 CPU 开销（<1%），可接受 |
| 额外依赖 | 无 |
| 注意 | 部分 App（1Password 等）使用"concealed"剪切板，读不到 |

#### Linux X11
| 项 | 说明 |
|----|------|
| 底层 API | X11 C API（`XGetSelectionOwner` / `XConvertSelection`） |
| 文本格式 | `UTF8_STRING` target |
| 图片格式 | `image/png` target |
| Watch 机制 | **事件驱动**：X11 Selection Event (`SelectionNotify`) |
| 性能 | 与 Windows 相当 |
| 额外依赖 | `xclip` 或 `xsel`（大多数桌面发行版默认已有） |

#### Linux Wayland
| 项 | 说明 |
|----|------|
| 底层 API | 通过外部工具 `wl-clipboard`（`wl-copy` / `wl-paste`） |
| Watch 机制 | 不稳定——Wayland 安全模型禁止非焦点窗口读取剪切板 |
| 性能 | 依赖子进程调用，较重 |
| 额外依赖 | 必须安装 `wl-clipboard` |
| 状态 | ⚠️ 部分桌面可用，不保证稳定 |

### 优点

1. **跨平台统一 API**：一套 `Watch` / `Read` / `Write` 接口三端通用
2. **事件驱动**（Windows/Linux X11）：无轮询，性能好
3. **成熟稳定**：GitHub 2k+ stars，持续维护
4. **图片自动转换**：各平台原生格式 → PNG 字节流

### 缺点

1. **仅支持文本 + 图片**：无法监听文件、富文本、HTML
2. **CGO 依赖**：交叉编译需要目标平台的 C 工具链（如 MinGW for Windows）
3. **Wayland 支持弱**：后台监听受安全模型限制
4. **v0.7.0+ 要求 Go 1.24**：较新版本可能需要升级 Go 版本

---

## 方案 B：原生平台 API（自实现文件监听）

### 简介

gopaste 自己编写的分平台文件剪切板监听模块，位于 `internal/clipboard/filewatcher_<os>.go`。

### 架构

```
filewatcher.go          ← 公共接口、轮询循环、去重
filewatcher_windows.go  ← Win32 CF_HDROP
filewatcher_darwin.go   ← macOS NSPasteboard + NSURL (CGO ObjC)
filewatcher_linux.go    ← xclip/wl-paste text/uri-list
```

### 各平台实现

#### Windows — `CF_HDROP`

```
OpenClipboard(0)
  → IsClipboardFormatAvailable(CF_HDROP)
  → GetClipboardData(CF_HDROP)
  → GlobalLock(hMem)
  → DragQueryFileW(hDrop, 0xFFFFFFFF, nil, 0)  // 获取文件数量
  → DragQueryFileW(hDrop, i, buf, len)          // 逐个获取路径
  → GlobalUnlock + CloseClipboard
```

| 项 | 说明 |
|----|------|
| API | Win32 syscall（不需要 CGO，纯 Go unsafe） |
| 格式 | `CF_HDROP`（格式 ID = 15） |
| 输出 | UTF-16 文件路径列表 |
| 依赖 | user32.dll / kernel32.dll / shell32.dll（Windows 自带） |

#### macOS — `NSPasteboard` + `NSURL`

```objc
NSPasteboard *pb = [NSPasteboard generalPasteboard];
NSArray *urls = [pb readObjectsForClasses:@[[NSURL class]]
                 options:@{NSPasteboardURLReadingFileURLsOnlyKey: @YES}];
// urls 包含 file:// URL 列表
```

| 项 | 说明 |
|----|------|
| API | Objective-C CGO |
| 格式 | `NSFilenamesPboardType` / `NSURL` |
| 输出 | POSIX 文件路径列表 |
| 依赖 | Cocoa framework（macOS 自带） |

#### Linux — `text/uri-list`

```bash
# X11
xclip -selection clipboard -target text/uri-list -o
# 输出: file:///home/user/photo.png\nfile:///home/user/doc.pdf

# Wayland
wl-paste --type text/uri-list
```

| 项 | 说明 |
|----|------|
| API | 子进程调用外部工具 |
| 格式 | `text/uri-list`（RFC 2483） |
| 输出 | `file://` URI 列表，需 URL decode |
| 依赖 | X11: `xclip`；Wayland: `wl-clipboard` |
| 注意 | 并非所有文件管理器都写 `text/uri-list`（大多数主流的如 Nautilus/Dolphin/Thunar 都会） |

### 监听机制

| 项 | 说明 |
|----|------|
| 方式 | **轮询**（500ms 间隔） |
| 去重 | 所有路径拼接后 SHA-256，与上次比较 |
| 性能 | Windows 每次 `OpenClipboard` 很轻（<1ms）；Linux 每次 fork xclip 较重（~5ms）但可接受 |

### 优点

1. **支持文件格式**：方案 A 不覆盖的盲区
2. **无第三方 Go 依赖**：纯 syscall / CGO
3. **平台隔离干净**：build tag 分文件，互不影响
4. **易于扩展**：后续可加富文本、HTML 等格式

### 缺点

1. **轮询方式**：不如事件驱动高效（但 500ms 对用户无感）
2. **需维护三份代码**：每新增一个格式需写三个平台文件
3. **Linux 依赖外部工具**：xclip/wl-paste 可能未安装
4. **写回剪切板**：把文件"重新复制回去"需要各平台单独写（当前暂用文本路径降级）

---

## 对比总结

| 维度 | 方案 A (`golang.design/x/clipboard`) | 方案 B（自实现原生 API） |
|------|:---:|:---:|
| 文本支持 | ✅ | ❌（不负责） |
| 图片支持 | ✅ | ❌（不负责） |
| 文件支持 | ❌ | ✅ |
| 富文本/HTML | ❌ | 🔮 可扩展 |
| 跨平台统一 API | ✅ | ⚠️ 需分文件实现 |
| Watch 机制 | 事件驱动（Win/X11）/ 轮询（macOS） | 轮询（500ms） |
| CGO 依赖 | 是 | Windows: 否（syscall）；macOS: 是（ObjC）；Linux: 否 |
| 第三方依赖 | `golang.design/x/clipboard` | 无 Go 依赖；Linux 需 xclip |
| 维护成本 | 低（库维护） | 中（自维护三平台代码） |
| 成熟度 | 高（2k+ stars） | 新写，需实际测试验证 |

## 选型决策

**两者互补，同时使用**：

```
┌─────────────────────────────────────────────┐
│           gopaste 剪切板监听                 │
│                                             │
│  ┌─────────────────────┐ ┌────────────────┐ │
│  │ golang.design/x/    │ │ 自实现原生 API  │ │
│  │ clipboard           │ │ (filewatcher)  │ │
│  │                     │ │                │ │
│  │ • FmtText (文本)    │ │ • CF_HDROP     │ │
│  │ • FmtImage (图片)   │ │ • NSPasteboard │ │
│  │                     │ │ • uri-list     │ │
│  │ Watch 事件驱动      │ │ 500ms 轮询     │ │
│  └─────────┬───────────┘ └───────┬────────┘ │
│            │                     │          │
│            ▼                     ▼          │
│       consumeEvents()    consumeFileEvents()│
│            │                     │          │
│            └────────┬────────────┘          │
│                     ▼                       │
│              repo.Save() → 前端通知          │
└─────────────────────────────────────────────┘
```

这样做的好处：
- **不重复造轮子**：文本/图片走成熟的第三方库
- **补齐短板**：文件格式走自实现
- **未来可扩展**：方案 B 可继续增加富文本、HTML 等格式的监听
