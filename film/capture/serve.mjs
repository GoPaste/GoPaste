// 以 GoPaste 前端源码为 root 启动 vite，并把 wailsjs 绑定替换为本目录的替身。
// 用法：node capture/serve.mjs [port]   （默认 5288）
// 业务代码零改动：替换只发生在本进程的 resolve.alias。
// 源码取自 git HEAD 的只读快照（film/.snapshot/frontend，由下面命令生成），
// 不读工作区 frontend/ —— 工作区可能正在进行 Wails v3 迁移，采集不能依赖未提交的中间态：
//   rm -rf .snapshot && mkdir .snapshot && git -C .. archive HEAD frontend | tar -x -C .snapshot
//   ln -s ../../../frontend/node_modules .snapshot/frontend/node_modules
import path from 'node:path'
import fs from 'node:fs'
import { fileURLToPath, pathToFileURL } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const filmRoot = path.resolve(here, '..')
const feRoot = path.resolve(filmRoot, '.snapshot/frontend')
const feMod = (p) => pathToFileURL(path.join(feRoot, 'node_modules', p)).href

const { createServer } = await import(feMod('vite/dist/node/index.js'))
const { default: vue } = await import(feMod('@vitejs/plugin-vue/dist/index.mjs'))

const port = Number(process.argv[2] || 5288)
const mockDir = path.join(here, 'mock')

const injectFonts = {
  name: 'gopaste-film-fonts',
  transformIndexHtml(html) {
    return html.replace('</head>', `<link rel="stylesheet" href="/@fs${path.join(mockDir, 'fonts.css')}"></head>`)
  },
}

const server = await createServer({
  configFile: false,
  root: feRoot,
  plugins: [vue(), injectFonts],
  resolve: {
    alias: [
      { find: /^(\.\.\/)+wailsjs\/go\/main\/App$/, replacement: path.join(mockDir, 'App.ts') },
      { find: /^(\.\.\/)+wailsjs\/runtime\/runtime$/, replacement: path.join(mockDir, 'runtime.ts') },
      { find: /^@fontsource\//, replacement: path.join(filmRoot, 'node_modules/@fontsource') + '/' },
      // frontend/node_modules 未装此依赖：从 film/ 提供，避免改动业务目录
      { find: /^@emoji-mart\/data/, replacement: path.join(filmRoot, 'node_modules/@emoji-mart/data') },
    ],
  },
  server: { port, strictPort: true, host: '127.0.0.1', fs: { allow: [feRoot, filmRoot, fs.realpathSync(path.join(feRoot, 'node_modules'))] } },
  logLevel: 'info',
})
await server.listen()
server.printUrls()
