import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
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
