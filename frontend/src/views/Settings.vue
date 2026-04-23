<script lang="ts" setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  GetSettings,
  UpdateSettings,
  ExportData,
  ClearHistory,
  DataDir,
  TrayNeedsRestart,
} from '../../wailsjs/go/main/App'
import {
  Settings as SettingsIcon, Keyboard, ClipboardList,
  Download, Trash2, Info, Database, X,
} from 'lucide-vue-next'
import { t, lang } from '../i18n'
import type { Lang } from '../i18n'

const emit = defineEmits<{ (e: 'close'): void }>()

type Tab = 'general' | 'clipboard' | 'shortcut' | 'backup' | 'about'
const activeTab = ref<Tab>('general')

const navItems = computed(() => [
  { key: 'general' as Tab, label: t('navGeneral'), icon: SettingsIcon },
  { key: 'clipboard' as Tab, label: t('navClipboard'), icon: ClipboardList },
  { key: 'shortcut' as Tab, label: t('navShortcut'), icon: Keyboard },
  { key: 'backup' as Tab, label: t('navBackup'), icon: Database },
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
  silentStart: false,
  showTrayIcon: true,
  showTaskbarIcon: false,
})

const dataDir = ref('')
const saveMsg = ref('')
const recording = ref(false)
// 托盘图标是否需要重启才能重新显示（仅当本进程内已关过一次才需要）
const trayRestartRequired = ref(false)
// 标记初次 load 完成前不触发自动保存
const loaded = ref(false)

const hotkeyDisplay = computed(() => {
  const parts = [...form.hotkeyModifiers.map(m => m.charAt(0).toUpperCase() + m.slice(1))]
  parts.push(form.hotkeyKey)
  return parts.join(' + ')
})

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
  form.silentStart = !!s.silentStart
  form.showTrayIcon = s.showTrayIcon !== false
  form.showTaskbarIcon = !!s.showTaskbarIcon
  dataDir.value = await DataDir()
  try { trayRestartRequired.value = await TrayNeedsRestart() } catch {}
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
      silentStart: form.silentStart,
      showTrayIcon: form.showTrayIcon,
      showTaskbarIcon: form.showTaskbarIcon,
    } as any)
    saveMsg.value = t('saved')
    setTimeout(() => { saveMsg.value = '' }, 1500)
    // 更新托盘"需要重启"标志：关闭后进程内无法再开启，需用户知晓
    try { trayRestartRequired.value = await TrayNeedsRestart() } catch {}
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

// 所有离散字段：变更即写盘
watchImmediate(() => form.theme)
watchImmediate(() => form.language)
watchImmediate(() => form.pasteTrigger)
watchImmediate(() => form.windowPosition)
watchImmediate(() => form.scrollTopOnShow)
watchImmediate(() => form.resetFilterOnShow)
watchImmediate(() => form.silentStart)
watchImmediate(() => form.showTrayIcon)
watchImmediate(() => form.showTaskbarIcon)
watchImmediate(() => form.hotkeyKey)
watchImmediate(() => form.hotkeyModifiers.join('+'))
// 注意：form.maxItems / form.maxDays 不使用 watch，改为输入框 blur/回车时触发保存

async function doExport() {
  const json = await ExportData()
  const blob = new Blob([json], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `GoPaste-export-${Date.now()}.json`
  a.click()
  URL.revokeObjectURL(url)
}

async function doClear() {
  if (!confirm(t('clearConfirm'))) return
  await ClearHistory()
  saveMsg.value = t('cleared')
  setTimeout(() => (saveMsg.value = ''), 2000)
}

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
          <div class="card-row disabled">
            <span>{{ t('launchOnLogin') }}</span>
            <span class="badge">{{ t('comingSoon') }}</span>
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
              <p v-if="trayRestartRequired && form.showTrayIcon" class="desc-inline">{{ t('restartRequired') }}</p>
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
        </div>

        <div class="section-title">{{ t('updateSettings') }}</div>
        <div class="section-card">
          <div class="card-row disabled">
            <span>{{ t('autoCheckUpdate') }}</span>
            <span class="badge">{{ t('comingSoon') }}</span>
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
              <Keyboard :size="16" />
              <span v-if="recording" class="rec-hint">{{ t('pressCombo') }}</span>
              <span v-else class="hotkey-display">{{ hotkeyDisplay }}</span>
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

      <!-- 关于 -->
      <div v-if="activeTab === 'about'" class="panel">
        <div class="about-card">
          <img class="about-logo" src="/appicon.png" />
          <div class="about-info">
            <div class="about-name">GoPaste</div>
            <div class="about-ver">v0.1.0</div>
          </div>
        </div>
        <div class="about-desc">
          <p>{{ t('aboutDesc1') }}</p>
          <p>{{ t('aboutDesc2') }}</p>
          <p>{{ t('aboutDesc3') }}</p>
        </div>
        <div class="about-links">
          <div class="link-row"><span class="link-label">{{ t('techStack') }}</span><span>Go · Vue 3 · Wails · SQLite</span></div>
          <div class="link-row"><span class="link-label">{{ t('license') }}</span><span>MIT</span></div>
        </div>
      </div>
    </main>
    </div>
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

.hotkey-recorder {
  display: flex; align-items: center; gap: 10px;
  padding: 10px 14px; margin-top: 4px;
  background: var(--bg-elevated); border: 2px solid var(--border);
  border-radius: 8px; cursor: pointer; transition: all .2s; outline: none;
  user-select: none;
}
.hotkey-recorder:hover { border-color: var(--accent); }
.hotkey-recorder.recording { border-color: var(--warning); background: var(--bg-elevated); }
.hotkey-display {
  font-size: 12px; font-weight: 600; color: var(--text);
  font-family: 'SF Mono', 'Consolas', monospace; letter-spacing: 1px;
  white-space: nowrap;
}
.rec-hint { font-size: 14px; color: var(--warning); animation: pulse 1s infinite; }
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
}
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
  flex: 1; min-width: 0; margin-left: 12px; text-align: right;
  word-break: break-all;
  font-size: 10px;
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
</style>
