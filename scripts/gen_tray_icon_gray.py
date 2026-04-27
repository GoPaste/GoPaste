#!/usr/bin/env python3
"""
gen_tray_icon_gray.py — 从 appicon.src.png 生成灰色系托盘图标。

处理流程：
  1. 缩放到目标尺寸
  2. 转灰度
  3. 颜色反转（深色变浅色，浅色变深色）→ 白底深灰P
  4. 适当提升亮度 + 对比度，让背景趋近白色同时保持P字清晰
  5. 保留原始 alpha 通道

输入：build/appicon.src.png
输出：
  internal/tray/icon_gray.png  — macOS 菜单栏图标（44×44）
  internal/tray/icon_gray.ico  — Windows 托盘图标（16/32/48/256 多尺寸）

运行：python3 scripts/gen_tray_icon_gray.py
"""

from __future__ import annotations
import sys
from pathlib import Path
from PIL import Image, ImageOps, ImageEnhance

ROOT     = Path(__file__).resolve().parent.parent
SRC      = ROOT / "build" / "appicon.src.png"
DST_PNG  = ROOT / "internal" / "tray" / "icons" / "tray-gray.png"
DST_ICO  = ROOT / "internal" / "tray" / "icons" / "tray-gray.ico"

TRAY_SIZE   = 44
BRIGHTNESS  = 1.3   # >1 = 更亮，让背景趋近白色
CONTRAST    = 1.4   # >1 = 增强对比，保持P字清晰

# Windows ICO：只存 256px 高分辨率层，让系统自行缩放（与 tray.ico 一致）
ICO_SIZE    = 256


def make_gray(src_img: Image.Image, size: int) -> Image.Image:
    """将源图缩放到 size×size 并转为灰色系（反转+亮度对比调整）。"""
    img = src_img.convert("RGBA").resize((size, size), Image.LANCZOS)
    _, _, _, a = img.split()

    lum = ImageOps.grayscale(img.convert("RGB"))
    inv = ImageOps.invert(lum)
    inv = ImageEnhance.Brightness(inv).enhance(BRIGHTNESS)
    inv = ImageEnhance.Contrast(inv).enhance(CONTRAST)

    return Image.merge("RGBA", [inv, inv, inv, a])


def main() -> int:
    if not SRC.exists():
        sys.exit(f"error: 找不到源图 {SRC}")

    src = Image.open(SRC)
    DST_PNG.parent.mkdir(parents=True, exist_ok=True)

    # macOS PNG（44×44）
    result_png = make_gray(src, TRAY_SIZE)
    result_png.save(DST_PNG, format="PNG", optimize=True)
    print(f"[write] {DST_PNG.relative_to(ROOT)}  {result_png.size}")

    # Windows ICO（256px 单层，与 tray.ico 一致，让系统自行缩放）
    ico_img = make_gray(src, ICO_SIZE)
    ico_img.save(DST_ICO, format="ICO", sizes=[(ICO_SIZE, ICO_SIZE)])
    print(f"[write] {DST_ICO.relative_to(ROOT)}  size={ICO_SIZE}x{ICO_SIZE}")

    print("Done. 重新构建后生效: make build")
    return 0


if __name__ == "__main__":
    sys.exit(main())
