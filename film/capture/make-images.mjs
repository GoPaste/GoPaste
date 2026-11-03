// 生成列表里"图片"条目使用的虚构插画（不使用任何真实照片），输出 mock/images/*.png
import path from 'node:path'
import fs from 'node:fs'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'
import { CHROME } from './browser.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
const outDir = path.join(here, 'mock/images')
fs.mkdirSync(outDir, { recursive: true })

const landscape = `<svg xmlns="http://www.w3.org/2000/svg" width="560" height="360" viewBox="0 0 560 360">
  <defs>
    <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#1e3a8a"/><stop offset=".55" stop-color="#6366f1"/><stop offset="1" stop-color="#f59e0b"/>
    </linearGradient>
    <linearGradient id="m1" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#312e81"/><stop offset="1" stop-color="#1e1b4b"/></linearGradient>
    <linearGradient id="m2" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#1e1b4b"/><stop offset="1" stop-color="#0b0f1a"/></linearGradient>
  </defs>
  <rect width="560" height="360" fill="url(#sky)"/>
  <circle cx="400" cy="210" r="46" fill="#fde68a" opacity=".95"/>
  <path d="M0 250 L90 170 L160 225 L250 140 L340 220 L420 175 L560 240 L560 360 L0 360Z" fill="url(#m1)"/>
  <path d="M0 300 L110 240 L210 290 L320 230 L430 285 L560 255 L560 360 L0 360Z" fill="url(#m2)"/>
</svg>`

const browser = await chromium.launch({ executablePath: CHROME })
const page = await browser.newPage({ viewport: { width: 560, height: 360 } })
await page.setContent(`<body style="margin:0">${landscape}</body>`)
await page.screenshot({ path: path.join(outDir, 'landscape.png') })
await browser.close()
console.log('wrote', path.join(outDir, 'landscape.png'))
