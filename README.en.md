<div align="center">

<img src="build/appicon.png" width="120" height="120" alt="GoPaste Logo" />

# GoPaste

**A lightweight, fast, and secure cross-platform clipboard manager**

[![Release](https://img.shields.io/github/v/release/larkwins/GoPaste?style=flat-square&logo=github)](https://github.com/larkwins/GoPaste/releases)
[![License](https://img.shields.io/github/license/larkwins/GoPaste?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-blue?style=flat-square)](https://github.com/larkwins/GoPaste/releases)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go)](https://golang.org)

[简体中文](README.md) · [English](#)

</div>

---

## ✨ Features

- 🕐 **Auto-record** — Silently monitors your clipboard in the background; supports text, images, links, code snippets, and files
- ⚡ **Instant access** — Global hotkey `Ctrl/Cmd + Shift + V` to summon the panel instantly, no window switching needed
- 🔍 **Smart search** — Real-time full-text search with type filtering, find any history item in milliseconds
- ⭐ **Favorites & Pin** — Mark important items to keep forever; pinned items always stay at the top
- 🔐 **Local encryption** — All content encrypted with AES-256-GCM; keys stored in the system Keychain — your data never leaves your device
- 🖥️ **Native cross-platform** — Deep integration with Windows, macOS, and Linux; installer ≤ 20 MB
- 🎨 **Dual themes** — Switch freely between dark and light mode
- 🌏 **Multilingual** — Simplified Chinese, Traditional Chinese, and English
- 📤 **Data export** — One-click JSON export; your data, your control

---

## 📦 Download

Visit [GitHub Releases](https://github.com/larkwins/GoPaste/releases) to download the latest version.

| Platform | File | Notes |
|----------|------|-------|
| Windows x64 | `GoPaste_x.x.x.exe` | Double-click to run, no installation required |
| Windows ARM64 | `GoPaste_x.x.x_arm64.exe` | For Surface Pro X and other ARM devices |
| macOS Universal | `GoPaste_x.x.x.app` (`.dmg`) | Supports both Apple Silicon & Intel |
| Linux x64 | `GoPaste_x.x.x_linux_amd64` | Requires system tray support (X11) |

---

## 🚀 Quick Start

1. Download and launch GoPaste — it will quietly reside in your system tray
2. Copy anything as usual (text, images, links…) — GoPaste records it automatically
3. Press `Alt + `` ` to open the panel
4. Search or browse your history, then click any item to paste it into the current app

**Keyboard shortcuts:**

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate items |
| `Enter` | Paste selected item |
| `Esc` | Close the panel |
| `Delete` | Delete selected item |
| `Tab` / `Shift+Tab` | Switch content type filter |

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

## 🖼️ Screenshots

> Coming soon…

---

## 🗺️ Roadmap

- [ ] Data import (JSON restore)
- [ ] Rich text / HTML format preservation
- [ ] Auto-update (in-app download & install)
- [ ] CI/CD automated builds & releases
- [ ] Linux AppImage / deb / rpm packages
- [ ] Quick-paste with number keys (`1~9` to paste corresponding items)
- [ ] Multi-device E2E encrypted sync

---

## 🤝 Contributing

Issues and Pull Requests are welcome!

- **Bug reports** → [GitHub Issues](https://github.com/larkwins/GoPaste/issues)
- **Feature requests** → [GitHub Discussions](https://github.com/larkwins/GoPaste/discussions)
- **Developer guide** → See [CONTRIBUTING.md](docs/CONTRIBUTING.md)

---

## 📄 License

[MIT](LICENSE) © 2026 larkwins
