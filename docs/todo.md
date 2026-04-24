# GoPaste 改进事项清单

> 最后更新：2026-04-23
> 本文根据当前代码现状 + `ecopaste-analysis.md` 对比结果整理，按**优先级**和**类别**分组。
> 勾选说明：`[ ]` 未做 · `[~]` 部分做 · `[x]` 已完成

---

## 🔥 P0：影响核心体验的问题

### 功能缺失

- [x] **开机自启动** ✅ 已接入 `emersion/go-autostart`（`internal/appguard/autostart.go`），设置页通用面板有开关。启动时会重新写入当前 exe 路径，兼容移动/升级。
- [x] **单实例保护** ✅ 已接入 `allan-simon/go-singleinstance`（`internal/appguard/singleinstance.go`），main.go 启动最早期检测，已有实例时直接 exit。
- [x] **自动更新检测** ✅ 已接入 `creativeprojects/go-selfupdate`（`internal/updater/`），关于页提供"检查更新"按钮。只做检测不自动替换 binary（安全起见让用户跳浏览器下载）。
- [x] **macOS 辅助功能权限引导** ✅ 已做：
  - `internal/paste/paste_darwin.go` 提供 `HasAccessibility()`（静默查）和 `RequestAccessibility()`（带系统弹框）两个 C 桥
  - `internal/paste/paste.go` `PasteItem()` 入口预检权限，未通过返回 `ErrNoAccessibility` 并异步触发引导弹框
  - `app.go` `showAccessibilityGuide()` 用 `sync.Once` 保证一次启动内只弹一次；"打开系统设置"按钮直跳 `x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility`
  - `app.go` `HasPastePermission()` / `RequestPastePermission()` 暴露为前端 RPC，方便 UI 主动查询
  - **坑点**：ad-hoc 签名下 TCC 按 CDHash 匹配，每次 rebuild 后"列表里勾着 ✓ 但系统仍判未授权"——引导文案已写明"先在列表里点 − 删除旧记录再重新授权"，完整说明见 `docs/macos-accessibility.md`

### Bug 类

- [ ] **BackgroundColour 硬编码浅色主题**
  `main.go` 里 `BackgroundColour: {R:245, G:245, B:245, A:255}`，深色主题用户拉伸窗口会闪一下浅灰。应该根据用户配置的主题动态设置。
- [ ] **Mac 端 `TitleBarHiddenInset` 的 topbar padding**
  `[data-os="mac"] .topbar { padding-left: 80px }` 写死 80px，不同 macOS 版本 traffic light 区域宽度不一，可能遮挡。考虑用 `env(titlebar-area-x)` 或动态测量。
- [ ] **`docs/issues.md` 只有标题没有描述**
  文件只是一个 TODO 草稿，内容形同虚设。要么补全变更历史，要么合并到本文件。

---

## 🟡 P1：影响易用性 / 代码质量

### 跨平台功能补齐

- [x] **Windows 粘贴快捷键改为 Shift+Insert** ✅ 已完成（2026-04-24）
  - 原因：Shift+Insert 是 Windows 自 DOS 起的通用粘贴键，兼容 cmd / PowerShell / Git Bash / WSL / RDP / VMware Console 等 Ctrl+V 不生效的场景。EcoPaste 同款方案。
  - 上次撤销的根因定位：Insert 是"扩展键"（和小键盘 Numpad0 共享 VK_INSERT=0x2D）。之前只改 VK 没设 `KEYEVENTF_EXTENDEDKEY`、也没填 `wScan`——NumLock 开时被当成 Numpad0 输入 "0"，NumLock 关时被多数应用忽略，Chromium/RDP 干脆不识别。
  - 现在实现（`internal/paste/paste_windows.go`）：
    1. `wScan` 用 `MapVirtualKeyW(VK, MAPVK_VK_TO_VSC)` 填入当前布局的硬件 scan code；
    2. Insert 键 `dwFlags |= KEYEVENTF_EXTENDEDKEY`，Shift 不加；
    3. 顺序 Shift↓ → Insert↓ → Insert↑ → Shift↑，与 enigo 的 `Key::Shift(Press) + Key::Other(0x2D)(Click) + Key::Shift(Release)` 等价；
    4. 保留 INPUT 40 字节 union 占位，防 cbSize 失败这个老坑。
- [ ] **追踪 Wails 对 `SetSkipTaskbar` 的支持**
  一旦 Wails 给 runtime 补上跨平台任务栏 API，可以删掉 `internal/window/taskbar_windows.go` 100+ 行手写 syscall。当前保持手写实现。
- [ ] **Linux 下 Wayland 粘贴支持**
  现在只写了 X11 路径，Wayland 下 `xdotool` 不工作。可用 `wtype` 或 D-Bus 接口 (`org.gnome.Shell.Introspect`)。
- [ ] **SQLite WAL / 备份**
  数据库突然掉电可能损坏。启用 WAL + 定时备份（每次启动时对 `clipboard.db` 做一份 `.bak`）。

### 代码质量

- [ ] **`app.go` 臃肿（840+ 行）**
  目前所有 Wails RPC 方法都塞在 `app.go` 里。拆分建议：
  - `app_clipboard.go` — 列表/搜索/增删
  - `app_window.go` — 窗口显隐/置顶
  - `app_settings.go` — 配置读写
  - `app_export.go` — 导出/清理
  - `app_url.go` — OpenURL / RevealInExplorer / SaveImageToFile
- [ ] **Lint 报的 3 个未使用变量**
  `frontend/src/App.vue`：`favoriteOnly`、`typeIconMap`、`ctxPin` 都声明了没用。要么用上，要么删掉。
- [ ] **wails.json 是否有平台分离的可能**
  EcoPaste 有 3 份 `tauri.{os}.conf.json`。Wails v2 目前只支持一份 `wails.json`——这是框架限制。先记录，等 Wails 升级观察。
- [ ] **去掉 go.mod 里没用的 replace/indirect**
  `go.mod` 末尾有一行 `// replace github.com/wailsapp/wails/v2 v2.12.0 => /root/go/pkg/mod`——看起来是调试残留。
- [ ] **前端 `App.vue` 超过 900 行**
  主组件承担太多职责：列表、搜索、详情、右键菜单、确认弹窗、备注弹窗全塞一起。考虑拆分：
  - `components/ItemList.vue`
  - `components/ItemDetail.vue`
  - `components/ContextMenu.vue`
  - `components/NoteDialog.vue`
  - `components/ConfirmDialog.vue`

### UX 改进

- [ ] **i18n 文案审计**
  `revealInExplorer` 简体是"在资源管理器中显示"，但 Mac 用户看到的应该是"在 Finder 中显示"。按平台动态切文案。
- [ ] **空状态 / 加载态**
  当前列表为空或搜索无结果时，是否有友好提示？"还没有任何记录，复制点东西试试？"
- [ ] **键盘导航**
  `↑/↓` 选择、`Enter` 粘贴、`Delete` 删除、`Esc` 关闭……这些有没有都做？无障碍访问也应该跟上。
- [ ] **图片预览大图**
  目前双击是粘贴，看图片大图可能需要右键 → 查看详情。是否支持 `Space` 预览？
- [ ] **链接预览（OG 卡片）**
  link 类型可以 fetch 一下 `og:title / og:image`，悬浮预览。（可选，增强体验）

---

## 🟢 P2：锦上添花

### 功能扩展

- [ ] **富文本 / HTML 支持**
  目前只有 plain text，复制带格式的文本会丢样式。考虑支持 HTML MIME 类型存储 + 粘贴时恢复。
- [ ] **文件类型 thumbnail / icon**
  `file` 类型现在显示为什么？能不能按扩展名展示对应文件 icon（PDF/DOCX/ZIP 各自图标）？
- [ ] **多设备同步（可选）**
  E2E 加密同步到用户自己的 WebDAV / S3 / GitHub Gist。纯客户端加密，无服务器。
- [ ] **快捷粘贴（1-9 数字键直接粘贴）**
  面板打开时按 `1`~`9` 立即粘贴对应条目，不用鼠标。
- [ ] **分类/标签**
  除了收藏/置顶，用户自定义 tag 归类（工作/代码/链接）。
- [ ] **自定义格式转换**
  "复制纯文本"（去除富文本格式）、"转 URL-encoded"、"转 base64"等管道操作。

### 工程化

- [ ] **CI/CD**
  GitHub Actions 自动：
  - PR 跑 test + lint
  - Tag 打自动构建三端 artifacts + 发 Release
  - macOS 侧还要考虑 codesign + notarize（没签名的 .app 用户打不开）
- [ ] **代码签名**
  - Windows: EV 证书（个人开发者难获取）或自签 + 用户手动信任
  - macOS: Apple Developer ID 证书 + notarize
- [ ] **单元测试覆盖率**
  `make test` 现状是啥覆盖率？核心的 `storage` / `crypto` / `types` 至少应该 80%+。
- [ ] **性能 benchmark**
  Makefile 里有 `make bench`，实际有几个 benchmark？列表 10 万条时的搜索 / 滚动性能应该压测。
- [ ] **错误上报（可选）**
  集成 Sentry 或自托管的错误收集，能看到用户端崩溃栈。

### 文档

- [ ] **补 CHANGELOG.md**
  记录每个版本的 feat/fix/chore，方便用户知道升级带来了什么。
- [ ] **补 CONTRIBUTING.md**
  外部贡献者的上手文档：分支策略、commit 规范、PR 模板。
- [ ] **DESIGN.md 保持同步**
  核心架构变化（比如 `internal/platform` + `winx` 合并成 `internal/window` 这类重构）后 DESIGN.md 需要更新。
- [ ] **README 里补截图 / GIF**
  现在 README 偏"干货"没演示，加几张截图 + 操作 GIF 更能吸引用户。
- [ ] **多语言 README**
  EcoPaste 有 `README.md / README.en-US.md / README.ja-JP.md / README.zh-TW.md`。GoPaste 只有中文，可以补英文版。

---

## 🔵 P3：长期方向 / 生态

- [ ] **Linux AppImage / deb / rpm 打包脚本**
  现在只能 `make build-linux` 出二进制，缺少分发格式。
- [ ] **插件机制**
  用户自己写 JS 脚本做"复制时自动格式化 JSON"、"复制链接时自动短网址"等。需要一套沙盒 + API 设计。
- [ ] **浏览器扩展协同**
  Chrome / Firefox 扩展发剪贴板给 GoPaste，GoPaste 返回粘贴目标。解决移动端复制 → PC 粘贴场景。
- [ ] **AI 辅助**
  - 复制一段代码 → 自动识别语言 + 语法高亮（目前只是 `typeCode` 标识，没 highlight）
  - "总结这段复制的长文"快捷操作（调用本地 LLM / OpenAI API）
- [ ] **Wails 升级到 v3**
  Wails v3 已在路上，API 变化较大。升级需要评估时间点与 breaking change。

---

## ✅ 已完成（最近这次 session）

- [x] 重构 `internal/platform` + `internal/winx` → `internal/window`（按功能划分）
- [x] Windows 11 DWM 原生圆角
- [x] macOS `TitleBarHiddenInset`（保留 traffic lights，Mac 风格）
- [x] 类型专属操作：link → 浏览器打开 / file → 资源管理器 / image → 保存
- [x] 右键菜单新增"用浏览器打开"
- [x] i18n 补全：`openInBrowser`、`typeXxx`、`chars` 三种语言
- [x] UI 打磨：快捷键输入框字号、设置页圆角对齐、移除冗余键盘 icon
- [x] 拉伸黑边修复（BackgroundColour 改浅灰）
- [x] EcoPaste 跨平台实现分析文档（`docs/ecopaste-analysis.md`）
- [x] **macOS 粘贴从 CGEventPost 切换到 osascript + resignKey 路线**（2026-04-23）
  - 症状：前版用 `CGEventPost(kCGHIDEventTap)` 注入 Cmd+V，orderOut 后面板仍是 keyWindow，键事件回落到自己 → 前端被触发再调 `PasteItem` → 死循环 → 被 WindowServer 以 HID flood 为由 SIGKILL，无 crash report，表现为"粘几次后闪退"
  - 参考 EcoPaste（`src-tauri/src/plugins/paste/src/commands/macos.rs`）的做法
  - 现在流程：`copy → ResignMainKey → sleep 50ms → osascript 'System Events keystroke v using command down' → 异步 HideMain`
  - 代码：`internal/paste/paste_darwin.go` 全部改写；`app.go` `PasteItem` mac 分支重写；`internal/window/panel_darwin.m` 的 `GoPasteResignKey` + `internal/window/showhide_darwin.go` 的 `ResignMainKey` 被正式接线
- [x] **关闭菜单栏图标后再开启无法显示** ✅ 已修复（2026-04-24）
  - 原因：`fyne.io/systray` 内部 `Quit()` 受 `sync.Once` 保护，一个进程只能关一次。`stopTray()` 标记 `trayQuit=true` 后，再调 `startTray()` 被挡住。
  - 方案（Windows）：参考 EcoPaste / Tauri 的 `tray_icon.set_visible(false/true)` 模式，systray 消息循环在 startup 时无条件启动并常驻，用 `Shell_NotifyIcon(NIM_DELETE/NIM_ADD)` 平滑切换通知区域图标显隐。新增 `tray.SetVisible(bool)` / `tray.CanToggle()` API，`visible_windows.go` 实现。
  - 方案（macOS/Linux）：`fyne.io/systray` 未暴露隐藏 API，关闭后再开启仍走 `restartApp()` 兜底。
  - 前端移除了"重启后生效"提示和 `TrayNeedsRestart()` RPC 调用。
- [ ] **Windows 粘贴图片到 VS Code 等编辑器无法识别** — 已撤销（`clipboard_windows.go` 多格式写入方案有问题），待重新排查

---

## 快速筛选

**想先解决哪一批？**
- "让 Mac 用户不报错" → P0 的 **辅助功能权限引导**
- "让用户觉得这是成品" → P0 的 **开机自启 / 单实例 / 自动更新**
- "让代码更好维护" → P1 的 **app.go 拆分 + App.vue 拆组件**
- "发出去让更多人用" → P2 的 **CI/CD + 代码签名 + 多语言 README**
