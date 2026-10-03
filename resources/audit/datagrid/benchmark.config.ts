import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'node:url'
const client = fileURLToPath(new URL('../../src/js', import.meta.url))
export default defineConfig({
  plugins: [vue()],
  resolve: { alias: { '@': client, '~': client } },
  test: {
    environment: 'jsdom',
    include: ['audit/datagrid/render.bench.test.ts'],
    testTimeout: 120000,
    fileParallelism: false,
  },
})
