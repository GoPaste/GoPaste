#!/usr/bin/env python3
"""
gen_appicon.py — 把原始方形图标处理成符合 macOS 规范的 appicon.png，
并同时生成菜单栏用的小尺寸彩色版本。

【为什么要做这一步】
macOS Big Sur 之后所有应用图标遵循统一规范：
  - 1024×1024 透明画布
  - 实体本身只占中间约 824×824（上下左右各留 ~100px 透明 padding）
  - 实体使用 squircle (superellipse) 圆角
系统在 Dock / Launchpad 渲染时会自动叠加圆角遮罩并按相对尺寸排版；
如果图标铺满整张画布，就会显得比旁边 app "大一圈"且边缘被切。

我们 Wails build 时会自动从 build/appicon.png 生成 .icns 嵌入 .app，
所以只需保证这张 png 本身符合规范即可。

输出：
  build/appicon.png                    # 1024×1024，带 padding 的主图标
  internal/tray/icon_color.png         # 44×44 彩色菜单栏图标（22pt @2x）

输入优先级：
  1) build/appicon.src.png（如果存在，用它，不被覆盖）
  2) build/appicon.png 自身（首次运行时直接覆盖；后续会保留 src）

运行：python3 scripts/gen_appicon.py
"""

from __future__ import annotations

import os
import shutil
import sys
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parent.parent
SRC_CANDIDATES = [
    ROOT / "build" / "appicon.src.png",   # 首选：用户保留的原图
    ROOT / "build" / "appicon.png",        # 兜底：当前 appicon
]

CANVAS = 1024            # 输出画布尺寸
INNER = 824              # 实体内容区域尺寸（约 80%，符合 macOS 视觉权重）
PAD = (CANVAS - INNER) // 2

OUT_APPICON = ROOT / "build" / "appicon.png"
OUT_SRC_BACKUP = ROOT / "build" / "appicon.src.png"
OUT_TRAY = ROOT / "internal" / "tray" / "icon_color.png"
# 菜单栏图标尺寸：经实测 fyne.io/systray 在 macOS 上把 NSImage 渲染到约
# 22pt × 22pt（retina 下 44px）。但我们提供 88px（22pt @4x）让系统自己降采样，
# 比 PIL 直接缩到 44px 锐利得多。同时图像本体必须铺满整个画布——菜单栏不
# 像 dock 会自动加圆角遮罩，任何透明 padding 都会让图标显得"小一圈"。
TRAY_SIZE = 88


def load_source() -> Image.Image:
    """加载源图。第一次运行时会自动把 appicon.png 备份成 appicon.src.png。"""
    src_path = None
    for cand in SRC_CANDIDATES:
        if cand.exists():
            src_path = cand
            break
    if src_path is None:
        sys.exit("error: 找不到源图 (build/appicon.src.png 或 build/appicon.png)")

    img = Image.open(src_path).convert("RGBA")
    print(f"[load] {src_path.relative_to(ROOT)}  {img.size}")

    # 首次运行：把当前 appicon 备份成 appicon.src.png 留底
    if src_path == OUT_APPICON and not OUT_SRC_BACKUP.exists():
        shutil.copyfile(OUT_APPICON, OUT_SRC_BACKUP)
        print(f"[backup] {OUT_APPICON.name} -> {OUT_SRC_BACKUP.name}")

    return img


def make_appicon(src: Image.Image) -> Image.Image:
    """生成 1024×1024 带 padding 的主图标。"""
    # 等比缩放源图到 INNER × INNER
    inner = src.resize((INNER, INNER), Image.LANCZOS)

    # 嵌入透明画布的中央
    canvas = Image.new("RGBA", (CANVAS, CANVAS), (0, 0, 0, 0))
    canvas.paste(inner, (PAD, PAD), inner)
    return canvas


def make_tray_icon(src: Image.Image) -> Image.Image:
    """生成菜单栏彩色图标。

    要点：
      1. 不加任何透明 padding——菜单栏不会像 dock 自动叠圆角遮罩，
         留 padding 就会显得比旁边 app 小一圈。
      2. 提供 88×88（22pt @4x）让 AppKit 自己降采样到当前菜单栏高度，
         比 PIL 直接缩到 44px 边缘锐利度更好。
      3. 用 LANCZOS 高质量重采样。
    """
    # 先确保源图是正方形 + 内容铺满（裁掉任何透明边）
    bbox = src.getbbox()
    if bbox and bbox != (0, 0, src.width, src.height):
        src = src.crop(bbox)
    # 等比缩放到 TRAY_SIZE × TRAY_SIZE
    return src.resize((TRAY_SIZE, TRAY_SIZE), Image.LANCZOS)


def main() -> int:
    src = load_source()

    appicon = make_appicon(src)
    appicon.save(OUT_APPICON, format="PNG", optimize=True)
    print(f"[write] {OUT_APPICON.relative_to(ROOT)}  {appicon.size}")

    OUT_TRAY.parent.mkdir(parents=True, exist_ok=True)
    tray = make_tray_icon(src)
    tray.save(OUT_TRAY, format="PNG", optimize=True)
    print(f"[write] {OUT_TRAY.relative_to(ROOT)}  {tray.size}")

    print()
    print("Done. 接下来:")
    print("  1) 重新构建 app:        make build-mac")
    print("  2) 重启已运行的 GoPaste 才能看到新菜单栏图标")
    print()
    print("如需重新生成: 编辑 build/appicon.src.png 后再次运行本脚本。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
