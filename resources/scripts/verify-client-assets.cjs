const fs = require('node:fs')
const path = require('node:path')
const { adminPublicRoot } = require('./admin-paths.cjs')

const templatesRoot = path.resolve(__dirname, '../../views')
const assets = new Set(['js/app.js', 'css/app.css', 'favicon.ico'])
// A host may copy only resources/ from the pinned module before building.
// In the source checkout, also catch drift in the embedded templates.
for (const template of ['app.gohtml', 'error.gohtml']) {
  const file = path.join(templatesRoot, template)
  if (!fs.existsSync(file)) continue
  const source = fs.readFileSync(file, 'utf8')
  for (const [, asset] of source.matchAll(/{{\s*asset\s+"([^"]+)"\s*}}/g)) {
    assets.add(asset)
  }
}

for (const asset of assets) {
  const file = path.join(adminPublicRoot, 'dist', asset)
  let info
  try {
    info = fs.statSync(file)
  } catch {
    throw new Error(`admin template requires missing build asset: ${asset}`)
  }
  if (!info.isFile() || info.size === 0) {
    throw new Error(`admin template requires non-empty build asset: ${asset}`)
  }
}
console.log(`Verified ${assets.size} admin template assets`)
