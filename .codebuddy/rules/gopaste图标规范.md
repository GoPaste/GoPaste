# 项目规范（gopaste）

本文档记录项目的硬性约定，AI 助手在生成/修改代码时必须遵守。

---

## 图标规范

**必须使用 [Lucide Icons](https://lucide.dev/icons) 作为项目唯一的图标库。**

### 规则

1. **禁止使用 emoji 作为 UI 图标**（包括但不限于 `📋 🔍 ⭐ 📌 ⚙ ✕ ─ ◻ 📝 🖼 🔗 💻 🗑 ✓ 📤 📥 👁 ←`）。
2. **禁止混用其他图标库**（Font Awesome / Iconify 非 Lucide 集合 / Material Icons 等）。
3. 所有图标统一从 **`lucide-vue-next`** 引入（Vue 3 官方包）。
4. 图标组件按需引入，命名遵循 Lucide 官方 PascalCase，例如：
   ```ts
   import { Search, Star, Pin, Settings, X, Minus, Square } from 'lucide-vue-next'
   ```
5. 默认图标大小 16px（列表/按钮），标题栏 14px，空状态插图 48px。通过 `:size="16"` 传入。
6. 图标颜色通过 CSS `color` 继承；不要写死 `stroke` / `fill`。
7. 文案（如 console.log、日志、Git commit）里仍可保留 emoji；**仅限 UI 层面的视觉元素**受此约束。

### Lucide 常用映射表

| 用途 | Lucide 图标 |
|------|------------|
| 搜索 | `Search` |
| 收藏 | `Star` |
| 置顶 | `Pin` |
| 删除 | `Trash2` / `X` |
| 复制 | `Copy` / `Clipboard` |
| 查看详情 | `Eye` |
| 设置 | `Settings` |
| 关闭（窗口） | `X` |
| 最小化 | `Minus` |
| 最大化/隐藏 | `Square` / `Minimize2` |
| 返回 | `ArrowLeft` / `ChevronLeft` |
| 导出 | `Download` / `Upload` |
| 清空 | `Trash2` |
| 文本类型 | `FileText` |
| 图片类型 | `Image` |
| 链接类型 | `Link` |
| 代码类型 | `Code2` |
| 全部/列表 | `List` / `LayoutList` |
| 时间 | `Clock` |
| 空状态（剪切板） | `ClipboardList` |

> 新增图标需求若无法在表中找到，先到 https://lucide.dev/icons 搜索，按"功能语义"优先于"外观相似"选择。

### 检查清单（Code Review）

- [ ] 不存在 emoji 作为图标
- [ ] 所有 `import` 来自 `lucide-vue-next`
- [ ] 图标尺寸遵守分级（16 / 14 / 48）
- [ ] 图标颜色通过 CSS 继承

---

## 其他约定（占位，后续补充）

- 所有前端提示文字保持**中文**，与剪切板工具定位一致
- 新增后端 RPC 方法需在 `app.go` 编写 GoDoc 注释
- `internal/` 下的包均需带 package-level 注释
