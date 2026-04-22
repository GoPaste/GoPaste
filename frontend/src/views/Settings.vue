<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  GetSettings,
  UpdateSettings,
  ExportData,
  ClearHistory,
  DataDir,
} from '../../wailsjs/go/main/App'
import {
  ArrowLeft, Settings as SettingsIcon, Keyboard, ClipboardList,
  Clock, Download, Trash2, Check, Info, Database, Shield,
  Monitor, MousePointer, RotateCcw, FolderOpen, X, Globe,
} from 'lucide-vue-next'
import { t, lang } from '../i18n'
import type { Lang } from '../i18n'

const emit = defineEmits<{ (e: 'close'): void }>()

type Tab = 'general' | 'clipboard' | 'shortcut' | 'history' | 'backup' | 'about'
const activeTab = ref<Tab>('general')

const navItems = computed(() => [
  { key: 'general' as Tab, label: t('navGeneral'), icon: SettingsIcon },
  { key: 'clipboard' as Tab, label: t('navClipboard'), icon: ClipboardList },
  { key: 'shortcut' as Tab, label: t('navShortcut'), icon: Keyboard },
  { key: 'history' as Tab, label: t('navHistory'), icon: Clock },
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
  autoPaste: true,
  hideOnPaste: true,
})

const dataDir = ref('')
const saving = ref(false)
const saveMsg = ref('')
const recording = ref(false)

const hotkeyDisplay = computed(() => {
  const parts = [...form.hotkeyModifiers.map(m => m.charAt(0).toUpperCase() + m.slice(1))]
  parts.push(form.hotkeyKey)
  return parts.join(' + ')
})

function onLangChange(e: Event) {
  const v = (e.target as HTMLSelectElement).value as Lang
  form.language = v
  lang.value = v // 实时生效
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
  form.autoPaste = !!s.autoPaste
  form.hideOnPaste = !!s.hideOnPaste
  dataDir.value = await DataDir()
}

async function save() {
  saving.value = true
  saveMsg.value = ''
  try {
    await UpdateSettings({
      hotkeyModifiers: form.hotkeyModifiers,
      hotkeyKey: form.hotkeyKey,
      maxItems: Number(form.maxItems),
      maxDays: Number(form.maxDays),
      theme: form.theme,
      language: form.language,
      autoPaste: form.autoPaste,
      hideOnPaste: form.hideOnPaste,
    } as any)
    saveMsg.value = t('saved')
    setTimeout(() => (saveMsg.value = ''), 2000)
  } catch (e: any) {
    saveMsg.value = t('saveFailed') + (e?.message || e)
  } finally {
    saving.value = false
  }
}

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
    'Backspace': 'Delete', 'Delete': 'Delete', 'Tab': 'Tab',
  }
  if (keyMap[key]) key = keyMap[key]
  else if (key.length === 1 && /[a-z]/.test(key)) key = key.toUpperCase()
  else if (/^F(\d+)$/.test(key)) { /* F keys ok */ }
  else if (key.length === 1 && /[A-Z0-9]/.test(key)) { /* ok */ }
  else return null
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
        <h2>{{ t('generalTitle') }}</h2>
        <div class="field">
          <label><Globe :size="14" /> {{ t('language') }}</label>
          <select :value="form.language" @change="onLangChange">
            <option value="zh">中文</option>
            <option value="en">English</option>
          </select>
        </div>
        <div class="field">
          <label><Monitor :size="14" /> {{ t('theme') }}</label>
          <div class="seg-ctrl">
            <button :class="{ active: form.theme === 'dark' }" @click="form.theme = 'dark'">{{ t('dark') }}</button>
            <button :class="{ active: form.theme === 'light' }" @click="form.theme = 'light'">{{ t('light') }}</button>
          </div>
        </div>
        <div class="field">
          <label><MousePointer :size="14" /> {{ t('pasteBehavior') }}</label>
        </div>
        <label class="check">
          <input type="checkbox" v-model="form.autoPaste" />
          {{ t('autoPasteLabel') }}
        </label>
        <label class="check">
          <input type="checkbox" v-model="form.hideOnPaste" />
          {{ t('hideOnPasteLabel') }}
        </label>

        <button class="btn-primary" :disabled="saving" @click="save">
          <Check :size="14" />
          {{ saving ? t('saving') : t('saveSettings') }}
        </button>
        <span v-if="saveMsg" class="save-msg">{{ saveMsg }}</span>
      </div>

      <!-- 剪贴板 -->
      <div v-if="activeTab === 'clipboard'" class="panel">
        <h2>{{ t('clipboardTitle') }}</h2>
        <div class="field">
          <label><Shield :size="14" /> {{ t('dataSecurity') }}</label>
          <p class="desc">{{ t('dataSecurityDesc') }}</p>
        </div>
        <div class="field">
          <label><FolderOpen :size="14" /> {{ t('dataDir') }}</label>
          <code class="path">{{ dataDir }}</code>
        </div>
      </div>

      <!-- 快捷键 -->
      <div v-if="activeTab === 'shortcut'" class="panel">
        <h2>{{ t('shortcutTitle') }}</h2>
        <div class="field">
          <label><Keyboard :size="14" /> {{ t('togglePanel') }}</label>
          <div class="hotkey-recorder" :class="{ recording }"
            tabindex="0" @click="startRecording" @keydown="recording && onRecordKey($event)">
            <Keyboard :size="16" />
            <span v-if="recording" class="rec-hint">{{ t('pressCombo') }}</span>
            <span v-else class="hotkey-display">{{ hotkeyDisplay }}</span>
            <span v-if="!recording" class="rec-label">{{ t('clickToModify') }}</span>
          </div>
          <p class="desc" style="white-space:pre-line">{{ t('shortcutDesc') }}</p>
        </div>

        <button class="btn-primary" :disabled="saving" @click="save">
          <Check :size="14" />
          {{ saving ? t('saving') : t('saveShortcut') }}
        </button>
        <span v-if="saveMsg" class="save-msg">{{ saveMsg }}</span>
      </div>

      <!-- 历史记录 -->
      <div v-if="activeTab === 'history'" class="panel">
        <h2>{{ t('historyTitle') }}</h2>
        <div class="field">
          <label>{{ t('maxItems') }}</label>
          <div class="input-group">
            <input type="number" min="0" v-model="form.maxItems" />
            <span class="unit">{{ t('maxItemsUnit') }}</span>
          </div>
        </div>
        <div class="field">
          <label>{{ t('maxDays') }}</label>
          <div class="input-group">
            <input type="number" min="0" v-model="form.maxDays" />
            <span class="unit">{{ t('maxDaysUnit') }}</span>
          </div>
        </div>
        <div class="field">
          <label>{{ t('cleanData') }}</label>
          <button class="btn-danger" @click="doClear">
            <Trash2 :size="14" />
            {{ t('clearUnfav') }}
          </button>
        </div>

        <button class="btn-primary" :disabled="saving" @click="save">
          <Check :size="14" />
          {{ saving ? t('saving') : t('saveSettings') }}
        </button>
        <span v-if="saveMsg" class="save-msg">{{ saveMsg }}</span>
      </div>

      <!-- 备份 -->
      <div v-if="activeTab === 'backup'" class="panel">
        <h2>{{ t('backupTitle') }}</h2>
        <div class="field">
          <label><Download :size="14" /> {{ t('exportData') }}</label>
          <p class="desc">{{ t('exportDesc') }}</p>
          <button class="btn-outline" @click="doExport">
            <Download :size="14" />
            {{ t('exportJson') }}
          </button>
        </div>
        <div class="field">
          <label><RotateCcw :size="14" /> {{ t('importData') }}</label>
          <p class="desc">{{ t('importDesc') }}</p>
        </div>
      </div>

      <!-- 关于 -->
      <div v-if="activeTab === 'about'" class="panel">
        <h2>{{ t('aboutTitle') }}</h2>
        <div class="about-card">
          <div class="about-logo">P</div>
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
          <div class="link-row"><span class="link-label">{{ t('dataDir') }}</span><code>{{ dataDir }}</code></div>
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
  font-size: 12px; text-align: left; transition: all .12s;
  --wails-draggable: no-drag; -webkit-app-region: no-drag;
}
.nav-item:hover { background: var(--bg-elevated); color: var(--text); }
.nav-item.active { background: var(--accent); color: #fff; }

.pref-content {
  flex: 1; overflow-y: auto; padding: 16px 14px;
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
.desc { font-size: 12px; color: var(--text-muted); margin: 4px 0 8px; line-height: 1.5; }
.path { font-size: 12px; background: var(--bg-elevated); padding: 5px 10px; border-radius: 4px; color: var(--text-secondary); display: inline-block; }

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
}
.btn-outline:hover { background: var(--bg-elevated); }

.btn-danger {
  display: inline-flex; align-items: center; gap: 6px;
  background: transparent; border: 1px solid var(--danger); color: var(--danger);
  padding: 7px 14px; border-radius: 6px; cursor: pointer; font-size: 13px;
}
.btn-danger:hover { background: rgba(239,68,68,.1); }

.save-msg { font-size: 12px; color: var(--success); margin-left: 10px; }

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
  font-size: 15px; font-weight: 600; color: var(--text);
  font-family: 'SF Mono', 'Consolas', monospace; letter-spacing: 1px;
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
  background: linear-gradient(135deg, #2196F3, #00BCD4);
  display: flex; align-items: center; justify-content: center;
  font-size: 28px; font-weight: 700; color: #fff;
  font-family: -apple-system, sans-serif;
}
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
  display: inline-flex; border-radius: 8px; overflow: hidden;
  border: 1px solid var(--border); background: var(--bg);
}
.seg-ctrl button {
  padding: 6px 16px; font-size: 13px;
  background: transparent; border: none; color: var(--text-secondary);
  cursor: pointer; transition: all .15s;
}
.seg-ctrl button:not(:last-child) { border-right: 1px solid var(--border); }
.seg-ctrl button.active { background: var(--accent); color: #fff; }
.seg-ctrl button:hover:not(.active) { background: var(--bg-hover); }

.drag-region { --wails-draggable: drag; -webkit-app-region: drag; }
.drag-region button { --wails-draggable: no-drag; -webkit-app-region: no-drag; }
</style>
