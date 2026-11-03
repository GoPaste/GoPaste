import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  // Wails v3 dev 模式：`wails3 dev` 通过 FRONTEND_DEVSERVER_URL 指向这个 dev server，
  // 端口由根 Taskfile 的 VITE_PORT（默认 9245）经 WAILS_VITE_PORT 传入，必须 strictPort。
  server: {
    host: '127.0.0.1',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [vue()],
  build: {
    // Prevent Vite from inlining small assets (like the ~700 B Fluent SVGs
    // used for hover previews) as data-URIs into the JS bundle. Without
    // this, the ~3000 emoji SVGs would balloon the main chunk by ~5 MB of
    // base64 data. As external files they're only fetched when the browser
    // actually renders an <img src="...">.
    assetsInlineLimit: 0,
  },
})
