// 复用本机 Playwright 缓存的 headless shell，避免重复下载浏览器。
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'

function findChrome() {
  if (process.env.CHROME_PATH) return process.env.CHROME_PATH
  const base = path.join(os.homedir(), '.cache/ms-playwright')
  const dirs = fs.existsSync(base) ? fs.readdirSync(base).filter((d) => d.startsWith('chromium')).sort().reverse() : []
  for (const d of dirs) {
    for (const rel of ['chrome-headless-shell-linux64/chrome-headless-shell', 'chrome-linux/chrome', 'chrome-linux64/chrome']) {
      const p = path.join(base, d, rel)
      if (fs.existsSync(p)) return p
    }
  }
  throw new Error('未找到 Chromium，请设置 CHROME_PATH')
}

export const CHROME = findChrome()
