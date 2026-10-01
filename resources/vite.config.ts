import { defineConfig } from 'vite'
import type { Plugin, ViteDevServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'
import { viteStaticCopy } from 'vite-plugin-static-copy'
import { fileURLToPath, URL } from 'node:url'
import { createRequire } from 'node:module'
import { existsSync, mkdirSync, writeFileSync } from 'fs'
import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'

// import laravel from 'laravel-vite-plugin'
// laravel({
//   input: ['src/js/app.js'],
//   refresh: ['src/**/*.vue', 'src/**/*.js', 'src/**/*.ts', 'src/**/*.css'],
//   publicDirectory: '../public',
//   hotFile: '../public/hot',
//   buildDirectory: 'dist',
// }),
const projectRoot = path.dirname(fileURLToPath(import.meta.url))
const require = createRequire(import.meta.url)
const {
  adminExtRoot,
  adminExtRootConfigured,
  adminExtensionManifestPaths,
  adminGeneratedRoot,
  adminPublicRoot,
} = require('./scripts/admin-paths.cjs') as {
  adminExtRoot: string
  adminExtRootConfigured: boolean
  adminExtensionManifestPaths: string[]
  adminGeneratedRoot: string
  adminPublicRoot: string
}
const { materializeAdminExtensions } = require('./scripts/materialize-admin-extensions.cjs') as {
  materializeAdminExtensions: (input: {
    generatedRoot: string
    manifestPaths: string[]
    legacyRoot: string
    includeLegacy: boolean
  }) => {
    bundleManifest: object
    generatedModulePath: string
    generatedRoot: string
    sourceRoots: string[]
  }
}

const materializedAdminExtensions = materializeAdminExtensions({
  generatedRoot: adminGeneratedRoot,
  manifestPaths: adminExtensionManifestPaths,
  legacyRoot: adminExtRoot,
  includeLegacy: adminExtRootConfigured,
})

const adminExtensionImporterRoots = [
  materializedAdminExtensions.generatedRoot,
  ...materializedAdminExtensions.sourceRoots,
].map((root) => path.resolve(root))
const adminHost = process.env.APP_DOMAIN ? `admin.${process.env.APP_DOMAIN}` : 'admin.localhost'
const wsAdminHost = process.env.APP_DOMAIN
  ? `wsadmin.${process.env.APP_DOMAIN}`
  : 'wsadmin.localhost'

// Простой плагин для создания hot файла
const hotFilePlugin = (): Plugin => ({
  name: 'hot-file-plugin',
  configureServer(server: ViteDevServer) {
    const protocol = server.config.server.https ? 'https' : 'http'
    const host = server.config.server.host || 'localhost'
    const port = server.config.server.port || 5173
    const hotUrl = `${protocol}://${host}:${port}`

    mkdirSync(adminPublicRoot, { recursive: true })
    // Создаем hot файл при запуске dev сервера
    writeFileSync(path.join(adminPublicRoot, 'hot'), hotUrl)
    console.log(`Hot file created: ${hotUrl}`)
  },
})

const adminExtNodeResolve = (): Plugin => ({
  name: 'admin-ext-node-resolve',
  async resolveId(source, importer) {
    if (!importer) return null
    if (source.startsWith('.') || source.startsWith('/') || source.startsWith('\u0000')) {
      return null
    }

    const importerPath = importer.split('?')[0]
    const isExtensionImport = adminExtensionImporterRoots.some(
      (root) => importerPath === root || importerPath.startsWith(root + path.sep),
    )
    if (!isExtensionImport) {
      return null
    }

    const fallbackImporter = path.join(projectRoot, 'src/js/app.ts')
    const resolved = await this.resolve(source, fallbackImporter, { skipSelf: true })
    return resolved || null
  },
})

const adminExtensionManifestPlugin = (): Plugin => ({
  name: 'admin-extension-manifest',
  generateBundle() {
    this.emitFile({
      type: 'asset',
      fileName: 'admin-extensions.manifest.json',
      source: `${JSON.stringify(materializedAdminExtensions.bundleManifest, null, 2)}\n`,
    })
  },
})

export default defineConfig({
  // Hosts mount the embedded bundle below /public/dist or their own asset prefix.
  base: './',
  plugins: [
    adminExtNodeResolve(),
    adminExtensionManifestPlugin(),
    vueDevTools(),
    hotFilePlugin(),
    tailwindcss(),
    vue({
      template: {
        transformAssetUrls: {
          base: null,
          includeAbsolute: false,
        },
      },
    }),
    viteStaticCopy({
      targets: [
        {
          src: 'src/images/**/*',
          dest: 'images',
        },
        {
          src: 'src/images/favicon.ico',
          dest: '.',
        },
      ],
    }),
  ],
  css: {
    postcss: './postcss.config.js',
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src/js', import.meta.url)),
      '~': fileURLToPath(new URL('./src/js', import.meta.url)),
      '@admin-core': fileURLToPath(new URL('./src/js', import.meta.url)),
      '@admin-extensions-generated': materializedAdminExtensions.generatedModulePath,
      '@admin-ext': (() => {
        const extSrc = path.join(adminExtRoot, 'src/js')
        if (existsSync(extSrc)) {
          return extSrc
        }
        return fileURLToPath(new URL('./src/js/empty-ext', import.meta.url))
      })(),
    },
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    allowedHosts: [adminHost, wsAdminHost, '.localhost', 'localhost'],
    hmr: {
      port: 5173,
      protocol: 'wss',
      clientPort: 443,
      host: wsAdminHost,
    },
    fs: {
      allow: [
        projectRoot,
        adminExtRoot,
        adminPublicRoot,
        materializedAdminExtensions.generatedRoot,
        ...materializedAdminExtensions.sourceRoots,
      ],
    },
    watch: {
      usePolling: true,
    },
    strictPort: true,
  },
  build: {
    outDir: path.join(adminPublicRoot, 'dist'),
    emptyOutDir: true,
    manifest: true, // Генерируем manifest.json
    rollupOptions: {
      input: 'src/js/app.ts', // Явно указываем entry point
      output: {
        entryFileNames: 'js/[name].js',
        chunkFileNames: 'js/[name]-[hash].js',
        assetFileNames: (assetInfo) => {
          if (assetInfo.names[0].endsWith('.css')) {
            return assetInfo.names[0] === 'app.css' ? 'css/app.css' : 'css/[name]-[hash][extname]'
          }
          if (/\.(png|jpe?g|svg|gif|ico|webp)$/i.test(assetInfo.names[0])) {
            return 'images/[name][extname]'
          }
          return 'assets/[name]-[hash][extname]'
        },
        manualChunks: (id) => {
          if (id.includes('node_modules')) {
            if (id.includes('@tiptap') || id.includes('prosemirror')) {
              return 'editor'
            }
            // Keep the rest of the dependency graph together. Aggressive
            // vendor splitting caused runtime cycles between `reka-ui`,
            // `@vueuse/core`, and Vue/Inertia chunks in production builds.
            return undefined
          }
        },
      },
    },
    copyPublicDir: false,
  },
})
