import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwind from '@tailwindcss/vite'
import { fileURLToPath } from 'node:url'
const root = fileURLToPath(new URL('./', import.meta.url))
const client = fileURLToPath(new URL('../../src/js', import.meta.url))
export default defineConfig({
  root,
  plugins: [vue(), tailwind()],
  resolve: { alias: { '@': client, '~': client } },
  server: { host: '127.0.0.1', port: 5184, strictPort: true },
  build: { outDir: './dist', emptyOutDir: true },
})
