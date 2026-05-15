<script lang="ts" setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick, shallowRef, watch } from 'vue'
import { ArrowUp } from 'lucide-vue-next'
import { lang } from '../i18n'
import { t as gt } from '../i18n'

const emit = defineEmits<{
  (e: 'select', emoji: string): void
  // Emitted on double-click — caller should paste directly into the
  // previously-focused window (PasteText pipeline) rather than just copy.
  // 'select' is intentionally NOT emitted alongside 'dblselect': the
  // pending single-click is cancelled in onEmojiDblClick to avoid double
  // feedback (copy toast + paste action firing for the same gesture).
  (e: 'dblselect', emoji: string): void
}>()

const props = defineProps<{
  theme: 'dark' | 'light'
  // External search query, typically bound to the app-wide top search input.
  // EmojiPicker used to own its own search box; we removed that in favour of
  // reusing the global search bar when the emoji view is active. Keeping the
  // prop optional and defaulting to '' preserves backward-compat if this
  // component is ever mounted standalone.
  search?: string
  // Whether the emoji view is currently the *visible* view in the parent.
  // Historically this prop gated a runtime sprite prewarm chain (tone 0
  // built eagerly, tones 1..5 lazily on first activation). Sprites are
  // now pre-bundled static WebP imports, so this prop has no effect on
  // the picker's internal pipeline anymore. It's kept on the public API
  // for backwards compat with App.vue's existing template wiring; future
  // cleanup can remove it once we're sure no other consumer depends on
  // it.
  active?: boolean
  // Whether to show the *full* emoji set. When false (the default), the
  // picker hides the bulky and rarely-used `objects` and `flags` categories
  // from the nav and the virtual list, plus hides the skin-tone picker
  // (which forces tone 0 at render time). Toggling this prop on at
  // runtime is supported — there's no async work to schedule, so the
  // change takes effect on the next render.
  extendedEmoji?: boolean
}>()

// Filtered category id-set: when `extendedEmoji` is off, exclude these from
// the user-visible flow (nav buttons, virtual list, search, scroll-to).
// We *do* keep them in `allCategories` so toggling back on doesn't require
// re-running the (~50ms) data import / sprite-index build.
const HIDDEN_WHEN_BASIC = new Set(['objects', 'flags'])

// ─── Data ────────────────────────────────────────────────────────
interface EmojiItem {
  id: string
  native: string
  keywords: string[]
  name: string
}

interface Category {
  id: string
  emojis: EmojiItem[]
}

// Flattened row for virtual scroll
interface VRow {
  type: 'header' | 'emoji-row'
  catId: string
  // For header
  label?: string
  // For emoji-row: array of emojis in this row
  emojis?: EmojiItem[]
}

// Cell vs glyph geometry (decoupled, see also SPRITE_CELL below).
//   EMOJI_SIZE / ROW_HEIGHT  = cell box (button hit-target, grid step)
//   SPRITE_CELL              = on-screen glyph size (sprite blit step)
// Earlier iterations kept these equal to simplify sprite math, but the
// reference layout calls for "small glyphs in roomy cells" — the glyph
// occupies only the inner ~28 px of a 36 px cell so neighbors visibly
// breathe and the hover affordance reads as a circle around the glyph
// rather than a tight halo on it. The 4 px ring of empty space on each
// side is achieved with `padding: 4px` + `background-clip: content-box`
// on the sprite layer (see CSS), which keeps the sprite blit centered.
const ROW_HEIGHT = 36
const HEADER_HEIGHT = 28
const EMOJI_SIZE = 36
const SCROLL_WINDOW_PADDING = 20 // px - .ep-scroll-window left+right padding

// Dynamic columns based on available width
const containerWidth = ref(0)
const emojisPerRow = computed(() => {
  const w = containerWidth.value
  if (w <= 0) return 6 // fallback before first measurement (1.5x cell ⇒ fewer cols)
  const usable = w - SCROLL_WINDOW_PADDING
  return Math.max(1, Math.floor(usable / EMOJI_SIZE))
})

// Use shallowRef for large lists
const allCategories = shallowRef<Category[]>([])
// View-layer source of truth: drops `objects` / `flags` when extendedEmoji
// is off. Keep `frequent` (user-curated) regardless. All downstream paths
// (search, virtual rows, nav buttons, cycleCategory, scrollToCategory)
// read from this — `allCategories` is only the raw store for `addFrequent`
// and the initial load.
const visibleCategories = computed<Category[]>(() => {
  if (props.extendedEmoji) return allCategories.value
  return allCategories.value.filter(c => !HIDDEN_WHEN_BASIC.has(c.id))
})
const loaded = ref(false)
// Search query is driven by the parent via the `search` prop. Reading it
// through a computed (instead of a ref + manual sync) keeps filtering and
// virtual-row rebuilds perfectly in sync with the external input without
// any debounce/watcher gymnastics.
const searchQuery = computed(() => (props.search ?? '').trim())
const activeCategory = ref('')
const scrollRef = ref<HTMLDivElement | null>(null)

// Virtual scroll state
const scrollTop = ref(0)
const containerHeight = ref(400)

// Frequently used (stored in localStorage)
const FREQ_KEY = 'gopaste-emoji-freq'
const MAX_FREQ = 32

// ─── Skin tone ──────────────────────────────────────────────────
// emoji-mart-data gives each supported emoji a `skins` array of length 1 (no
// skin variants) or 6 (default yellow at index 0, then Fitzpatrick scale
// light → dark at indices 1..5). We store the user's preferred tone and
// apply it both at pick-time (emit) and at render-time (list display).
//
// Rendering strategy (see `useSprite` computed and SPRITE_SLOTS):
//   - tone 0 builds a sprite eagerly on mount → fast bitmap-blit path
//   - tones 1..5 build their own sprite lazily on FIRST selection. While
//     building (1-2s on a fast machine), the list temporarily falls back
//     to the text path (DOM <span> with the toned native). Once the slot
//     is ready, the template flips back to the sprite path automatically.
//   - Switching back to a previously-built tone is instant (sprites cached).
// Memory cost: each tone sprite is ~10 MB GPU texture, 6 tones max.
// Only tones the user actually selects are ever built.
//
// Values: 0 = default yellow, 1..5 = Fitzpatrick 1-2 (light) through 6 (dark)
const TONE_KEY = 'gopaste-emoji-skin-tone'
type SkinTone = 0 | 1 | 2 | 3 | 4 | 5
const skinTone = ref<SkinTone>(((): SkinTone => {
  const raw = parseInt(localStorage.getItem(TONE_KEY) || '0', 10)
  return (raw >= 0 && raw <= 5 ? raw : 0) as SkinTone
})())
// Effective tone for *rendering and emitting*. When extendedEmoji is off
// we force tone 0 — the user's persisted preference is preserved in
// localStorage and re-applied automatically when they re-enable the
// extended set. Use `currentTone()` everywhere instead of reading
// `skinTone.value` directly, so the picker-off case is honoured uniformly
// (sprite cell selection, copy/paste glyph).
function currentTone(): SkinTone {
  return props.extendedEmoji ? skinTone.value : 0
}
const showTonePicker = ref(false)
// Popup coordinates in viewport space. We render the flyout with
// `position: fixed` (not absolute under .ep-tone-wrap) because the nearest
// scroll/overflow ancestor is `.ep-root { overflow: hidden }`, which clips
// any absolutely-positioned child that extends past the nav row. `fixed`
// escapes all ancestor clipping and is anchored directly to the viewport.
// Recomputed from the button's bounding rect on every open and on window
// resize/scroll while the popup is visible.
const tonePopupPos = ref<{ top: number; right: number }>({ top: 0, right: 0 })
const toneBtnRef = ref<HTMLButtonElement | null>(null)
// Map<emoji.id, skins[]> for O(1) lookup at pick-time. Keeping this outside
// EmojiItem avoids inflating the virtual-scroll row data (3600+ emojis).
const skinsMap = new Map<string, { native: string }[]>()

// Visual dot colors for the tone picker. Order matches skinTone values.
// These are perceptually-tuned approximations of the Unicode reference
// swatches rendered by major platforms.
const TONE_COLORS = [
  '#ffd93b', // 0 default yellow
  '#ffdbb4', // 1 light (Fitzpatrick 1-2)
  '#e0bb95', // 2 medium-light (3)
  '#bf8f68', // 3 medium (4)
  '#9b643d', // 4 medium-dark (5)
  '#5c4033', // 5 dark (6)
]

function setSkinTone(t: SkinTone) {
  skinTone.value = t
  localStorage.setItem(TONE_KEY, String(t))
  showTonePicker.value = false
  // No build step: every tone's sprite is a pre-bundled WebP imported at
  // module load, so switching tones is just a different SPRITE_SLOTS entry
  // taking effect on next render — already loaded, already decoded.
}

// Compute popup coordinates from the tone button's current bounding rect.
// We anchor the popup's top-right corner to just below the button's
// bottom-right, matching the previous absolute layout (`top: 100%+4px;
// right: 4px`) but now relative to the viewport. Called on open and on
// resize/scroll so the popup tracks the button if the window is resized
// while it's open.
function updateTonePopupPos() {
  const el = toneBtnRef.value
  if (!el) return
  const r = el.getBoundingClientRect()
  tonePopupPos.value = {
    top: r.bottom + 4,
    // Distance from viewport right edge to button's right edge, so the
    // popup's `right` CSS matches visually (popup grows leftward).
    right: Math.max(4, window.innerWidth - r.right),
  }
}

function toggleTonePicker() {
  if (showTonePicker.value) {
    showTonePicker.value = false
    return
  }
  updateTonePopupPos()
  showTonePicker.value = true
}

// Close the tone flyout when clicking anywhere outside. The popup now lives
// directly on document.body (via Teleport) so we must check both the
// trigger wrapper AND the popup element — otherwise clicking inside the
// popup would close it before the tone-choice click handler fires.
function onDocMouseDown(e: MouseEvent) {
  if (!showTonePicker.value) return
  const target = e.target as HTMLElement | null
  if (!target) return
  if (target.closest('.ep-tone-wrap')) return
  if (target.closest('.ep-tone-popup')) return
  showTonePicker.value = false
}
// If the window resizes or the user scrolls the page while the popup is
// open, the button's on-screen position changes — recompute so the popup
// stays glued to it. Throttled via rAF would be nicer but tone picker is
// rarely open long enough to matter.
function onWindowChange() {
  if (showTonePicker.value) updateTonePopupPos()
}
onMounted(() => {
  document.addEventListener('mousedown', onDocMouseDown)
  window.addEventListener('resize', onWindowChange)
  window.addEventListener('scroll', onWindowChange, true)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocMouseDown)
  window.removeEventListener('resize', onWindowChange)
  window.removeEventListener('scroll', onWindowChange, true)
})

function getFrequent(): string[] {
  try {
    return JSON.parse(localStorage.getItem(FREQ_KEY) || '[]')
  } catch { return [] }
}

function addFrequent(native: string) {
  const list = getFrequent().filter(e => e !== native)
  list.unshift(native)
  if (list.length > MAX_FREQ) list.length = MAX_FREQ
  localStorage.setItem(FREQ_KEY, JSON.stringify(list))
  // Update frequent category in-place. We resolve each native back to its
  // canonical EmojiItem (with the real emoji-mart id and name) via
  // `nativeToItem` so the hover-tip's `tipSvgUrlFor(emoji)` can hit the
  // Fluent SVG manifest (keyed by `${id}__${tone}`); falling back to a
  // shallow `{ id: native, native, ... }` would make the tip render
  // through the system-font glyph fallback path — exactly the bug this
  // map was added to fix. Items not yet seen (extremely rare; only if
  // frequent was populated before the data import finished) get the
  // legacy shallow shape so the cell still renders something.
  const cats = allCategories.value
  if (cats.length > 0 && cats[0].id === 'frequent') {
    cats[0] = {
      id: 'frequent',
      emojis: list.map(n => nativeToItem.get(n) ?? { id: n, native: n, keywords: [], name: '' }),
    }
    allCategories.value = [...cats]
  }
}

// ─── i18n ────────────────────────────────────────────────────────
const i18nData: Record<string, Record<string, string>> = {
  zh: {
    search: '搜索表情符号…',
    noResults: '没有找到相关表情',
    frequent: '常用',
    people: '表情与角色',
    nature: '动物与自然',
    foods: '食物与饮品',
    activity: '活动',
    places: '旅行与景点',
    objects: '物品',
    symbols: '符号',
    flags: '旗帜',
    skinTone: '肤色',
  },
  en: {
    search: 'Search emoji…',
    noResults: 'No emoji found',
    frequent: 'Frequently used',
    people: 'Smileys & People',
    nature: 'Animals & Nature',
    foods: 'Food & Drink',
    activity: 'Activity',
    places: 'Travel & Places',
    objects: 'Objects',
    symbols: 'Symbols',
    flags: 'Flags',
    skinTone: 'Skin tone',
  },
}
i18nData['zh-TW'] = i18nData.zh

function tt(key: string): string {
  const d = i18nData[lang.value] || i18nData.en
  return d[key] || key
}

// ─── Category nav icons ──────────────────────────────────────────
const categoryIcons: Record<string, string> = {
  frequent: '🕐',
  people: '😀',
  nature: '🐻',
  foods: '🍔',
  activity: '⚽',
  places: '✈️',
  objects: '💡',
  symbols: '🔣',
  flags: '🏁',
}

// ─── Search ──────────────────────────────────────────────────────
// Weighted match: emoji-mart's keyword lists contain many historical
// unicode-name fragments (e.g. "Pen" has keyword "left" from its legacy
// "lower-left ballpoint pen" name). A flat OR-match causes surprising
// hits like searching "left" returning pen before arrows. We still want
// those keywords for useful recall ("sad" → pensive), so instead of
// filtering them out we score each candidate and sort by score.
//
// Score hierarchy (higher = more relevant, ties broken by name length
// so shorter/more-specific names float up):
//   100  exact name match                  "heart" == "Heart"
//    80  name starts with query            "left"  → "Leftwards Arrow"
//    60  name contains query as substring  "left"  → "Up-Left Arrow"
//    40  any keyword equals query          "sad"   == keyword "sad"
//    20  any keyword starts with query     "smi"   → keyword "smile"
//    10  any keyword contains query        "left"  → keyword "left" in Pen
//     5  emoji character itself contains query (rare, e.g. flag codes)
function scoreEmoji(e: EmojiItem, q: string): number {
  const name = e.name.toLowerCase()
  if (name === q) return 100
  if (name.startsWith(q)) return 80
  if (name.includes(q)) return 60
  let best = 0
  for (const k of e.keywords) {
    const kl = k.toLowerCase()
    if (kl === q) { best = Math.max(best, 40); continue }
    if (kl.startsWith(q)) { best = Math.max(best, 20); continue }
    if (kl.includes(q)) best = Math.max(best, 10)
  }
  if (best > 0) return best
  if (e.native.includes(q)) return 5
  return 0
}

const filteredCategories = computed(() => {
  const q = searchQuery.value.toLowerCase().trim()
  if (!q) return visibleCategories.value
  const results: Category[] = []
  for (const cat of visibleCategories.value) {
    // Score, drop zeros, then sort in-place. Sort is per-category which
    // keeps total work O(n log n) overall — no global flatten needed.
    const scored: { e: EmojiItem; s: number }[] = []
    for (const e of cat.emojis) {
      const s = scoreEmoji(e, q)
      if (s > 0) scored.push({ e, s })
    }
    if (scored.length === 0) continue
    scored.sort((a, b) => {
      if (b.s !== a.s) return b.s - a.s
      // Tie-break: shorter name first (more specific match).
      return a.e.name.length - b.e.name.length
    })
    results.push({ ...cat, emojis: scored.map(x => x.e) })
  }
  return results
})

// ─── Virtual scroll rows ─────────────────────────────────────────
// Build flat row list from categories (each category = 1 header + N emoji rows)
// Use shallowRef + explicit rebuild to avoid reactive cascade on v-show toggle
const virtualRows = shallowRef<VRow[]>([])
let lastPerRow = 0
let lastCatsKey = ''

function rebuildRows() {
  const cats = searchQuery.value ? filteredCategories.value : visibleCategories.value
  const perRow = emojisPerRow.value
  const rows: VRow[] = []
  for (const cat of cats) {
    rows.push({ type: 'header', catId: cat.id, label: tt(cat.id) })
    for (let i = 0; i < cat.emojis.length; i += perRow) {
      rows.push({
        type: 'emoji-row',
        catId: cat.id,
        emojis: cat.emojis.slice(i, i + perRow),
      })
    }
  }
  virtualRows.value = rows
}

// Watch dependencies and only rebuild when truly needed
watch(
  [emojisPerRow, filteredCategories, visibleCategories, searchQuery],
  () => {
    const perRow = emojisPerRow.value
    const cats = searchQuery.value ? filteredCategories.value : visibleCategories.value
    // Generate a lightweight identity key to skip unnecessary rebuilds
    const catsKey = cats.map(c => c.id + ':' + c.emojis.length).join(',')
    if (perRow === lastPerRow && catsKey === lastCatsKey) return
    lastPerRow = perRow
    lastCatsKey = catsKey
    rebuildRows()
  },
  { immediate: true }
)

// When the search query toggles (becomes non-empty, or clears back to empty),
// snap the scroll position to the top. Without this, typing in the external
// search box while the user is scrolled deep into a category leaves them
// looking at an empty area of the filtered result set.
watch(searchQuery, () => {
  if (scrollRef.value) {
    scrollRef.value.scrollTop = 0
    scrollTop.value = 0
  }
})

// Total scroll height
const totalHeight = computed(() => {
  let h = 0
  for (const row of virtualRows.value) {
    h += row.type === 'header' ? HEADER_HEIGHT : ROW_HEIGHT
  }
  return h
})

// Category offset map for scroll-to
const categoryOffsets = computed(() => {
  const map: Record<string, number> = {}
  let offset = 0
  for (const row of virtualRows.value) {
    if (row.type === 'header') {
      map[row.catId] = offset
    }
    offset += row.type === 'header' ? HEADER_HEIGHT : ROW_HEIGHT
  }
  return map
})

// Visible rows (with buffer). Larger buffer = more rows pre-rendered above/
// below viewport so fast scrolling doesn't expose blank area. With the sprite
// sheet path, each extra emoji cell is a ~zero-cost bitmap blit instead of
// a color-emoji glyph raster, so we can keep a generous buffer for smoothness.
const BUFFER_PX = 600

const visibleSlice = computed(() => {
  const rows = virtualRows.value
  if (rows.length === 0) return { startIdx: 0, endIdx: 0, offsetY: 0 }

  const top = scrollTop.value - BUFFER_PX
  const bottom = scrollTop.value + containerHeight.value + BUFFER_PX

  let startIdx = 0
  let offsetY = 0
  let cumH = 0
  for (let i = 0; i < rows.length; i++) {
    const rh = rows[i].type === 'header' ? HEADER_HEIGHT : ROW_HEIGHT
    if (cumH + rh > top) {
      startIdx = i
      offsetY = cumH
      break
    }
    cumH += rh
  }

  let endIdx = startIdx
  let endH = offsetY
  for (let i = startIdx; i < rows.length; i++) {
    const rh = rows[i].type === 'header' ? HEADER_HEIGHT : ROW_HEIGHT
    endH += rh
    endIdx = i + 1
    if (endH >= bottom) break
  }

  return { startIdx, endIdx, offsetY }
})

const renderedRows = computed(() => {
  const { startIdx, endIdx } = visibleSlice.value
  return virtualRows.value.slice(startIdx, endIdx)
})

// ─── Scroll handling ─────────────────────────────────────────────
let rafId: number | null = null

// "Back to top" floating button visibility — mirrors App.vue's main list
// behavior (threshold 200px). Kept reactive so the <Transition> fade
// can react. We update it inside the rAF below to coalesce with the
// existing scrollTop write and avoid an extra reflow.
const showBackTop = ref(false)

function onScroll() {
  // Virtualized rows may be recycled under a visible tip, leaving it
  // pointing at the wrong cell. Hiding on scroll is cheap and feels
  // natural — the user can rest the pointer again to re-trigger.
  if (tip.value || tipShowTimer != null) hideTip()
  if (rafId != null) return
  rafId = requestAnimationFrame(() => {
    rafId = null
    if (!scrollRef.value) return
    const st = scrollRef.value.scrollTop
    scrollTop.value = st
    showBackTop.value = st > 200
    updateActiveCategory()
  })
}

// Smooth-scroll the virtual scroll container to the top. Same UX as
// App.vue's scrollToTop on the main list. Note: we also reset the
// active-category indicator to the first category synchronously so
// the sidebar highlight doesn't lag behind the smooth-scroll.
function scrollToTop() {
  const el = scrollRef.value
  if (!el) return
  el.scrollTo({ top: 0, behavior: 'smooth' })
  showBackTop.value = false
}

function updateActiveCategory() {
  if (searchQuery.value) return
  // Find which category is at the top of the viewport
  const rows = virtualRows.value
  const st = scrollTop.value + 10 // small offset
  let cumH = 0
  let lastCat = ''
  for (const row of rows) {
    if (row.type === 'header') lastCat = row.catId
    const rh = row.type === 'header' ? HEADER_HEIGHT : ROW_HEIGHT
    if (cumH + rh > st) break
    cumH += rh
  }
  if (lastCat) activeCategory.value = lastCat
}

// ─── Load data ───────────────────────────────────────────────────
// Wait for the scroll container's real size before setting allCategories.
// This is critical: if we set categories first with a fallback emojisPerRow (e.g. 9),
// then later containerWidth changes -> rebuild rows -> Vue must patch every row's
// children (different emoji counts), causing a costly DOM/paint storm.
async function waitForContainer(maxAttempts = 30): Promise<HTMLDivElement | null> {
  for (let i = 0; i < maxAttempts; i++) {
    if (scrollRef.value && scrollRef.value.clientWidth > 0) {
      return scrollRef.value
    }
    await new Promise(r => requestAnimationFrame(() => r(null)))
  }
  return scrollRef.value
}

// ─── Spritesheet: pre-bundled WebP per tone ──────────────────────
// Earlier iterations of this picker rasterized emoji at runtime — first
// via canvas.fillText against the OS color-emoji font, then via Microsoft
// Fluent UI Emoji SVGs streamed from jsDelivr and drawn into a canvas
// chunk by chunk. Both worked but had real costs:
//
//   - System-font path: tab-switch jank from DirectWrite COLR/CPAL raster
//     pinning the GPU process for hundreds of ms per scroll tile.
//   - CDN path: ~1870 HTTPS round trips for first-paint, 30-60s on slow
//     links, and a hard online dependency the rest of the app didn't have.
//
// Current strategy: ship six pre-rendered WebP sheets (one per skin tone)
// inside the bundle. Each cell is exactly SPRITE_CELL_NATIVE px, the grid
// is SPRITE_COLS wide, layout is identical across all six sheets. At
// render time a cell is just a <span> with `background-image: url(sprite)`
// + a (col,row) offset — pure GPU blit, no font shaping, no network.
//
// Layout & data shape MUST match scripts/build-emoji-sprite.mjs exactly,
// because the runtime computes (col,row) from the cell's linear index
// without any manifest. If you change SPRITE_COLS or the cell pixel size
// here, change them there and re-run `npm run build:emoji`.
//
// Memory: six WebPs × ~250-1500 KB on disk each → decoded ~10 MB GPU
// texture for tone 0 + ~2 MB each for tones 1..5 (only ~305 toned cells
// painted, rest is transparent and compresses to near-zero). Total
// ~20 MB GPU residency once all tones have been rendered to screen.
//
// Why WebP not PNG: lossless WebP is 50-70% smaller than equivalent PNG
// for this kind of flat-color content, decodes nearly as fast in
// Chromium, and was natively supported by every WebView2/WKWebView/
// CEF/Tauri runtime we care about by 2022.
import spriteUrl0 from '../assets/emoji-sprite/sprite-tone0.webp'
import spriteUrl1 from '../assets/emoji-sprite/sprite-tone1.webp'
import spriteUrl2 from '../assets/emoji-sprite/sprite-tone2.webp'
import spriteUrl3 from '../assets/emoji-sprite/sprite-tone3.webp'
import spriteUrl4 from '../assets/emoji-sprite/sprite-tone4.webp'
import spriteUrl5 from '../assets/emoji-sprite/sprite-tone5.webp'
// Build-time manifest. Its only runtime-relevant field is `missingIds`:
// emoji that @emoji-mart/data knows about but Fluent flat has no SVG
// for (merperson, family-*, most country flags, …). We hide these in
// the grid entirely so users don't see blank cells — rather than
// letting the ghost sprite slot render as empty space. Their slot in
// the sprite index is still reserved (keeps every other emoji's
// (col,row) math stable), they're just skipped when building the
// visible category buckets.
import spriteMeta from '../assets/emoji-sprite/build-meta.json'
const MISSING_IDS: Set<string> = new Set(
  (spriteMeta as { missingIds?: string[] }).missingIds ?? [],
)

// ─── Hi-res hover-preview SVG support ───────────────────────────
// The sprite rasters each emoji at 48 px (SPRITE_CELL_NATIVE). The hover
// tip displays at 96 px, which means a 2× bilinear upscale — noticeably
// soft on flat artwork. To fix this the build script now copies every
// matched Fluent SVG into `assets/emoji-svg/` and emits a manifest that
// maps `<emojiId>__<tone>` → stem. At runtime we resolve the stem to a
// Vite-hashed asset URL via `import.meta.glob`, then render the tip as a
// crisp `<img src="...svg">` instead of a sprite blit. Because the glob
// uses `{ eager: false }` (lazy), Vite emits each SVG as its own chunk
// and the WebView only fetches the one(s) the user actually hovers.
import svgManifest from '../assets/emoji-svg/manifest.json'

// Eager glob: keys are relative paths like `../assets/emoji-svg/foo.svg`,
// values are the resolved Vite-hashed asset URL strings. Using eager:true
// embeds all ~3000 URL strings (~120 KB) into the main chunk at build time
// but avoids creating 3000 individual JS wrapper chunks (which would bloat
// the dist directory and slow down bundling). The SVG *files* themselves
// are still emitted as separate assets and only fetched when an <img src>
// references them — so runtime memory stays minimal.
const svgGlob: Record<string, string> = import.meta.glob(
  '../assets/emoji-svg/*.svg',
  { query: '?url', import: 'default', eager: true },
)
// Pre-build a stem → resolved-URL map for O(1) lookup in tipSvgUrlFor().
const svgUrls = new Map<string, string>()
for (const [relPath, url] of Object.entries(svgGlob)) {
  // relPath looks like "../assets/emoji-svg/grinning-face.svg"
  const stem = relPath.slice(relPath.lastIndexOf('/') + 1, -4) // strip dir + ".svg"
  svgUrls.set(stem, url)
}
// No async loading needed — URLs are resolved at module init.

// SPRITE_CELL is now the *glyph* step, decoupled from the cell box
// (EMOJI_SIZE = 36). 28 = previous 24 ×1.2 (glyph 20 % bigger), and
// 48→28 is still a clean GPU downscale ratio. background-position
// offsets remain integer CSS px so we keep the sub-pixel-stable sample
// path. The 8 px difference between EMOJI_SIZE and SPRITE_CELL is
// rendered via `padding: 4px` + `background-clip: content-box` on the
// sprite layer — the sprite blit shows up as 28 px centered inside a
// 36 px cell, with 4 px of breathable air on every side.
const SPRITE_CELL = 28
const SPRITE_CELL_NATIVE = 48 // physical px per cell in the WebP source
const SPRITE_COLS = 32        // sprite layout: 32 cells per row

// Per-tone slot: holds the URL the <img> background uses. `ready` is
// always true after construction now — sprites are static assets, no
// async build phase. Kept for cellStyle's existing branching shape so
// downstream code didn't have to be rewritten.
type SpriteSlot = { url: string; ready: boolean }
const SPRITE_SLOTS: SpriteSlot[] = [
  { url: spriteUrl0, ready: true },
  { url: spriteUrl1, ready: true },
  { url: spriteUrl2, ready: true },
  { url: spriteUrl3, ready: true },
  { url: spriteUrl4, ready: true },
  { url: spriteUrl5, ready: true },
]

const emojiIndex = new Map<string, number>() // canonical native -> linear index
// native -> canonical EmojiItem (with real emoji-mart id + name + keywords).
// Populated in onMounted alongside `emojiIndex` / `orderedEmojis`. Used by
// the "frequent" category builder and `addFrequent` to reconstruct full
// items from the localStorage native-only list — without this, frequent
// items would carry id=native (e.g. "🥰"), and `tipSvgUrlFor` would miss
// the Fluent SVG manifest (keyed by emoji-mart id), forcing the hover
// preview into its system-font fallback path.
const nativeToItem = new Map<string, EmojiItem>()
// Parallel array: orderedEmojis[i] is the EmojiItem at index i. Mirrors the
// same iteration order the build script uses, so (col,row) computed from
// `i % SPRITE_COLS` / `i / SPRITE_COLS` lines up with the WebP cells.
let orderedEmojis: EmojiItem[] = []
// Mirror of toneable membership keyed by emoji.id. Toneable cells follow
// the current `skinTone` (use SPRITE_SLOTS[tone]); non-toneable cells are
// permanently anchored to SPRITE_SLOTS[0], so changing tone doesn't
// invalidate their backgroundImage URL and they don't repaint. This is
// what makes tone switching feel like "only the people change" rather
// than every emoji blinking.
const tonedIds = new Set<string>()


// ─── Sprite background-size ──────────────────────────────────────
// `background-size` is computed once per total-emoji-count change and
// shared across every cell's inline style. Specifying it in CSS px
// (SPRITE_COLS × SPRITE_CELL) instead of letting the browser use the
// WebP's native dimensions does two things:
//   1. The GPU compositor downscales the SPRITE_CELL_NATIVE-px source
//      sheet to the SPRITE_CELL-px on-screen cell — same path that
//      keeps photo thumbnails crisp on HiDPI displays.
//   2. background-position offsets become CSS-px integers (`-34px
//      -68px`), avoiding sub-pixel sample drift between cells that you
//      get if `background-size: auto` and the browser scales independently.
const spriteBgSize = computed(() => {
  const total = emojiIndex.size
  if (total === 0) return ''
  const rows = Math.ceil(total / SPRITE_COLS)
  return `${SPRITE_COLS * SPRITE_CELL}px ${rows * SPRITE_CELL}px`
})

// Returns the inline `style` object for one emoji cell, or null if the
// emoji isn't in the sprite (shouldn't happen post-mount; defensive).
//
// Per-cell tone resolution: non-toned cells permanently anchor to
// SPRITE_SLOTS[0]; toned cells follow `currentTone()`. This is what makes
// tone switching feel like "only the people change" rather than every
// emoji blinking — the 1565 non-toned cells keep the exact same
// backgroundImage URL across tones, so Vue's diff produces an identity
// update and the browser doesn't repaint them.
//
// Note: every slot's `ready` flag is true at module load (sprites are
// pre-bundled WebP imports, decoded synchronously by the browser on first
// reference), so there is no fallback path / null return for "still
// building" — that whole code branch existed for the old runtime canvas
// pipeline and was removed when sprites became static assets.
function cellStyle(emoji: EmojiItem): Record<string, string> | null {
  const idx = emojiIndex.get(emoji.native)
  if (idx == null) return null
  const tone = tonedIds.has(emoji.id) ? currentTone() : 0
  const slot = SPRITE_SLOTS[tone]
  const col = idx % SPRITE_COLS
  const row = (idx / SPRITE_COLS) | 0
  // We expose the sprite URL via a CSS custom property (`--ep-sprite`)
  // *in addition to* setting `background-image` directly. The custom
  // property is what the :hover rule reads when it composes a two-layer
  // background list (sprite under, hover disc over) — without it, the
  // inline `background-image` would win the cascade against any
  // stylesheet rule and the hover disc could never be added without an
  // !important fight that's hard to keep clean across themes.
  return {
    '--ep-sprite': `url(${slot.url})`,
    backgroundImage: `url(${slot.url})`,
    backgroundSize: spriteBgSize.value,
    backgroundPosition: `-${col * SPRITE_CELL}px -${row * SPRITE_CELL}px`,
  }
}

onMounted(async () => {
  const dataModule = await import('@emoji-mart/data')
  const raw = dataModule.default as {
    categories: { id: string; emojis: string[] }[]
    emojis: Record<string, { id: string; name: string; keywords?: string[]; skins: { native: string }[] }>
  }

  const cats: Category[] = []

  // Map raw data. We also build `emojiIndex` (canonical native → sprite
  // index) and `orderedEmojis` (parallel array of EmojiItems in the same
  // order) here, so per-tone sprite builders can look up the correct toned
  // glyph for each cell. All tone sprites share this layout.
  const orderedItems: EmojiItem[] = []
  emojiIndex.clear()
  nativeToItem.clear()
  for (const rawCat of raw.categories) {
    const emojis: EmojiItem[] = []
    for (const emojiId of rawCat.emojis) {
      const emojiData = raw.emojis[emojiId]
      if (emojiData?.skins?.[0]) {
        const native = emojiData.skins[0].native
        const item: EmojiItem = {
          id: emojiData.id,
          native,
          keywords: emojiData.keywords || [],
          name: emojiData.name,
        }
        // Is this emoji present in the Fluent sprite? If not, it would
        // render as a blank transparent cell — so we drop it from the
        // *visible* category bucket entirely. We still advance the
        // sprite-index below though: the sprite was built with this
        // emoji occupying a slot (just an unpainted one), and every
        // subsequent emoji's (col,row) depends on that slot existing.
        // Skipping orderedItems.push would shift all later emoji by one
        // column and break sprite alignment globally.
        const inSprite = !MISSING_IDS.has(emojiData.id)
        if (inSprite) emojis.push(item)
        // Deduplicate across categories — only the first occurrence gets a
        // sprite slot. Frequent items will reuse the index from their
        // canonical category.
        if (!emojiIndex.has(native)) {
          emojiIndex.set(native, orderedItems.length)
          orderedItems.push(item)
          // Mirror native → item so addFrequent / the frequent-category
          // builder below can resolve full EmojiItems (with real id) from
          // the localStorage native-only list.
          nativeToItem.set(native, item)
        }
        // Stash the full skins array (if >1) so pick-time can select the
        // user's preferred tone. Single-skin emojis (no tone variants) are
        // skipped to save memory - lookup will miss and default to .native.
        if (emojiData.skins.length > 1) {
          skinsMap.set(emojiData.id, emojiData.skins)
        }
      }
    }
    cats.push({ id: rawCat.id, emojis })
  }

  // Build the "frequent" category *after* the main loop so we can resolve
  // each stored native back to its canonical EmojiItem (with real
  // emoji-mart id), letting `tipSvgUrlFor` hit the Fluent SVG manifest
  // for hover-preview rendering. Prepended with unshift so it stays the
  // first category, preserving the existing nav-button order.
  const freq = getFrequent()
  if (freq.length > 0) {
    cats.unshift({
      id: 'frequent',
      emojis: freq.map(n => nativeToItem.get(n) ?? { id: n, native: n, keywords: [], name: '' }),
    })
  }
  orderedEmojis = orderedItems
  // Mirror skin-tone variant membership keyed by id for O(1) lookup in
  // cellStyle. Non-toned cells permanently anchor to SPRITE_SLOTS[0] so
  // changing tone never invalidates their backgroundImage URL — only the
  // toned ~305 cells repaint.
  tonedIds.clear()
  for (const item of orderedItems) {
    if (skinsMap.has(item.id)) tonedIds.add(item.id)
  }

  // First mark loaded so the scroll container actually renders into DOM
  loaded.value = true
  await nextTick()

  // Ensure the scroll container has a real size before populating data
  const el = await waitForContainer()
  if (el) {
    containerHeight.value = el.clientHeight
    containerWidth.value = el.clientWidth
  }

  // Populate data - first render uses correct dimensions, no rebuild storm
  allCategories.value = cats
  activeCategory.value = cats[0]?.id || ''

  // Sprites are pre-bundled WebP assets imported at the top of this file;
  // no build/prewarm step at runtime. The browser decodes each WebP on
  // first paint that references it (tone 0 ≈ 1.5 MB → ~30 ms decode on
  // a typical machine, tones 1..5 ≈ 250 KB each → ~5 ms each).
})

// Watch container resize (debounced to avoid v-show toggle cascade)
const resizeObs = ref<ResizeObserver | null>(null)
let resizeRafId: number | null = null
watch(scrollRef, (el) => {
  if (resizeObs.value) resizeObs.value.disconnect()
  if (el) {
    resizeObs.value = new ResizeObserver(() => {
      // Use rAF to batch resize updates - avoid sync recompute on v-show toggle
      if (resizeRafId != null) return
      resizeRafId = requestAnimationFrame(() => {
        resizeRafId = null
        if (!el) return
        const h = el.clientHeight
        const w = el.clientWidth
        // Only update if values actually changed
        if (h !== containerHeight.value) containerHeight.value = h
        if (w !== containerWidth.value) containerWidth.value = w
      })
    })
    resizeObs.value.observe(el)
  }
}, { immediate: true })

onBeforeUnmount(() => {
  if (rafId != null) cancelAnimationFrame(rafId)
  resizeObs.value?.disconnect()
})

// ─── Scroll to category ──────────────────────────────────────────
function scrollToCategory(catId: string) {
  const offset = categoryOffsets.value[catId]
  if (offset != null && scrollRef.value) {
    scrollRef.value.scrollTop = offset
    scrollTop.value = offset
    activeCategory.value = catId
  }
}

// ─── Cycle category (keyboard shortcut target) ───────────────────
// Called by App.vue when Tab/Shift+Tab is pressed while the emoji view
// is active. Wraps around at both ends so users can loop through
// categories without ever leaving the emoji view.
function cycleCategory(dir: 1 | -1) {
  const cats = visibleCategories.value
  if (cats.length === 0) return
  const curIdx = cats.findIndex(c => c.id === activeCategory.value)
  // If current is unknown (e.g. during search), treat as 0 so +1 goes to idx 1
  // and -1 wraps to the last category.
  const base = curIdx < 0 ? 0 : curIdx
  const nextIdx = (base + dir + cats.length) % cats.length
  scrollToCategory(cats[nextIdx].id)
}

// ─── Emoji click ─────────────────────────────────────────────────
// Resolve the displayed/emitted native for an emoji given the current tone.
//   - tone 0 (default yellow) OR emoji has no tone variants → use emoji.native
//   - tone 1..5 → look up skins[tone] from skinsMap; fall back to default
//     if the array is shorter than expected (defensive against dataset
//     irregularities).
// Used both at pick-time (emit) and at render-time (text fallback path,
// which kicks in whenever tone !== 0 because the sprite was rasterized in
// default tone only).
function tonedNative(emoji: EmojiItem): string {
  const t = currentTone()
  if (t === 0) return emoji.native
  const skins = skinsMap.get(emoji.id)
  return skins?.[t]?.native || emoji.native
}

function onEmojiClick(emoji: EmojiItem) {
  // Defer the single-click action so a follow-up dblclick can cancel it.
  // Without this, double-clicking would first fire the 'select' (copy +
  // toast) and only then 'dblselect' (paste) — the user sees a brief
  // "copied" toast right before the paste, which feels janky and also
  // races the system clipboard write inside PasteText.
  //
  // 220ms is just over the typical OS dblclick threshold (Windows default
  // 500ms is high but the WebView's native dblclick event itself fires
  // around ~200–300ms after the second mousedown in practice; pick a
  // value that beats most dblclicks but stays imperceptible for normal
  // single-clicks).
  if (clickTimer != null) {
    clearTimeout(clickTimer)
    clickTimer = null
  }
  pendingClick = emoji
  clickTimer = setTimeout(() => {
    clickTimer = null
    const e = pendingClick
    pendingClick = null
    if (!e) return
    emit('select', tonedNative(e))
    // Store the default-tone version in the "frequent" list so the row stays
    // stable across tone changes (otherwise switching tones would explode
    // the frequent list into tone-specific duplicates).
    addFrequent(e.native)
  }, 220)
}

function onEmojiDblClick(emoji: EmojiItem) {
  // Cancel the pending single-click (copy) — we want the dblclick path
  // to be the *only* action, not in addition to the click.
  if (clickTimer != null) {
    clearTimeout(clickTimer)
    clickTimer = null
  }
  pendingClick = null
  emit('dblselect', tonedNative(emoji))
  // Still record into "frequent" — a paste is at least as strong a
  // signal of usage as a copy.
  addFrequent(emoji.native)
}

// State for click/dblclick disambiguation. Defined at module scope (not
// reactive) — these are pure event-loop bookkeeping and re-rendering
// would only add overhead.
let clickTimer: ReturnType<typeof setTimeout> | null = null
let pendingClick: EmojiItem | null = null

// ─── Hover preview tooltip ───────────────────────────────────────
// Shows a large glyph + name when the pointer dwells on a cell for a
// moment. Teleported to <body> with position:fixed so it can escape the
// picker's `overflow: hidden` root and overlap the window chrome if
// needed. Lifecycle:
//   - pointerenter on cell → start a delay timer
//   - pointerleave / click / scroll → hide immediately, clear timer
//   - while visible, a second pointerenter on a *different* cell
//     re-anchors and refreshes content without re-delaying (sticky
//     hover, same UX as native browser tooltips on a toolbar)
interface TipBg {
  // Asset URL for the hi-res SVG preview. Resolved lazily from the
  // build-time manifest + Vite glob on first hover. When present, the
  // tip renders as an `<img :src>` (vector, crisp at any size); when
  // null, it falls back to the OS color-emoji font.
  svgUrl: string
}
interface TipState {
  glyph: string
  name: string
  // Hi-res SVG URL for the big preview. Null when the emoji has no
  // matched Fluent SVG (defensive; every visible emoji should have one
  // since we hide missingIds from the grid).
  bg: TipBg | null
  // Anchor rect of the hovered cell in viewport coords; used by the
  // post-mount nextTick to flip above/below based on actual tip size.
  anchor: { left: number; top: number; right: number; bottom: number }
  // Final computed tip position (updated after the tip element mounts
  // and we can read its bounding rect for edge clamping).
  pos: { left: number; top: number }
}
const tip = shallowRef<TipState | null>(null)
const tipRef = ref<HTMLDivElement | null>(null)
let tipShowTimer: ReturnType<typeof setTimeout> | null = null
const TIP_DELAY_MS = 350
const TIP_MARGIN = 8 // px gap between anchor and tip
// Pixel size of the big preview glyph inside the tip. Now rendered via
// an `<img>` element using a vector SVG, so this is just the CSS box
// size — no upscale softness. Keep in sync with .ep-tip-glyph in CSS.
const TIP_GLYPH_SIZE = 96

function clearTipTimer() {
  if (tipShowTimer != null) {
    clearTimeout(tipShowTimer)
    tipShowTimer = null
  }
}

function hideTip() {
  clearTipTimer()
  if (tip.value) tip.value = null
}

// Resolve the hi-res SVG asset URL for a given emoji + tone. Returns
// the URL string synchronously (URLs are resolved at module init via
// eager glob) or null if no SVG exists for this emoji.
function tipSvgUrlFor(emoji: EmojiItem): string | null {
  const tone = tonedIds.has(emoji.id) ? currentTone() : 0
  const key = `${emoji.id}__${tone}`
  const stem = (svgManifest as Record<string, string>)[key]
  if (!stem) return null
  return svgUrls.get(stem) ?? null
}

// Open (or refresh) the hover tip for a cell. If a tip is already
// visible we skip the delay — feels instant when sliding across the
// grid, matching OS-native tooltip behavior.
function onCellPointerEnter(e: PointerEvent, emoji: EmojiItem) {
  // Ignore touch — tap-to-open-tip doesn't make sense on touch devices
  // (there's no hover) and would fight with click.
  if (e.pointerType === 'touch') return
  const el = e.currentTarget as HTMLElement
  const r = el.getBoundingClientRect()
  const anchor = { left: r.left, top: r.top, right: r.right, bottom: r.bottom }
  const glyph = tonedNative(emoji)
  const name = emoji.name
  const already = tip.value != null
  clearTipTimer()

  const show = () => {
    const svgUrl = tipSvgUrlFor(emoji)
    const bg: TipBg | null = svgUrl ? { svgUrl } : null
    tip.value = {
      glyph,
      name,
      bg,
      anchor,
      // Initial guess; nextTick will clamp/flip after the tip measures.
      pos: { left: anchor.left, top: anchor.top - TIP_MARGIN },
    }
    nextTick(() => positionTip())
  }
  if (already) show()
  else tipShowTimer = setTimeout(show, TIP_DELAY_MS)
}

function onCellPointerLeave() {
  hideTip()
}

// Measure tip, then clamp to viewport. Preferred placement is centered
// above the anchor; if that would clip the top, flip below.
function positionTip() {
  const el = tipRef.value
  const t = tip.value
  if (!el || !t) return
  const tw = el.offsetWidth
  const th = el.offsetHeight
  const a = t.anchor
  const cx = (a.left + a.right) / 2
  let left = cx - tw / 2
  let top = a.top - th - TIP_MARGIN
  // Flip below if not enough room above
  if (top < 4) top = a.bottom + TIP_MARGIN
  // Horizontal clamp
  const maxLeft = window.innerWidth - tw - 4
  if (left < 4) left = 4
  else if (left > maxLeft) left = maxLeft
  tip.value = { ...t, pos: { left, top } }
}

onBeforeUnmount(() => {
  clearTipTimer()
  if (clickTimer != null) {
    clearTimeout(clickTimer)
    clickTimer = null
  }
})

// Expose to parent (App.vue) so the global keydown handler can drive
// category switching while keeping all emoji-picker state encapsulated here.
//
// The old surface also exposed `spriteReady` to drive a pulse indicator on
// the emoji tab button while the tone-0 sprite was being built in the
// background. Sprites are now pre-bundled WebP imports — there is no
// build phase and nothing for the parent to wait on — so that flag was
// dropped along with the indicator in App.vue.
defineExpose({ cycleCategory })
</script>

<template>
  <div class="ep-root" v-if="loaded">
    <!-- Navigation bar -->
    <div class="ep-nav">
      <button
        v-for="cat in visibleCategories"
        :key="cat.id"
        :class="{ active: activeCategory === cat.id }"
        :title="tt(cat.id)"
        @click="scrollToCategory(cat.id)"
        @mousedown.prevent
      >
        <span class="ep-nav-icon">{{ categoryIcons[cat.id] || '📁' }}</span>
      </button>
      <!-- Skin tone picker. Sits at the far right of the nav, separated from
           category buttons by a vertical divider (see .ep-tone-wrap::before)
           because it's a tool, not a category. Clicking the dot toggles a
           small flyout. The flyout is teleported to <body> with fixed
           positioning to escape `.ep-root { overflow: hidden }` clipping.
           Hidden in basic mode (extendedEmoji=false): the picker would be
           moot since rendering is forced to tone 0 there. -->
      <div v-if="extendedEmoji" class="ep-tone-wrap" @mousedown.stop>
        <button
          ref="toneBtnRef"
          class="ep-tone-btn"
          :title="tt('skinTone')"
          :aria-expanded="showTonePicker"
          @click="toggleTonePicker"
          @mousedown.prevent
        >
          <span
            class="ep-tone-dot"
            :style="{ background: TONE_COLORS[skinTone] }"
          ></span>
        </button>
        <Teleport to="body">
          <div
            v-if="showTonePicker"
            class="ep-tone-popup"
            :style="{ top: tonePopupPos.top + 'px', right: tonePopupPos.right + 'px' }"
            @mousedown.stop
          >
            <button
              v-for="(color, idx) in TONE_COLORS"
              :key="idx"
              class="ep-tone-choice"
              :class="{ active: skinTone === idx }"
              :style="{ background: color }"
              :title="tt('skinTone') + ' ' + idx"
              @click="setSkinTone(idx as SkinTone)"
              @mousedown.prevent
            ></button>
          </div>
        </Teleport>
      </div>
    </div>

    <!-- Search bar removed: the emoji picker now reuses the app-wide top
         search input (App.vue "topbar") whenever view === 'emoji'. This
         avoids two stacked search boxes and gives the user a single entry
         point regardless of which tab they're on. The search string flows
         in via the `search` prop and drives filteredCategories below. -->

    <!-- Emoji virtual scroll area -->
    <div class="ep-scroll" ref="scrollRef" @scroll="onScroll"
         @pointerleave="hideTip">
      <div class="ep-scroll-spacer" :style="{ height: totalHeight + 'px' }">
        <div
          class="ep-scroll-window"
          :style="{ top: visibleSlice.offsetY + 'px' }"
        >
          <template v-for="row in renderedRows" :key="row.type === 'header' ? 'h:' + row.catId : 'r:' + row.catId + ':' + (row.emojis![0]?.id || '')">
            <!-- Category header -->
            <div
              v-if="row.type === 'header'"
              class="emoji-cat-header"
              :style="{ height: HEADER_HEIGHT + 'px' }"
            >
              {{ row.label }}
            </div>
            <!-- Emoji row -->
            <div
              v-else
              class="ep-emoji-row"
              :class="{ 'ep-emoji-row--full': row.emojis!.length === emojisPerRow }"
              :style="{ height: ROW_HEIGHT + 'px' }"
            >
              <span
                v-for="emoji in row.emojis"
                :key="emoji.id"
                class="ep-emoji-btn"
                :class="{ 'ep-emoji-sprite': !!cellStyle(emoji) }"
                :style="cellStyle(emoji)"
                @click="onEmojiClick(emoji); hideTip()"
                @dblclick="onEmojiDblClick(emoji); hideTip()"
                @mousedown.prevent
                @pointerenter="onCellPointerEnter($event, emoji)"
                @pointerleave="onCellPointerLeave"
              >{{ cellStyle(emoji) ? '' : tonedNative(emoji) }}</span>
            </div>
          </template>
        </div>
      </div>
      <!-- No results -->
      <div v-if="virtualRows.length === 0 && searchQuery" class="ep-no-results">
        <span class="ep-no-results-emoji">😢</span>
        <span>{{ tt('noResults') }}</span>
      </div>
    </div>

    <!-- Back-to-top floating button. Mirrors App.vue's `.back-top` —
         same 36px circle, same 200px scrollTop threshold, same fade
         transition. Positioned absolute (not fixed) inside .ep-root so
         it stays inside the picker viewport even when the emoji view
         is rendered alongside other panels. -->
    <Transition name="ep-fade">
      <button
        v-if="showBackTop"
        class="ep-back-top"
        :title="gt('backTop')"
        @click="scrollToTop"
        @mousedown.prevent
      >
        <ArrowUp :size="18" />
      </button>
    </Transition>

    <!-- Hover preview tooltip. Teleported to <body> so it escapes the
         picker's overflow:hidden clipping; fixed-positioned and clamped
         to the viewport by positionTip(). The big preview renders the
         matched Fluent SVG via an <img> element (vector, crisp at any
         size) instead of the sprite bitmap. If the emoji somehow isn't
         in the SVG manifest we fall back to the OS color-emoji font
         glyph — the .fallback class re-enables the font sizing that's
         normally suppressed for sprite mode. -->
    <Teleport to="body">
      <div
        v-if="tip"
        ref="tipRef"
        class="ep-tip"
        :style="{ left: tip.pos.left + 'px', top: tip.pos.top + 'px' }"
      >
        <img
          v-if="tip.bg"
          class="ep-tip-glyph"
          :src="tip.bg.svgUrl"
          :alt="tip.glyph"
          :aria-label="tip.glyph"
          role="img"
        >
        <div v-else class="ep-tip-glyph fallback">{{ tip.glyph }}</div>
        <div class="ep-tip-name">{{ tip.name }}</div>
      </div>
    </Teleport>
  </div>
  <div v-else class="ep-loading">
    <span class="ep-spinner"></span>
  </div>
</template>

<style scoped>
/* ─── Root container ─────────────────────────────────────── */
.ep-root {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  overflow: hidden;
  /* Anchor for the absolute-positioned .ep-back-top button so it stays
     inside the picker viewport regardless of where this view is mounted
     in the outer app shell. */
  position: relative;
}

/* ─── Back-to-top floating button ────────────────────────────
   History of iterations (kept for context — please don't re-do them):
   1. absolute inside .ep-root @ right:18 bottom:16
      → fine when the last row was full, but if a category ended on a
        partial row the button looked like it sat in dead space to the
        right of (not on top of) emoji content.
   2. fixed to viewport @ right:20 bottom:40 (matched App.vue .back-top)
      → looked aligned at narrow widths, but as the window grew wider,
        emojisPerRow = floor((containerWidth - 20) / 36) leaves up to
        ~35 px of slack to the right of the last column. The button,
        anchored 20 px from the *window* edge, drifted into that slack
        band — visually disconnected from the emoji grid again.
   3. (current) absolute inside .ep-root @ right:18 bottom:16
      → this is the original layout, brought back deliberately. The
        button hovers at the picker's right-inside edge, just left of
        the 12 px scrollbar (18 = 12 scrollbar + 6 visual gap), so it
        sits directly above the emoji grid no matter how the window is
        resized. The "partial last row" concern from iteration 1 is
        a non-issue: the button is always visually inside the picker
        viewport on top of *something*, and if the last visible row is
        partial, that's fine — it's still hovering over the scroll
        viewport, not stranded outside it.
   Note: .ep-root sets `position: relative` to anchor this absolute. */
.ep-back-top {
  position: absolute;
  right: 18px;
  bottom: 16px;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--border);
  border: 1px solid var(--border);
  color: var(--text);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
  transition: all 0.2s;
  z-index: 50;
}
.ep-back-top:hover {
  background: var(--accent);
  border-color: #3b82f6;
}
.ep-fade-enter-active,
.ep-fade-leave-active {
  transition: opacity 0.2s;
}
.ep-fade-enter-from,
.ep-fade-leave-to {
  opacity: 0;
}

/* ─── Navigation bar ─────────────────────────────────────── */
.ep-nav {
  display: flex;
  align-items: center;
  padding: 0 8px;
  height: 40px;
  flex-shrink: 0;
  border-bottom: 1px solid var(--border);
}

.ep-nav button {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
  position: relative;
}

.ep-nav-icon {
  font-size: 18px;
  line-height: 1;
  opacity: 0.5;
}

.ep-nav button:hover .ep-nav-icon {
  opacity: 0.8;
}

.ep-nav button.active .ep-nav-icon {
  opacity: 1;
}

.ep-nav button.active::after {
  content: '';
  position: absolute;
  bottom: 0;
  left: 25%;
  right: 25%;
  height: 3px;
  background: var(--accent);
  border-radius: 3px 3px 0 0;
}

/* ─── Skin tone picker ───────────────────────────────────────
   The tone button is a wrapper (not a raw .ep-nav button) so the
   `.ep-nav button { flex: 1 }` rule doesn't stretch it across a whole
   slot. Wrapper is flex: 0 0 auto, with a subtle left divider that
   separates the tool from category buttons. The flyout is absolute-
   positioned below, anchored to the wrapper. */
.ep-tone-wrap {
  position: relative;
  flex: 0 0 auto;
  height: 100%;
  display: flex;
  align-items: center;
  padding: 0 6px 0 10px;
  margin-left: 4px;
}
.ep-tone-wrap::before {
  content: '';
  position: absolute;
  left: 0;
  top: 8px;
  bottom: 8px;
  width: 1px;
  background: var(--border);
}
.ep-tone-wrap .ep-tone-btn {
  /* Override `.ep-nav button { flex: 1 }` - the tone button must stay
     a compact circular control, not stretch like category buttons. */
  flex: 0 0 auto;
  width: 24px;
  height: 24px;
  padding: 0;
  border-radius: 50%;
  background: none;
  border: 1px solid var(--border);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: border-color .12s;
}
.ep-tone-wrap .ep-tone-btn:hover {
  border-color: var(--accent);
}
.ep-tone-dot {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  display: block;
  box-shadow: inset 0 0 0 1px rgba(0,0,0,0.08);
}
/* NOTE: .ep-tone-popup styles live in the non-scoped <style> block below.
   The popup is teleported to <body> and Vue's scoped attribute selectors
   don't propagate to teleported content reliably. Keeping popup styles
   global is simpler than chasing :deep() quirks. */
.ep-tone-choice.active {
  border-color: var(--accent);
}

/* ─── Search bar ─────────────────────────────────────────── */
.ep-search-wrap {
  padding: 8px 10px;
  flex-shrink: 0;
}

.ep-search-box {
  display: flex;
  align-items: center;
  gap: 6px;
  background: var(--bg-elevated);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0 10px;
  height: 34px;
}

.ep-search-box:focus-within {
  border-color: var(--accent);
}

.ep-search-icon {
  color: var(--text-muted);
  flex-shrink: 0;
  width: 16px;
  height: 16px;
}

.ep-search-box input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: var(--text);
  font-size: 13px;
  line-height: 34px;
  padding: 0;
}

.ep-search-box input::placeholder {
  color: var(--text-muted);
}

.ep-search-clear {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  background: var(--text-muted);
  border: none;
  border-radius: 50%;
  cursor: pointer;
  color: var(--bg-elevated);
  padding: 0;
  opacity: 0.7;
}
.ep-search-clear:hover {
  opacity: 1;
}

/* ─── Scroll area (virtual) ──────────────────────────────── */
.ep-scroll {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  position: relative;
}

.ep-scroll::-webkit-scrollbar {
  /* Bumped from 6px → 12px: 6px was technically draggable but practically
     impossible to grab without precision aiming. 12px aligns with the
     "comfortable but not chunky" tier used by VSCode / GitHub / most
     modern desktop UIs. */
  width: 12px;
}
.ep-scroll::-webkit-scrollbar-track {
  background: transparent;
}
.ep-scroll::-webkit-scrollbar-thumb {
  background: var(--border);
  /* Use a transparent inner border + background-clip to inset the visible
     thumb inside the 12px track. Result: 12px hit-area but the colored
     pill is ~6px wide, so the bar still looks slim while being twice as
     easy to grab. On hover we shrink the inset to make the thumb feel
     "thicker" as a subtle affordance. */
  background-clip: padding-box;
  border: 3px solid transparent;
  border-radius: 6px;
}
.ep-scroll::-webkit-scrollbar-thumb:hover {
  background: var(--text-muted);
  background-clip: padding-box;
  border: 2px solid transparent;
}

.ep-scroll-spacer {
  position: relative;
  width: 100%;
}

.ep-scroll-window {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  padding: 0 10px;
}

/* ─── Category header ────────────────────────────────────── */
.emoji-cat-header {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
  padding: 10px 4px 6px;
  display: flex;
  align-items: center;
  line-height: 1;
}

/* ─── Emoji row ──────────────────────────────────────────── */
.ep-emoji-row {
  display: flex;
  align-items: center;
  /* Default = partial (last) row of a category: pack tight to the left.
     Stretching e.g. 8 emojis across the full width would leave huge ugly
     gaps (see "flowers/leaves" tail row). Full rows override this below. */
  justify-content: flex-start;
  contain: layout paint style;
}

/* Full rows fill the entire row width by distributing slack between cells.
   Why: emojisPerRow = floor((containerWidth - 20) / 36) leaves up to ~35 px
   of slack at the right edge. With flex-start, that slack manifested as a
   visible gap between the rightmost emoji column and the scrollbar — which
   made the floating .ep-back-top button (anchored to the picker viewport's
   right edge) look "detached" from the emoji grid as the window resized.
   With space-between, the slack is silently absorbed into inter-cell
   spacing (a fraction of a pixel per gap, imperceptible), the rightmost
   column hugs the right edge like the main clipboard list does, and
   .ep-back-top sits cleanly on top of the last column at any width. */
/* Full row: same flex-start as partial rows. We deliberately do NOT use
   `space-between` here anymore. Earlier iterations did, to absorb the
   right-edge slack (containerWidth - emojisPerRow * EMOJI_SIZE, up to
   ~EMOJI_SIZE-1 px) into invisibly-distributed inter-cell spacing so the
   rightmost column hugged the scrollbar. The cost was a real, perceivable
   gap between every cell — exactly what we wanted to eliminate to match
   the dense reference layout. With cells now at 24 px and flex-start the
   gap is a hard 0 px between siblings; the residual slack lives as a
   single quiet stripe at the right edge of the row, which is fine — it's
   smaller than one cell and the floating .ep-back-top button still reads
   as anchored to the picker viewport (right: 18px) rather than to the
   emoji column. */
.ep-emoji-row--full {
  justify-content: flex-start;
}

.ep-emoji-btn {
  display: inline-block;
  /* 36 px cell ≫ 28 px glyph — the 4 px ring of empty space on each
     side IS the inter-cell breathing room. Cells themselves still butt
     up flush (flex-start, no flex gap), so the visible separation
     between adjacent emoji is exactly 2 × 4 = 8 px. Box-sizing:
     border-box so the padding doesn't push width past 36. */
  width: 36px;
  height: 36px;
  padding: 4px;
  box-sizing: border-box;
  /* IMPORTANT: do NOT round the cell with `border-radius: 50%`. The
     fluent sprites for square shapes (white/black squares, diamonds,
     flags, …) are intentionally drawn as full rectangular glyphs that
     extend to the cell edges; rounding the cell visually clips their
     four corners and makes them look "trimmed". The hover disc is
     instead painted via a radial-gradient on the background-color
     layer (see :hover rule below), which gives us a circular hover
     affordance without imposing any clipping on the sprite itself.
     Position relative is needed because the hover disc is layered via
     additional background, and we want the cell to establish its own
     stacking context for any future overlays. */
  position: relative;
  font-size: 18px;
  line-height: 28px;
  text-align: center;
  cursor: pointer;
  user-select: none;
  font-family: 'Apple Color Emoji', 'Segoe UI Emoji', 'Noto Color Emoji', sans-serif;
  /* NOTE: do NOT add `will-change` or `contain` here. With ~150 emoji spans in
     the viewport, those would promote each span to its own compositor layer
     (150+ layers) and cause severe jank on Windows Chromium/WebView2. */
}

/* Sprite-rendered variant: after the spritesheet is ready we swap every cell
   from a text glyph to a bitmap blit. This bypasses the expensive DirectWrite
   color-emoji raster path, which was the true bottleneck of tab-switch jank
   (GPU/raster pipeline was saturated on every tile invalidation).

   `background-origin: content-box` makes the 28×28 sprite tile align to the
   cell's content-box (28×28 after 4 px padding), centering the glyph inside
   the 36×36 cell. `background-clip: content-box` clips the sprite to that
   inner box so neighbor sprite cells don't bleed into the 4 px padding ring
   even though `background-size` is much bigger than the cell. The hover
   disc is painted on a separate background layer (radial-gradient) — see
   :hover rule below — so the sprite stays unclipped at all times. */
.ep-emoji-btn.ep-emoji-sprite {
  background-repeat: no-repeat;
  background-origin: content-box;
  background-clip: content-box;
  /* Let the GPU compositor smoothly downscale the HiDPR sprite back to CSS px. */
  image-rendering: auto;
  /* Hide any residual text (font-size 0 to be safe even though v-text is ''). */
  font-size: 0;
  color: transparent;
}

/* Hover state: paint a soft gray *disc* behind the emoji using a radial-
   gradient as a second background layer. We can't reuse `border-radius:
   50%` for this because that would clip the sprite itself — and the
   fluent sprite includes square / rectangular glyphs (white/black
   squares, diamonds, flags) that must keep their corners intact (see
   user feedback "表情图片不要圆角处理啊，本来是方形的，现在看起来像是
   四个角裁剪了似的").

   Two-layer approach:
     - layer 1 (top, listed first): the sprite, sourced from the
       `--ep-sprite` custom property that cellStyle() sets in the inline
       style. Clipped to content-box (28×28) so neighbors don't bleed.
     - layer 2 (bottom): a radial-gradient drawing an opaque circle of
       `var(--bg-hover)` out to ~47% of the cell (≈ 17 px radius from
       the 36 px cell's center → ~34 px diameter disc with a 1 px
       hairline of breathing room before the cell edge). Clipped to
       border-box (full 36 × 36). Transparent beyond the disc so square
       sprite glyphs remain unclipped at the cell's actual corners.

   We use `!important` on this rule because the inline-style
   `background-image: url(...)` set by cellStyle() would otherwise win
   the cascade — and we explicitly want the stylesheet to compose a
   two-layer list at hover time using the `--ep-sprite` token. The
   `background-clip` / `background-origin` values are matched 1:1 to
   the layer order: sprite → content-box, disc → border-box. */
.ep-emoji-btn.ep-emoji-sprite:hover {
  background-image:
    var(--ep-sprite),
    radial-gradient(circle at center, var(--bg-hover) 0 47%, transparent 49%) !important;
  background-clip: content-box, border-box !important;
  background-origin: content-box, border-box !important;
}
/* Non-sprite (text glyph) fallback: just a flat disc since there's no
   sprite layer to preserve. Same gradient technique, no clipping. */
.ep-emoji-btn:not(.ep-emoji-sprite):hover {
  background-image:
    radial-gradient(circle at center, var(--bg-hover) 0 47%, transparent 49%);
}

.ep-emoji-btn:active {
  transform: scale(0.9);
}

/* ─── No results ─────────────────────────────────────────── */
.ep-no-results {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 40px 20px;
  color: var(--text-muted);
  font-size: 13px;
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
}
.ep-no-results-emoji {
  font-size: 36px;
}

/* ─── Loading state ──────────────────────────────────────── */
.ep-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
}
.ep-spinner {
  width: 24px;
  height: 24px;
  border: 2px solid var(--border);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: ep-spin 0.6s linear infinite;
}
@keyframes ep-spin {
  to { transform: rotate(360deg); }
}
</style>

<!--
  Non-scoped style block for the skin-tone popup. The popup is teleported
  to <body> via <Teleport>, which moves it out of this component's DOM
  subtree. Vue's `scoped` styles work via auto-injected `[data-v-xxx]`
  attribute selectors on rendered elements, but teleported nodes don't
  receive those attributes consistently. Putting the popup's styles here
  (global) sidesteps that brittleness entirely.

  Class names are still namespaced with the `ep-` prefix to avoid
  colliding with anything else in the app.
-->
<style>
.ep-tone-popup {
  position: fixed;
  display: flex;
  gap: 4px;
  padding: 6px;
  background: var(--bg-elevated, var(--bg, #fff));
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0,0,0,0.12);
  z-index: 1000;
}
.ep-tone-choice {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 2px solid transparent;
  padding: 0;
  cursor: pointer;
  box-shadow: inset 0 0 0 1px rgba(0,0,0,0.08);
  transition: transform .08s, border-color .12s;
}
.ep-tone-choice:hover {
  transform: scale(1.1);
}
.ep-tone-choice.active {
  border-color: var(--accent);
}

/* Hover preview tooltip. Same rationale as .ep-tone-popup above for
   living in this non-scoped block: it's teleported to <body>.
   pointer-events: none so the tip never intercepts the mouse — this
   matters because the tip is positioned over the grid and could
   otherwise cause pointerleave/enter thrash when the cursor crosses it. */
.ep-tip {
  position: fixed;
  z-index: 1001;
  pointer-events: none;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 10px 14px;
  min-width: 96px;
  max-width: 240px;
  background: var(--bg-elevated, var(--bg, #fff));
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: 0 6px 20px rgba(0,0,0,0.18);
  /* Slight fade-in; the tip only appears after a 350ms dwell so there's
     no flicker when briefly sliding across cells. */
  animation: ep-tip-in .12s ease-out;
}
@keyframes ep-tip-in {
  from { opacity: 0; transform: translateY(2px); }
  to { opacity: 1; transform: translateY(0); }
}
.ep-tip-glyph {
  /* Hi-res SVG preview rendered as an <img>. The vector source renders
     at device-native resolution so it's crisp at any size — no more
     2× bilinear upscale softness from the 48 px sprite bitmap. */
  width: 96px;
  height: 96px;
  object-fit: contain;
}
.ep-tip-glyph.fallback {
  /* Defensive fallback when the emoji isn't in the sprite index (e.g. a
     future emoji-mart entry that the build script hasn't covered yet).
     Reset the box to inline-text geometry and let the OS color-emoji
     font render the glyph. */
  width: auto;
  height: auto;
  background: none;
  font-size: 96px;
  line-height: 1;
}
.ep-tip-name {
  font-size: 12px;
  color: var(--text);
  text-align: center;
  word-break: break-word;
  /* :first-letter capitalize is fine — emoji-mart names are lowercase. */
  text-transform: capitalize;
}
</style>
