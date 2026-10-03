const fs = require('node:fs')
const path = require('node:path')

// Preserve the old src/images/**/* target, including the flattened aliases
// produced when the glob matched both a directory and its descendants.
function collectImageAssets(projectRoot) {
  const root = path.resolve(projectRoot, 'src/images')
  if (fs.realpathSync(root) !== root) {
    throw new Error('admin images must not use symlinks')
  }
  const assets = new Map()
  const add = (name, source) => {
    const previous = assets.get(name)
    if (previous && previous !== source) {
      throw new Error(`ambiguous admin image output: ${name}`)
    }
    assets.set(name, source)
  }
  const walk = (directory, parts = []) => {
    for (const entry of fs
      .readdirSync(directory, { withFileTypes: true })
      .sort((a, b) => a.name.localeCompare(b.name))) {
      const source = path.join(directory, entry.name)
      if (entry.isSymbolicLink() || entry.name.includes('\\')) {
        throw new Error(`unsupported admin image path: ${source}`)
      }
      const relative = [...parts, entry.name]
      if (entry.isDirectory()) {
        walk(source, relative)
      } else if (entry.isFile()) {
        for (let index = 0; index < relative.length; index++) {
          add(`images/${relative.slice(index).join('/')}`, source)
        }
      } else {
        throw new Error(`admin image is not a regular file: ${source}`)
      }
    }
  }
  walk(root)
  const favicon = path.join(root, 'favicon.ico')
  if (assets.get('images/favicon.ico') !== favicon) {
    throw new Error('admin images require src/images/favicon.ico')
  }
  add('favicon.ico', favicon)
  return assets
}

function adminImageAssets(projectRoot) {
  let base = '/'
  return {
    name: 'admin-image-assets',
    configResolved(config) {
      base = config.base
    },
    buildStart() {
      this.addWatchFile(path.resolve(projectRoot, 'src/images'))
      for (const source of collectImageAssets(projectRoot).values()) this.addWatchFile(source)
    },
    generateBundle(_options, bundle) {
      for (const [fileName, source] of collectImageAssets(projectRoot)) {
        if (bundle[fileName]) throw new Error(`ambiguous admin image output: ${fileName}`)
        this.emitFile({ type: 'asset', fileName, source: fs.readFileSync(source) })
      }
    },
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        if (!['GET', 'HEAD'].includes(request.method)) return next()
        const [pathname, ...query] = (request.url || '').split('?')
        let decoded
        try {
          decoded = decodeURIComponent(pathname)
        } catch {
          return next()
        }
        if (!decoded.startsWith(base)) return next()
        const asset = decoded.slice(base.length)
        if (asset !== 'favicon.ico' && !asset.startsWith('images/')) return next()
        // Match known output paths only. Never normalize a request traversal
        // into an allowed image, or open a path supplied by the requester.
        if (
          asset.includes('\\') ||
          asset.includes('\0') ||
          asset.split('/').some((part) => part === '.' || part === '..')
        ) {
          response.statusCode = 400
          return response.end()
        }
        try {
          const source = collectImageAssets(projectRoot).get(asset)
          if (!source) return next()
          const relative = path
            .relative(projectRoot, source)
            .split(path.sep)
            .map(encodeURIComponent)
            .join('/')
          request.url = `${base}${relative}${query.length ? `?${query.join('?')}` : ''}`
          // Vite owns serving, filesystem allow/deny rules, MIME, caching and HEAD.
          next()
        } catch (error) {
          next(error)
        }
      })
    },
  }
}

module.exports = { adminImageAssets, collectImageAssets }
