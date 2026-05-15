## Release v0.5.0

### ✨ New Features
- Emoji 表情面板：全新自研 Emoji Picker，基于 Fluent UI Emoji sprite 渲染，支持 6 种肤色切换、分类导航、搜索过滤、hover 大图预览（矢量 SVG 高清）、单击复制 / 双击粘贴
- Emoji 主开关 + 扩展模式：可在设置中开关 Emoji 功能，扩展模式显示完整分类（物品/旗帜）与肤色选择器，关闭时节省 ~50MB GPU 内存
- 激活时清空搜索栏：新增窗口设置开关，激活窗口时自动清空主视图和 Emoji 视图的搜索栏
- 激活时切换至全部分组现在也覆盖 Emoji tab，从 Emoji 视图自动切回主列表
- 托盘菜单新增"打开设置"入口
- 代码类型识别增强：改进 JSON/XML/HTML 的检测准确率
- 官网链接集中管理 + Apache-2.0 协议 + 关于对话框改进

### 🐛 Bug Fixes
- Windows 剪贴板：自研原生读取实现，修复 access violation 崩溃问题
- Emoji hover 大图不再模糊：从 48px sprite 2× 放大改为按需加载矢量 SVG
- 去除 Emoji 网格 border-radius 圆角裁切，方形 emoji（方块、菱形、旗帜）不再被切角
- 修复 Tab 切换时 Emoji 面板白屏闪烁和重复渲染
- 修复详情页按钮对齐 + 常用 Emoji 行显示 Fluent 风格预览
- 修复 macOS NSStatusItem tray 缺少 onWebsite 回调

### 🔧 Improvements
- Emoji 单元格从 36px 放大到 43px（1.2×），视觉更舒适
- Back-to-top 按钮与网格对齐优化
- 构建脚本输出 hi-res SVG 资源 + manifest，Vite 按需加载，dist 主 chunk 无膨胀
- CI 自动从 tag annotation 读取 Release notes，无需手动填写

### 📄 Other Changes
- 新增 push-tag-public 部署命令
- 同步 wailsjs ExportDataToFile 绑定
