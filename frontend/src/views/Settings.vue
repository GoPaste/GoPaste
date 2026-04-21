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
  Monitor, MousePointer, RotateCcw, FolderOpen, X,
} from 'lucide-vue-next'

const emit = defineEmits<{ (e: 'close'): void }>()

// 当前选中的菜单
type Tab = 'general' | 'clipboard' | 'shortcut' | 'history' | 'backup' | 'about'
const activeTab = ref<Tab>('general')

const navItems: { key: Tab; label: string; icon: any }[] = [
  { key: 'general', label: '通用', icon: SettingsIcon },
  { key: 'clipboard', label: '剪贴板', icon: ClipboardList },
  { key: 'shortcut', label: '快捷键', icon: Keyboard },
  { key: 'history', label: '历史记录', icon: Clock },
  { key: 'backup', label: '备份', icon: Database },
  { key: 'about', label: '关于', icon: Info },
]

const form = reactive({
  hotkeyModifiers: [] as string[],
  hotkeyKey: 'V',
  maxItems: 1000,
  maxDays: 30,
  theme: 'dark',
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

async function load() {
  const s: any = await GetSettings()
  form.hotkeyModifiers = [...(s.hotkeyModifiers || ['ctrl', 'shift'])]
  form.hotkeyKey = s.hotkeyKey || 'V'
  form.maxItems = s.maxItems ?? 1000
  form.maxDays = s.maxDays ?? 30
  form.theme = s.theme || 'dark'
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
      autoPaste: form.autoPaste,
      hideOnPaste: form.hideOnPaste,
    } as any)
    saveMsg.value = '已保存'
    setTimeout(() => (saveMsg.value = ''), 2000)
  } catch (e: any) {
    saveMsg.value = '保存失败: ' + (e?.message || e)
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
  a.download = `gopaste-export-${Date.now()}.json`
  a.click()
  URL.revokeObjectURL(url)
}

async function doClear() {
  if (!confirm('确定清空所有非收藏、非置顶的历史？')) return
  await ClearHistory()
  saveMsg.value = '已清空'
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
    <!-- 顶部标题栏 -->
    <div class="pref-titlebar drag-region">
      <SettingsIcon :size="14" class="titlebar-icon" />
      <span class="titlebar-text">设置</span>
      <button class="titlebar-close" @click="emit('close')" title="关闭设置">
        <X :size="16" />
      </button>
    </div>

    <div class="pref-body">
      <!-- 左侧导航 -->
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

      <!-- 右侧内容 -->
      <main class="pref-content">
      <!-- 通用 -->
      <div v-if="activeTab === 'general'" class="panel">
        <h2>通用设置</h2>
        <div class="field">
          <label><Monitor :size="14" /> 主题</label>
          <div class="seg-ctrl">
            <button :class="{ active: form.theme === 'dark' }" @click="form.theme = 'dark'">深色</button>
            <button :class="{ active: form.theme === 'light' }" @click="form.theme = 'light'">浅色</button>
          </div>
        </div>
        <div class="field">
          <label><MousePointer :size="14" /> 粘贴后行为</label>
        </div>
        <label class="check">
          <input type="checkbox" v-model="form.autoPaste" />
          选中条目后自动发送粘贴键
        </label>
        <label class="check">
          <input type="checkbox" v-model="form.hideOnPaste" />
          粘贴后自动隐藏窗口
        </label>

        <button class="btn-primary" :disabled="saving" @click="save">
          <Check :size="14" />
          {{ saving ? '保存中...' : '保存设置' }}
        </button>
        <span v-if="saveMsg" class="save-msg">{{ saveMsg }}</span>
      </div>

      <!-- 剪贴板 -->
      <div v-if="activeTab === 'clipboard'" class="panel">
        <h2>剪贴板设置</h2>
        <div class="field">
          <label><Shield :size="14" /> 数据安全</label>
          <p class="desc">所有文本内容使用 AES-256-GCM 加密存储，密钥保存在系统密钥环中。</p>
        </div>
        <div class="field">
          <label><FolderOpen :size="14" /> 数据目录</label>
          <code class="path">{{ dataDir }}</code>
        </div>
      </div>

      <!-- 快捷键 -->
      <div v-if="activeTab === 'shortcut'" class="panel">
        <h2>快捷键设置</h2>
        <div class="field">
          <label><Keyboard :size="14" /> 呼出/隐藏面板</label>
          <div class="hotkey-recorder" :class="{ recording }"
            tabindex="0" @click="startRecording" @keydown="recording && onRecordKey($event)">
            <Keyboard :size="16" />
            <span v-if="recording" class="rec-hint">请按下组合键...</span>
            <span v-else class="hotkey-display">{{ hotkeyDisplay }}</span>
            <span v-if="!recording" class="rec-label">点击修改</span>
          </div>
          <p class="desc">支持 A-Z, 0-9, F1-F12, Space, Tab, Enter, Escape, Delete, 方向键。<br/>需包含至少一个修饰键（Ctrl / Shift / Alt / Cmd）。</p>
        </div>

        <button class="btn-primary" :disabled="saving" @click="save">
          <Check :size="14" />
          {{ saving ? '保存中...' : '保存快捷键' }}
        </button>
        <span v-if="saveMsg" class="save-msg">{{ saveMsg }}</span>
      </div>

      <!-- 历史记录 -->
      <div v-if="activeTab === 'history'" class="panel">
        <h2>历史记录</h2>
        <div class="field">
          <label>最大保留条数</label>
          <div class="input-group">
            <input type="number" min="0" v-model="form.maxItems" />
            <span class="unit">条 (0=不限制)</span>
          </div>
        </div>
        <div class="field">
          <label>最大保留天数</label>
          <div class="input-group">
            <input type="number" min="0" v-model="form.maxDays" />
            <span class="unit">天 (0=不限制)</span>
          </div>
        </div>
        <div class="field">
          <label>清理数据</label>
          <button class="btn-danger" @click="doClear">
            <Trash2 :size="14" />
            清空非收藏历史
          </button>
        </div>

        <button class="btn-primary" :disabled="saving" @click="save">
          <Check :size="14" />
          {{ saving ? '保存中...' : '保存设置' }}
        </button>
        <span v-if="saveMsg" class="save-msg">{{ saveMsg }}</span>
      </div>

      <!-- 备份 -->
      <div v-if="activeTab === 'backup'" class="panel">
        <h2>数据备份</h2>
        <div class="field">
          <label><Download :size="14" /> 导出数据</label>
          <p class="desc">导出所有剪切板记录为 JSON 文件（不含图片二进制数据）。</p>
          <button class="btn-outline" @click="doExport">
            <Download :size="14" />
            导出 JSON
          </button>
        </div>
        <div class="field">
          <label><RotateCcw :size="14" /> 导入数据</label>
          <p class="desc">暂未支持，后续版本将提供 JSON 导入功能。</p>
        </div>
      </div>

      <!-- 关于 -->
      <div v-if="activeTab === 'about'" class="panel">
        <h2>关于 gopaste</h2>
        <div class="about-card">
          <div class="about-logo">p</div>
          <div class="about-info">
            <div class="about-name">gopaste</div>
            <div class="about-ver">v0.1.0</div>
          </div>
        </div>
        <div class="about-desc">
          <p>跨平台剪切板管理工具</p>
          <p>基于 Wails v2 + Go + Vue 3 构建。</p>
          <p>数据本地 AES-256-GCM 加密存储，永不上云。</p>
        </div>
        <div class="about-links">
          <div class="link-row"><span class="link-label">技术栈</span><span>Go · Vue 3 · Wails · SQLite</span></div>
          <div class="link-row"><span class="link-label">开源协议</span><span>MIT</span></div>
          <div class="link-row"><span class="link-label">数据目录</span><code>{{ dataDir }}</code></div>
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

/* 顶部标题栏 */
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

/* 左侧导航 */
.pref-nav {
  width: 150px; flex-shrink: 0;
  background: var(--bg-sidebar);
  border-right: 1px solid var(--bg-elevated);
  padding: 10px 0; overflow-y: auto;
}
.nav-list { display: flex; flex-direction: column; gap: 2px; padding: 0 6px; }
.nav-item {
  display: flex; align-items: center; gap: 8px;
  background: transparent; border: none; color: var(--text-secondary);
  padding: 9px 12px; border-radius: 6px; cursor: pointer;
  font-size: 13px; text-align: left; transition: all .12s;
  --wails-draggable: no-drag; -webkit-app-region: no-drag;
}
.nav-item:hover { background: var(--bg-elevated); color: var(--text); }
.nav-item.active { background: var(--accent); color: #fff; }

/* 右侧内容 */
.pref-content {
  flex: 1; overflow-y: auto; padding: 20px 24px;
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

/* 按钮 */
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

/* 快捷键录制器 */
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

/* 关于页 */
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

/* Segmented Control（主题切换） */
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

/* 拖拽 */
.drag-region { --wails-draggable: drag; -webkit-app-region: drag; }
.drag-region button { --wails-draggable: no-drag; -webkit-app-region: no-drag; }
</style>
