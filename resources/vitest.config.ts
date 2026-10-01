import { fileURLToPath } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

const root = fileURLToPath(new URL('./', import.meta.url))
const clientRoot = fileURLToPath(new URL('./src/js', import.meta.url))

// Unit tests need Vue and the application aliases, but not the Vite development
// plugins that write public/hot or materialize a host extension bundle.
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': clientRoot,
      '~': clientRoot,
      '@admin-core': clientRoot,
    },
  },
  test: {
    root,
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{ts,tsx,js}'],
  },
})
