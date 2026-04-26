# macOS 辅助功能权限 & ad-hoc 签名陷阱

## TL;DR（用户视角）

> **每次升级 GoPaste 后，你都需要在"辅助功能"里把 GoPaste 删掉再重新授权。**
>
> 这不是 bug，是 macOS 的安全机制 + 当前发行版未做 Apple Developer 签名共同导致的。

| 现象 | 根因 | 解法 |
|---|---|---|
| 升级后粘贴失效，但"辅助功能"开关看着仍是开的 | TCC 按 CDHash 追踪授权，新版二进制 CDHash 变了 → 旧授权失效；但 UI 仍按 bundle id 显示，看起来没变 | 在"辅助功能"列表里**删掉 GoPaste**这一行，回到 App 再触发粘贴，系统会重新请求授权 |

要彻底消除这个问题，需要项目侧使用 **Developer ID Application 证书签名**
（年费 $99 的 Apple Developer 账号）。详见下文"长期解法"。

---

## 症状

> 明明在"系统设置 → 隐私与安全 → 辅助功能"里**已经勾选了 GoPaste**，
> 但 GoPaste 粘贴时仍然报"accessibility permission not granted"，
> 看起来像"点一下面板消失、什么也没粘上"——疑似闪退。

## 根因：ad-hoc 签名 + TCC 按 CDHash 匹配

`codesign -dvvv build/bin/GoPaste.app` 会看到：

```
Signature=adhoc
TeamIdentifier=not set
```

Wails 默认用 ad-hoc 签名（无 Apple 开发者证书即用此方式）。

macOS 的 **TCC（Transparency, Consent, and Control）**数据库
（`~/Library/Application Support/com.apple.TCC/TCC.db`）记录授权时：

- **有正式签名的 app**：按 **TeamIdentifier + bundle id** 匹配，稳定。
- **ad-hoc 签名的 app**：按 **CDHash** 匹配。CDHash 是对二进制 +
  `Info.plist` + 资源内容算的哈希——**二进制一变，CDHash 就变**，
  TCC 里保存的那条 `csreq` 就和当前进程对不上，系统内部认定"未授权"。

但系统设置 UI 是按 **bundle identifier** 展示的——
所以用户会看到：
- 列表里有 "GoPaste"，开关是**亮的** ✓
- 程序里 `AXIsProcessTrusted()` 返回 **false** ✗

这是开发阶段每次重新 `wails build` 都会中招的陷阱，
EcoPaste/Raycast/Alfred 等项目的 dev 文档都反复提醒过。

## 用户侧解法（临时）

1. 系统设置 → 隐私与安全 → 辅助功能
2. **选中列表里的 GoPaste，点 −（减号）删除这条记录**
3. 回到 GoPaste，再触发一次粘贴
4. 系统会重新弹框请求授权（因为 TCC 里已无记录）
5. 勾选后即生效——**直到下次重新构建 GoPaste**

这就是为什么 `showAccessibilityGuide()` 里的文案必须写清楚"先删除再重授权"，
而不是简单的"请去系统设置勾选"。

## 长期解法：正式开发者证书

拿到 Apple Developer 账号（个人 $99/年）后：

1. 在 Keychain 里导入开发者证书（Developer ID Application）
2. `wails build -platform darwin/arm64 -obfuscated` 时会自动用证书签名
3. TCC 按 TeamIdentifier + bundle id 匹配，构建多少次 CDHash 都不影响授权

这是 EcoPaste/Raycast 正式发布版走的路子。

## 开发时诊断命令

```bash
# 看当前 GoPaste.app 的签名信息
codesign -dvvv build/bin/GoPaste.app

# 看 TCC 里 GoPaste 的授权记录（需要 Full Disk Access）
sqlite3 "$HOME/Library/Application Support/com.apple.TCC/TCC.db" \
  "SELECT service, client, auth_value FROM access WHERE client LIKE '%gopaste%';"

# 直接跳转到"辅助功能"面板
open "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
```

## 代码里的对应位置

- `internal/paste/paste_darwin.go`
  - `HasAccessibility()`：静默查询（调 `AXIsProcessTrustedWithOptions(prompt=NO)`）
  - `PromptAccessibility()`：首次触发系统授权弹框（`prompt=YES`，`sync.Once` 保护）
  - `ErrNoAccessibility`：未授权时返回的哨兵错误
  - `sendPasteImpl()`：**通过 `osascript` + System Events 发送 Cmd+V**（不是 CGEventPost，见下节"为什么换路线"）
- `internal/window/panel_darwin.m` / `showhide_darwin.go`
  - `GoPasteResignKey` / `ResignMainKey`：粘贴前让 NSPanel 交还 keyWindow 状态
- `app.go`
  - `PasteItem`：进入时预检权限 → `CopyToClipboard` → `ResignMainKey` → 50ms sleep → `SendPaste (osascript)` → 异步 `HideMain`
  - `showAccessibilityGuide`：权限未授予时弹引导对话框（`accessGuideOnce` 保证只弹一次）
  - `HasPastePermission` / `RequestPastePermission`：前端可用的 RPC

## 为什么 macOS 粘贴从 CGEventPost 换成 osascript

**2026-04-23 重构**。历史实现用 `CGEventPost(kCGHIDEventTap)` 直接向系统 HID 流
注入 Cmd+V 键事件。看起来更底层、更"优雅"，但实测遇到两类致命问题：

### 问题 1：事件回落到自己，形成死循环

`CGEventPost` 是**全局 HID 注入**，没有"目标 app"概念——谁是当前 keyWindow，
事件就落到谁身上。我们的流程是 `HideMain (orderOut:) → SendPaste`，但：

- **`orderOut:` 不会让面板让出 keyWindow**。AppKit 的设计：窗口只是从屏幕上
  移走，key 状态不会因视觉隐藏而自动转移。
- 所以 `CGEventPost` 发出的 Cmd+V 仍被 GoPaste 自己的 NSPanel 收走 → WebView
  接到键事件 → 前端的某个 handler 被触发 → 又调 `PasteItem` → 死循环。

### 问题 2：HID flood 被 WindowServer 杀

死循环跑起来后每秒数次向 HID 注入，会被 macOS 的 WindowServer 识别为异常
并直接 **SIGKILL** 发起进程——**无 crash report、无 `log show` 事件**。
这就是用户看到的"粘几次后闪退"，日志上表现为最后一条 `sent id=X` 之后
boot log 再无输出、进程消失。

### 新路线（参考 EcoPaste）

[EcoPaste](https://github.com/EcoPasteHub/EcoPaste) 是 Tauri 生态里最成熟的剪贴板
工具，macOS 实现见 `src-tauri/src/plugins/paste/src/commands/macos.rs`：

```rust
set_macos_panel(&app_handle, &window, MacOSPanelStatus::Resign);   // 1) 让出 key
let script = r#"tell application "System Events" to keystroke "v" using command down"#;
Command::new("osascript").args(["-e", script]).output().expect("failed");  // 2) 发 Cmd+V
```

关键差异：

| 步骤 | 旧 CGEventPost 路线 | 新 osascript 路线 |
|---|---|---|
| 隐藏面板 | `orderOut:`（不让出 key） | `resignKeyWindow`（让 AppKit 把 key 交给上一个 key window 的 app） |
| 发键事件 | `CGEventPost(kCGHIDEventTap)`，全局 HID 注入 | `osascript` + System Events，**面向当前前台 app** 的按键投递 |
| 目标可控性 | ❌ 盲注，谁是 key 谁收 | ✅ 由 System Events 查询"frontmost process"后定向投递 |
| HID throttle 风险 | ❌ 有，高频注入会被 WindowServer 杀 | ✅ 无，走 AppleEvent IPC 不算 HID |
| 面板隐藏 | 在 SendPaste **之前** | 在 SendPaste **之后异步** 做，保证目标 app 在 resign 状态下收到键 |

权限要求**没变**：两种路线都需要 Accessibility 授权，所以本文前面讲的
"TCC + CDHash 陷阱"依然适用。

### 为什么不继续用 CGEventPost + resignKey 组合

理论上如果我们在 `CGEventPost` 前先 `resignKeyWindow`、让键事件落到正确的
前台 app，也能避免死循环。但 `CGEventPost` 相对 `osascript` 没有任何优势：

- HID flood 风险仍在（只要出现任何形式的反复触发就会被 SIGKILL）
- 需要手动维护 CGEventSource、flags、key down/up 两次注入的时序
- System Events 其实内部也是调 `CGEventPost`，但由 System Events 自己负责把
  目标 app 锁定、处理重试——macOS 层级更高更稳

`osascript` 的唯一"代价"是 fork 一个子进程，但剪贴板粘贴是秒级频率操作，
这点开销在用户感知里是零。
