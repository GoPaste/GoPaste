// 素材采集：主面板各状态的 3x 截图 + 元素坐标表（layout.json）
// 前置：node capture/serve.mjs 5288 已在运行（虚构数据替身，业务代码零改动）
// 用法：node capture/capture.mjs [--only=name1,name2] [--out=dir]
import path from 'node:path'
import fs from 'node:fs'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'
import { CHROME } from './browser.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
const args = Object.fromEntries(process.argv.slice(2).map((a) => a.replace(/^--/, '').split('=')))
const BASE = process.env.BASE || 'http://127.0.0.1:5288/'
const OUT = path.resolve(here, '..', args.out || 'public/textures/panel')
const only = args.only ? new Set(args.only.split(',')) : null
const DPR = 3
fs.mkdirSync(OUT, { recursive: true })

const typeInto = (text) => async (p) => { await p.fill('.search input', text); await p.waitForTimeout(200) }

// 每个状态：主题 + 视口高 + 进入页面后的一组操作 + 可选注入样式
const SHOTS = [
  { name: 'panel-dark', theme: 'dark', act: async () => {} },
  { name: 'panel-light', theme: 'light', act: async () => {} },
  { name: 'panel-dark-selected', theme: 'dark', act: async (p) => { await p.keyboard.press('ArrowDown') } },
  // 打字中间态：'发'（结果与'发布'相同，但输入框显示不同）
  { name: 'panel-dark-search1', theme: 'dark', act: typeInto('发') },
  { name: 'panel-dark-search', theme: 'dark', act: typeInto('发布') },
  { name: 'panel-dark-code', theme: 'dark', act: async (p) => { await p.click('.filters button:nth-child(7)') } },
  { name: 'panel-dark-code-selected', theme: 'dark', act: async (p) => { await p.click('.filters button:nth-child(7)'); await p.waitForTimeout(150); await p.keyboard.press('ArrowDown') } },
  // 全量列表（加高视口，所有行都渲染出来，供逐行裁片）
  { name: 'panel-dark-tall', theme: 'dark', height: 1500, act: async () => {} },
  // Tab 条：全部未激活（emoji 视图下内容 Tab 都不高亮；只裁 Tab 条使用）
  { name: 'tabs-inactive', theme: 'dark', act: async (p) => { await p.click('.filters .tab-emoji') } },
  // Tab 条：透明底 + 白字（叠在自绘的滑动 thumb 里面，做"滑过即点亮"）
  {
    name: 'tabs-white', theme: 'dark', act: async (p) => { await p.click('.filters .tab-emoji') },
    css: `.filters button{background:transparent!important;color:#ffffff!important;box-shadow:none!important;border-color:transparent!important}
          .app > *:not(.filters){visibility:hidden!important} .app{background:transparent!important} html,body{background:transparent!important}`,
    transparent: true,
  },
]

const BOXES = {
  topbar: '.topbar', search: '.search', searchInput: '.search input', filters: '.filters', list: '.list', statusbar: '.statusbar',
  tabs: '.filters button', items: '.list .item',
}

const browser = await chromium.launch({ executablePath: CHROME })
const layout = {}
for (const s of SHOTS) {
  if (only && !only.has(s.name)) continue
  const h = s.height || 680
  const ctx = await browser.newContext({ viewport: { width: 680, height: h }, deviceScaleFactor: DPR })
  const page = await ctx.newPage()
  // 冻结时钟：所有状态的"x 秒前"元信息完全一致，状态间交叉淡化时不闪字
  await page.clock.setFixedTime(new Date('2026-10-30T10:00:00+08:00'))
  page.on('pageerror', (e) => console.warn(`[${s.name}] pageerror:`, e.message))
  await page.goto(`${BASE}?theme=${s.theme}`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.list .item')
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(400)
  await s.act(page)
  if (s.css) await page.addStyleTag({ content: s.css })
  await page.mouse.move(-10, -10)
  await page.waitForTimeout(300)
  await page.screenshot({ path: path.join(OUT, `${s.name}.png`), omitBackground: !!s.transparent })
  const L = {}
  for (const [k, sel] of Object.entries(BOXES)) {
    L[k] = await page.$$eval(sel, (els) => els.map((e) => {
      const r = e.getBoundingClientRect()
      return { x: Math.round(r.x * 10) / 10, y: Math.round(r.y * 10) / 10, w: Math.round(r.width * 10) / 10, h: Math.round(r.height * 10) / 10 }
    }))
  }
  // 光标位置：输入框文字末端（canvas 按输入框实际字体量宽）
  L.caret = await page.$eval('.search input', (el) => {
    const cs = getComputedStyle(el)
    const c = document.createElement('canvas').getContext('2d')
    c.font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`
    const r = el.getBoundingClientRect()
    return { x: r.x + parseFloat(cs.paddingLeft) + c.measureText(el.value).width, y: r.y, h: r.height, fontSize: parseFloat(cs.fontSize) }
  })
  L.viewport = { w: 680, h }
  layout[s.name] = L
  console.log('shot', s.name)
  await ctx.close()
}
await browser.close()
const layoutPath = path.join(OUT, 'layout.json')
const prev = fs.existsSync(layoutPath) ? JSON.parse(fs.readFileSync(layoutPath, 'utf8')) : {}
fs.writeFileSync(layoutPath, JSON.stringify({ ...prev, ...layout, _meta: { dpr: DPR } }, null, 1))
