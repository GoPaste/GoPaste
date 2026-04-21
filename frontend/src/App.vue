<script lang="ts" setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
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
} from '../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
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

// 图片缩略图缓存
const imageThumbs = ref<Record<number, string>>({})

// 回到顶部
const listRef = ref<HTMLElement | null>(null)
const showBackTop = ref(false)

function onListScroll() {
  if (listRef.value) {
    showBackTop.value = listRef.value.scrollTop > 200
  }
}
function scrollToTop() {
  listRef.value?.scrollTo({ top: 0, behavior: 'smooth' })
}

const typeOptions: { key: ItemType | 'fav'; label: string; icon: any }[] = [
  { key: '', label: '全部', icon: List },
  { key: 'text', label: '文本', icon: FileText },
  { key: 'image', label: '图片', icon: ImageIcon },
  { key: 'file', label: '文件', icon: FileIcon },
  { key: 'link', label: '链接', icon: LinkIcon },
  { key: 'code', label: '代码', icon: Code2 },
  { key: 'fav', label: '收藏', icon: Star },
]

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
  try {
    const isFav = typeFilter.value === ('fav' as any)
    const r = await ListItems({
      keyword: keyword.value,
      type: isFav ? '' : typeFilter.value,
      favorite: isFav,
      page: 1,
      pageSize: 200,
    } as any)
    items.value = (r?.items || []) as Item[]
    total.value = r?.total || 0
    if (selectedIdx.value >= items.value.length) selectedIdx.value = 0
    for (const it of items.value) {
      if (it.type === 'image' && !imageThumbs.value[it.id]) {
        loadThumb(it.id)
      }
    }
  } finally {
    loading.value = false
  }
}

async function loadThumb(id: number) {
  try {
    const b64 = await GetContent(id)
    imageThumbs.value[id] = `data:image/png;base64,${b64}`
  } catch {
    // 忽略
  }
}

async function showDetail(it: Item) {
  try {
    const b64 = await GetContent(it.id)
    if (it.type === 'image') {
      detailContent.value = `data:image/png;base64,${b64}`
    } else {
      detailContent.value = atob(b64)
    }
    detailVisible.value = true
  } catch (e) {
    console.error(e)
  }
}

async function doPaste(it: Item) { await PasteItem(it.id) }
async function doCopy(it: Item) { await CopyToClipboard(it.id) }

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
  const ok = await showConfirm('确定删除这条记录？')
  if (!ok) return
  await DeleteItem(it.id)
  delete imageThumbs.value[it.id]
  await refresh()
}
async function doTogglePin(it: Item) { await TogglePin(it.id, !it.pinned); await refresh() }
async function doToggleFav(it: Item) { await ToggleFavorite(it.id, !it.favorite); await refresh() }

// 主题
const theme = ref<'dark' | 'light'>('dark')

function applyTheme(t: string) {
  theme.value = (t === 'light') ? 'light' : 'dark'
  document.documentElement.setAttribute('data-theme', theme.value)
}

async function loadTheme() {
  try {
    const s: any = await GetSettings()
    applyTheme(s?.theme || 'dark')
  } catch { /* ignore */ }
}

// 当前筛选 tab 的索引（用于 Tab/左右键切换）
const filterIdx = computed(() => typeOptions.findIndex(o => o.key === typeFilter.value))

function switchFilter(dir: number) {
  const newIdx = (filterIdx.value + dir + typeOptions.length) % typeOptions.length
  typeFilter.value = typeOptions[newIdx].key as any
  refresh()
}

function onKeyDown(e: KeyboardEvent) {
  if (view.value !== 'main') return

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
  window.addEventListener('keydown', onKeyDown)
  window.addEventListener('blur', onWindowBlur)
  await loadTheme()
  await refresh()
  EventsOn('clipboard:new', async () => { await refresh() })
  unsubscribe = () => EventsOff('clipboard:new')
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
  window.removeEventListener('blur', onWindowBlur)
  if (unsubscribe) unsubscribe()
})

watch(view, async (v) => { if (v === 'main') { await loadTheme(); await refresh() } })

function formatTime(t: string | Date): string {
  const d = new Date(t)
  const diff = (Date.now() - d.getTime()) / 1000
  if (diff < 60) return `${Math.floor(diff)}s 前`
  if (diff < 3600) return `${Math.floor(diff / 60)}m 前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h 前`
  return d.toLocaleDateString()
}

const typeLabel: Record<string, string> = {
  text: 'Plain Text',
  image: 'Image',
  link: 'Link',
  code: 'Code',
  file: 'File',
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function metaLine(it: Item): string {
  const parts: string[] = [typeLabel[it.type] || it.type]
  if (it.size) parts.push(formatSize(it.size))
  if (it.charCount && it.type !== 'image') parts.push(`${it.charCount} Character(s)`)
  parts.push(formatTime(it.updatedAt))
  return parts.join('  ')
}

// 窗口失焦自动隐藏
function onWindowBlur() {
  // 延迟 150ms：避免点击收藏/删除等操作时误触隐藏（某些操作会短暂失焦）
  setTimeout(() => {
    if (!document.hasFocus()) {
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
          <input v-model="keyword" placeholder="搜索..." @input="refresh" autofocus />
        </div>
        <button class="icon-btn" title="设置" @click="view = 'settings'"><SettingsIcon :size="16" /></button>
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
        <div v-if="loading" class="empty">加载中...</div>
        <div v-else-if="items.length === 0" class="empty">
          <ClipboardList :size="48" class="empty-icon" />
          <div>暂无历史记录</div>
          <div class="hint">复制一些内容试试吧</div>
        </div>

        <div v-for="(it, idx) in items" :key="it.id" class="item"
          :class="{ active: idx === selectedIdx, pinned: it.pinned }"
          @click="selectedIdx = idx" @dblclick="doPaste(it)">

          <!-- 第一行：元信息 + 操作按钮 -->
          <div class="item-row1">
            <span class="item-meta">{{ metaLine(it) }}</span>
            <div class="item-actions" @click.stop>
              <button title="复制" @click="doCopy(it)"><Copy :size="14" /></button>
              <button title="收藏" :class="{ active: it.favorite }" @click="doToggleFav(it)">
                <Star :size="14" :fill="it.favorite ? 'currentColor' : 'none'" />
              </button>
              <button title="删除" @click="doDelete(it)"><Trash2 :size="14" /></button>
            </div>
          </div>

          <!-- 第二行：内容预览 -->
          <div class="item-row2">
            <!-- 图片：缩略图 + 描述 -->
            <template v-if="it.type === 'image'">
              <div class="thumb" @click.stop="showDetail(it)">
                <img v-if="imageThumbs[it.id]" :src="imageThumbs[it.id]" />
                <div v-else class="thumb-ph">
                  <ImageIcon :size="20" />
                </div>
              </div>
            </template>
            <!-- 文本/代码/链接/文件 -->
            <div v-else class="item-preview" @click.stop="showDetail(it)">{{ it.preview || '(空)' }}</div>
          </div>
        </div>
      </main>

      <!-- 回到顶部浮动按钮 -->
      <Transition name="fade">
        <button v-if="showBackTop" class="back-top" title="回到顶部" @click="scrollToTop">
          <ArrowUp :size="18" />
        </button>
      </Transition>

      <footer class="statusbar">
        <span>{{ total }} 条记录</span>
        <span class="dim">Tab 切换 · ↑↓ 选择 · ↵ 粘贴 · Esc 关闭</span>
      </footer>

      <div v-if="detailVisible" class="detail-mask" @click.self="detailVisible = false">
        <div class="detail">
          <div class="detail-head">
            <span>详情</span>
            <button @click="detailVisible = false"><Trash2 :size="0" /><span style="font-size:16px;color:#a8adbd;cursor:pointer">✕</span></button>
          </div>
          <div class="detail-body">
            <img v-if="selected?.type === 'image'" :src="detailContent" />
            <pre v-else>{{ detailContent }}</pre>
          </div>
        </div>
      </div>
    </template>

    <Settings v-else @close="view = 'main'" />

    <!-- 自定义确认弹窗 -->
    <Transition name="fade">
      <div v-if="confirmVisible" class="confirm-mask" @click.self="onConfirmCancel">
        <div class="confirm-box">
          <div class="confirm-icon"><AlertTriangle :size="24" /></div>
          <div class="confirm-body">
            <div class="confirm-title">确认操作</div>
            <div class="confirm-msg">{{ confirmMsg }}</div>
          </div>
          <div class="confirm-actions">
            <button class="cbtn cbtn-cancel" @click="onConfirmCancel">取消</button>
            <button class="cbtn cbtn-ok" @click="onConfirmOk">确定</button>
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
.app { display: flex; flex-direction: column; height: 100vh; }

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
.search { flex: 1; display: flex; align-items: center; gap: 6px; background: var(--bg-elevated); border-radius: 8px; padding: 6px 10px; }
.search input { flex: 1; background: transparent; border: none; outline: none; color: var(--text); font-size: 14px; }
.search-icon { color: var(--text-muted); flex-shrink: 0; }
.icon-btn {
  background: transparent; border: 1px solid var(--border); color: var(--text-secondary);
  width: 34px; height: 34px; border-radius: 8px; cursor: pointer;
  transition: all .15s;
  display: flex; align-items: center; justify-content: center;
}
.icon-btn:hover { background: var(--bg-elevated); color: #fff; }
.icon-btn.active { background: var(--accent); border-color: #3b82f6; color: #fff; }

.filters { display: flex; gap: 4px; padding: 6px 10px; border-bottom: 1px solid var(--border); overflow-x: auto; flex-wrap: nowrap; }
.filters button {
  display: inline-flex; align-items: center; gap: 3px;
  background: var(--bg-elevated); border: 1px solid transparent; color: var(--text-secondary);
  padding: 4px 8px; border-radius: 14px; font-size: 11px; cursor: pointer; white-space: nowrap;
}
.filters button.active { background: var(--accent); color: #fff; }

.list { flex: 1; overflow-y: auto; padding: 4px 0; }
.empty { text-align: center; padding: 60px 20px; color: var(--text-muted); display: flex; flex-direction: column; align-items: center; gap: 10px; }
.empty-icon { color: #2a2f3a; }
.empty .hint { font-size: 12px; opacity: .6; }

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
.item-preview:hover { color: #fff; }

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
.detail { background: var(--bg-elevated); border-radius: 10px; width: 80%; max-width: 640px; max-height: 80vh; display: flex; flex-direction: column; box-shadow: 0 10px 30px rgba(0,0,0,.5); }
.detail-head { display: flex; justify-content: space-between; align-items: center; padding: 10px 14px; border-bottom: 1px solid var(--border); font-size: 13px; }
.detail-head button { background: transparent; border: none; color: var(--text-secondary); cursor: pointer; display: flex; align-items: center; }
.detail-head button:hover { color: #fff; }
.detail-body { padding: 14px; overflow: auto; }
.detail-body pre { margin: 0; white-space: pre-wrap; word-break: break-all; font-size: 13px; color: var(--text); }
.detail-body img { max-width: 100%; border-radius: 6px; }

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
</style>
