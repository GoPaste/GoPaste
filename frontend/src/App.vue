<script lang="ts" setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import hljs from 'highlight.js/lib/common'
// 默认使用 github-dark（与深色背景匹配）；浅色模式下在 CSS 里作用域覆盖为 GitHub 浅色调色板。
// 原先用的 github.css 是浅色主题，在深色背景上字符串等 token 几乎不可见。
import 'highlight.js/styles/github-dark.css'
import { t, lang } from './i18n'
import type { Lang } from './i18n'
import {
  ListItems,
  DeleteItem,
  PasteItem,
  PasteText,
  CopyToClipboard,
  TogglePin,
  ToggleFavorite,
  GetContent,
  HideWindow,
  GetSettings,
  RevealInExplorer,
  SaveImageToFile,
  GetFileThumbnail,
  SetNote,
  OpenURL,
} from '../wailsjs/go/main/App'
import { EventsOn, EventsOff, WindowSetAlwaysOnTop } from '../wailsjs/runtime/runtime'
import type { types } from '../wailsjs/go/models'
import Settings from './views/Settings.vue'
import EmojiPicker from './views/EmojiPicker.vue'
import {
  Search,
  Star,
  Settings as SettingsIcon,
  ClipboardList,
  FileText,
  File as FileIcon,
  Image as ImageIcon,
  Link as LinkIcon,
  Code2,
  List,
  Pin,
  Eye,
  Copy,
  Trash2,
  ArrowUp,
  AlertTriangle,
  FolderOpen,
  Download,
  Edit3,
  ExternalLink,
  X,
  Smile,
} from 'lucide-vue-next'

type Item = types.Item
type ItemType = '' | 'text' | 'image' | 'link' | 'code' | 'file'

const view = ref<'main' | 'settings' | 'emoji'>('main')

// Emoji picker component ref:
//   - `cycleCategory(dir)`: called by the global Tab/Shift+Tab handler so
//     those keys switch emoji categories while view==='emoji' (instead of
//     cycling the main filter tabs and leaving emoji view).
//
// Older versions also exposed `spriteReady` to drive a pulse indicator on
// the emoji tab button while a tone-0 sprite was being built in the
// background. Sprites are now pre-bundled WebP imports — there is no
// build phase to wait on — so that flag and the indicator were removed.
const emojiPickerRef = ref<{
  cycleCategory: (dir: 1 | -1) => void
} | null>(null)


// 单输入框 <-> 双 ref 的双向桥。v-model 绑到这个 computed：
//   读：根据当前 view 返回对应 ref 的值（主视图读 keyword，emoji 视图读 emojiKeyword）
//   写：把新值写回对应 ref，另一个 ref 原封不动保留
// 这样用户切 tab 时搜索框会自动显示该 tab 上次的搜索词；在任一 tab 输入都
// 不会影响另一 tab 的搜索状态。
const currentKeyword = computed<string>({
  get: () => (view.value === 'emoji' ? emojiKeyword.value : keyword.value),
  set: (v: string) => {
    if (view.value === 'emoji') emojiKeyword.value = v
    else keyword.value = v
  },
})

const items = ref<Item[]>([])
const total = ref(0)
// 两个搜索词完全独立：
//   keyword       → 主视图（粘贴板内容）的搜索词，驱动后端 ListItems。
//   emojiKeyword  → Emoji 视图的搜索词，只做前端 filter（EmojiPicker 的 `search` prop）。
// 分开存储避免两边互相污染：用户在主视图搜 "cat" 切到 emoji 再搜 "smile"，
// 切回主视图时主列表仍然是 "cat" 的结果，反之亦然。
// 模板里的单一 <input> 通过下面的 `currentKeyword` 计算属性按当前 view
// 自动读写对应的 ref，所以 DOM 层不需要任何双输入框。
const keyword = ref('')
const emojiKeyword = ref('')
const typeFilter = ref<ItemType>('')
const favoriteOnly = ref(false)
const selectedIdx = ref(0)
const detailContent = ref<string>('')
const detailVisible = ref(false)
const detailType = ref<string>('')
// 当前详情条目的原始引用：标题栏渲染 metaParts() 时复用，与列表卡片下方那行元信息保持一致。
// 用 ref<Item> 而不是再加一堆 size/charCount/time 散 ref，避免重复维护。
const detailItem = ref<Item | null>(null)
const loading = ref(false)

// 分页
const PAGE_SIZE = 20
const page = ref(1)
const hasMore = computed(() => items.value.length < total.value)
const loadingMore = ref(false)

// 图片缩略图缓存
const imageThumbs = ref<Record<number, string>>({})
// 文件类型的图片缩略图缓存
const fileThumbs = ref<Record<number, string>>({})

// 判断文件名是否为图片
function isImageFile(name: string): boolean {
  return /\.(png|jpe?g|gif|bmp|webp|ico|svg)$/i.test(name)
}

// 回到顶部
const listRef = ref<HTMLElement | null>(null)
const searchRef = ref<HTMLInputElement | null>(null)
const showBackTop = ref(false)

function onListScroll() {
  const el = listRef.value
  if (!el) return
  showBackTop.value = el.scrollTop > 200
  // 滚动到距离底部还剩总高度的 1/4 时触发加载更多（动态阈值）
  const remaining = el.scrollHeight - el.scrollTop - el.clientHeight
  if (remaining < el.scrollHeight / 4 && hasMore.value && !loadingMore.value) {
    loadMore()
  }
}
function scrollToTop() {
  listRef.value?.scrollTo({ top: 0, behavior: 'smooth' })
}

// 让当前选中项滚动到可视区域内。
// 用于键盘 ArrowUp/ArrowDown 切换选中时，避免选中跑出视口。
// 'nearest' 策略：只在不可见时滚动最小距离，已可见则不动，体验最稳。
async function scrollSelectedIntoView() {
  await nextTick()
  const root = listRef.value
  if (!root) return
  const el = root.querySelector<HTMLElement>(`.item[data-idx="${selectedIdx.value}"]`)
  el?.scrollIntoView({ block: 'nearest' })
}

const typeOptions = computed(() => [
  { key: '' as ItemType | 'fav', label: t('all'), icon: List },
  { key: 'fav' as ItemType | 'fav', label: t('favorite'), icon: Star },
  { key: 'text' as ItemType | 'fav', label: t('text'), icon: FileText },
  { key: 'image' as ItemType | 'fav', label: t('image'), icon: ImageIcon },
  { key: 'file' as ItemType | 'fav', label: t('file'), icon: FileIcon },
  { key: 'link' as ItemType | 'fav', label: t('link'), icon: LinkIcon },
  { key: 'code' as ItemType | 'fav', label: t('code'), icon: Code2 },
])

const typeIconMap: Record<string, any> = {
  text: FileText,
  image: ImageIcon,
  link: LinkIcon,
  code: Code2,
  file: FileIcon,
}

const selected = computed<Item | undefined>(() => items.value[selectedIdx.value])

async function refresh(opts?: { silent?: boolean }) {
  // silent=true：切 filter / 切 keyword 等"原地替换"场景，不显示 loading 占位，
  // 避免列表瞬间清空再填回造成的视觉抖动。
  // 默认 false：初次加载、增删改后刷新——这些场景需要明确反馈。
  const silent = opts?.silent === true
  if (!silent) loading.value = true
  page.value = 1
  try {
    const isFav = typeFilter.value === ('fav' as any)
    const r = await ListItems({
      keyword: keyword.value,
      type: isFav ? '' : typeFilter.value,
      favorite: isFav,
      page: 1,
      pageSize: PAGE_SIZE,
    } as any)
    items.value = (r?.items || []) as Item[]
    total.value = r?.total || 0
    if (selectedIdx.value >= items.value.length) selectedIdx.value = 0
    loadThumbs(items.value)
  } finally {
    if (!silent) loading.value = false
  }
}

// 点击搜索框右侧 X：清空「当前视图」的关键字并保持焦点。
// - 主视图：清空 keyword 并刷新列表
// - Emoji 视图：清空 emojiKeyword（EmojiPicker 通过 prop 自动重建，不需后端调用）
function clearKeyword() {
  if (view.value === 'emoji') {
    if (!emojiKeyword.value) return
    emojiKeyword.value = ''
  } else {
    if (!keyword.value) return
    keyword.value = ''
    refresh({ silent: true })
  }
  searchRef.value?.focus()
}

// IME 组合输入状态。中文/日文/韩文等 IME 在拼音→候选→上屏的过程中，
// 浏览器会在每次按键触发 `input` 事件，但 Vue 的 v-model 默认在 IME
// composition 期间不更新绑定值（避免脏数据）。如果不区分 composing，
// 模板里 `@input="refresh(...)"` 会用旧的（空）keyword 反复发起搜索，
// 而 compositionend 时不一定再触发 input —— 最终用户输入的中文搜索词
// 一次都不会真正命中。
//
// 处理：
//   - compositionstart → 标记正在组合，input 阶段跳过 refresh
//   - compositionend  → 解除标记，并手动同步 v-model 与触发一次 refresh
//     （compositionend 在 Chromium 下 input 事件之前触发，需要主动读 DOM 值）
const isComposingSearch = ref(false)
function onSearchCompositionStart() {
  isComposingSearch.value = true
}
function onSearchCompositionEnd(e: CompositionEvent) {
  isComposingSearch.value = false
  // 主动把 IME 上屏的最终值同步到 v-model 并触发一次搜索。
  // 不依赖紧随其后的 input 事件 —— 不同浏览器/IME 时序不一致，
  // 主动同步能确保最终一次搜索一定执行。
  const el = e.target as HTMLInputElement | null
  if (el) {
    // 写到当前视图对应的 ref（currentKeyword 是桥）
    if (currentKeyword.value !== el.value) currentKeyword.value = el.value
  }
  // Emoji 视图的搜索是纯前端 filter，EmojiPicker 已经通过 prop 响应变化，
  // 不需要调用后端 refresh（后端 ListItems 和 emoji 无关）。
  if (view.value !== 'emoji') refresh({ silent: true })
}
function onSearchInput() {
  // composing 中的 input 事件直接丢弃 —— v-model 此时也不会更新值，
  // 跑 refresh 只会用空 keyword 反复扫库。
  if (isComposingSearch.value) return
  // Emoji 视图下 v-model 已把新值写进 emojiKeyword，EmojiPicker 响应式重算，
  // 无需触发剪贴板列表刷新。
  if (view.value === 'emoji') return
  refresh({ silent: true })
}

async function loadMore() {
  if (loadingMore.value || !hasMore.value) return
  loadingMore.value = true
  try {
    const nextPage = page.value + 1
    const isFav = typeFilter.value === ('fav' as any)
    const r = await ListItems({
      keyword: keyword.value,
      type: isFav ? '' : typeFilter.value,
      favorite: isFav,
      page: nextPage,
      pageSize: PAGE_SIZE,
    } as any)
    const newItems = (r?.items || []) as Item[]
    items.value.push(...newItems)
    page.value = nextPage
    loadThumbs(newItems)
  } finally {
    loadingMore.value = false
  }
}

function loadThumbs(list: Item[]) {
  for (const it of list) {
    if (it.type === 'image' && !imageThumbs.value[it.id]) {
      loadThumb(it.id)
    }
    if (it.type === 'file' && !fileThumbs.value[it.id]) {
      const names = (it.preview || '').split('\n')
      if (names.length === 1 && isImageFile(names[0])) {
        loadFileThumb(it.id)
      }
    }
  }
}

async function loadThumb(id: number) {
  try {
    const b64 = await GetContent(id)
    imageThumbs.value[id] = `data:image/png;base64,${b64}`
  } catch { /* ignore */ }
}

async function loadFileThumb(id: number) {
  try {
    const b64 = await GetFileThumbnail(id)
    if (b64) fileThumbs.value[id] = `data:image/png;base64,${b64}`
  } catch { /* ignore */ }
}

// base64 → UTF-8 字符串（支持中文）
function b64ToUtf8(b64: string): string {
  const bytes = Uint8Array.from(atob(b64), c => c.charCodeAt(0))
  return new TextDecoder('utf-8').decode(bytes)
}

// HTML 转义。用于非代码场景的纯文本展示防御。
function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

/**
 * 通用代码高亮：基于 highlight.js。
 * - 若上层指定了 language（来自后端 internal/lang.Detect 的检测结果），走精确 highlight，
 *   保证详情面板与列表展示同一种语言；
 * - 否则退化为 highlightAuto 自动识别。
 * 仅引入 lib/common（覆盖 ~35 种主流语言，体积约 80KB），主题用 GitHub 浅色。
 * 高亮失败兜底为转义后的纯文本，永远不会破坏 DOM。
 */
function highlightCode(src: string, lang?: string): string {
  try {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(src, { language: lang, ignoreIllegals: true }).value
    }
    return hljs.highlightAuto(src).value
  } catch {
    return escapeHtml(src)
  }
}

// 详情类型
const detailIsImage = ref(false)
// 详情面板当前条目的语言（由 showDetail 设置；为 hljs 已知语言时强制按其高亮，
// 与列表的 metaParts 显示保持一致；其它语言由 highlightCode 自动检测兜底）。
const detailLanguage = ref<string>('')

async function showDetail(it: Item) {
  detailType.value = it.type
  detailLanguage.value = it.language || ''
  detailItem.value = it
  try {
    // 图片类型：应用内预览
    if (it.type === 'image') {
      const b64 = await GetContent(it.id)
      detailContent.value = `data:image/png;base64,${b64}`
      detailIsImage.value = true
      detailVisible.value = true
      return
    }
    // 文件类型：单个图片文件，应用内预览
    if (it.type === 'file') {
      const names = (it.preview || '').split('\n')
      if (names.length === 1 && isImageFile(names[0])) {
        const b64 = await GetFileThumbnail(it.id)
        if (b64) {
          detailContent.value = `data:image/png;base64,${b64}`
          detailIsImage.value = true
          detailVisible.value = true
          return
        }
      }
    }
    // 其他：文本预览
    const b64 = await GetContent(it.id)
    detailContent.value = b64ToUtf8(b64)
    detailIsImage.value = false
    detailVisible.value = true
  } catch (e) {
    console.error(e)
  }
}

async function doPaste(it: Item) {
  try {
    console.log('[paste] trigger=', pasteTrigger.value, 'id=', it.id)
    await PasteItem(it.id)
  } catch (e) {
    console.error('[paste] PasteItem failed', e)
  }
}
async function doCopy(it: Item) { await CopyToClipboard(it.id) }

// 粘贴触发方式：'single' 单击立即粘贴 | 'double' 双击粘贴（默认，单击仅选中）
const pasteTrigger = ref<'single' | 'double'>('double')
// 是否启用 Alt+1..6 全局/应用内切分类热键。后端配置同名字段镜像，关闭后两端一并生效。
const tabHotkeysEnabled = ref(true)
// Emoji 功能总开关（默认开启）。
//   关闭：emoji tab、emoji 视图都隐藏；EmojiPicker 组件被 v-if 卸载，
//        随 GC 回收数据结构（emoji-mart-data ~3MB JSON、orderedEmojis 数组、
//        skinsMap 等）。注意：sprite WebP 资源由 Vite 作为静态资产导入，
//        浏览器/WebView 全局缓存命中持续，关掉组件并不会释放它们。
//   开启：组件重新挂载，sprite WebP 已在缓存中（首次后是即时命中），
//        渲染开销退化为一次 emoji-mart-data 的动态 import + map 构建，
//        ~50ms 即可第一帧出图。
// 通过 emojiMounted（防抖派生）控制实际挂载，避免快速反复 toggle 时
// 反复触发 mount/unmount → 刚开始构建 tone 0 又被打断。
const emojiEnabled = ref(true)
const emojiMounted = ref(true)
let emojiMountDebounceTimer: number | null = null
const EMOJI_MOUNT_DEBOUNCE_MS = 300
// 是否启用扩展 Emoji（显示物品/旗帜分类 + 肤色按钮）。设置变更时由 loadSettings 同步过来，
// EmojiPicker 通过 prop 接收，决定 visibleCategories 是否过滤 objects/flags
// 以及是否渲染肤色按钮。所有 sprite 资源都已是预打包 WebP，切换 prop 不
// 触发任何异步构建/下载。
const extendedEmoji = ref(false)
// 单击模式下的延时触发器：区分单击/双击（双击时取消单击的 doPaste）
let singleClickTimer: number | null = null
const DBLCLICK_THRESHOLD_MS = 250
// 空格键的"延迟单击"定时器：第一次空格延迟打开预览；阈值内又一次 → 升级为双击专属操作。
// 与 singleClickTimer 同范式，但通道独立（鼠标点条目 / 键盘按空格 互不影响）。
// 阈值取 200ms（< 鼠标 250ms）：键盘连按比鼠标快，留出舒适的双击窗口同时控制单击预览的延迟感。
let spacePreviewTimer: number | null = null
const SPACE_DBLCLICK_THRESHOLD_MS = 200

function onItemClick(idx: number, it: Item) {
  selectedIdx.value = idx
  if (pasteTrigger.value !== 'single') return
  // 延迟执行，若在阈值内触发 dblclick，由 onItemDblClick 取消
  if (singleClickTimer != null) window.clearTimeout(singleClickTimer)
  singleClickTimer = window.setTimeout(() => {
    singleClickTimer = null
    doPaste(it)
  }, DBLCLICK_THRESHOLD_MS)
}

function onItemDblClick(it: Item) {
  // 取消单击模式下排队的 doPaste，避免双重触发
  if (singleClickTimer != null) {
    window.clearTimeout(singleClickTimer)
    singleClickTimer = null
  }
  doPaste(it)
}

// 自定义确认弹窗
const confirmVisible = ref(false)
const confirmMsg = ref('')
let confirmResolve: ((v: boolean) => void) | null = null

function showConfirm(msg: string): Promise<boolean> {
  confirmMsg.value = msg
  confirmVisible.value = true
  return new Promise(resolve => { confirmResolve = resolve })
}
function onConfirmOk() { confirmVisible.value = false; confirmResolve?.(true) }
function onConfirmCancel() { confirmVisible.value = false; confirmResolve?.(false) }

// 提示弹窗（只有确定按钮，无取消）
const alertVisible = ref(false)
const alertMsg = ref('')
let alertResolve: (() => void) | null = null

function showAlert(msg: string): Promise<void> {
  alertMsg.value = msg
  alertVisible.value = true
  return new Promise(resolve => { alertResolve = resolve })
}
function onAlertOk() { alertVisible.value = false; alertResolve?.() }

async function doDelete(it: Item) {
  if (it.favorite) {
    await showAlert(t('deleteFavoriteTip'))
    return
  }
  const ok = await showConfirm(t('confirmDelete'))
  if (!ok) return
  await DeleteItem(it.id)
  delete imageThumbs.value[it.id]
  await refresh()
}
async function doTogglePin(it: Item) { await TogglePin(it.id, !it.pinned); await refresh() }
async function doToggleFav(it: Item) { await ToggleFavorite(it.id, !it.favorite); await refresh() }

// 按内容类型的专属操作：链接 → 浏览器打开；文件 → 资源管理器；图片 → 保存
async function doOpenUrl(it: Item) {
  try {
    const b64 = await GetContent(it.id)
    const url = b64ToUtf8(b64).trim()
    if (url) await OpenURL(url)
  } catch (e) { console.error(e) }
}
async function doRevealFile(it: Item) {
  try {
    const b64 = await GetContent(it.id)
    const paths = b64ToUtf8(b64).split('\n')
    if (paths[0]) await RevealInExplorer(paths[0])
  } catch (e) { console.error(e) }
}
async function doSaveImage(it: Item) {
  // 见 suppressBlurHide 的注释：期间禁用自动隐藏，避免主面板被 orderOut
  // 导致依附其上的 NSSavePanel sheet 被一起隐藏。
  suppressBlurHide.value++
  try { await SaveImageToFile(it.id) }
  catch (e) { console.error(e) }
  finally { suppressBlurHide.value-- }
}

// 按内容类型分发"专属操作"：双击空格的目标动作。
//   image → 保存（弹系统对话框）
//   file  → 在资源管理器/Finder 中显示
//   link  → 浏览器打开
//   其它  → 无事发生（不报错、不弹提示，与设计预期一致）
function runPrimaryAction(it: Item) {
  switch (it.type) {
    case 'image': doSaveImage(it); break
    case 'file':  doRevealFile(it); break
    case 'link':  doOpenUrl(it); break
    // text / code 等：无专属动作，按需求"无事发生"
  }
}

// 是否存在双击专属动作。无则空格走"零延迟 toggle"快路径，
// 避免给 text/code 这些没有目的地的类型平白塞 150ms 卡顿。
function hasPrimaryAction(it: Item): boolean {
  return it.type === 'image' || it.type === 'file' || it.type === 'link'
}

// 备注弹窗
const noteVisible = ref(false)
const noteText = ref('')
const noteItem = ref<Item | null>(null)

function showNoteDialog(it: Item) {
  noteItem.value = it
  noteText.value = it.note || ''
  noteVisible.value = true
}
async function onNoteOk() {
  if (noteItem.value) {
    await SetNote(noteItem.value.id, noteText.value.trim())
    await refresh()
  }
  noteVisible.value = false
}
function onNoteCancel() { noteVisible.value = false }

// 右键菜单
const ctxMenu = ref<{ visible: boolean; x: number; y: number; item: Item | null }>({
  visible: false, x: 0, y: 0, item: null
})

function onItemContextMenu(e: MouseEvent, it: Item) {
  e.preventDefault()
  ctxMenu.value = { visible: true, x: e.clientX, y: e.clientY, item: it }
}
function closeCtxMenu() { ctxMenu.value.visible = false }

// 菜单定位：确保不超出窗口边界
const ctxMenuStyle = computed(() => {
  const menuW = 180, menuH = 300
  let x = ctxMenu.value.x
  let y = ctxMenu.value.y
  if (x + menuW > window.innerWidth) x = window.innerWidth - menuW - 4
  if (y + menuH > window.innerHeight) y = window.innerHeight - menuH - 4
  if (x < 0) x = 4
  if (y < 0) y = 4
  return { left: x + 'px', top: y + 'px' }
})

async function ctxCopy() { if (ctxMenu.value.item) await doCopy(ctxMenu.value.item); closeCtxMenu() }
async function ctxPaste() { if (ctxMenu.value.item) await doPaste(ctxMenu.value.item); closeCtxMenu() }
async function ctxFav() { if (ctxMenu.value.item) await doToggleFav(ctxMenu.value.item); closeCtxMenu() }
async function ctxPin() { if (ctxMenu.value.item) await doTogglePin(ctxMenu.value.item); closeCtxMenu() }
async function ctxDelete() { if (ctxMenu.value.item) await doDelete(ctxMenu.value.item); closeCtxMenu() }
async function ctxDetail() { if (ctxMenu.value.item) await showDetail(ctxMenu.value.item); closeCtxMenu() }
function ctxNote() { if (ctxMenu.value.item) showNoteDialog(ctxMenu.value.item); closeCtxMenu() }
async function ctxReveal() { if (ctxMenu.value.item) await doRevealFile(ctxMenu.value.item); closeCtxMenu() }
async function ctxSaveImage() { if (ctxMenu.value.item) await doSaveImage(ctxMenu.value.item); closeCtxMenu() }
async function ctxOpenInBrowser() { if (ctxMenu.value.item) await doOpenUrl(ctxMenu.value.item); closeCtxMenu() }

// 主题
const theme = ref<'dark' | 'light'>('dark')

// 窗口置顶
const alwaysOnTop = ref(false)

// 原生系统对话框（NSSavePanel / OpenFileDialog 等）会抢走 WebView 焦点，
// 此时必须**临时**禁用 onWindowBlur 的自动隐藏，否则：
//   1. 点"保存图片"→ 后端调 Wails SaveFileDialog（beginSheetModalForWindow 附着主窗口）
//   2. WebView 失焦 → onWindowBlur 150ms 后 HideWindow() → orderOut 主面板
//   3. sheet 依附的 window 被 orderOut，save 对话框跟着一起不可见
//      ——用户看到的现象就是"点了保存没反应 / 什么都没发生"。
// 用 counter 而非 bool，允许多个异步原生对话框嵌套（虽然当前只有一个）。
const suppressBlurHide = ref(0)

function toggleAlwaysOnTop() {
  alwaysOnTop.value = !alwaysOnTop.value
  WindowSetAlwaysOnTop(alwaysOnTop.value)
}

// 同步 WebView2 底层背景色：
// Windows WebView2 在非透明模式下，自身底色默认白色。窗口拉伸/缩放时 WebView
// 尚未重绘的新增区域会短暂露出该底色——暗色主题下表现为白边。
// 通过设置 document.documentElement.style.backgroundColor 为当前主题的 --bg 值，
// 让 WebView2 把这个颜色作为底色，拉伸时就不会闪白/闪黑。
// 此方法不依赖 Wails 的 BackgroundColour，运行中切换主题也能即时跟随。
function syncWebViewBg() {
  const bg = getComputedStyle(document.documentElement).getPropertyValue('--bg').trim()
  document.documentElement.style.backgroundColor = bg
  document.body.style.backgroundColor = bg
}

function applyTheme(t: string) {
  theme.value = (t === 'light') ? 'light' : 'dark'
  document.documentElement.setAttribute('data-theme', theme.value)
  // 主题切换后立刻同步 WebView 底色
  syncWebViewBg()
}

async function loadSettings() {
  try {
    const s: any = await GetSettings()
    applyTheme(s?.theme || 'dark')
    if (s?.language) lang.value = s.language as Lang
    pasteTrigger.value = (s?.pasteTrigger === 'single' ? 'single' : 'double')
    tabHotkeysEnabled.value = s?.tabHotkeysEnabled !== false // 缺失/旧配置默认 true
    emojiEnabled.value = s?.emojiEnabled !== false           // 缺失/旧配置默认 true
    extendedEmoji.value = !!s?.extendedEmoji
    return s
  } catch { return null }
}

// emojiEnabled → emojiMounted 的防抖桥：
//   - 关闭：立即把 view 切回 main（避免停留在已隐藏的 emoji 视图），
//          并立刻卸载组件以尽快回收 emoji-mart-data 的 ~3MB JSON 与
//          组件内部数据结构（ESC 出去等场景，关掉就是为了省内存，
//          没理由再 debounce 延迟）。sprite WebP 是静态资产，由浏览器
//          缓存层管理，组件卸载本身不影响其驻留。
//   - 开启：300ms 防抖才挂载——避免用户在设置面板快速 off→on→off→on 时
//          反复 mount/unmount 触发 emoji-mart-data 的反复动态 import。
watch(emojiEnabled, (v) => {
  if (emojiMountDebounceTimer != null) {
    window.clearTimeout(emojiMountDebounceTimer)
    emojiMountDebounceTimer = null
  }
  if (!v) {
    if (view.value === 'emoji') view.value = 'main'
    emojiMounted.value = false
    return
  }
  emojiMountDebounceTimer = window.setTimeout(() => {
    emojiMountDebounceTimer = null
    if (emojiEnabled.value) emojiMounted.value = true
  }, EMOJI_MOUNT_DEBOUNCE_MS)
})

// 窗口激活时：根据设置回到顶部 / 切换至全部分组
// 注意：用 visibilitychange 而不是 focus 事件——因为在 WebView2 里
// 拖动窗口（drag-region）会触发 blur/focus 但不会改 visibilityState，
// 只有 HideWindow + ShowWindow 才会让 document 从 hidden 变 visible。
// 这样拖窗口时不会误重置筛选/滚动位置。
async function onWindowShow() {
  // 清空搜索栏对所有视图生效（主视图 + emoji 视图共用同一个搜索框），
  // 所以放在 view === 'main' 守卫之前。
  const s: any = await GetSettings()
  let hadKeyword = false
  if (s?.clearSearchOnShow) {
    hadKeyword = keyword.value !== ''
    keyword.value = ''
    emojiKeyword.value = ''
  }
  // "激活时切换至全部分组"也应覆盖 emoji tab：用户期望激活后回到主列表的
  // "全部"分组，而不是停留在 emoji 视图。
  if (s?.resetFilterOnShow && view.value !== 'main') {
    view.value = 'main'
  }
  if (view.value !== 'main') {
    searchRef.value?.focus()
    return
  }
  // 若本次激活是由 Alt+`/Alt+1..6 触发的 → 用户明确指定了分类，
  // "激活时切换至全部分组 / 回到顶部"应该让位给用户的指定，避免反向覆盖。
  if (Date.now() < suppressActivateUntil) {
    searchRef.value?.focus()
    return
  }
  if (!s) return
  let needRefresh = hadKeyword
  if (s.resetFilterOnShow && typeFilter.value !== '') {
    typeFilter.value = ''
    needRefresh = true
  }
  if (needRefresh) await refresh({ silent: true })
  if (s.scrollTopOnShow) {
    selectedIdx.value = 0
    listRef.value?.scrollTo({ top: 0 })
  }
  // 聚焦搜索框
  searchRef.value?.focus()
}

// 由 'tab:switch' 事件设置：Date.now() < 此值 时跳过激活副作用。
// 用时间戳而非布尔，是为了应对一次激活触发多次 onWindowShow 的情况
// （后端 EventsEmit window:show + 浏览器原生 visibilitychange）。
let suppressActivateUntil = 0

function onVisibilityChange() {
  if (document.visibilityState === 'visible') {
    onWindowShow()
  } else {
    // 窗口隐藏时退出设置，下次显示直接回到主列表
    if (view.value === 'settings') view.value = 'main'
  }
}

// 当前筛选 tab 的索引（用于 Tab/左右键切换）
const filterIdx = computed(() => typeOptions.value.findIndex(o => o.key === typeFilter.value))

function switchFilter(dir: number) {
  const opts = typeOptions.value
  const newIdx = (filterIdx.value + dir + opts.length) % opts.length
  typeFilter.value = opts[newIdx].key as any
  refresh({ silent: true })
}

// Alt+1..6 → 对应分类（收藏/文本/图片/文件/链接/代码）
// 索引与 typeOptions 中的顺序一致：0=全部, 1=收藏, 2=文本, 3=图片, 4=文件, 5=链接, 6=代码
// 与 onWindowShow 中"typeFilter 没变就不 refresh"的逻辑保持一致 ——
// 否则用户在已选中的分类上重复按 Alt+N 会触发无意义的 loading 闪烁。
function switchFilterByIndex(idx: number) {
  const opts = typeOptions.value
  if (idx < 0 || idx >= opts.length) return
  const next = opts[idx].key as any
  if (typeFilter.value === next) return // 已在此分类，无需重拉
  typeFilter.value = next
  refresh({ silent: true })
}

function onKeyDown(e: KeyboardEvent) {
  // 确认弹窗显示期间：仅响应 Enter（确认）/ Esc（取消），其它键一律拦截，
  // 避免：按 Enter 既弹框又触发 doPaste；按 Backspace 在弹框开启时再次触发 doDelete
  // 造成弹框叠压；按空格触发预览/专属操作等。
  // 放在函数最顶部、早于任何分支——包括 view !== 'main' 的设置界面返回逻辑。
  if (confirmVisible.value) {
    if (e.key === 'Enter') {
      e.preventDefault(); e.stopPropagation()
      onConfirmOk()
    } else if (e.key === 'Escape') {
      e.preventDefault(); e.stopPropagation()
      onConfirmCancel()
    } else {
      // 吞掉其它所有键，防止冒到默认处理（如 Backspace 再次弹一个新确认框）
      e.preventDefault(); e.stopPropagation()
    }
    return
  }

  // 提示弹窗显示期间：Enter / Esc 关闭，其它键拦截
  if (alertVisible.value) {
    if (e.key === 'Enter' || e.key === 'Escape') {
      e.preventDefault(); e.stopPropagation()
      onAlertOk()
    } else {
      e.preventDefault(); e.stopPropagation()
    }
    return
  }

  // 设置界面 / Emoji 界面：Esc 退出回到主列表
  if (view.value !== 'main') {
    if (e.key === 'Escape') {
      e.preventDefault()
      view.value = 'main'
      return
    }
    // Emoji 视图下 Tab/Shift+Tab 切换 emoji 分类（循环，不跳主 tab）。
    // 阻止浏览器默认焦点跳转行为，避免把焦点带离搜索框、造成"再按就无效"。
    if (view.value === 'emoji' && e.key === 'Tab') {
      e.preventDefault()
      emojiPickerRef.value?.cycleCategory(e.shiftKey ? -1 : 1)
      return
    }
    return
  }

  // 数字键切换分类 tab（应用内）
  // macOS：Cmd+1..6；Windows/Linux：Alt+1..6
  // 与后端 app.go registerHotkey() 的平台判断保持一致。
  // 受 tabHotkeysEnabled 开关控制：关掉后应用内也不再拦截，与全局热键行为一致。
  const isMacPlatform = navigator.platform.toUpperCase().includes('MAC') || navigator.userAgent.includes('Mac')
  const tabKeyPressed = isMacPlatform
    ? (e.metaKey && !e.ctrlKey && !e.altKey)
    : (e.altKey && !e.ctrlKey && !e.metaKey)
  if (tabHotkeysEnabled.value && tabKeyPressed) {
    if (/^[1-6]$/.test(e.key)) {
      const n = parseInt(e.key, 10)
      if (n < typeOptions.value.length) {
        e.preventDefault()
        switchFilterByIndex(n)
        return
      }
    }
    // Alt+Space 兜底（仅 Windows 场景需要，跨平台代码无副作用）：
    // 真正的根因在 Win32 消息层 —— 全局热键消费 Alt 后窗口侧菜单激活态没被清，
    // 后续按 Space 会被 OS 派发为 WM_SYSCOMMAND(SC_KEYMENU) 弹出系统菜单。
    // 主修复在 internal/window/sysmenu_windows.go（子类化拦截 SC_KEYMENU）。
    // 这里保留 JS 兜底用于覆盖窗口子类化尚未装上的极短启动窗口期，
    // 命中时退化为预览动作。
    if (e.key === ' ') {
      e.preventDefault()
      if (selected.value) {
        if (detailVisible.value) detailVisible.value = false
        else showDetail(selected.value)
      }
      return
    }
  }

  // 搜索框获焦时不拦截左右键
  const targetEl = e.target as HTMLElement | null
  const inSearch = targetEl?.tagName === 'INPUT'
  // 搜索框已有输入内容时（如用户正在搜"foo bar"），空格应作为普通字符输入，
  // 不触发预览；仅当搜索框为空时才把空格升级为"预览"快捷键。
  // 背景：窗口唤起时焦点默认在搜索框，若无此判断则空格会被直接吞成预览动作，
  // 用户无法在搜索时输入空格。
  const searchHasValue = inSearch && !!(targetEl as HTMLInputElement | null)?.value

  if (e.key === 'Tab') {
    e.preventDefault()
    switchFilter(e.shiftKey ? -1 : 1)
  } else if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
    // 左右键：切换 filter tab。
    // 历史版本曾用 !inSearch 限制——焦点在搜索框时退化为光标移动；
    // 但搜索框是默认获焦点，导致用户冷启动后按左右键完全无反应（必须先用
    // 鼠标点窗口移走焦点才行）。改为：搜索框为空时一律切 tab；搜索框
    // 有内容时才让左右键作为光标移动（用户正在编辑，更需要光标控制）。
    if (inSearch && searchHasValue) return
    e.preventDefault()
    switchFilter(e.key === 'ArrowLeft' ? -1 : 1)
  } else if (e.key === 'ArrowDown') {
    selectedIdx.value = Math.min(selectedIdx.value + 1, items.value.length - 1)
    e.preventDefault()
    scrollSelectedIntoView()
  } else if (e.key === 'ArrowUp') {
    selectedIdx.value = Math.max(selectedIdx.value - 1, 0)
    e.preventDefault()
    scrollSelectedIntoView()
  } else if (e.key === ' ' && !searchHasValue && selected.value) {
    // 空格键：单击预览 / 双击执行类型专属操作。
    //   - 单击：toggle 详情面板（夹在"选择"与"粘贴"之间的轻量查看动作）
    //   - 双击（200ms 内连按两次）：按 selected.type 分发到专属操作
    //       image → 保存图片 / file → 资源管理器中显示 / link → 浏览器打开
    //       其它类型无事发生
    //   - 焦点在搜索框时整段 if 不命中（让空格作为搜索词输入）
    //   - preventDefault 避免触发页面滚动
    //
    // 实现策略：延迟单击 + 双击抢占（与鼠标 onItemClick 同范式）。
    // 第一次空格不立刻打开预览，先 setTimeout 200ms；若期间又来一次空格，
    // 取消 pending 的预览、改执行专属动作。
    //
    // e.repeat 过滤：长按空格时 OS 自动重复的 keydown 间隔通常 30~50ms，
    // 小于 200ms 阈值，会被误判为"双击"。直接丢掉自动重复事件即可。
    e.preventDefault()
    if (e.repeat) return
    const it = selected.value
    if (!hasPrimaryAction(it)) {
      // 无专属动作的类型（text/code 等）：不进双击窗口，立即 toggle 预览，零延迟。
      detailVisible.value ? (detailVisible.value = false) : showDetail(it)
    } else if (spacePreviewTimer != null) {
      // 第二次空格 → 双击命中
      window.clearTimeout(spacePreviewTimer)
      spacePreviewTimer = null
      runPrimaryAction(it)
    } else if (detailVisible.value) {
      // 详情已开 → 任意单击空格直接关闭（不进双击窗口，避免"关了又被双击重开"歧义）
      detailVisible.value = false
    } else {
      // 第一次空格 → 延迟开预览，等待可能的第二次
      spacePreviewTimer = window.setTimeout(() => {
        spacePreviewTimer = null
        showDetail(it)
      }, SPACE_DBLCLICK_THRESHOLD_MS)
    }
  } else if (e.key === 'Enter' && selected.value) {
    doPaste(selected.value)
  } else if (e.key === 'Escape') {
    if (detailVisible.value) detailVisible.value = false
    else HideWindow()
  } else if ((e.key === 'Delete' || e.key === 'Backspace') && !searchHasValue && selected.value) {
    // 删除选中项：
    //   - Delete：Win/Linux 键盘 + Mac 外接键盘
    //   - Backspace：Mac 笔记本主键盘区的 ⌫（Mac 上没有独立 Delete 键）
    // gate 条件用 !searchHasValue（而非 !inSearch）：
    //   - 搜索框有内容时：让 Delete/Backspace 作为原生文本编辑键，避免每按一次
    //     退格就删一条历史记录
    //   - 搜索框为空时（含默认焦点在搜索框但用户还没输入）：空框里按这两个键
    //     本来就是 no-op，此时重定向到"删除历史项"更符合预期
    //   - 焦点不在搜索框（inSearch=false → searchHasValue 必然也为 false）：照常触发
    // preventDefault 避免 WebView2/Chromium 把空框里的 Backspace 当"导航后退"吞掉。
    e.preventDefault()
    doDelete(selected.value)
  }
}

let unsubscribe: (() => void) | null = null

onMounted(async () => {
  // 平台标识：Mac 下给红黄绿按钮预留空间
  const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent) || /Mac|iPhone|iPad/.test((navigator as any).platform || '')
  document.documentElement.setAttribute('data-os', isMac ? 'mac' : 'other')
  window.addEventListener('keydown', onKeyDown)
  window.addEventListener('blur', onWindowBlur)
  window.addEventListener('resize', syncWebViewBg)
  document.addEventListener('visibilitychange', onVisibilityChange)
  // 监听后端 emit 的 window:show 事件——Windows 上 visibilitychange 不一定触发，
  // 需要后端显式通知才能执行"激活时回到顶部 / 切换至全部分组"等功能。
  EventsOn('window:show', () => { onWindowShow() })
  // 全局 Alt+`/Alt+1..6 唤起并切到第 N 个 tab：后端 hotkey.Manager 注册的系统级快捷键
  // 即使应用未聚焦也能命中，命中后唤起面板并 emit 此事件。
  EventsOn('tab:switch', (idx: number) => {
    if (typeof idx !== 'number') return
    // 用户用 Alt+N 明确指定了分类 → 让接下来这段时间内的窗口激活
    // 跳过 "切换至全部分组 / 回到顶部" 副作用，避免反向覆盖。
    // 600ms 经验值：覆盖 window:show 事件 + visibilitychange 双触发的时间差。
    suppressActivateUntil = Date.now() + 600
    // 设置页时也允许切换：先回到主列表，再切 tab，符合"快捷键即唤起"的预期
    if (view.value !== 'main') view.value = 'main'
    switchFilterByIndex(idx)
  })
  await loadSettings()
  // loadSettings → applyTheme 已调 syncWebViewBg，这里再兜底一次确保初始化正确
  syncWebViewBg()
  await refresh()
  EventsOn('clipboard:new', async () => { await refresh() })
  EventsOn('view:settings', () => { view.value = 'settings' })
  unsubscribe = () => { EventsOff('clipboard:new'); EventsOff('window:show'); EventsOff('tab:switch'); EventsOff('view:settings') }

  // 冷启动焦点：由 Go 端 domReady 在窗口可见时主动 emit 'window:show'，
  // 走和热启动一致的 onWindowShow 路径（聚焦搜索框、激活副作用）。
  // 之前在这里加 setTimeout focus 是无效的——冷启动时 webview 进程根本没
  // 拿到键盘焦点（background process 限制），DOM focus 只改 activeElement，
  // OS 不会把键盘消息派给我们。必须由 Go 端 forceForeground 抢前台后，
  // 浏览器层的 focus() 才有意义。详见 internal/window/showhide_windows.go。
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
  window.removeEventListener('blur', onWindowBlur)
  window.removeEventListener('resize', syncWebViewBg)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  if (spacePreviewTimer != null) { window.clearTimeout(spacePreviewTimer); spacePreviewTimer = null }
  if (unsubscribe) unsubscribe()
})

watch(view, async (v) => {
  if (v === 'main') {
    await loadSettings()
    await refresh()
  }
})

function formatTime(ts: string | Date): string {
  const d = new Date(ts)
  const diff = (Date.now() - d.getTime()) / 1000
  if (diff < 60) return t('timeSecAgo', { n: Math.floor(diff) })
  if (diff < 3600) return t('timeMinAgo', { n: Math.floor(diff / 60) })
  if (diff < 86400) return t('timeHourAgo', { n: Math.floor(diff / 3600) })
  return d.toLocaleDateString()
}

const typeLabel = computed<Record<string, string>>(() => ({
  text: t('typePlainText'),
  image: t('typeImage'),
  link: t('typeLink'),
  code: t('typeCode'),
  file: t('typeFile'),
}))

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

// 后端语言名（小写，由 internal/lang.Detect 返回，详见 chroma 与启发式规则）
// → 前端展示名映射。未命中走首字母大写兜底；空串显示通用 Code。
const LANG_LABEL: Record<string, string> = {
  javascript: 'JavaScript', typescript: 'TypeScript', python: 'Python', go: 'Go',
  java: 'Java', kotlin: 'Kotlin', swift: 'Swift', rust: 'Rust',
  c: 'C', 'c++': 'C++', cpp: 'C++', csharp: 'C#', objectivec: 'Objective-C',
  ruby: 'Ruby', php: 'PHP', shell: 'Shell', bash: 'Bash',
  sql: 'SQL', json: 'JSON', yaml: 'YAML', xml: 'XML', html: 'HTML', css: 'CSS', scss: 'SCSS',
  markdown: 'Markdown', dockerfile: 'Dockerfile', makefile: 'Makefile',
  ini: 'INI', toml: 'TOML', diff: 'Diff', plaintext: 'Text',
}
// 把后端 it.language 转成显示标签。后端会主动放弃低置信度内容（含中文笔记），
// 因此空值 → "Code"，避免 hljs.highlightAuto 在前端再来一次（性能/一致性双赢）。
function detectLanguage(it: Item): string {
  const name = (it.language || '').toLowerCase()
  if (!name) return 'Code'
  return LANG_LABEL[name] || (name.charAt(0).toUpperCase() + name.slice(1))
}

function metaParts(it: Item): string[] {
  // code 类型用语言名替代类型标签；其它类型显示类型标签
  const parts: string[] = it.type === 'code'
    ? [detectLanguage(it)]
    : [typeLabel.value[it.type] || it.type]
  // 文件大小（XX B / KB）仅对 image / file 类型有意义——
  // 文本/链接/代码看字符数即可，字节数对用户没参考价值且属于视觉噪声。
  if ((it.type === 'image' || it.type === 'file') && it.size) {
    parts.push(formatSize(it.size))
  }
  if (it.type === 'file' && it.charCount) {
    parts.push(t('nFiles', { n: it.charCount }))
  } else if (it.charCount && it.type !== 'image') {
    parts.push(`${it.charCount} ${t('chars')}`)
  }
  parts.push(formatTime(it.updatedAt))
  return parts
}

// 窗口失焦自动隐藏
function onWindowBlur() {
  if (alwaysOnTop.value) return
  if (suppressBlurHide.value > 0) return  // 原生对话框打开期间不自动隐藏
  setTimeout(() => {
    if (!document.hasFocus()) {
      if (suppressBlurHide.value > 0) return  // 延迟期间也可能新开了 dialog
      // 隐藏前退出设置，下次显示直接回到主列表
      if (view.value === 'settings') view.value = 'main'
      HideWindow()
    }
  }, 150)
}

// Emoji 选中：写入系统剪贴板并显示 toast 反馈
const emojiToast = ref('')
let emojiToastTimer: number | null = null

async function onEmojiSelect(emoji: string) {
  try {
    // 使用 Clipboard API 写入系统剪贴板
    await navigator.clipboard.writeText(emoji)
    showEmojiToast(`${emoji} ${t('emojiCopied')}`)
  } catch (e) {
    console.error('[emoji] copy failed', e)
  }
}

// 双击 emoji：直接粘贴到上一个前台窗口（复用 PasteItem 同款流水线）。
// 注意：toast 在面板隐藏前就显示意义不大（看不到），但为保留"操作有反馈"
// 的一致性还是写一句——若后续做"粘贴成功后短暂回显"再考虑。
async function onEmojiPaste(emoji: string) {
  try {
    await PasteText(emoji)
    showEmojiToast(`${emoji} ${t('emojiPasted')}`)
  } catch (e) {
    console.error('[emoji] paste failed', e)
  }
}

function showEmojiToast(msg: string) {
  emojiToast.value = msg
  if (emojiToastTimer != null) window.clearTimeout(emojiToastTimer)
  emojiToastTimer = window.setTimeout(() => {
    emojiToast.value = ''
    emojiToastTimer = null
  }, 1500)
}
</script>

<template>
  <div class="app">
    <template v-if="view === 'main' || view === 'emoji'">
      <header class="topbar drag-region">
        <div class="search">
          <Search :size="16" class="search-icon" />
          <input
            ref="searchRef"
            v-model="currentKeyword"
            :placeholder="view === 'emoji' ? t('searchEmoji') : t('search')"
            @keydown="(e) => { if (e.key === ' ' && !currentKeyword.trim()) e.preventDefault() }"
            @input="onSearchInput"
            @compositionstart="onSearchCompositionStart"
            @compositionend="onSearchCompositionEnd"
            autofocus
          />
          <button
            v-if="currentKeyword"
            class="search-clear"
            type="button"
            :title="t('clearSearch')"
            :aria-label="t('clearSearch')"
            @click="clearKeyword"
            @mousedown.prevent
          >
            <X :size="14" />
          </button>
        </div>
        <button class="icon-btn" :class="{ active: alwaysOnTop }" @click="toggleAlwaysOnTop">
          <Pin :size="16" :fill="alwaysOnTop ? 'currentColor' : 'none'" />
        </button>
        <button class="icon-btn" @click="view = 'settings'"><SettingsIcon :size="16" /></button>
      </header>

      <nav class="filters drag-region">
        <!--
          @mousedown.prevent: 阻止鼠标点击让 <button> 获得焦点。
          原生 <button> 获焦后，按空格在 HTML 层会触发 click（吞掉预览快捷键）；
          更严重的是 Windows WebView2 下，带 Alt 的组合键被 preventDefault 后
          Alt 的 keyup 偶发不上报，此时再按空格会被 OS 当成 Alt+Space → 弹出
          "还原/移动/大小/最小化/最大化/关闭"系统菜单。让按钮永不获焦即可绕开。
        -->
        <button v-for="(opt, idx) in typeOptions" :key="opt.key"
          :class="{ active: typeFilter === opt.key && view === 'main' }"
          :title="idx === 0 ? opt.label : `${opt.label} (Alt+${idx})`"
          tabindex="-1"
          @mousedown.prevent
          @click="view = 'main'; typeFilter = opt.key as any; refresh({ silent: true }); ($event.currentTarget as HTMLElement).blur()">
          <component :is="opt.icon" :size="13" />
          <span>{{ opt.label }}</span>
        </button>
        <!-- Emoji 是"工具型" tab，不属于剪贴板内容筛选（全部/收藏/文本/...）。
             用 `tab-emoji` + margin-left:auto 把它推到最右端，并在左侧加一条
             细分隔线，在视觉上把它与内容筛选 tab 区隔开。
             受 emojiEnabled 总开关控制：关闭时整个入口消失。 -->
        <button
          v-if="emojiEnabled"
          class="tab-emoji"
          :class="{ active: view === 'emoji' }"
          :title="t('emoji')"
          tabindex="-1"
          @mousedown.prevent
          @click="view = 'emoji'">
          <Smile :size="13" />
          <span>{{ t('emoji') }}</span>
        </button>
      </nav>

      <div class="content-area">
        <main v-show="view === 'main'" class="list" ref="listRef" @scroll="onListScroll">
        <div v-if="loading" class="empty">{{ t('loading') }}</div>
        <div v-else-if="items.length === 0" class="empty">
          <ClipboardList :size="48" class="empty-icon" />
          <div>{{ t('empty') }}</div>
          <div class="hint">{{ t('emptyHint') }}</div>
        </div>

        <div v-for="(it, idx) in items" :key="it.id" class="item"
          :data-idx="idx"
          :class="{ active: idx === selectedIdx, pinned: it.pinned }"
          @click="onItemClick(idx, it)" @dblclick="onItemDblClick(it)"
          @contextmenu="onItemContextMenu($event, it)">

          <!-- 第一行：元信息 + 操作按钮 -->
          <div class="item-row1">
            <span class="item-meta">
              <span v-for="(p, i) in metaParts(it)" :key="i">
                <span class="meta-sep" v-if="i > 0">·</span>{{ p }}
              </span>
            </span>
            <div class="item-actions" @click.stop>
              <button v-if="it.type === 'link'" :title="t('openInBrowser')" @click="doOpenUrl(it)">
                <ExternalLink :size="14" />
              </button>
              <button v-if="it.type === 'file'" :title="t('revealInExplorer')" @click="doRevealFile(it)">
                <FolderOpen :size="14" />
              </button>
              <button v-if="it.type === 'image'" :title="t('saveImage')" @click="doSaveImage(it)">
                <Download :size="14" />
              </button>
              <button :title="t('copy')" @click="doCopy(it)"><Copy :size="14" /></button>
              <button :title="t('favorite')" :class="{ active: it.favorite }" @click="doToggleFav(it)">
                <Star :size="14" :fill="it.favorite ? 'currentColor' : 'none'" />
              </button>
              <button :title="t('delete')" @click="doDelete(it)"><Trash2 :size="14" /></button>
            </div>
          </div>

          <!-- 第二行：内容预览 -->
          <div class="item-row2">
            <!-- 图片 -->
            <template v-if="it.type === 'image'">
              <div class="thumb">
                <img v-if="imageThumbs[it.id]" :src="imageThumbs[it.id]" />
                <div v-else class="thumb-ph"><ImageIcon :size="20" /></div>
              </div>
            </template>
            <!-- 文件 -->
            <template v-else-if="it.type === 'file'">
              <div v-if="fileThumbs[it.id]" class="file-with-thumb">
                <div class="thumb">
                  <img :src="fileThumbs[it.id]" />
                </div>
                <div class="file-row">
                  <ImageIcon :size="14" class="file-icon" />
                  <span class="file-name">{{ (it.preview || '').split('\n')[0] }}</span>
                </div>
              </div>
              <div v-else class="file-list">
                <div v-for="(fname, fi) in (it.preview || '').split('\n').slice(0, 5)" :key="fi" class="file-row">
                  <component :is="isImageFile(fname) ? ImageIcon : FileIcon" :size="14" class="file-icon" />
                  <span class="file-name">{{ fname }}</span>
                </div>
                <div v-if="(it.preview || '').split('\n').length > 5" class="file-more">
                  {{ t('moreFiles', { n: (it.preview || '').split('\n').length - 5 }) }}
                </div>
              </div>
            </template>
            <!-- 文本/代码/链接 -->
            <div v-else class="item-preview">
              <span v-if="it.note" class="note-tag"><Edit3 :size="13" /> {{ it.note }}</span>
              <template v-else>{{ it.preview || t('empty2') }}</template>
            </div>
          </div>
        </div>
        <div v-if="loadingMore" class="loading-more">{{ t('loading') }}</div>
      </main>

      <!-- Emoji Picker 视图: 始终保持布局(有宽高)，visibility 控制可见性，切换零延迟。
           外层 v-if 受 emojiMounted 控制（emojiEnabled 的防抖派生）：关闭总开关
           会卸载组件以释放 emoji-mart-data 的常驻数据；再开启时挂回，
           sprite WebP 已在浏览器缓存中，首帧近似零延迟。 -->
      <main v-if="emojiMounted" class="emoji-main" :class="{ 'emoji-hidden': view !== 'emoji' }">
        <EmojiPicker ref="emojiPickerRef" :theme="theme" :search="emojiKeyword" :active="view === 'emoji'" :extended-emoji="extendedEmoji" @select="onEmojiSelect" @dblselect="onEmojiPaste" />
      </main>
      </div>

      <!-- 回到顶部 -->
      <Transition name="fade">
        <button v-if="showBackTop && view === 'main'" class="back-top" :title="t('backTop')" @click="scrollToTop">
          <ArrowUp :size="18" />
        </button>
      </Transition>

      <footer v-if="view === 'main'" class="statusbar">
        <span>{{ total }} {{ t('records') }}</span>
        <span class="dim">{{ t('statusHint') }}</span>
      </footer>
      <footer v-else-if="view === 'emoji'" class="statusbar">
        <span></span>
        <span class="dim">{{ t('emojiHint') }}</span>
      </footer>

      <!-- Emoji 复制成功提示 -->
      <Transition name="fade">
        <div v-if="emojiToast" class="emoji-toast">{{ emojiToast }}</div>
      </Transition>

      <!-- 图片预览 -->
      <div v-if="view === 'main' && detailVisible && detailIsImage" class="detail-mask" @click.self="detailVisible = false">
        <img class="detail-img-only" :src="detailContent" @click="detailVisible = false" />
      </div>
      <!-- 文本详情 -->
      <div v-else-if="view === 'main' && detailVisible" class="detail-mask" @click.self="detailVisible = false">
        <div class="detail">
          <div class="detail-head">
            <!-- 标题栏直接复用列表卡片下方的元信息（metaParts），保持视觉一致：
                 类型/语言 · 大小 · 字符数/文件数 · 时间。无 detailItem 时回退到 t('detail')。
                 用 · 分隔与 .meta 列表渲染保持同款。 -->
            <span class="detail-meta">{{ detailItem ? metaParts(detailItem).join(' · ') : t('detail') }}</span>
            <!-- 详情头右侧的"轻量动作组"：当前只有复制 + 关闭。
                 复制直接复用列表卡片同款 doCopy(item)（按 id 让后端读原始内容写回剪贴板，
                 不依赖前端已截断/转义后的 detailContent，链接/代码等都安全）。
                 仅在有 detailItem 时显示——空态/异常态隐藏避免点击空操作。 -->
            <div class="detail-actions">
              <button v-if="detailItem" :title="t('copy')" @click="doCopy(detailItem)"><Copy :size="14" /></button>
              <button class="detail-close" @click="detailVisible = false">✕</button>
            </div>
          </div>
          <div class="detail-body">
            <pre v-if="detailType === 'code'" class="code-hl hljs" v-html="highlightCode(detailContent, detailLanguage)"></pre>
            <!-- 链接：渲染为可点击 anchor，点击交给系统浏览器打开。
                 必须 prevent，否则 Wails webview 会试图把 href 当成当前页导航 → 白屏。
                 href 仍写真实 url，便于用户右键复制 / 看 hover 时浏览器状态栏式提示。 -->
            <a v-else-if="detailType === 'link'" class="detail-link" :href="detailContent"
               @click.prevent="OpenURL(detailContent)">{{ detailContent }}</a>
            <pre v-else>{{ detailContent }}</pre>
          </div>
        </div>
      </div>
    </template>

    <Settings v-else-if="view === 'settings'" @close="view = 'main'" />

    <!-- 右键菜单 -->
    <Transition name="fade">
      <div v-if="ctxMenu.visible" class="ctx-mask" @click="closeCtxMenu" @contextmenu.prevent="closeCtxMenu">
        <div class="ctx-menu" :style="ctxMenuStyle" @click.stop>
          <button @click="ctxPaste"><Copy :size="14" /> {{ t('paste') }}</button>
          <button @click="ctxCopy"><Copy :size="14" /> {{ t('copy') }}</button>
          <button @click="ctxDetail"><Eye :size="14" /> {{ t('viewDetail') }}</button>
          <button @click="ctxNote"><Edit3 :size="14" /> {{ t('note') }}</button>
          <div class="ctx-sep"></div>
          <button @click="ctxFav">
            <Star :size="14" /> {{ ctxMenu.item?.favorite ? t('unfavorite') : t('favorite') }}
          </button>
          <button v-if="ctxMenu.item?.type === 'image'" @click="ctxSaveImage">
            <Download :size="14" /> {{ t('saveImage') }}
          </button>
          <button v-if="ctxMenu.item?.type === 'file'" @click="ctxReveal">
            <FolderOpen :size="14" /> {{ t('revealInExplorer') }}
          </button>
          <button v-if="ctxMenu.item?.type === 'link'" @click="ctxOpenInBrowser">
            <ExternalLink :size="14" /> {{ t('openInBrowser') }}
          </button>
          <div class="ctx-sep"></div>
          <button class="ctx-danger" @click="ctxDelete"><Trash2 :size="14" /> {{ t('delete') }}</button>
        </div>
      </div>
    </Transition>

    <!-- 确认弹窗 -->
    <Transition name="fade">
      <div v-if="confirmVisible" class="confirm-mask" @click.self="onConfirmCancel">
        <div class="confirm-box">
          <div class="confirm-icon"><AlertTriangle :size="24" /></div>
          <div class="confirm-body">
            <div class="confirm-title">{{ t('confirmOp') }}</div>
            <div class="confirm-msg">{{ confirmMsg }}</div>
          </div>
          <div class="confirm-actions">
            <button class="cbtn cbtn-cancel" @click="onConfirmCancel">{{ t('cancel') }}</button>
            <button class="cbtn cbtn-ok" @click="onConfirmOk">{{ t('ok') }}</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 提示弹窗（仅确定按钮） -->
    <Transition name="fade">
      <div v-if="alertVisible" class="confirm-mask" @click.self="onAlertOk">
        <div class="confirm-box">
          <div class="confirm-icon"><AlertTriangle :size="24" /></div>
          <div class="confirm-body">
            <div class="confirm-title">{{ t('tip') }}</div>
            <div class="confirm-msg">{{ alertMsg }}</div>
          </div>
          <div class="confirm-actions">
            <button class="cbtn cbtn-ok" @click="onAlertOk">{{ t('ok') }}</button>
          </div>
        </div>
      </div>
    </Transition>
    <!-- 备注弹窗 -->
    <Transition name="fade">
      <div v-if="noteVisible" class="confirm-mask" @click.self="onNoteCancel">
        <div class="note-box">
          <div class="note-head">
            <span>{{ t('note') }}</span>
            <button class="note-close" @click="onNoteCancel"><span style="font-size:16px;color:#a8adbd">✕</span></button>
          </div>
          <input ref="noteInputRef" class="note-input" v-model="noteText" :placeholder="t('noteHint')"
            @keydown.enter="onNoteOk" @keydown.escape="onNoteCancel" />
          <div class="confirm-actions">
            <button class="cbtn cbtn-cancel" @click="onNoteCancel">{{ t('cancel') }}</button>
            <button class="cbtn cbtn-primary" @click="onNoteOk">{{ t('ok') }}</button>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style>
* { box-sizing: border-box; }
:root, [data-theme="dark"] {
  --bg: #14161c; --bg-elevated: #242833; --bg-hover: #1e222c; --bg-active: #2b3140;
  --bg-sidebar: #0f1117; --bg-statusbar: #171a22;
  --border: #2a2f3a; --border-light: #1c2029;
  --text: #e6e8ef; --text-secondary: #a8adbd; --text-muted: #6b7280;
  --accent: #3b82f6; --accent-hover: #2563eb;
  --danger: #ef4444; --warning: #fbbf24; --success: #4ade80;
  --scrollbar: #2a2f3a;
}
[data-theme="light"] {
  --bg: #f5f5f5; --bg-elevated: #ffffff; --bg-hover: #eeeeee; --bg-active: #e0e7f1;
  --bg-sidebar: #f0f1f3; --bg-statusbar: #f0f1f3;
  --border: #dcdee3; --border-light: #eeeeee;
  --text: #1a1a1a; --text-secondary: #555555; --text-muted: #888888;
  --accent: #3b82f6; --accent-hover: #2563eb;
  --danger: #ef4444; --warning: #f59e0b; --success: #22c55e;
  --scrollbar: #cccccc;
}
html, body, #app {
  margin: 0; padding: 0;
  height: 100vh; width: 100vw;
  font-family: -apple-system, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
  background: var(--bg); color: var(--text); overflow: hidden;
}
.app {
  display: flex; flex-direction: column; height: 100vh;
  background: var(--bg); overflow: hidden;
}

/* 可拖拽区域：本容器可拖；内部所有输入/按钮自动排除 */
.drag-region {
  --wails-draggable: drag;
  -webkit-app-region: drag;
}
.drag-region input,
.drag-region button,
.drag-region select,
.drag-region textarea,
.drag-region a {
  --wails-draggable: no-drag;
  -webkit-app-region: no-drag;
}

.topbar { display: flex; gap: 8px; padding: 10px 12px; border-bottom: 1px solid var(--border); align-items: center; }
/* 过去 Mac 下给红黄绿按钮预留了 80px，现在按钮已在 panel_darwin.m 里隐藏，
   这里不再需要 padding-left 偏移。 */
.search { flex: 1; display: flex; align-items: center; gap: 6px; background: var(--bg-elevated); border-radius: 8px; padding: 6px 10px; }
.search input { flex: 1; background: transparent; border: none; outline: none; color: var(--text); font-size: 14px; }
.search-icon { color: var(--text-muted); flex-shrink: 0; }
.search-clear {
  display: inline-flex; align-items: center; justify-content: center;
  width: 18px; height: 18px; padding: 0;
  background: var(--text-muted); color: var(--bg-elevated);
  border: none; border-radius: 50%;
  cursor: pointer; flex-shrink: 0;
  opacity: .75; transition: opacity .15s, background-color .15s;
}
.search-clear:hover { opacity: 1; background: var(--text-secondary); }
.search-clear:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.icon-btn {
  background: transparent; border: 1px solid var(--border); color: var(--text-secondary);
  width: 34px; height: 34px; border-radius: 8px; cursor: pointer;
  transition: all .15s;
  display: flex; align-items: center; justify-content: center;
}
.icon-btn:hover { background: var(--bg-elevated); color: var(--text-secondary); }
.icon-btn.active { background: var(--accent); border-color: #3b82f6; color: #fff; }

.filters { display: flex; gap: 4px; padding: 6px 10px; border-bottom: 1px solid var(--border); overflow-x: auto; flex-wrap: nowrap; }
.filters button {
  display: inline-flex; align-items: center; gap: 3px;
  background: var(--bg-elevated); border: 1px solid transparent; color: var(--text-secondary);
  padding: 3px 7px; border-radius: 12px; font-size: 11px; cursor: pointer; white-space: nowrap;
}
.filters button.active { background: var(--accent); color: #fff; }

/* Emoji tab: pushed to the far right with margin-left:auto, separated from
   the clipboard-content filters by a subtle vertical divider on its left.
   Conceptually this button is a "tool" (opens a different view) rather than
   a content filter, so the layout hints at that distinction. */
.filters button.tab-emoji {
  margin-left: auto;
  position: relative;
}
.filters button.tab-emoji::before {
  content: '';
  position: absolute;
  left: -6px;
  top: 4px;
  bottom: 4px;
  width: 1px;
  background: var(--border);
}

/* Pulse-dot indicator that used to live on the emoji tab button while a
   runtime sprite build was in progress is gone — sprites are now pre-bundled
   WebP imports (always ready), so there's nothing to indicate. The CSS
   block (and its companion @keyframes / reduced-motion override) was
   removed along with the `.emoji-pulse` span and the `emojiSpriteBuilding`
   computed in the script. If you ever reintroduce a runtime build phase,
   bring all three back together. */

.list { height: 100%; overflow-y: auto; padding: 4px 0; }
.empty { text-align: center; padding: 60px 20px; color: var(--text-muted); display: flex; flex-direction: column; align-items: center; gap: 10px; }
.empty-icon { color: #2a2f3a; }
.empty .hint { font-size: 12px; opacity: .6; }
.loading-more { text-align: center; padding: 12px; font-size: 12px; color: var(--text-muted); }

.item {
  display: flex; flex-direction: column; gap: 6px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--border-light);
  cursor: pointer; transition: all .12s;
}
.item:hover { background: var(--bg-hover); }
.item.active { background: var(--bg-active); border-left: 3px solid #3b82f6; }
.item.pinned { border-left: 3px solid #3b82f6; }

.item-row1 {
  display: flex; justify-content: space-between; align-items: center;
}
.item-meta {
  font-size: 11px; color: var(--text-muted); letter-spacing: .2px;
}
.meta-sep {
  margin: 0 5px; opacity: .4;
}
.item-actions { display: flex; gap: 2px; flex-shrink: 0; visibility: hidden; }
.item:hover .item-actions, .item.active .item-actions { visibility: visible; }
.item-actions button {
  background: transparent; border: none; color: var(--text-secondary);
  width: 26px; height: 26px; border-radius: 6px; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
}
.item-actions button:hover { background: var(--border); color: #fff; }
.item-actions button.active { color: var(--warning); }

.item-row2 { min-height: 20px; text-align: left; }

.item-preview {
  font-size: 13px; line-height: 1.5; color: var(--text);
  text-align: left;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-all;
  cursor: pointer;
}
.item-preview:hover { }
.note-tag {
  display: inline-flex; align-items: center; gap: 4px;
  font-weight: 600; color: var(--text);
}
.note-tag svg { color: var(--accent); flex-shrink: 0; }

.thumb {
  width: 140px; height: 90px; border-radius: 6px;
  background: var(--bg-elevated); overflow: hidden;
  display: flex; align-items: center; justify-content: center;
  cursor: pointer;
}
.thumb img { width: 100%; height: 100%; object-fit: cover; }
.thumb-ph { color: var(--text-muted); }

.statusbar {
  display: flex; justify-content: space-between;
  padding: 6px 12px; font-size: 11px; color: var(--text-muted);
  border-top: 1px solid var(--border); background: var(--bg-statusbar);
}
.statusbar .dim { opacity: .7; }

/* 回到顶部浮动按钮 */
.back-top {
  position: fixed;
  right: 20px;
  bottom: 40px;
  width: 36px; height: 36px;
  border-radius: 50%;
  background: var(--border);
  border: 1px solid var(--border);
  color: var(--text);
  cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  box-shadow: 0 2px 8px rgba(0,0,0,.3);
  transition: all .2s;
  z-index: 50;
}
.back-top:hover { background: var(--accent); border-color: #3b82f6; }
.fade-enter-active, .fade-leave-active { transition: opacity .2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

.detail-mask { position: fixed; inset: 0; background: rgba(0,0,0,.8); display: flex; align-items: center; justify-content: center; z-index: 100; }
.detail { background: var(--bg-elevated); border-radius: 10px; width: calc(100% - 16px); max-height: 80vh; display: flex; flex-direction: column; box-shadow: 0 10px 30px rgba(0,0,0,.3); margin: 0 8px; }
.detail-head { display: flex; justify-content: space-between; align-items: center; padding: 10px 14px; border-bottom: 1px solid var(--border); font-size: 13px; }
/* 关闭按钮：尺寸/圆角/hover 底色与列表卡片右侧的 .item-actions button 保持一致，
   视觉上对齐"功能按钮"语言。字号 16 是因为里面是字符 ✕（lucide 图标都是 14，
   但这里是文本字形，需要稍大才平衡）。 */
.detail-head button {
  background: transparent; border: none; color: var(--text-secondary);
  width: 26px; height: 26px; border-radius: 6px; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  font-size: 16px; line-height: 1; padding: 0;
}
.detail-head button:hover { background: var(--border); color: #fff; }
/* 详情头右侧动作组：与列表卡片的 .item-actions 视觉语言一致（gap:2px 紧凑排布）。 */
.detail-actions { display: flex; gap: 2px; flex-shrink: 0; }
/* 详情标题：复用 metaParts，颜色用次级文本色弱化、字号略小，避免与正文抢视觉权重。
   超长 URL/文件名截断防止把 ✕ 按钮挤出去（加 min-width:0 让 flex 子项可收缩）。 */
.detail-meta {
  font-size: 12px;
  color: var(--text-secondary);
  letter-spacing: .2px;
  flex: 1; min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-right: 12px;
  /* 显式靠左：父级（.detail / 模态外层）继承了 text-align:center，
     不指定的话 12px 短文本在 flex:1 容器里会视觉居中。 */
  text-align: left;
}
.detail-body { padding: 14px; overflow: auto; text-align: left; }
.detail-body pre { margin: 0; white-space: pre-wrap; word-break: break-all; font-size: 13px; color: var(--text); text-align: left; }
.detail-body img { max-width: 100%; border-radius: 6px; }
/* 链接详情：经典浏览器链接观感（蓝/紫 + 下划线 + 手形光标），
   保留与 pre 一致的等宽字体和长串换行，避免超长 url 撑破面板。 */
.detail-link {
  display: block;
  font-family: 'SF Mono', 'Consolas', 'Menlo', monospace;
  font-size: 13px;
  line-height: 1.5;
  color: #2563eb;
  text-decoration: underline;
  text-underline-offset: 2px;
  word-break: break-all;
  white-space: pre-wrap;
  cursor: pointer;
}
.detail-link:hover { color: #1d4ed8; }
.detail-link:active { color: #1e40af; }

/* 代码高亮：仅 code 类型条目使用，token 颜色由 highlight.js 的 GitHub 主题提供。
   这里只覆写字体/行距，并把主题默认的灰底改为透明，融入 .detail 面板背景。 */
.detail-body pre.code-hl {
  font-family: 'SF Mono', 'Consolas', 'Menlo', monospace;
  font-size: 12.5px;
  line-height: 1.55;
  background: transparent;
  padding: 0;
}

/* 浅色主题下反向覆盖为 GitHub 官方浅色调色板（默认 import 的是 github-dark）。 */
[data-theme="light"] .hljs { color: #24292e; background: transparent; }
[data-theme="light"] .hljs-doctag,
[data-theme="light"] .hljs-keyword,
[data-theme="light"] .hljs-meta .hljs-keyword,
[data-theme="light"] .hljs-template-tag,
[data-theme="light"] .hljs-template-variable,
[data-theme="light"] .hljs-type,
[data-theme="light"] .hljs-variable.language_ { color: #d73a49; }
[data-theme="light"] .hljs-title,
[data-theme="light"] .hljs-title.class_,
[data-theme="light"] .hljs-title.class_.inherited__,
[data-theme="light"] .hljs-title.function_ { color: #6f42c1; }
[data-theme="light"] .hljs-attr,
[data-theme="light"] .hljs-attribute,
[data-theme="light"] .hljs-literal,
[data-theme="light"] .hljs-meta,
[data-theme="light"] .hljs-number,
[data-theme="light"] .hljs-operator,
[data-theme="light"] .hljs-variable,
[data-theme="light"] .hljs-selector-attr,
[data-theme="light"] .hljs-selector-class,
[data-theme="light"] .hljs-selector-id { color: #005cc5; }
[data-theme="light"] .hljs-regexp,
[data-theme="light"] .hljs-string,
[data-theme="light"] .hljs-meta .hljs-string { color: #032f62; }
[data-theme="light"] .hljs-built_in,
[data-theme="light"] .hljs-symbol { color: #e36209; }
[data-theme="light"] .hljs-comment,
[data-theme="light"] .hljs-code,
[data-theme="light"] .hljs-formula { color: #6a737d; }
[data-theme="light"] .hljs-name,
[data-theme="light"] .hljs-quote,
[data-theme="light"] .hljs-selector-tag,
[data-theme="light"] .hljs-selector-pseudo { color: #22863a; }
[data-theme="light"] .hljs-subst { color: #24292e; }
[data-theme="light"] .hljs-section { color: #005cc5; font-weight: bold; }
[data-theme="light"] .hljs-bullet { color: #735c0f; }
[data-theme="light"] .hljs-emphasis { color: #24292e; font-style: italic; }
[data-theme="light"] .hljs-strong { color: #24292e; font-weight: bold; }
[data-theme="light"] .hljs-addition { color: #22863a; background-color: #f0fff4; }
[data-theme="light"] .hljs-deletion { color: #b31d28; background-color: #ffeef0; }

/* 图片纯预览（无边框） */
.detail-img-only {
  max-width: calc(100% - 32px); max-height: 80vh;
  border-radius: 8px; object-fit: contain;
  cursor: pointer;
  box-shadow: 0 8px 30px rgba(0,0,0,.4);
}

/* 文件列表样式 */
.file-list { display: flex; flex-direction: column; gap: 4px; }
.file-with-thumb { display: flex; flex-direction: column; gap: 6px; }
.file-row { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--text); }
.file-icon { color: var(--accent); flex-shrink: 0; }
.file-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.file-more { font-size: 11px; color: var(--text-muted); padding-left: 20px; }

/* 右键菜单 */
.ctx-mask { position: fixed; inset: 0; z-index: 180; }
.ctx-menu {
  position: fixed; z-index: 181;
  background: var(--bg-elevated); border: 1px solid var(--border);
  border-radius: 8px; padding: 4px 0;
  min-width: 180px;
  box-shadow: 0 6px 20px rgba(0,0,0,.3);
}
.ctx-menu button {
  display: flex; align-items: center; gap: 8px;
  width: 100%; background: transparent; border: none;
  color: var(--text); padding: 7px 14px;
  font-size: 13px; cursor: pointer; text-align: left;
}
.ctx-menu button:hover { background: var(--bg-hover); }
.ctx-menu .ctx-danger { color: var(--danger); }
.ctx-menu .ctx-danger:hover { background: rgba(239,68,68,.1); }
.ctx-sep { height: 1px; background: var(--border); margin: 4px 8px; }

/* Global scrollbar — bumped from 6px → 12px for the same reason as
   EmojiPicker's `.ep-scroll`: 6px is too thin to grab reliably. We use
   `background-clip: padding-box` + transparent border to inset the
   visible thumb (~6px) inside a 12px hit-area, so the bar still looks
   slim while being twice as easy to drag. On hover the inset shrinks
   so the thumb feels "thicker" as a subtle affordance. */
::-webkit-scrollbar { width: 12px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb {
  background: var(--scrollbar);
  background-clip: padding-box;
  border: 3px solid transparent;
  border-radius: 6px;
}
::-webkit-scrollbar-thumb:hover {
  background: var(--text-muted);
  background-clip: padding-box;
  border: 2px solid transparent;
}

/* 自定义确认弹窗 */
.confirm-mask { position: fixed; inset: 0; background: rgba(0,0,0,.45); display: flex; align-items: center; justify-content: center; z-index: 200; }
.confirm-box {
  background: var(--bg-elevated); border-radius: 12px; padding: 20px;
  width: 320px; box-shadow: 0 8px 30px rgba(0,0,0,.3);
  display: flex; flex-direction: column; gap: 14px;
}
.confirm-icon { color: var(--warning); display: flex; justify-content: center; }
.confirm-body { text-align: center; }
.confirm-title { font-size: 15px; font-weight: 600; color: var(--text); margin-bottom: 6px; }
.confirm-msg { font-size: 13px; color: var(--text-secondary); }
.confirm-actions { display: flex; gap: 10px; justify-content: center; }
.cbtn { padding: 8px 24px; border-radius: 6px; font-size: 13px; cursor: pointer; border: none; transition: all .15s; }
.cbtn-cancel { background: var(--bg); color: var(--text-secondary); border: 1px solid var(--border); }
.cbtn-cancel:hover { background: var(--bg-hover); color: var(--text); }
.cbtn-ok { background: var(--danger); color: #fff; }
.cbtn-ok:hover { background: #dc2626; }
.cbtn-primary { background: var(--accent); color: #fff; }
.cbtn-primary:hover { background: var(--accent-hover); }

/* 备注弹窗 */
.note-box {
  background: var(--bg-elevated); border-radius: 12px; padding: 18px 20px;
  width: 360px; box-shadow: 0 8px 30px rgba(0,0,0,.3);
  display: flex; flex-direction: column; gap: 14px;
}
.note-head { display: flex; justify-content: space-between; align-items: center; }
.note-head span { font-size: 15px; font-weight: 600; color: var(--text); }
.note-close { background: transparent; border: none; cursor: pointer; padding: 0; }
.note-input {
  background: var(--bg); border: 1.5px solid var(--accent); color: var(--text);
  padding: 8px 12px; border-radius: 6px; font-size: 14px; outline: none;
}

/* 内容区域容器：包含 list 和 emoji picker，用于绝对定位参照 */
.content-area {
  position: relative;
  flex: 1;
  overflow: hidden;
}

/* Emoji Picker 视图 */
.emoji-main {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  z-index: 10;
}
/* 隐藏时保持在 paint 管线中（不能用 visibility:hidden！）
   原因：visibility:hidden 会让 Chromium 退出该子树的 paint pipeline，
   切回 visible 时要一次性重新 paint 全部 150+ emoji span，造成肉眼可见的卡顿。
   用 opacity:0 + pointer-events:none 则子树一直在 paint 中，切换只是合成层
   的透明度变化（GPU 搞定），切 tab 零成本。 */
.emoji-main.emoji-hidden {
  opacity: 0;
  pointer-events: none;
  z-index: -1;
}

/* Emoji 复制成功 Toast */
.emoji-toast {
  position: fixed;
  bottom: 40px;
  left: 50%;
  transform: translateX(-50%);
  background: var(--bg-elevated);
  border: 1px solid var(--border);
  color: var(--text);
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 14px;
  box-shadow: 0 4px 12px rgba(0,0,0,.3);
  z-index: 300;
  pointer-events: none;
}
</style>
