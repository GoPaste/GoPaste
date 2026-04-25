#!/usr/bin/env python3
"""
gen_tray_icon_gray.py — 从 appicon.src.png 生成灰色系菜单栏图标。

处理流程：
  1. 缩放到 44×44（22pt @2x）
  2. 转灰度
  3. 颜色反转（深色变浅色，浅色变深色）→ 白底深灰P
  4. 适当提升亮度 + 对比度，让背景趋近白色同时保持P字清晰
  5. 保留原始 alpha 通道

输入：build/appicon.src.png
输出：internal/tray/icon_gray.png

运行：python3 scripts/gen_tray_icon_gray.py
"""

from __future__ import annotations
import sys
from pathlib import Path
from PIL import Image, ImageOps, ImageEnhance

ROOT = Path(__file__).resolve().parent.parent
SRC  = ROOT / "build" / "appicon.src.png"
DST  = ROOT / "internal" / "tray" / "icon_gray.png"

TRAY_SIZE   = 44
BRIGHTNESS  = 1.3   # >1 = 更亮，让背景趋近白色
CONTRAST    = 1.4   # >1 = 增强对比，保持P字清晰


def main() -> int:
    if not SRC.exists():
        sys.exit(f"error: 找不到源图 {SRC}")

    img = Image.open(SRC).convert("RGBA").resize((TRAY_SIZE, TRAY_SIZE), Image.LANCZOS)
    r, g, b, a = img.split()

    # 转灰度 → 反转（白底深灰P）
    lum = ImageOps.grayscale(img.convert("RGB"))
    inv = ImageOps.invert(lum)

    # 亮度 + 对比度调整
    inv = ImageEnhance.Brightness(inv).enhance(BRIGHTNESS)
    inv = ImageEnhance.Contrast(inv).enhance(CONTRAST)

    # 合并回 RGBA，保留原始 alpha
    result = Image.merge("RGBA", [inv, inv, inv, a])

    DST.parent.mkdir(parents=True, exist_ok=True)
    result.save(DST, format="PNG", optimize=True)
    print(f"[write] {DST.relative_to(ROOT)}  {result.size}")
    print("Done. 重新构建后生效: make build-mac-arm")
    return 0


if __name__ == "__main__":
    sys.exit(main())
