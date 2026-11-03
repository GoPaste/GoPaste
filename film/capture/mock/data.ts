// 宣传片专用虚构数据：全部为编造内容，不含任何真实剪贴板 / 个人 / 内部信息。
// 时间以「距页面加载的秒数」表达，保证每次采集元信息一致（formatTime 只看差值）。

export interface MockItem {
  id: number
  type: 'text' | 'image' | 'link' | 'code' | 'file'
  preview: string
  content?: string
  language?: string
  size?: number
  charCount?: number
  pinned?: boolean
  favorite?: boolean
  note?: string
  ago: number
  imageKey?: string
}

const GO_SNIPPET = `func (r *Repo) Search(q types.SearchQuery) ([]types.Item, error) {
	var items []types.Item
	tx := r.db.Where("preview LIKE ?", "%"+q.Keyword+"%")
	return items, tx.Order("updated_at DESC").Find(&items).Error
}`

const SQL_SNIPPET = `SELECT id, preview, updated_at
FROM items
WHERE favorite = 1
ORDER BY updated_at DESC
LIMIT 20;`

const TS_SNIPPET = `export const debounce = <T extends (...a: any[]) => void>(fn: T, ms = 200) => {
  let t: number | undefined
  return (...a: Parameters<T>) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms) }
}`

export const ITEMS: MockItem[] = [
  { id: 1, type: 'code', language: 'go', preview: GO_SNIPPET.replace(/\s+/g, ' '), content: GO_SNIPPET, ago: 4 },
  { id: 2, type: 'link', preview: 'https://gopaste.wetools.cc/docs/hotkeys', ago: 38 },
  { id: 3, type: 'text', preview: '周五 16:00 产品评审 · 3F 会议室 A，记得带上新版交互稿', favorite: true, ago: 95 },
  { id: 4, type: 'image', preview: '', size: 248_320, imageKey: 'landscape', ago: 260 },
  { id: 5, type: 'file', preview: '品牌手册_v3.pdf\n首页设计稿.fig\n发布海报.png', size: 18_874_368, charCount: 3, ago: 610 },
  { id: 6, type: 'code', language: 'sql', preview: SQL_SNIPPET.replace(/\s+/g, ' '), content: SQL_SNIPPET, favorite: true, ago: 1_320 },
  { id: 7, type: 'text', preview: 'Ship small, ship often.', ago: 2_700 },
  { id: 13, type: 'text', preview: 'v0.3 发布说明：新增 Emoji 面板、三语界面与深浅双主题', ago: 4_100 },
  { id: 8, type: 'link', preview: 'https://developer.mozilla.org/zh-CN/docs/Web/API/Clipboard_API', ago: 5_400 },
  { id: 9, type: 'code', language: 'typescript', preview: TS_SNIPPET.replace(/\s+/g, ' '), content: TS_SNIPPET, ago: 9_000 },
  { id: 10, type: 'text', preview: '#3B82F6', ago: 14_400 },
  { id: 11, type: 'text', preview: '本周待办：① 修复托盘图标闪烁 ② 补充三语文案 ③ 发布 v0.3', note: '', ago: 30_000 },
  { id: 12, type: 'text', preview: 'Thanks for the quick review — merging after CI turns green.', ago: 52_000 },
]

/** 列表展示总数：营造"用了一段时间"的真实感（只影响状态栏文案）。 */
export const TOTAL = 128
