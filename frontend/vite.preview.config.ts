import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'node:path'
// Isolated local UI preview. No production proxy, credentials or auth bypass in
// the regular application. Only preview.html imports the fixture adapter.
export default defineConfig({
  plugins: [vue()],
  resolve: { alias: { '@': resolve(__dirname, 'src'), 'vue-i18n': 'vue-i18n/dist/vue-i18n.runtime.esm-bundler.js' }, dedupe: ['vue', 'pinia', 'vue-router'] },
  define: { __INTLIFY_JIT_COMPILATION__: true },
  server: { host: '127.0.0.1', port: 4317, strictPort: true, fs: { allow: [resolve(__dirname), resolve(__dirname, '../../kitty-easypay-fix-20261007/frontend')] } },
  build: { outDir: '../preview-dist', rollupOptions: { input: resolve(__dirname, 'preview.html') } },
})
