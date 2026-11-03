# GoPaste 宣传片 · 设计 spec（终检基准）

> 本文件是制作与独立终检的唯一基准。制作模式：**共同创作**（阶段 0–3 由用户逐项确认）→ 用户于分镜阶段指示
> "别问我了，你直接做"，自此切换为**自主自由创作**，分镜细化与全部执行决策由 Agent 记录于本文件。

## 1. 产品简报（用户已确认）

| 项目 | 结论 | 依据 |
|---|---|---|
| 定位 | 轻量、快速、安全的跨平台剪贴板管理工具，开源免费 | README.md:7 |
| 目标用户 | 重度复制粘贴的开发者 / 文字工作者，键盘流、在意隐私 | 热键体系、代码高亮、本地加密 |
| 用途 | 官网首屏 / README 头图位 / 社交平台 | 官网 gopaste.wetools.cc；docs/todo.md 缺视频 |
| 核心卖点 | 一键呼出 · 不丢历史 · 一键贴回 · 数据只在本机 | docs/DESIGN.md 设计愿景 |
| 必须展示 | 热键呼出、多类型历史、搜索、分类 Tab、回车粘贴、AES 本地加密、三平台 | 均已实现 |
| 禁止宣传 | 自动更新安装（只做了检查更新）、JSON 导入、多设备同步、富文本 | README 路线图未勾选 |
| 规格 | 1920×1080 · 30fps · 约 35s · **简体中文** | 用户确认 |
| 音乐 | `house-vibez.mp3`（Mixkit，Lily J），卡拍；交付带 BGM / 无 BGM 两版 | 用户确认（tech-house 授权不可追溯，弃用） |
| 片尾 CTA | 「开源免费 · Windows / macOS / Linux」+ gopaste.wetools.cc；不放 GitHub | 用户确认 |
| 数据口径 | **全部虚构**：mock 替身注入编造条目，不读本机剪贴板，不改业务代码 | 剪贴板=个人数据 |

## 2. 需求 → 执行决策表

| 需求 | 执行 |
|---|---|
| 用途：官网/社媒 | 前 8s 讲清"热键呼出"；片尾官网 URL 为全片最大辅助文字之一 |
| 受众：键盘流 | 键帽只在两处当主角：`Alt` + `` ` ``（呼出）、`↵`（粘贴） |
| 三卖点 | 中段 `bottom-push-stack-wipe` 三章：01 找得到（蓝）/ 02 贴回去（黄）/ 03 只在本机（绿） |
| 中文、35s | 每章一句 112px 大标题 + 2 行 34px 说明，不另加底部字幕条（大标题即解说，C1） |
| 音乐卡拍 | 所有换章与字标落地钉 `beatF(n)`，见 §6 |
| 数据合规 | 采集服务 `capture/serve.mjs` 以 alias 替换 wailsjs 绑定；数据见 `capture/mock/data.ts`；图片条目为程序生成插画 |
| 视觉 | §3 tokens |

## 3. 视觉方向（用户确认：A「深夜工作台」为主 + B 键帽借用两处）

styleframe：`out/styleframes/sf1–sf4.png`、`contact-sheet.png`（用户已确认）。

| Token | 值 | 来源 |
|---|---|---|
| 暗底 / 表面 / 边 | `#14161c` / `#242833` / `#2a2f3a` | App.vue :root |
| 正文 / 次级 / 弱 | `#e6e8ef` / `#a8adbd` / `#6b7280` | App.vue |
| 强调蓝（UI） | `#3b82f6` | App.vue --accent |
| 品牌蓝（章 1 底） | `#1770ff`（hi `#2f80ff` / lo `#0f5fe6` / shade `#062a73`） | build/appicon.png 实测 |
| 章 2 底 | 收藏黄 `#fbbf24`（深色字 `#1a1406`） | App.vue --warning（收藏星标色） |
| 章 3 底 | 成功绿 `#22c55e`（深色字 `#052e16`） | App.vue light --success |
| 字体 | Noto Sans SC 400/500/700/900（替代 PingFang，OFL）；JetBrains Mono（替代 SF Mono，OFL） | 渲染机无 PingFang |
| 图标 | Lucide（lucide-static，与产品 lucide-vue-next 同源） | 项目图标规范 |
| 圆角 | 面板 16 / 卡 10–12 / 胶囊 12 | App.vue |

偏离说明：三章满屏饱和底色是对产品低饱和暗色的扩展（用户在选择 A 方案时已知情确认），色值全部取自产品自身状态色。

## 4. 功能 → 镜头映射（Gallery 卡名均经 `gallery/api/library.json` 校验，均为默认变体）

| 功能 | 镜头卡（style-key） | 准确 demo |
|---|---|---|
| 品牌开场 | `icon-field-colorize` | demos/opening/icon-field-colorize/IconFieldColorize.tsx |
| 热键呼出 | `spotlight-hero-card` | demos/opening/spotlight-hero-card/SpotlightHeroCard.tsx |
| 章节骨架 | `bottom-push-stack-wipe` | demos/transition/bottom-push-stack-wipe/BottomPushStackWipe.tsx |
| 自动记录 | `row-embed` | demos/ui-entrance/row-embed/RowEmbed.tsx |
| 搜索命中 | `type-and-filter` | demos/interaction/type-and-filter/TypeAndFilter.tsx |
| 分类切换 | `segmented-thumb-hero`（只取 thumb 8f 滑动 + 橡皮筋 + 1.5% 回弹） | demos/interaction/segmented-thumb-hero/SegmentedThumbHero.tsx |
| 回车贴回 | `document-typewriter-reveal`（只取"块两两一对 wipe + 单笔尖"写入法） | demos/ui-entrance/document-typewriter-reveal/DocumentTypewriterReveal.tsx |
| 本地加密 | `scan-bracket-sweep` | demos/effects/scan-bracket-sweep/ScanBracketSweep.tsx |
| 章 3 → 片尾 | `shot-transitions` A 式 flash-cut | assets/lib/FlashCut.tsx |
| 片尾 | `outro-group-photo-launch` | demos/outro/outro-group-photo-launch/OutroGroupPhotoLaunch.tsx |

## 5. 最终分镜（帧级，30fps，总长 1064f = 35.47s）

> 与用户确认稿相比的执行期调整（自主模式记录）：
> ① 三章边界改落在乐句（16 拍）上：b16 / b32 / b48，片尾 b60 起、字标 b64 落地——b32 是乐曲第一次能量抬升、
>   b60 有一次小 breakdown（RMS 0.243→0.199），推入/起势与音乐结构对齐；
> ② 章 1 前段补 `row-embed`（"自动记录"），与说明文案第一行对应，避免章内 4s 空停；
> ③ ↵ 粘贴落在 b40；总长 1050f → 1064f。
> ④ 章 1、章 2 各含两个顺序子动作（嵌入→搜索；切 Tab→回车），同一面板、同一机位，记为对"每镜一个动效"的有意识放宽。

| # | 帧 | 拍 | 内容 | 镜头卡 | 文案 | SFX（详见 src/timeline.ts SFX 表） |
|---|---|---|---|---|---|---|
| S1 | 0–120 | b0–b8 | 键帽式类型图标场（Lucide：文本/图片/链接/代码/文件/星/钉/搜索/剪贴板）浮现 0–37 → b3 起 4 道蓝色系色波向下扫翻 → 中心暗幕 + appicon 字标 + 副标升起，字标落定后 hold ≥1s | icon-field-colorize | GoPaste / 轻量 · 快速 · 安全的剪贴板管理工具 | 翻色 shimmer |
| S2 | 120–264 | b8–b16（+26f 被顶出） | 关灯桌面 + 虚化编辑器；推闸开灯；`Alt` f124 / `` ` `` f135(b9) 按下；面板从编辑器平面弹起（rise 过冲）→ 悬浮 bob → 蓝色轮廓光束两圈 → 回落到浮层高度 + 落地光；3D 注记 | spotlight-hero-card | 任意窗口，/ 一键呼出 | 灯闸、两声键、whoosh、sparkle、snap |
| CH1 | 238–500 | b16–b32 | 蓝章推入；面板钉右栏：6 行从空中嵌入槽位（强调缝）→ 搜索条放大镜 → f350「发」f356「布」→ f371(b25) 非目标行错峰淡出下沉，3 条命中滑到槽位，状态栏 128→3 | bottom-push + row-embed + type-and-filter | 01 找得到 / 复制过的，都找得到。/ 文本、图片、链接、代码、文件自动记录 · 输入关键字，即刻命中 | 推入 thump、两声键、轻掠 |
| CH2 | 474–736 | b32–b48 | 黄章推入；编辑器 + 浮在其上的面板；Tab 条放大：thumb f533(b36) 全部→代码 8f 滑动，列表换为代码条目；f563(b38) 选中首条；`↵` f592(b40) 按下 → 面板收起 → 代码在编辑器光标处逐行落入 | bottom-push + segmented-thumb + typewriter | 02 贴回去 / 一键，贴回原处。/ 切换分类，回车即贴 · 内容直接落进当前窗口 | 推入、Tab tick、回车、落入 |
| CH3 | 710–892 | b48–b60 | 绿章推入；面板静止；四角括号 f740(b50) 落位 → 扫描光带往复 4 趟 → f828(b56) 括号转亮 + 三枚标签 b56/b57/b58 依次弹出；尾段推进 + 白闪 | bottom-push + scan-bracket-sweep + flash-cut | 03 只在本机 / 你的剪贴板，只属于你。/ AES-256-GCM 本地加密 · 密钥交给系统钥匙串 / 标签：AES-256-GCM · 系统钥匙串 · 从不上云 | 扫描低鸣、锁扣 |
| OUT | 882–1064 | b60–b72 | 暗场；片中元素四方飞入合影（代码行、文件行、图片缩略、搜索条、Tab 条、浅色面板切片、三键帽、三色图标块）→ crane → b64(f946) appicon + GoPaste letterpress 落地 → 线 + 副标 + URL → hold → 尾 12f 淡出 | outro-group-photo-launch | GoPaste / 开源免费 · Windows / macOS / Linux / gopaste.wetools.cc | riser → impact(f946) → sparkle |

整画面级冲击（R4）：仅 b64 字标落地 1 处（推入是转场，不计）。

## 6. 节拍网格（analysis/beat_data.json、grid_drift.json）

BPM 122.002，t0 0.0669s，T 0.49180s；拟合残差 max 15.6ms；match 96.5%（miss 全在前奏 b7–b14 与间奏，未用作钉点）；
各钉点实测瞬态误差：b16 −0ms、b32 −9ms、b48 −6ms、b60 −4ms、b64 −3ms。`beatF(n) = round((t0 + n·T)·30)`。

## 7. 素材

- `public/textures/panel/*.png`（3x）+ `layout.json`：`capture/capture.mjs` 从 git HEAD 前端快照 + mock 采集（时钟冻结）。
- 编辑器窗口、桌面、键帽、标签为非复刻手搓件（Q1 允许范围），面板一律真实截图。
- `public/brand/appicon.png` ← build/appicon.png。字体 `@fontsource/noto-sans-sc`、`@fontsource/jetbrains-mono`（OFL）。
- SFX：Freesound CC0，来源见 `public/audio/sfx/FREESOUND-CREDITS.md` 与 `audio-manifest.json`。
