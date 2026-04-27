<script lang="ts" setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { t, lang } from './i18n'
import type { Lang } from './i18n'
import {
  ListItems,
  DeleteItem,
  PasteItem,
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
} from 'lucide-vue-next'

type Item = types.Item
type ItemType = '' | 'text' | 'image' | 'link' | 'code' | 'file'

const view = ref<'main' | 'settings'>('main')

const items = ref<Item[]>([])
const total = ref(0)
const keyword = ref('')
const typeFilter = ref<ItemType>('')
const favoriteOnly = ref(false)
const selectedIdx = ref(0)
const detailContent = ref<string>('')
const detailVisible = ref(false)
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

async function refresh() {
  loading.value = true
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
    loading.value = false
  }
}

// 点击搜索框右侧 X：清空关键字、刷新并保持焦点
function clearKeyword() {
  if (!keyword.value) return
  keyword.value = ''
  refresh()
  searchRef.value?.focus()
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

// 详情类型
const detailIsImage = ref(false)

async function showDetail(it: Item) {
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
// 单击模式下的延时触发器：区分单击/双击（双击时取消单击的 doPaste）
let singleClickTimer: number | null = null
const DBLCLICK_THRESHOLD_MS = 250

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

async function doDelete(it: Item) {
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
    return s
  } catch { return null }
}

// 窗口激活时：根据设置回到顶部 / 切换至全部分组
// 注意：用 visibilitychange 而不是 focus 事件——因为在 WebView2 里
// 拖动窗口（drag-region）会触发 blur/focus 但不会改 visibilityState，
// 只有 HideWindow + ShowWindow 才会让 document 从 hidden 变 visible。
// 这样拖窗口时不会误重置筛选/滚动位置。
async function onWindowShow() {
  if (view.value !== 'main') return
  const s: any = await GetSettings()
  if (!s) return
  let needRefresh = false
  if (s.resetFilterOnShow && typeFilter.value !== '') {
    typeFilter.value = ''
    needRefresh = true
  }
  if (needRefresh) await refresh()
  if (s.scrollTopOnShow) {
    selectedIdx.value = 0
    listRef.value?.scrollTo({ top: 0 })
  }
  // 聚焦搜索框
  searchRef.value?.focus()
}

function onVisibilityChange() {
  if (document.visibilityState === 'visible') {
    onWindowShow()
  } else {
    // 窗口隐藏时退出设置，下次显示直接回到主列表
    if (view.value !== 'main') view.value = 'main'
  }
}

// 当前筛选 tab 的索引（用于 Tab/左右键切换）
const filterIdx = computed(() => typeOptions.value.findIndex(o => o.key === typeFilter.value))

function switchFilter(dir: number) {
  const opts = typeOptions.value
  const newIdx = (filterIdx.value + dir + opts.length) % opts.length
  typeFilter.value = opts[newIdx].key as any
  refresh()
}

function onKeyDown(e: KeyboardEvent) {
  // 设置界面：Esc 退出设置回到主列表
  if (view.value !== 'main') {
    if (e.key === 'Escape') {
      e.preventDefault()
      view.value = 'main'
    }
    return
  }

  // 搜索框获焦时不拦截左右键
  const inSearch = (e.target as HTMLElement)?.tagName === 'INPUT'

  if (e.key === 'Tab') {
    e.preventDefault()
    switchFilter(e.shiftKey ? -1 : 1)
  } else if ((e.key === 'ArrowLeft' || e.key === 'ArrowRight') && !inSearch) {
    e.preventDefault()
    switchFilter(e.key === 'ArrowLeft' ? -1 : 1)
  } else if (e.key === 'ArrowDown') {
    selectedIdx.value = Math.min(selectedIdx.value + 1, items.value.length - 1)
    e.preventDefault()
  } else if (e.key === 'ArrowUp') {
    selectedIdx.value = Math.max(selectedIdx.value - 1, 0)
    e.preventDefault()
  } else if (e.key === 'Enter' && selected.value) {
    doPaste(selected.value)
  } else if (e.key === 'Escape') {
    if (detailVisible.value) detailVisible.value = false
    else HideWindow()
  } else if (e.key === 'Delete' && selected.value) {
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
  await loadSettings()
  // loadSettings → applyTheme 已调 syncWebViewBg，这里再兜底一次确保初始化正确
  syncWebViewBg()
  await refresh()
  EventsOn('clipboard:new', async () => { await refresh() })
  unsubscribe = () => { EventsOff('clipboard:new'); EventsOff('window:show') }
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
  window.removeEventListener('blur', onWindowBlur)
  window.removeEventListener('resize', syncWebViewBg)
  document.removeEventListener('visibilitychange', onVisibilityChange)
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

function metaParts(it: Item): string[] {
  const parts: string[] = [typeLabel.value[it.type] || it.type]
  if (it.size) parts.push(formatSize(it.size))
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
      if (view.value !== 'main') view.value = 'main'
      HideWindow()
    }
  }, 150)
}
</script>

<template>
  <div class="app">
    <template v-if="view === 'main'">
      <header class="topbar drag-region">
        <div class="search">
          <Search :size="16" class="search-icon" />
          <input ref="searchRef" v-model="keyword" :placeholder="t('search')" @input="refresh" autofocus />
          <button
            v-if="keyword"
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
        <button v-for="opt in typeOptions" :key="opt.key"
          :class="{ active: typeFilter === opt.key }"
          @click="typeFilter = opt.key as any; refresh()">
          <component :is="opt.icon" :size="13" />
          <span>{{ opt.label }}</span>
        </button>
      </nav>

      <main class="list" ref="listRef" @scroll="onListScroll">
        <div v-if="loading" class="empty">{{ t('loading') }}</div>
        <div v-else-if="items.length === 0" class="empty">
          <ClipboardList :size="48" class="empty-icon" />
          <div>{{ t('empty') }}</div>
          <div class="hint">{{ t('emptyHint') }}</div>
        </div>

        <div v-for="(it, idx) in items" :key="it.id" class="item"
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

      <!-- 回到顶部 -->
      <Transition name="fade">
        <button v-if="showBackTop" class="back-top" :title="t('backTop')" @click="scrollToTop">
          <ArrowUp :size="18" />
        </button>
      </Transition>

      <footer class="statusbar">
        <span>{{ total }} {{ t('records') }}</span>
        <span class="dim">{{ t('statusHint') }}</span>
      </footer>

      <!-- 图片预览 -->
      <div v-if="detailVisible && detailIsImage" class="detail-mask" @click.self="detailVisible = false">
        <img class="detail-img-only" :src="detailContent" @click="detailVisible = false" />
      </div>
      <!-- 文本详情 -->
      <div v-else-if="detailVisible" class="detail-mask" @click.self="detailVisible = false">
        <div class="detail">
          <div class="detail-head">
            <span>{{ t('detail') }}</span>
            <button @click="detailVisible = false"><Trash2 :size="0" /><span style="font-size:16px;color:#a8adbd;cursor:pointer">✕</span></button>
          </div>
          <div class="detail-body">
            <pre>{{ detailContent }}</pre>
          </div>
        </div>
      </div>
    </template>

    <Settings v-else @close="view = 'main'" />

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
  --bg: #14161c; --bg-elevated: #1c2029; --bg-hover: #1a1e27; --bg-active: #21283a;
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

.list { flex: 1; overflow-y: auto; padding: 4px 0; }
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

.detail-mask { position: fixed; inset: 0; background: rgba(0,0,0,.5); display: flex; align-items: center; justify-content: center; z-index: 100; }
.detail { background: var(--bg-elevated); border-radius: 10px; width: calc(100% - 16px); max-height: 80vh; display: flex; flex-direction: column; box-shadow: 0 10px 30px rgba(0,0,0,.3); margin: 0 8px; }
.detail-head { display: flex; justify-content: space-between; align-items: center; padding: 10px 14px; border-bottom: 1px solid var(--border); font-size: 13px; }
.detail-head button { background: transparent; border: none; color: var(--text-secondary); cursor: pointer; display: flex; align-items: center; }
.detail-head button:hover { color: #fff; }
.detail-body { padding: 14px; overflow: auto; text-align: left; }
.detail-body pre { margin: 0; white-space: pre-wrap; word-break: break-all; font-size: 13px; color: var(--text); text-align: left; }
.detail-body img { max-width: 100%; border-radius: 6px; }

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

::-webkit-scrollbar { width: 6px; }
::-webkit-scrollbar-thumb { background: var(--scrollbar); border-radius: 3px; }

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
</style>
