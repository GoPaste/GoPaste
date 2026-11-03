// 截取 styleframe：每张 1920×1080 + 一张 2×2 对比总览（out/styleframes/）
// 用法：node styleframes/shoot.mjs
import path from 'node:path'
import fs from 'node:fs'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { chromium } from 'playwright-core'
import { CHROME } from '../capture/browser.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
const out = path.resolve(here, '../out/styleframes')
fs.mkdirSync(out, { recursive: true })

const iconDir = path.resolve(here, '../node_modules/lucide-static/icons')
const ICONS = Object.fromEntries(
  ['file-text', 'image', 'link', 'code-xml', 'file', 'star', 'pin', 'search', 'clipboard', 'smile', 'lock']
    .map((n) => [n, fs.readFileSync(path.join(iconDir, `${n}.svg`), 'utf8')]),
)
const FRAMES = fs.readdirSync(here).filter((f) => /^sf\d.*\.html$/.test(f)).sort()

const browser = await chromium.launch({ executablePath: CHROME, args: ['--allow-file-access-from-files'] })
const ctx = await browser.newContext({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 })
await ctx.addInitScript((icons) => { window.ICONS = icons }, ICONS)
const shots = []
for (const f of FRAMES) {
  const page = await ctx.newPage()
  page.on('pageerror', (e) => console.warn(f, e.message))
  await page.goto(pathToFileURL(path.join(here, f)).href, { waitUntil: 'load' })
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(300)
  const p = path.join(out, f.replace('.html', '.png'))
  await page.screenshot({ path: p })
  shots.push(p)
  console.log('shot', p)
  await page.close()
}
// 2×2 总览
const sheet = await ctx.newPage()
const imgs = shots.map((p) => `<img src="${pathToFileURL(p).href}">`).join('')
await sheet.setContent(`<body style="margin:0;background:#0b0c10;display:grid;grid-template-columns:952px 952px;gap:16px;padding:0;width:1920px">
  <style>img{width:952px;height:535.5px;display:block}</style>${imgs}</body>`)
await sheet.waitForTimeout(300)
await sheet.screenshot({ path: path.join(out, 'contact-sheet.png'), fullPage: true })
await browser.close()
console.log('sheet', path.join(out, 'contact-sheet.png'))
