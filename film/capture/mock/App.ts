// Wails 后端 RPC 的浏览器端替身（仅用于宣传片素材采集）。
// 通过 vite alias 替换 frontend/wailsjs/go/main/App，业务代码零改动。
import { ITEMS, TOTAL, type MockItem } from './data'
import landscapeUrl from './images/landscape.png?url'

const IMAGE_URLS: Record<string, string> = { landscape: landscapeUrl }
const LOADED_AT = Date.now()
const params = new URLSearchParams(location.search)

const ok = <T>(v: T) => Promise.resolve(v)
const noop = () => ok(undefined as any)

function utf8ToB64(s: string): string {
  const bytes = new TextEncoder().encode(s)
  let bin = ''
  bytes.forEach((b) => (bin += String.fromCharCode(b)))
  return btoa(bin)
}

async function urlToB64(url: string): Promise<string> {
  const buf = new Uint8Array(await (await fetch(url)).arrayBuffer())
  let bin = ''
  buf.forEach((b) => (bin += String.fromCharCode(b)))
  return btoa(bin)
}

function toItem(m: MockItem) {
  const ts = new Date(LOADED_AT - m.ago * 1000).toISOString()
  const text = m.content ?? m.preview
  return {
    id: m.id,
    hash: `mock-${m.id}`,
    type: m.type,
    preview: m.preview,
    imagePath: '',
    size: m.size ?? new TextEncoder().encode(text).length,
    charCount: m.charCount ?? [...text].length,
    language: m.language ?? '',
    pinned: !!m.pinned,
    favorite: !!m.favorite,
    note: m.note ?? '',
    sourceApp: '',
    createdAt: ts,
    updatedAt: ts,
  }
}

const find = (id: number) => ITEMS.find((i) => i.id === id)

export function ListItems(q: any) {
  if ((q?.page ?? 1) > 1) return ok({ items: [], total: TOTAL, page: q.page })
  const kw = String(q?.keyword ?? '').trim().toLowerCase()
  let list = ITEMS.filter((i) => (q?.type ? i.type === q.type : true))
  if (q?.favorite) list = list.filter((i) => i.favorite)
  if (kw) list = list.filter((i) => (i.content ?? i.preview).toLowerCase().includes(kw))
  const filtered = !!(kw || q?.type || q?.favorite)
  return ok({ items: list.map(toItem), total: filtered ? list.length : TOTAL, page: 1 })
}

export async function GetContent(id: number) {
  const m = find(id)
  if (!m) return ''
  if (m.type === 'image' && m.imageKey) return urlToB64(IMAGE_URLS[m.imageKey])
  return utf8ToB64(m.content ?? m.preview)
}

export const GetFileThumbnail = (_id: number) => ok('')

export async function GetThumbnail(id: number) {
  const m = find(id)
  if (m?.type === 'image' && m.imageKey) return urlToB64(IMAGE_URLS[m.imageKey])
  return ''
}

export function GetSettings() {
  return ok({
    hotkeyModifiers: ['alt'],
    hotkeyKey: '`',
    maxItems: 1000,
    maxDays: 30,
    theme: params.get('theme') === 'light' ? 'light' : 'dark',
    language: 'zh-CN',
    autoPaste: true,
    hideOnPaste: true,
    pasteTrigger: 'double',
    windowPosition: 'cursor',
    scrollTopOnShow: true,
    resetFilterOnShow: false,
    silentStart: false,
    showTrayIcon: true,
    showTaskbarIcon: false,
    trayIconStyle: 'color',
    autoStart: true,
    tabHotkeysEnabled: true,
    emojiEnabled: true,
    extendedEmoji: false,
  })
}

export const GetAppVersion = () => ok('0.3.0')
export const GetWebsite = () => ok('https://gopaste.wetools.cc/')
export const DataDir = () => ok('~/.config/gopaste')
export const CheckForUpdate = () => ok({ hasUpdate: false, currentVersion: '0.3.0' })
export const HasPastePermission = () => ok(true)
export const RequestPastePermission = () => ok(true)
export const TrayNeedsRestart = () => ok(false)
export const ExportData = () => ok('')
export const ExportDataToFile = () => ok('')
export const ExportDiagnostics = () => ok('')
export const SaveImageToFile = () => ok('')

export const ClearHistory = noop
export const CopyToClipboard = noop
export const DeleteItem = noop
export const HideWindow = noop
export const OpenImageExternal = noop
export const OpenURL = noop
export const PasteItem = noop
export const PasteText = noop
export const QuitApp = noop
export const RevealInExplorer = noop
export const SetNote = noop
export const ToggleFavorite = noop
export const TogglePin = noop
export const UpdateSettings = noop
export const SetIsComposing = noop
export const Heartbeat = noop
