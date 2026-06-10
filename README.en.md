<div align="center">

<img src="assets/images/icon.png" width="120" height="120" alt="GoPaste Logo" />

# GoPaste

**A lightweight, fast, and secure cross-platform clipboard manager**

[![Release](https://img.shields.io/github/v/release/GoPaste/GoPaste?style=flat-square&logo=github)](https://github.com/GoPaste/GoPaste/releases)
[![License](https://img.shields.io/github/license/GoPaste/GoPaste?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-blue?style=flat-square)](https://github.com/GoPaste/GoPaste/releases)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go)](https://golang.org)

[简体中文](README.md) · [English](#) · [Website](https://gopaste.wetools.cc/)

</div>

![](./assets/images/poster.png)

---

## ✨ Features

- 🕐 **Auto-record** — Silently monitors your clipboard in the background; supports text, images, links, code snippets, and files
- ⚡ **Instant access** — Global hotkey <kbd>Alt</kbd> + <kbd>`</kbd> to summon the panel instantly (customizable)
- 🔍 **Smart search** — Real-time full-text search with type filtering, find any history item in milliseconds
- ⭐ **Favorites & Pin** — Mark important items to keep forever; pinned items always stay at the top
- 📂 **Smart actions** — Context-aware actions based on content type: image→preview/save, file→reveal in explorer, link→open in browser, code→syntax highlighting, all one-click away
- 😀 **Emoji picker** — Built-in emoji library; click to copy, double-click to paste directly; supports search and skin-tone switching (can be toggled in settings)
- 🔐 **Local encryption** — All content encrypted with AES-256-GCM; keys stored in the system Keychain — your data never leaves your device
- 🖥️ **Native cross-platform** — Deep integration with Windows, macOS, and Linux; installer ≤ 20 MB
- 🎨 **Dual themes** — Switch freely between dark and light mode
- 🌏 **Multilingual** — Simplified Chinese, Traditional Chinese, and English
- 📤 **Data export** — One-click JSON export; your data, your control

---

## 📦 Download

Visit [GitHub Releases](https://github.com/GoPaste/GoPaste/releases) to download the latest version.

| Platform | File | Notes |
|----------|------|-------|
| Windows x64 Portable | `GoPaste_x.x.x_windows_x64-portable.exe` | Double-click to run, no installation required |
| Windows x64 Installer | `GoPaste_x.x.x_windows_x64-setup.exe` | Installer with setup wizard |
| Windows x86 Portable | `GoPaste_x.x.x_windows_x86-portable.exe` | For 32-bit systems |
| macOS Apple Silicon | `GoPaste_x.x.x_darwin_aarch64.dmg` | M-series chips |
| macOS Intel | `GoPaste_x.x.x_darwin_x64.dmg` | Intel chips |
| macOS Universal | `GoPaste_x.x.x_darwin_universal.dmg` | Supports both Apple Silicon & Intel |
| Linux x64 | `GoPaste_x.x.x_linux_x64.tar.gz` | Requires system tray support (X11) |

---

## 🚀 Quick Start

1. Download and launch GoPaste — it will quietly reside in your system tray
2. Copy anything as usual (text, images, links…) — GoPaste records it automatically
3. Press <kbd>Alt</kbd> + <kbd>`</kbd> to open the panel
4. Search or browse your history, then click any item to paste it into the current app

**Keyboard shortcuts:**

**Global shortcuts (work even when panel is not focused)**

| Shortcut | Action |
|----------|--------|
| `Alt` + `` ` `` | Toggle panel |
| `Alt` + `1` ~ `6` (Windows) / `Cmd` + `1` ~ `6` (Mac) | Switch category tab |
| `Alt` + `Space` (Windows) | Preview selected item (fallback) |

**In-app shortcuts (when panel is focused)**

| Shortcut | Action |
|----------|--------|
| `Tab` / `Shift`+`Tab` | Switch content type (main view) / Switch emoji category (emoji view) |
| `←` / `→` | Switch content type |
| `↑` / `↓` | Navigate items |
| `Enter` | Paste selected item |
| `Space` (single press) | Preview / view detail |
| `Space` (double press) | Primary action (image→save / file→reveal in explorer / link→open in browser) |
| `Ctrl` + click (Windows) / `Cmd` + click (Mac) | Toggle selection (multiselect、deselect) |
| `Ctrl` + right-click (Windows) / `Cmd` + right-click (Mac) | Add to selection (context menu) |
| `Esc` | Close panel / clear selection / exit settings or emoji view |
| `Delete` / `Backspace` | Delete selected item (supports batch) |
| Double-click item | Paste (when paste trigger mode is set to "double-click") |

---

## 🖼️ Screenshots

<details open>
<summary>Simplified Chinese</summary>

![Simplified Chinese](./assets/images/main_zh_sc.png)

</details>

<details>
<summary>English</summary>

![English](./assets/images/main_en.png)

</details>

<details>
<summary>Traditional Chinese</summary>

![Traditional Chinese](./assets/images/settings_zh_tc.png)

</details>

<details>
<summary>Emoji Picker</summary>

![Emoji Picker](./assets/images/gopaste_emoji.png)

</details>

---

## 🗺️ Roadmap

- [ ] Data import (JSON restore)
- [ ] Rich text / HTML format preservation
- [ ] Auto-update (in-app download & install)
- [x] CI/CD automated builds & releases
- [ ] Linux AppImage / deb / rpm packages
- [ ] Quick-paste with number keys (`1~9` to paste corresponding items)
- [ ] Multi-device E2E encrypted sync

---

## ⚠️ macOS Notes

### "Cannot open — unidentified developer"

GoPaste is currently distributed without an Apple Developer certificate (ad-hoc signed). macOS Gatekeeper may block it on first launch.

**Fix:**

1. Drag `GoPaste.app` into your **Applications** folder
2. Double-click to open (being blocked is expected — dismiss the dialog)
3. Go to **System Settings → Privacy & Security**, scroll to the bottom, and click **"Open Anyway"**

> Alternatively, **right-click (Control + click) → Open** in Finder, then click "Open" in the dialog.

### "GoPaste.app is damaged and can't be opened"

Run this command in Terminal:

```bash
xattr -cr /Applications/GoPaste.app
```

This removes the quarantine attribute that triggers Gatekeeper. Then try opening again.

### Accessibility Permission (required for auto-paste)

GoPaste simulates `Cmd+V` to paste content into the target app, which requires Accessibility permission.

The system will prompt you the first time you trigger a paste — just follow the instructions.

> ⚠️ **After every upgrade**, you must remove the old GoPaste entry from **System Settings → Privacy & Security → Accessibility** and re-authorize. Otherwise the panel closes but content doesn't paste.
>
> See [macOS Accessibility Guide](docs/macos-accessibility.md) for details.

---

## 🤝 Contributing

Issues and Pull Requests are welcome!

- **Bug reports** → [GitHub Issues](https://github.com/GoPaste/GoPaste/issues)
- **Feature requests** → [GitHub Discussions](https://github.com/GoPaste/GoPaste/discussions)
- **Developer guide** → See [CONTRIBUTING.md](docs/CONTRIBUTING.md) (coming soon)

---

## 📄 License

[Apache-2.0](LICENSE) © 2026 larkwins
