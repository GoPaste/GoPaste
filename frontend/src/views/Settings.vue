<script lang="ts" setup>
import { computed, inject, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import {
  GetSettings,
  UpdateSettings,
  ExportData,
  ExportDataToFile,
  ClearHistory,
  DataDir,
  GetAppVersion,
  GetWebsite,
  CheckForUpdate,
  OpenURL,
} from '../../wailsjs/go/main/App'
import {
  Settings as SettingsIcon, Keyboard, ClipboardList,
  Download, Trash2, Info, Database, X, AlertTriangle,
  Star, FileText, Image as ImageIcon, File as FileIcon,
  Link as LinkIcon, Code2, Puzzle, Smile,
} from 'lucide-vue-next'
import { t, lang } from '../i18n'
import type { Lang } from '../i18n'

const emit = defineEmits<{ (e: 'close'): void }>()

// 父组件 App.vue 提供：在已知会副作用性地把其它进程（如「系统设置」）
// 拉到前台、从而触发主面板 onWindowBlur 自动隐藏的操作前后，包一层即可
// 在窗口失焦的短暂时间窗内禁用自动隐藏。
// 详见 App.vue 中 withSuppressBlur 的定义注释，以及本文件 cmdQBehavior
// 切换 watcher 的注释。
const blurGuard = inject<{ withSuppressBlur: <T>(fn: () => T | Promise<T>, ms?: number) => Promise<T> } | null>('suppressBlurHide', null)

type Tab = 'general' | 'clipboard' | 'shortcut' | 'backup' | 'extensions' | 'about'
const activeTab = ref<Tab>('general')

// 扩展功能里有跨平台项（Emoji 总开关 / 显示完整表情库）和仅 macOS 项
// （Cmd+Q 行为）。扩展入口本身在所有平台都显示；仅平台相关的子区块在
// 模板中按 isMacPlatform 行级隐藏，与 tabModKey 的平台检测方式保持一致。
const isMacPlatform = navigator.platform.toUpperCase().includes('MAC') || navigator.userAgent.includes('Mac')

const navItems = computed(() => [
  { key: 'general' as Tab, label: t('navGeneral'), icon: SettingsIcon },
  { key: 'clipboard' as Tab, label: t('navClipboard'), icon: ClipboardList },
  { key: 'shortcut' as Tab, label: t('navShortcut'), icon: Keyboard },
  { key: 'backup' as Tab, label: t('navBackup'), icon: Database },
  { key: 'extensions' as Tab, label: t('navExtensions'), icon: Puzzle },
  { key: 'about' as Tab, label: t('navAbout'), icon: Info },
])

const form = reactive({
  hotkeyModifiers: [] as string[],
  hotkeyKey: 'V',
  maxItems: 1000,
  maxDays: 30,
  theme: 'dark',
  language: 'zh' as Lang,
  pasteTrigger: 'double' as 'single' | 'double',
  windowPosition: 'center',
  scrollTopOnShow: true,
  resetFilterOnShow: true,
  clearSearchOnShow: true,
  silentStart: false,
  showTrayIcon: true,
  showTaskbarIcon: false,
  trayIconStyle: 'color' as 'color' | 'gray',
  autoStart: false,
  tabHotkeysEnabled: true,
  // 扩展功能 —— Emoji
  emojiEnabled: true,
  extendedEmoji: false,
  // 扩展功能 —— macOS Cmd+Q 防误触
  cmdQBehavior: 'default' as 'default' | 'confirm' | 'disable',
  cmdQConfirmWindow: 1500,
})

const dataDir = ref('')
const saveMsg = ref('')
const recording = ref(false)
// 标记初次 load 完成前不触发自动保存
const loaded = ref(false)

// 关于页：版本号 + 检查更新
const appVersion = ref('0.0.0')
const websiteUrl = ref('https://gopaste.wetools.cc/')
const updateChecking = ref(false)
const updateMsg = ref('')
const updateHasNew = ref(false)
const updateUrl = ref('')

async function onCheckUpdate() {
  if (updateChecking.value) return
  updateChecking.value = true
  updateMsg.value = ''
  updateHasNew.value = false
  updateUrl.value = ''
  try {
    const res: any = await CheckForUpdate()
    if (res && res.hasUpdate) {
      updateHasNew.value = true
      updateUrl.value = res.releaseUrl || ''
      updateMsg.value = t('newVersionAvailable', { v: res.latestVersion || '' })
    } else {
      updateMsg.value = t('upToDate')
    }
  } catch (e) {
    console.error(e)
    updateMsg.value = t('upToDate') // 无网络等错误静默当作"已是最新"
  } finally {
    updateChecking.value = false
  }
}
async function onOpenRelease() {
  if (updateUrl.value) await OpenURL(updateUrl.value)
}

const hotkeyDisplay = computed(() => {
  const parts = [...form.hotkeyModifiers.map(m => m.charAt(0).toUpperCase() + m.slice(1))]
  parts.push(form.hotkeyKey)
  return parts.join(' + ')
})

// 切换分类的固定快捷键。
// keyDisplay 仅作展示用——实际绑定在后端 hotkey.Manager 与 App.vue 的 onKeyDown 中。
// 与 App.vue 的 typeOptions 顺序保持一致（1..6=收藏/文本/图片/文件/链接/代码）。
// 注：0="全部" 不在此列表 —— 由主热键直接唤起，无需独立分类热键。
// macOS 上 Option(Alt)+数字 被系统拦截，改用 Cmd+数字。
// 复用顶部定义的 isMacPlatform，避免重复检测平台。
const tabModKey = isMacPlatform ? 'Cmd' : 'Alt'
const tabShortcuts = computed(() => [
  { keys: [`${tabModKey} + 1`], label: t('favorite'), icon: Star },
  { keys: [`${tabModKey} + 2`], label: t('text'), icon: FileText },
  { keys: [`${tabModKey} + 3`], label: t('image'), icon: ImageIcon },
  { keys: [`${tabModKey} + 4`], label: t('file'), icon: FileIcon },
  { keys: [`${tabModKey} + 5`], label: t('link'), icon: LinkIcon },
  { keys: [`${tabModKey} + 6`], label: t('code'), icon: Code2 },
])

// 应用内快捷键。仅在主面板有焦点时生效，不参与全局热键注册。
// keys 多按键以 sep（默认 '/'）分隔展示，表示"任一可用"。
// 与 App.vue::onKeyDown 中的实际绑定保持同步，改一处需要同改另一处。
const appShortcuts = computed(() => [
  { label: t('appSwitchTab'),  keys: ['Tab', 'Shift + Tab', '←', '→'] },
  { label: t('appMoveSelect'), keys: ['↑', '↓'] },
  // 空格：在"选择"和"粘贴"之间，用于预览/关闭详情面板。
  // 与 App.vue::onKeyDown 中的实际绑定保持同步——仅当搜索框为空时
  // 才升级为预览快捷键，否则作为普通字符输入。
  { label: t('appPreview'),    keys: ['Space'] },
  // 双击空格：按选中项类型分发到"专属操作"——image→保存、file→在文件夹中显示、link→浏览器打开。
  // text/code 等无专属动作的类型不进双击窗口（见 App.vue::hasPrimaryAction）。
  // 两个 Space 是"先后连按"语义，故用空串 sep（不走默认的"/"任一分隔符）。
  { label: t('appPrimaryAction'), keys: ['Space', 'Space'], sep: '' },
  { label: t('appPaste'),      keys: [t('triggerDouble'), 'Enter'] },
  { label: t('appDelete'),     keys: ['Delete', 'Backspace'] },
  { label: t('appClose'),      keys: ['Esc'] },
])

function setLang(v: Lang) {
  form.language = v
  lang.value = v
}

function setTheme(val: string) {
  form.theme = val
  document.documentElement.setAttribute('data-theme', val)
}

async function load() {
  const s: any = await GetSettings()
  form.hotkeyModifiers = [...(s.hotkeyModifiers || ['ctrl', 'shift'])]
  form.hotkeyKey = s.hotkeyKey || 'V'
  form.maxItems = s.maxItems ?? 1000
  form.maxDays = s.maxDays ?? 30
  form.theme = s.theme || 'dark'
  form.language = (s.language || 'zh') as Lang
  lang.value = form.language
  form.pasteTrigger = (s.pasteTrigger === 'single' ? 'single' : 'double')
  form.windowPosition = s.windowPosition || 'center'
  form.scrollTopOnShow = s.scrollTopOnShow !== false
  form.resetFilterOnShow = s.resetFilterOnShow !== false
  form.clearSearchOnShow = s.clearSearchOnShow !== false
  form.silentStart = !!s.silentStart
  form.showTrayIcon = s.showTrayIcon !== false
  form.showTaskbarIcon = !!s.showTaskbarIcon
  form.trayIconStyle = (s.trayIconStyle === 'gray' ? 'gray' : 'color')
  form.autoStart = !!s.autoStart
  form.tabHotkeysEnabled = s.tabHotkeysEnabled !== false // 缺失/旧配置默认开启
  // 扩展功能 —— Emoji
  form.emojiEnabled = s.emojiEnabled !== false // 缺失/旧配置默认开启
  form.extendedEmoji = !!s.extendedEmoji
  // 扩展功能 —— Cmd+Q 策略。旧配置缺失时默认 "default"。
  form.cmdQBehavior = (['confirm', 'disable'].includes(s.cmdQBehavior) ? s.cmdQBehavior : 'default') as typeof form.cmdQBehavior
  form.cmdQConfirmWindow = (typeof s.cmdQConfirmWindow === 'number' && s.cmdQConfirmWindow > 0) ? s.cmdQConfirmWindow : 1500
  dataDir.value = await DataDir()
  try { appVersion.value = await GetAppVersion() } catch {}
  try { websiteUrl.value = await GetWebsite() } catch {}
  // load 完成后再允许自动保存，避免 watch 初始触发回写
  loaded.value = true
}

// 将当前 form 同步到后端。调用方负责做必要的值校验。
async function autoSave() {
  try {
    await UpdateSettings({
      hotkeyModifiers: form.hotkeyModifiers,
      hotkeyKey: form.hotkeyKey,
      maxItems: Number(form.maxItems),
      maxDays: Number(form.maxDays),
      theme: form.theme,
      language: form.language,
      pasteTrigger: form.pasteTrigger,
      windowPosition: form.windowPosition,
      scrollTopOnShow: form.scrollTopOnShow,
      resetFilterOnShow: form.resetFilterOnShow,
      clearSearchOnShow: form.clearSearchOnShow,
      silentStart: form.silentStart,
      showTrayIcon: form.showTrayIcon,
      showTaskbarIcon: form.showTaskbarIcon,
      trayIconStyle: form.trayIconStyle,
      autoStart: form.autoStart,
      tabHotkeysEnabled: form.tabHotkeysEnabled,
      emojiEnabled: form.emojiEnabled,
      extendedEmoji: form.extendedEmoji,
      cmdQBehavior: form.cmdQBehavior,
      cmdQConfirmWindow: Number(form.cmdQConfirmWindow),
    } as any)
    saveMsg.value = t('saved')
    setTimeout(() => { saveMsg.value = '' }, 1500)
  } catch (e: any) {
    saveMsg.value = t('saveFailed') + (e?.message || e)
  }
}

// 立即保存（用于 toggle/seg-ctrl/hotkey 这类离散变更）
function watchImmediate<T>(getter: () => T) {
  watch(getter, () => { if (loaded.value) autoSave() })
}

// 数值输入失焦时保存：规范化非法值（空/负数 -> 0），避免写入中间态
function onNumberBlur(field: 'maxItems' | 'maxDays') {
  const raw = form[field] as unknown
  let n = Number(raw)
  if (!Number.isFinite(n) || n < 0) n = 0
  n = Math.floor(n)
  form[field] = n
  if (loaded.value) autoSave()
}

// Cmd+Q 确认时间窗：限定 300..10000，避免写入极端值
function onCmdQWindowBlur() {
  let n = Number(form.cmdQConfirmWindow)
  if (!Number.isFinite(n) || n < 300) n = 300
  if (n > 10000) n = 10000
  form.cmdQConfirmWindow = Math.floor(n)
  if (loaded.value) autoSave()
}

// 所有离散字段：变更即写盘
watchImmediate(() => form.theme)
watchImmediate(() => form.language)
watchImmediate(() => form.pasteTrigger)
watchImmediate(() => form.windowPosition)
watchImmediate(() => form.scrollTopOnShow)
watchImmediate(() => form.resetFilterOnShow)
watchImmediate(() => form.clearSearchOnShow)
watchImmediate(() => form.silentStart)
watchImmediate(() => form.showTrayIcon)
watchImmediate(() => form.showTaskbarIcon)
watchImmediate(() => form.trayIconStyle)
watchImmediate(() => form.autoStart)
watchImmediate(() => form.tabHotkeysEnabled)
watchImmediate(() => form.emojiEnabled)
watchImmediate(() => form.extendedEmoji)
watchImmediate(() => form.hotkeyKey)
watchImmediate(() => form.hotkeyModifiers.join('+'))
// 扩展功能：Cmd+Q 策略切换即写盘；时间窗用 number 输入的 blur 触发。
//
// 注意：在 macOS 上，把策略切到 confirm/disable 时，后端会尝试启用 L0
// 全局拦截（CGEventTap）。若用户尚未授予「输入监控」权限，后端可能会
// OpenInputMonitoringPrefs() 把「系统设置」拉到前台 —— 这会让 GoPaste
// 主面板失去 key window 状态，触发 App.vue 的 onWindowBlur 自动 HideWindow。
// 用户感知是"来回切几下按钮，主界面突然消失了"。
// 这里用 blurGuard 在切换前后压住自动隐藏一段时间窗作为前端兜底；
// 后端也已做"进程内仅引导一次"去重（见 app.go cmdQTapPermGuideOnce）。
watch(() => form.cmdQBehavior, () => {
  if (!loaded.value) return
  if (blurGuard) {
    blurGuard.withSuppressBlur(() => autoSave(), 1200)
  } else {
    autoSave()
  }
})
// 注意：form.maxItems / form.maxDays / form.cmdQConfirmWindow 不使用 watch，
// 改为输入框 blur/回车时触发保存

async function doExport() {
  await ExportDataToFile()
}

async function doClear() {
  if (!(await showConfirm(t('clearConfirm')))) return
  await ClearHistory()
  saveMsg.value = t('cleared')
  setTimeout(() => (saveMsg.value = ''), 2000)
}

// 自定义确认弹窗（替代浏览器原生 window.confirm，保持应用内一致的 UI 风格）
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

// 确认弹窗显示期间接管键盘：Enter=确认 / Esc=取消。
// 用 capture 阶段 + stopPropagation 吞掉所有其他键，防止冒到输入框/按钮的原生行为
// （例如弹框时 Delete/Backspace 不应触发数字输入框的清空、Esc 不应触发其他层级的关闭）。
function onConfirmKey(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    e.preventDefault(); e.stopPropagation()
    onConfirmOk()
  } else if (e.key === 'Escape') {
    e.preventDefault(); e.stopPropagation()
    onConfirmCancel()
  } else {
    e.preventDefault(); e.stopPropagation()
  }
}
watch(confirmVisible, (v) => {
  if (v) window.addEventListener('keydown', onConfirmKey, true)
  else window.removeEventListener('keydown', onConfirmKey, true)
})
onBeforeUnmount(() => window.removeEventListener('keydown', onConfirmKey, true))

function startRecording() { recording.value = true }

function onRecordKey(e: KeyboardEvent) {
  e.preventDefault()
  e.stopPropagation()
  const result = normalizeKey(e)
  if (!result) return
  form.hotkeyModifiers = result.mods
  form.hotkeyKey = result.key
  recording.value = false
}

function normalizeKey(e: KeyboardEvent): { mods: string[], key: string } | null {
  const mods: string[] = []
  if (e.ctrlKey) mods.push('ctrl')
  if (e.shiftKey) mods.push('shift')
  if (e.altKey) mods.push('alt')
  if (e.metaKey) mods.push('cmd')
  const ignoredKeys = ['Control', 'Shift', 'Alt', 'Meta']
  if (ignoredKeys.includes(e.key)) return null
  if (mods.length === 0) return null
  let key = e.key
  const keyMap: Record<string, string> = {
    ' ': 'Space', 'ArrowUp': 'Up', 'ArrowDown': 'Down',
    'ArrowLeft': 'Left', 'ArrowRight': 'Right',
    'Enter': 'Enter', 'Return': 'Enter',
    'Escape': 'Escape', 'Esc': 'Escape',
    'Backspace': 'Backspace', 'Delete': 'Delete', 'Tab': 'Tab',
  }
  if (keyMap[key]) key = keyMap[key]
  else if (key.length === 1 && /[a-z]/.test(key)) key = key.toUpperCase()
  else if (/^F(\d+)$/.test(key)) { /* F keys */ }
  else if (key.length === 1) { /* any single char */ }
  else key = key // accept as-is
  return { mods, key }
}

onMounted(load)
</script>

<template>
  <div class="pref">
    <div class="pref-titlebar drag-region">
      <SettingsIcon :size="14" class="titlebar-icon" />
      <span class="titlebar-text">{{ t('settings') }}</span>
      <button class="titlebar-close" @click="emit('close')" :title="t('closeSettings')">
        <X :size="16" />
      </button>
    </div>

    <div class="pref-body">
      <aside class="pref-nav">
        <nav class="nav-list">
          <button v-for="item in navItems" :key="item.key"
            class="nav-item" :class="{ active: activeTab === item.key }"
            @click="activeTab = item.key">
            <component :is="item.icon" :size="16" />
            <span>{{ item.label }}</span>
          </button>
        </nav>
      </aside>

      <main class="pref-content">
      <!-- 通用 -->
      <div v-if="activeTab === 'general'" class="panel">
        <div class="section-title">{{ t('appSettings') }}</div>
        <div class="section-card">
          <div class="card-row">
            <span>{{ t('launchOnLogin') }}</span>
            <label class="toggle"><input type="checkbox" v-model="form.autoStart" /><span class="slider"></span></label>
          </div>
          <div class="card-row">
            <div>
              <span>{{ t('silentStart') }}</span>
              <p class="desc-inline">{{ t('silentStartDesc') }}</p>
            </div>
            <label class="toggle"><input type="checkbox" v-model="form.silentStart" /><span class="slider"></span></label>
          </div>
          <div class="card-row">
            <div>
              <span>{{ t('showTrayIcon') }}</span>
            </div>
            <label class="toggle"><input type="checkbox" v-model="form.showTrayIcon" /><span class="slider"></span></label>
          </div>
          <div class="card-row">
            <div>
              <span>{{ t('showTaskbarIcon') }}</span>
            </div>
            <label class="toggle"><input type="checkbox" v-model="form.showTaskbarIcon" /><span class="slider"></span></label>
          </div>
        </div>

        <div class="section-title">{{ t('appearSettings') }}</div>
        <div class="section-card">
          <div class="card-row">
            <span>{{ t('language') }}</span>
            <div class="seg-ctrl seg-sm">
              <button :class="{ active: form.language === 'zh' }" @click="setLang('zh')">简体中文</button>
              <button :class="{ active: form.language === 'zh-TW' }" @click="setLang('zh-TW')">繁體中文</button>
              <button :class="{ active: form.language === 'en' }" @click="setLang('en')">English</button>
            </div>
          </div>
          <div class="card-row">
            <span>{{ t('theme') }}</span>
            <div class="seg-ctrl seg-sm">
              <button :class="{ active: form.theme === 'dark' }" @click="setTheme('dark')">{{ t('dark') }}</button>
              <button :class="{ active: form.theme === 'light' }" @click="setTheme('light')">{{ t('light') }}</button>
            </div>
          </div>
          <div class="card-row">
            <span>{{ t('trayIconStyle') }}</span>
            <div class="seg-ctrl seg-sm">
              <button :class="{ active: form.trayIconStyle === 'color' }" @click="form.trayIconStyle = 'color'">{{ t('trayIconColor') }}</button>
              <button :class="{ active: form.trayIconStyle === 'gray' }" @click="form.trayIconStyle = 'gray'">{{ t('trayIconGray') }}</button>
            </div>
          </div>
        </div>

        <div class="section-title">{{ t('updateSettings') }}</div>
        <div class="section-card">
          <div class="card-row">
            <span>{{ t('autoCheckUpdate') }}</span>
            <span class="badge">{{ t('comingSoon') }}</span>
          </div>
          <div class="card-row">
            <span>{{ t('checkUpdate') }}</span>
            <div class="check-update-row">
              <span class="check-update-msg" :class="{ 'has-update': updateHasNew }">
                {{ updateMsg }}
                <button v-if="updateHasNew && updateUrl" class="btn-link" @click="onOpenRelease">{{ t('download') }}</button>
              </span>
              <button class="btn-outline btn-sm" :disabled="updateChecking" @click="onCheckUpdate">
                {{ updateChecking ? t('checking') : t('checkUpdate') }}
              </button>
            </div>
          </div>
        </div>

        <span v-if="saveMsg" class="save-msg floating">{{ saveMsg }}</span>
      </div>

      <!-- 剪贴板 -->
      <div v-if="activeTab === 'clipboard'" class="panel">
        <div class="section-title">{{ t('contentSettings') }}</div>
        <div class="section-card">
          <div class="card-row">
            <span>{{ t('pasteBehavior') }}</span>
            <div class="seg-ctrl compact">
              <button :class="{ active: form.pasteTrigger === 'single' }" @click="form.pasteTrigger = 'single'">{{ t('triggerSingle') }}</button>
              <button :class="{ active: form.pasteTrigger === 'double' }" @click="form.pasteTrigger = 'double'">{{ t('triggerDouble') }}</button>
            </div>
          </div>
        </div>

        <div class="section-title">{{ t('windowSettings') }}</div>
        <div class="section-card">
          <div class="card-row">
            <div>
              <span>{{ t('windowPosition') }}</span>
            </div>
            <div class="seg-ctrl compact">
              <button :class="{ active: form.windowPosition === 'follow' }" @click="form.windowPosition = 'follow'">{{ t('wpFollow') }}</button>
              <button :class="{ active: form.windowPosition === 'remember' }" @click="form.windowPosition = 'remember'">{{ t('wpRemember') }}</button>
              <button :class="{ active: form.windowPosition === 'center' }" @click="form.windowPosition = 'center'">{{ t('wpCenter') }}</button>
            </div>
          </div>
          <div class="card-row">
            <div>
              <span>{{ t('scrollTopOnShow') }}</span>
              <p class="desc-inline">{{ t('scrollTopOnShowDesc') }}</p>
            </div>
            <label class="toggle"><input type="checkbox" v-model="form.scrollTopOnShow" /><span class="slider"></span></label>
          </div>
          <div class="card-row">
            <span>{{ t('resetFilterOnShow') }}</span>
            <label class="toggle"><input type="checkbox" v-model="form.resetFilterOnShow" /><span class="slider"></span></label>
          </div>
          <div class="card-row">
            <span>{{ t('clearSearchOnShow') }}</span>
            <label class="toggle"><input type="checkbox" v-model="form.clearSearchOnShow" /><span class="slider"></span></label>
          </div>
        </div>

        <span v-if="saveMsg" class="save-msg floating">{{ saveMsg }}</span>
      </div>

      <!-- 快捷键 -->
      <div v-if="activeTab === 'shortcut'" class="panel">
        <div class="section-title">{{ t('shortcutTitle') }}</div>
        <div class="section-card">
          <div class="card-row">
            <div>
              <span>{{ t('togglePanel') }}</span>
              <p class="desc-inline">{{ t('shortcutDesc') }}</p>
            </div>
            <div class="hotkey-recorder" :class="{ recording }"
              tabindex="0" @click="startRecording" @keydown="recording && onRecordKey($event)"
              @blur="recording = false">
              <span v-if="recording" class="rec-hint">{{ t('pressCombo') }}</span>
              <span v-else class="hotkey-display">{{ hotkeyDisplay }}</span>
            </div>
          </div>
        </div>

        <div class="section-title">{{ t('switchTabTitle') }}</div>
        <div class="section-card">
          <div class="card-row">
            <div>
              <span>{{ t('tabHotkeysEnabled') }}</span>
              <p class="desc-inline">{{ t('switchTabDesc') }}</p>
            </div>
            <label class="toggle"><input type="checkbox" v-model="form.tabHotkeysEnabled" /><span class="slider"></span></label>
          </div>
          <div v-for="s in tabShortcuts" :key="s.label" class="card-row" :class="{ disabled: !form.tabHotkeysEnabled }">
            <div class="tab-shortcut-label">
              <component :is="s.icon" :size="16" />
              <span>{{ s.label }}</span>
            </div>
            <div class="kbd-group">
              <template v-for="(k, i) in s.keys" :key="k">
                <span v-if="i > 0" class="kbd-sep">/</span>
                <kbd class="kbd">{{ k }}</kbd>
              </template>
            </div>
          </div>
        </div>

        <!-- 应用内快捷键：仅在面板内生效，不参与全局注册，无需开关。
             视觉上与上方"全局/分类"快捷键区分：左侧纯文字标签（无图标），右侧 kbd 徽章风格。 -->
        <div class="section-title">{{ t('appShortcutsTitle') }}</div>
        <div class="section-card">
          <div class="card-row stack">
            <p class="desc-inline">{{ t('appShortcutsDesc') }}</p>
          </div>
          <div v-for="s in appShortcuts" :key="s.label" class="card-row">
            <span class="app-shortcut-label">{{ s.label }}</span>
            <div class="kbd-group">
              <template v-for="(k, i) in s.keys" :key="i">
                <span v-if="i > 0 && (s.sep ?? '/') !== ''" class="kbd-sep">{{ s.sep ?? '/' }}</span>
                <kbd class="kbd">{{ k }}</kbd>
              </template>
            </div>
          </div>
        </div>

        <span v-if="saveMsg" class="save-msg floating">{{ saveMsg }}</span>
      </div>

      <!-- 数据管理 -->
      <div v-if="activeTab === 'backup'" class="panel">
        <div class="section-title">{{ t('dataSecurity') }}</div>
        <div class="section-card">
          <div class="card-row stack">
            <p class="desc-inline">{{ t('dataSecurityDesc') }}</p>
          </div>
          <div class="card-row">
            <span>{{ t('dataDir') }}</span>
            <code class="path">{{ dataDir }}</code>
          </div>
        </div>

        <div class="section-title">{{ t('storageSettings') }}</div>
        <div class="section-card">
          <div class="card-row">
            <span>{{ t('maxItems') }}</span>
            <div class="input-group">
              <input
                type="number" min="0" v-model="form.maxItems"
                @blur="onNumberBlur('maxItems')"
                @keydown.enter.prevent="($event.target as HTMLInputElement).blur()"
              />
              <span class="unit">{{ t('maxItemsUnit') }}</span>
            </div>
          </div>
          <div class="card-row">
            <span>{{ t('maxDays') }}</span>
            <div class="input-group">
              <input
                type="number" min="0" v-model="form.maxDays"
                @blur="onNumberBlur('maxDays')"
                @keydown.enter.prevent="($event.target as HTMLInputElement).blur()"
              />
              <span class="unit">{{ t('maxDaysUnit') }}</span>
            </div>
          </div>
        </div>

        <div class="section-title">{{ t('importExport') }}</div>
        <div class="section-card">
          <div class="card-row">
            <div>
              <span>{{ t('exportData') }}</span>
              <p class="desc-inline">{{ t('exportDesc') }}</p>
            </div>
            <button class="btn-outline" @click="doExport">
              <Download :size="14" />
              {{ t('exportJson') }}
            </button>
          </div>
          <div class="card-row disabled">
            <div>
              <span>{{ t('importData') }}</span>
              <p class="desc-inline">{{ t('importDesc') }}</p>
            </div>
            <span class="badge">{{ t('comingSoon') }}</span>
          </div>
        </div>

        <div class="danger-zone">
          <button class="btn-danger" @click="doClear">
            <Trash2 :size="14" />
            {{ t('clearUnfav') }}
          </button>
        </div>

        <span v-if="saveMsg" class="save-msg floating">{{ saveMsg }}</span>
      </div>

      <!-- 扩展功能：包含跨平台的 Emoji 区块和仅 macOS 的 Cmd+Q 区块。 -->
      <div v-if="activeTab === 'extensions'" class="panel">
        <!-- Emoji 总开关 + 显示完整表情库（所有平台） -->
        <div class="section-title">
          <span>{{ t('extEmojiTitle') }}</span>
        </div>
        <div class="section-card">
          <div class="card-row">
            <div>
              <span>{{ t('extEmojiEnabled') }}</span>
              <p class="desc-inline">{{ t('extEmojiEnabledDesc') }}</p>
            </div>
            <label class="toggle"><input type="checkbox" v-model="form.emojiEnabled" /><span class="slider"></span></label>
          </div>
          <div class="card-row" :class="{ disabled: !form.emojiEnabled }">
            <div>
              <span>{{ t('extEmojiFull') }}</span>
              <p class="desc-inline">{{ t('extEmojiFullDesc') }}</p>
            </div>
            <label class="toggle"><input type="checkbox" v-model="form.extendedEmoji" /><span class="slider"></span></label>
          </div>
        </div>

        <!-- Cmd+Q 防误触（仅 macOS，非 mac 平台行级隐藏整段） -->
        <template v-if="isMacPlatform">
          <div class="section-title">{{ t('cmdQSection') }}</div>
          <div class="section-card">
            <div class="card-row stack">
              <p class="desc-inline">{{ t('cmdQDesc') }}</p>
            </div>
            <div class="card-row">
              <div>
                <span>{{ t('cmdQBehavior') }}</span>
                <p class="desc-inline">
                  <template v-if="form.cmdQBehavior === 'default'">{{ t('cmdQDefaultDesc') }}</template>
                  <template v-else-if="form.cmdQBehavior === 'confirm'">{{ t('cmdQConfirmDesc') }}</template>
                  <template v-else>{{ t('cmdQDisableDesc') }}</template>
                </p>
              </div>
              <div class="seg-ctrl seg-sm">
                <button :class="{ active: form.cmdQBehavior === 'default' }" @click="form.cmdQBehavior = 'default'">{{ t('cmdQDefault') }}</button>
                <button :class="{ active: form.cmdQBehavior === 'confirm' }" @click="form.cmdQBehavior = 'confirm'">{{ t('cmdQConfirm') }}</button>
                <button :class="{ active: form.cmdQBehavior === 'disable' }" @click="form.cmdQBehavior = 'disable'">{{ t('cmdQDisable') }}</button>
              </div>
            </div>
            <!-- 仅在 "confirm" 模式下展示时间窗输入，避免迷惑 -->
            <div v-if="form.cmdQBehavior === 'confirm'" class="card-row">
              <span>{{ t('cmdQConfirmWindow') }}</span>
              <div class="input-group">
                <input
                  type="number" min="300" max="10000" step="100"
                  v-model="form.cmdQConfirmWindow"
                  @blur="onCmdQWindowBlur"
                  @keydown.enter.prevent="($event.target as HTMLInputElement).blur()"
                />
                <span class="unit">{{ t('cmdQConfirmWindowUnit') }}</span>
              </div>
            </div>
          </div>
        </template>

        <span v-if="saveMsg" class="save-msg floating">{{ saveMsg }}</span>
      </div>

      <!-- 关于 -->
      <div v-if="activeTab === 'about'" class="panel">
        <div class="about-card">
          <img class="about-logo" src="/appicon.png" />
          <div class="about-info">
            <div class="about-name">GoPaste</div>
            <div class="about-ver">v{{ appVersion }}</div>
          </div>
        </div>
        <div class="about-desc">
          <p>{{ t('aboutDesc1') }}</p>
          <p>{{ t('aboutDesc2') }}</p>
          <p>{{ t('aboutDesc3') }}</p>
        </div>
        <div class="about-links">
          <div class="link-row"><span class="link-label">{{ t('techStack') }}</span><span>Go · Vue 3 · Wails · SQLite</span></div>
          <div class="link-row"><span class="link-label">{{ t('website') }}</span><a class="about-link" :href="websiteUrl" target="_blank" rel="noopener noreferrer">{{ websiteUrl.replace(/^https?:\/\//, '').replace(/\/$/, '') }}</a></div>
          <div class="link-row"><span class="link-label">{{ t('license') }}</span><span>Apache-2.0</span></div>
        </div>
      </div>
    </main>
    </div>

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
  </div>
</template>

<style scoped>
.pref {
  display: flex; flex-direction: column; height: 100%; background: var(--bg);
}

.pref-titlebar {
  display: flex; align-items: center; gap: 8px;
  height: 36px; padding: 0 12px;
  background: var(--bg-sidebar);
  border-bottom: 1px solid var(--bg-elevated);
  flex-shrink: 0; user-select: none;
}
/* 原先 Mac 下给红黄绿按钮预留 80px 并隐藏自定义关闭按钮；
   traffic lights 已统一隐藏，关闭按钮在所有平台一致显示。 */
.titlebar-icon { color: var(--text-muted); }
.titlebar-text { font-size: 13px; color: var(--text-secondary); flex: 1; }
.titlebar-close {
  width: 28px; height: 28px; border-radius: 6px;
  background: transparent; border: none; color: var(--text-secondary);
  cursor: pointer; display: flex; align-items: center; justify-content: center;
  transition: all .15s;
  --wails-draggable: no-drag; -webkit-app-region: no-drag;
}
.titlebar-close:hover { background: #e81123; color: #fff; }

.pref-body { display: flex; flex: 1; overflow: hidden; }

.pref-nav {
  width: 120px; flex-shrink: 0;
  background: var(--bg-sidebar);
  border-right: 1px solid var(--bg-elevated);
  padding: 10px 0; overflow-y: auto;
}
.nav-list { display: flex; flex-direction: column; gap: 2px; padding: 0 4px; }
.nav-item {
  display: flex; align-items: center; gap: 6px;
  background: transparent; border: none; color: var(--text-secondary);
  padding: 8px 10px; border-radius: 6px; cursor: pointer;
  font-size: 13px; line-height: 20px; text-align: left; transition: all .12s;
  --wails-draggable: no-drag; -webkit-app-region: no-drag;
}
.nav-item:hover { background: var(--bg-elevated); color: var(--text); }
.nav-item.active { background: var(--accent); color: #fff; }

.pref-content {
  flex: 1; overflow-y: auto; padding: 16px 14px; text-align: left;
}
.panel h2 {
  font-size: 16px; font-weight: 600; color: var(--text);
  margin: 0 0 20px; padding-bottom: 10px;
  border-bottom: 1px solid var(--bg-elevated);
}

.field { margin-bottom: 18px; }
.field label {
  display: flex; align-items: center; gap: 6px;
  font-size: 13px; color: var(--text-secondary); margin-bottom: 6px; font-weight: 500;
}
.field select {
  background: var(--bg-elevated); border: 1px solid var(--border); color: var(--text);
  padding: 6px 10px; border-radius: 6px; font-size: 13px; outline: none;
}
.field select:focus { border-color: var(--accent); }
.desc { font-size: 12px; color: var(--text-muted); margin: 4px 0 8px; line-height: 1.5; text-align: left; }
.path { font-size: 12px; background: var(--bg-elevated); padding: 5px 10px; border-radius: 4px; color: var(--text-secondary); display: inline-block; text-align: left; }

.input-group { display: flex; align-items: center; gap: 8px; }
.input-group input {
  width: 100px; background: var(--bg-elevated); border: 1px solid var(--border);
  color: var(--text); padding: 6px 10px; border-radius: 6px; font-size: 13px; outline: none;
}
.input-group input:focus { border-color: var(--accent); }
.unit { font-size: 12px; color: var(--text-muted); }

.check {
  display: flex; align-items: center; gap: 8px;
  margin-bottom: 10px; font-size: 13px; color: var(--text); cursor: pointer;
}
.check input { cursor: pointer; }

.btn-primary {
  display: inline-flex; align-items: center; gap: 6px;
  background: var(--accent); border: none; color: #fff;
  padding: 8px 18px; border-radius: 6px; cursor: pointer;
  font-size: 13px; margin-top: 10px; transition: background .15s;
}
.btn-primary:hover { background: var(--accent-hover); }
.btn-primary:disabled { opacity: .6; cursor: not-allowed; }

.btn-outline {
  display: inline-flex; align-items: center; gap: 6px;
  background: transparent; border: 1px solid var(--border); color: var(--text);
  padding: 7px 14px; border-radius: 6px; cursor: pointer; font-size: 13px;
  white-space: nowrap; flex-shrink: 0;
}
.btn-outline:hover { background: var(--bg-elevated); }
.btn-outline.btn-sm { padding: 4px 10px; font-size: 12px; border-radius: 5px; }
.check-update-row { display: flex; align-items: center; gap: 8px; }
.check-update-msg { font-size: 12px; color: var(--text-secondary); min-width: 80px; text-align: right; white-space: nowrap; }
.check-update-msg.has-update { color: var(--warning); font-weight: 500; }

.btn-danger {
  display: inline-flex; align-items: center; gap: 6px;
  background: transparent; border: 1px solid var(--danger); color: var(--danger);
  padding: 7px 14px; border-radius: 6px; cursor: pointer; font-size: 13px;
}
.btn-danger:hover { background: rgba(239,68,68,.1); }

.save-msg { font-size: 12px; color: var(--success); margin-left: 10px; }
.save-msg.floating {
  position: fixed; right: 18px; bottom: 14px;
  background: var(--bg-elevated); border: 1px solid var(--border);
  padding: 6px 12px; border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0,0,0,.12);
  margin-left: 0; z-index: 20;
  animation: save-pop .18s ease-out;
}
@keyframes save-pop {
  from { opacity: 0; transform: translateY(4px); }
  to   { opacity: 1; transform: translateY(0); }
}

/* 可编辑快捷键录制框：用 accent 蓝色边框 + 蓝字，明确"可点击修改"。
   与下方只读快捷键（黑字灰边）形成视觉区分。
   注：默认蓝态与录制橙态都使用 2px 边框；为抵消蓝色饱和度低于橙色带来的"边框看起来偏细"
   的视觉错觉，额外叠一层同色 inset shadow，让两态感知粗度一致。 */
.hotkey-recorder {
  display: inline-flex; align-items: center; justify-content: center; gap: 10px;
  /* 固定宽高（非 min-*），保证三态尺寸完全一致：
     - 默认态 "Alt + `"（短）
     - 录制态 "Press shortcut..."（长，且 .rec-hint 带 pulse 动画）
     - 长组合态 "Ctrl + Shift + Alt + V"（更长）
     宽 160px 容下常见提示文案与 4 键组合；高 40px 与下方 toggle 开关行视觉等高，
     防止点击后内容微小行高差导致整页内容跳动。
     box-sizing: border-box 保证 2px 边框不会让总尺寸变成 164×44。 */
  width: 160px;
  height: 40px;
  box-sizing: border-box;
  padding: 0 14px; margin-top: 4px;
  background: var(--bg-elevated);
  border: 2px solid var(--accent);
  box-shadow: inset 0 0 0 0.5px var(--accent);
  border-radius: 8px; cursor: pointer; transition: all .2s; outline: none;
  user-select: none;
}
.hotkey-recorder:hover {
  background: color-mix(in srgb, var(--accent) 8%, var(--bg-elevated));
  border-color: var(--accent-hover);
  box-shadow: inset 0 0 0 0.5px var(--accent-hover);
}
.hotkey-recorder.recording {
  border-color: var(--warning);
  background: var(--bg-elevated);
  box-shadow: inset 0 0 0 0.5px var(--warning);
}
.hotkey-recorder.recording .hotkey-display { color: var(--warning); }
.hotkey-display {
  font-size: 12px; font-weight: 600;
  color: var(--accent);
  font-family: 'SF Mono', 'Consolas', monospace; letter-spacing: 1px;
  white-space: nowrap;
}
.tab-shortcut-label {
  display: flex; align-items: center; gap: 8px;
  font-size: 13px; color: var(--text);
}

/* 只读快捷键徽章：用于「切换分类（Alt+1..6）」「应用内快捷键」等所有不可修改的展示。
   与可编辑的 .hotkey-recorder（蓝边蓝字）形成对比，统一弱化为浅色 kbd 风格。
   - 单键：小圆角 + 浅边框 + 双层下边框模拟实体键
   - 多键：以 / 分隔，紧凑居右 */
.app-shortcut-label {
  font-size: 13px; color: var(--text-muted);
  /* 允许文案在键帽组占用空间后自适应换行，避免把键帽挤到下一行 */
  flex: 1 1 auto; min-width: 0; line-height: 1.5;
}
.kbd-group {
  display: inline-flex; align-items: center; gap: 6px;
  /* 键帽组作为整体不收缩、不换行——宁可让左侧文案换行，也要保证多个键帽横排 */
  flex: 0 0 auto; flex-wrap: nowrap; justify-content: flex-end;
}
.kbd {
  display: inline-block;
  min-width: 22px;
  padding: 2px 8px;
  font-size: 11px; font-weight: 600;
  font-family: 'SF Mono', 'Consolas', monospace;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--border);
  border-bottom-width: 2px;
  border-radius: 4px;
  line-height: 1.4;
  text-align: center;
  white-space: nowrap;
}
.kbd-sep { font-size: 11px; color: var(--text-muted); user-select: none; }
.rec-hint { font-size: 12px; color: var(--warning); animation: pulse 1s infinite; white-space: nowrap; }
.rec-label { font-size: 11px; color: var(--text-muted); margin-left: auto; }
@keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: .5; } }

.about-card {
  display: flex; align-items: center; gap: 14px;
  padding: 16px; background: var(--bg-elevated); border-radius: 10px; margin-bottom: 16px;
}
.about-logo {
  width: 48px; height: 48px; border-radius: 12px;
  object-fit: cover; flex-shrink: 0;
}
.about-info { text-align: left; }
.about-name { font-size: 16px; font-weight: 600; color: var(--text); }
.about-ver { font-size: 12px; color: var(--text-muted); margin-top: 2px; }
.about-desc { font-size: 13px; color: var(--text-secondary); line-height: 1.6; margin-bottom: 16px; }
.about-desc p { margin: 4px 0; }
.about-links { border-top: 1px solid var(--bg-elevated); padding-top: 12px; }
.about-link { color: var(--accent); text-decoration: none; }
.about-link:hover { text-decoration: underline; }
.about-update {
  margin-top: 16px; padding-top: 12px;
  border-top: 1px solid var(--bg-elevated);
  display: flex; align-items: center; justify-content: center; gap: 10px; flex-wrap: wrap;
}
.about-update .btn-outline { min-width: 100px; justify-content: center; }
.update-msg { font-size: 12px; color: var(--text-secondary); display: inline-flex; align-items: center; gap: 8px; }
.update-msg.has-update { color: var(--warning); font-weight: 500; }
.btn-link {
  background: none; border: none; padding: 0;
  color: var(--accent); cursor: pointer; font-size: 12px; text-decoration: underline;
}
.btn-link:hover { color: var(--accent-hover); }
.link-row {
  display: flex; gap: 12px; padding: 6px 0; font-size: 13px;
}
.link-label { color: var(--text-muted); min-width: 70px; }
.link-row span:last-child, .link-row code { color: var(--text-secondary); }
.link-row code { font-size: 12px; background: var(--bg-elevated); padding: 2px 6px; border-radius: 3px; }

.seg-ctrl {
  display: inline-flex; flex-shrink: 0; border-radius: 8px; overflow: hidden;
  border: 1px solid var(--border); background: var(--bg); white-space: nowrap;
}
.seg-ctrl button {
  padding: 6px 16px; font-size: 13px; white-space: nowrap;
  background: transparent; border: none; color: var(--text-secondary);
  cursor: pointer; transition: all .15s;
}
.seg-ctrl button:not(:last-child) { border-right: 1px solid var(--border); }
.seg-ctrl button.active { background: var(--accent); color: #fff; }
.seg-ctrl button:hover:not(.active) { background: var(--bg-hover); }
.seg-sm button { padding: 4px 10px; font-size: 12px; }
.seg-ctrl.compact button { padding: 4px 10px; font-size: 12px; }

/* 区块标题 */
.section-title {
  font-size: 13px; font-weight: 600; color: var(--accent);
  margin: 16px 0 8px; padding: 0;
  display: flex; align-items: center; gap: 6px;
}
.section-title-icon { flex-shrink: 0; }
.section-title:first-child { margin-top: 0; }

/* 卡片容器 */
.section-card {
  background: var(--bg-elevated); border-radius: 10px;
  border: 1px solid var(--border-light); overflow: hidden;
  margin-bottom: 6px;
}
.card-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 14px; font-size: 13px; color: var(--text);
  border-bottom: 1px solid var(--border-light); gap: 8px;
}
.card-row:last-child { border-bottom: none; }
.card-row.disabled { opacity: .45; pointer-events: none; }
.card-row.stack { display: block; }
.card-row.end { justify-content: flex-end; }
.danger-zone { display: flex; justify-content: center; margin-top: 18px; }
.card-row.stack .desc-inline { font-size: 12px; color: var(--text-secondary); line-height: 1.6; }
.card-row > div:not([class]) { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.card-row .path {
  flex: 1; min-width: 0; margin-left: 10px; text-align: right;
  white-space: nowrap; overflow-x: auto; overflow-y: hidden;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.desc-inline { font-size: 11px; color: var(--text-muted); margin: 0; }

/* Badge */
.badge {
  font-size: 10px; padding: 2px 8px; border-radius: 10px;
  background: var(--border); color: var(--text-muted);
}

/* Toggle switch */
.toggle { position: relative; width: 40px; height: 22px; flex-shrink: 0; }
.toggle input { opacity: 0; width: 0; height: 0; }
.toggle .slider {
  position: absolute; inset: 0; cursor: pointer;
  background: var(--border); border-radius: 11px; transition: .2s;
}
.toggle .slider::before {
  content: ''; position: absolute; width: 16px; height: 16px;
  left: 3px; bottom: 3px; background: #fff; border-radius: 50%; transition: .2s;
}
.toggle input:checked + .slider { background: var(--accent); }
.toggle input:checked + .slider::before { transform: translateX(18px); }

.drag-region { --wails-draggable: drag; -webkit-app-region: drag; }
.drag-region button { --wails-draggable: no-drag; -webkit-app-region: no-drag; }

/* 自定义确认弹窗（样式与 App.vue 对齐，保持跨视图一致） */
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

.fade-enter-active, .fade-leave-active { transition: opacity .15s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
