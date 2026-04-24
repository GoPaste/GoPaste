# Windows 发布形式建议

> 记录时间：2026-04-24
> 背景：当前 GoPaste 以绿色免安装 `.exe` 形式分发，是否需要改/加 Installer 形式？
> 结论：**当前阶段继续绿色版，准 1.0 时再加 NSIS Installer**。

---

## TL;DR

- 🟢 **现阶段（v0.x 快速迭代期）**：继续绿色版 `.exe` + zip 便携包，**不上 Installer**
- 🟡 **准 1.0 时**：加 NSIS Installer，与绿色版**并存发布**
- 🔴 **1.0 正式发布后**：申请代码签名证书（或用 signpath.io 的开源免费签名），Installer 才能真正发挥价值

---

## 一、两种形式对比

| 维度 | 绿色免安装 (`.exe`) | Installer (`.msi` / `.exe`) |
|------|--------------------|----------------------------|
| **用户心理门槛** | 低（下载即用） | 高（怕安装流氓软件） |
| **首次体验** | 双击即运行 | 走向导、选路径、等进度 |
| **SmartScreen 警告** | 未签名每次警告 | 同样警告；安装一次后 exe 被系统信任 |
| **多用户共享** | 每个用户各一份 | 可装到 Program Files，全机共享 |
| **开机自启** | 已用启动文件夹方案 ✅ | 通常同方案，可在向导里引导勾选 |
| **升级** | 下载新 exe 覆盖旧 exe | 向导式升级（但用户其实更烦） |
| **卸载** | 删文件夹 + `%APPDATA%/gopaste` | 控制面板 → 卸载（干净） |
| **文件关联 / 协议注册** | ❌ 不方便 | ✅ 可配合注册表写入 |
| **开始菜单快捷方式** | ❌ 需手动拖 | ✅ 自动添加 |
| **包体积** | 10~20 MB | 15~25 MB |
| **CI 构建复杂度** | 简单 | 需配置 NSIS/Inno Setup/WiX |

---

## 二、剪贴板工具类的行业惯例

调研同类软件的分发方式：

| 工具 | 分发形式 |
|------|---------|
| **Ditto** | 绿色 + 安装版 |
| **1Clipboard** | 安装版 |
| **ClipboardFusion** | 安装版 |
| **CopyQ** | 绿色 + 安装版 |
| **EcoPaste**（对标项目） | `.exe`（绿色）+ `.msi`（安装版） |
| **Raycast / Alfred**（macOS） | `.dmg` 拖拽安装 |

**规律**：工具类软件**基本都提供两种**，主推安装版，绿色版作为便携/进阶选项。

---

## 三、为什么剪贴板工具需要 Installer？

### 支持 Installer 的理由

1. **开机自启** — Installer 能在向导里勾"登录时启动"，体验更丝滑
2. **托盘常驻** — Installer 更符合"后台服务类工具"的心智
3. **SmartScreen 信誉** — 新 exe 每次发布会触发 Windows Defender 警告。Installer 的 `.exe` 发布频率低，一旦被足够用户"信任"后会被 SmartScreen 放行
4. **清洁卸载** — 有控制面板卸载入口，清理 `%APPDATA%/gopaste/`、注册表条目、自启项等

### 保留绿色版的理由

1. **便携办公**（U 盘带走）
2. **公司电脑无管理员权限** — 绿色版无需权限
3. **尝鲜 / 评估用户** — 不想装进系统先试用
4. **开发者自己** — 方便测试、不污染系统

---

## 四、推荐分阶段路线

### 🟢 阶段 1：现状（v0.x，快速迭代期）

**做法**：只发绿色版 `.exe` + 打包成 `.zip`

**发布页结构**：
```
GoPaste_0.1.0_windows_x64.zip       # 解压即用
GoPaste_0.1.0_windows_x64.exe       # 直接下载 exe（不推荐，SmartScreen 警告更强）
```

**README 写清楚**：
- 下载 → 解压 → 双击 `gopaste.exe`
- 首次启动 SmartScreen 会警告 → 点「更多信息 → 仍要运行」（因为未签名）
- 设置页可勾选"登录时启动"

**优点**：零额外成本，每次发版只要构建 exe。

### 🟡 阶段 2：准 1.0 时

**做法**：同时提供 Installer 和便携版

**发布页结构**：
```
GoPaste_1.0.0_windows_x64_setup.exe    # 主推（安装版）
GoPaste_1.0.0_windows_x64_portable.zip # 绿色便携版
```

**打包工具选择**：

| 方案 | 推荐度 | 说明 |
|------|-------|------|
| **NSIS**（Nullsoft Scriptable Install System） | ⭐⭐⭐⭐⭐ **首选** | Wails v2 内置 `-nsis` 支持，零额外配置 |
| Inno Setup | ⭐⭐⭐⭐ | 脚本清晰，但需手动集成 |
| WiX Toolset (MSI) | ⭐⭐⭐ | 企业级，配置繁琐，大项目才值得 |
| MSIX | ⭐⭐⭐ | 微软新方案，Wails v3 内置，v2 需自己打 |

**仍不签名**——但 installer 一次性接受警告后，后续升级更友好。

### 🔴 阶段 3：1.0 正式发布后

**做法**：申请代码签名证书，Installer 的真正价值显现。

**签名方案**：

| 方式 | 成本 | 说明 |
|------|-----|------|
| **signpath.io 开源免费签名** | 0 | 对开源项目提供免费 EV 代码签名，要求 GitHub 公开 |
| DigiCert / Sectigo EV 证书 | 约 ¥2000~3000 / 年 | 商业方案，SmartScreen 立刻信任 |
| DigiCert / Sectigo OV 证书 | 约 ¥800~1500 / 年 | 需要积累信誉，初期仍有警告 |
| 自签名 | 0 | 用户必须手动信任，实用性差 |

**推荐**：开源项目走 [signpath.io](https://about.signpath.io/open-source)，零成本拿到 EV 签名。

---

## 五、NSIS Installer 快速落地（未来参考）

Wails v2 自带 NSIS 支持，到阶段 2 实施时：

### 1. 生成默认模板

```bash
wails generate template -name nsis
# 或直接 build
wails build -platform windows/amd64 -nsis
```

生成物位置：
- `build/bin/GoPaste-amd64-installer.exe` — Installer 本体
- `build/windows/installer/` — 模板文件（可自定义界面、图标、协议）

### 2. 建议加到 Makefile

```makefile
build-win-installer: ## 构建 Windows NSIS Installer
	CGO_ENABLED=1 CC=$(WIN_CC) CXX=$(WIN_CXX) \
	$(XVFB) $(WAILS) build -clean -platform windows/amd64 -nsis $(WAILS_FLAGS)
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME)-amd64-installer.exe$(RESET)"
```

### 3. Installer 自定义清单（到时候做）

- [ ] 自定义安装向导 Banner（建议 150×57 px BMP）
- [ ] 欢迎页 LICENSE 协议内容
- [ ] 默认安装到 `%LOCALAPPDATA%\GoPaste`（无需管理员）或 `Program Files`（需管理员）
- [ ] 创建开始菜单 / 桌面快捷方式（可选勾选）
- [ ] 安装后选"是否立即启动"
- [ ] 卸载时提示是否清理 `%APPDATA%/gopaste`
- [ ] 注册卸载条目到 `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\GoPaste`

### 4. 可选：MSIX 支持（Wails v3 时代）

Wails v3 原生支持 MSIX，对应 Windows 11 商店分发。**v2 时代可以先不考虑**。

---

## 六、当前决策

| 项 | 决定 |
|---|------|
| **v0.x 还出不出 Installer？** | ❌ 不出。绿色 zip 够用。 |
| **README 是否写安装说明？** | ✅ 写清楚解压、首次警告、如何勾开机自启。 |
| **代码签名现在买吗？** | ❌ 暂不买。先申请 [signpath.io](https://about.signpath.io/open-source) 开源免费签名（达到 GitHub 公开条件）。 |
| **Makefile 加 installer 目标？** | ⏸ 先不加，用户规模起来再说。 |
| **发布页 artifact 结构？** | `GoPaste_<version>_windows_x64.zip` 一份即可。 |

---

## 七、附：现阶段 README 建议补充的内容

> 这部分文案可以直接放到 `README.md` 或 GitHub Release notes 里：

### Windows 用户首次运行

1. 从 Releases 页下载 `GoPaste_x.y.z_windows_x64.zip`
2. 解压到任意目录（如 `D:\Tools\GoPaste\`）
3. 双击 `gopaste.exe`
4. Windows Defender SmartScreen 会提示"已保护你的电脑"——这是因为当前版本尚未购买代码签名证书，而非恶意软件。你可以：
   - 点击 **更多信息**
   - 点击 **仍要运行**
5. 应用会启动并显示在系统托盘
6. 如需开机自启，打开设置页 → 通用 → 勾选"登录时启动"

### 卸载

1. 退出托盘程序
2. 删除应用所在文件夹
3. 如需完全清理数据：删除 `%APPDATA%\gopaste\`（含历史数据、配置）
4. 如开过"登录时启动"：Win+R 输入 `shell:startup` → 删除 `gopaste.lnk`

---

## 八、参考资料

- [Wails v2 NSIS 文档](https://wails.io/docs/guides/windows-installer)
- [NSIS 官网](https://nsis.sourceforge.io/)
- [signpath.io 开源免费签名](https://about.signpath.io/open-source)
- [Microsoft SmartScreen 白名单机制](https://docs.microsoft.com/en-us/windows/security/threat-protection/windows-defender-smartscreen/windows-defender-smartscreen-overview)
