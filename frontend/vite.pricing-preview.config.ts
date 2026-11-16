import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
const snapshot: Plugin = { name: 'read-only-public-price-snapshot',
  async generateBundle() {
    this.emitFile({ type: 'asset', fileName: 'preview-data.json', source: await readFile(resolve(__dirname, '../../public-model-plaza.json')) })
  }, configureServer(server) {
  server.middlewares.use(async (req, res, next) => {
    if (req.url?.split('?')[0] !== '/preview-data.json') return next()
    if (req.method !== 'GET') { res.statusCode = 405; res.end('Read-only preview'); return }
    try { const data = await readFile(resolve(__dirname, '../../public-model-plaza.json'));
      res.setHeader('Content-Type', 'application/json; charset=utf-8'); res.setHeader('Cache-Control', 'no-store'); res.end(data) }
    catch { res.statusCode = 503; res.end('Public snapshot unavailable') }
  })
} }
export default defineConfig({ plugins: [vue(), snapshot], resolve: { alias: { '@': resolve(__dirname, 'src') } },
  server: { host: '127.0.0.1', port: 19130, strictPort: true, fs: { allow: [resolve(__dirname)] } },
  build: { outDir: '../../preview-dist', emptyOutDir: false, rollupOptions: { input: resolve(__dirname, 'pricing-preview.html') } } })
