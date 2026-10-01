const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')

const CLIENT_BUNDLE_FORMAT_VERSION = 1
const CLIENT_EXTENSION_API_VERSION = 1
const CORE_EXTENSION_KEY = 'goadmin.core'

const toPosix = (value) => value.split(path.sep).join('/')

const validKey = (value) => /^[a-z][a-z0-9._-]*$/.test(value)

const listFiles = (root) => {
  if (!fs.existsSync(root)) {
    return []
  }

  const files = []
  const visit = (dir) => {
    const entries = fs.readdirSync(dir, { withFileTypes: true }).sort((a, b) =>
      a.name.localeCompare(b.name),
    )
    for (const entry of entries) {
      const absolute = path.join(dir, entry.name)
      if (entry.isSymbolicLink()) {
        throw new Error(`admin extension source must not contain symlinks: ${absolute}`)
      }
      if (entry.isDirectory()) {
        visit(absolute)
        continue
      }
      if (entry.isFile()) {
        files.push(absolute)
      }
    }
  }

  visit(root)
  return files
}

const fingerprintSource = (sourceRoot) => {
  const hash = crypto.createHash('sha256')
  for (const file of listFiles(sourceRoot)) {
    hash.update(toPosix(path.relative(sourceRoot, file)))
    hash.update('\0')
    hash.update(fs.readFileSync(file))
    hash.update('\0')
  }
  return `sha256:${hash.digest('hex')}`
}

const parseManifest = (manifestPath) => {
  const absoluteManifestPath = path.resolve(manifestPath)
  let manifest
  try {
    manifest = JSON.parse(fs.readFileSync(absoluteManifestPath, 'utf8'))
  } catch (error) {
    throw new Error(`cannot read admin extension manifest ${absoluteManifestPath}: ${error.message}`)
  }

  if (manifest.formatVersion !== CLIENT_BUNDLE_FORMAT_VERSION) {
    throw new Error(
      `admin extension manifest ${absoluteManifestPath} uses formatVersion ${manifest.formatVersion}; expected ${CLIENT_BUNDLE_FORMAT_VERSION}`,
    )
  }
  if (!Array.isArray(manifest.extensions)) {
    throw new Error(`admin extension manifest ${absoluteManifestPath} must contain extensions[]`)
  }

  return manifest.extensions.map((raw) => {
    if (!raw || typeof raw !== 'object' || !validKey(raw.key)) {
      throw new Error(`admin extension manifest ${absoluteManifestPath} contains an invalid key`)
    }
    if (raw.apiVersion !== CLIENT_EXTENSION_API_VERSION) {
      throw new Error(
        `admin extension ${raw.key} uses apiVersion ${raw.apiVersion}; expected ${CLIENT_EXTENSION_API_VERSION}`,
      )
    }
    if (typeof raw.sourceRoot !== 'string' || raw.sourceRoot.trim() === '') {
      throw new Error(`admin extension ${raw.key} must declare sourceRoot`)
    }
    if (!Array.isArray(raw.requires ?? [])) {
      throw new Error(`admin extension ${raw.key} requires must be an array`)
    }

    const sourceRoot = path.resolve(path.dirname(absoluteManifestPath), raw.sourceRoot)
    const sourceInfo = fs.lstatSync(sourceRoot, { throwIfNoEntry: false })
    if (!sourceInfo?.isDirectory() || sourceInfo.isSymbolicLink()) {
      throw new Error(`admin extension ${raw.key} sourceRoot is not a directory: ${sourceRoot}`)
    }

    const requires = [...(raw.requires ?? [])]
    const seenRequires = new Set()
    for (const required of requires) {
      if (typeof required !== 'string' || !validKey(required)) {
        throw new Error(`admin extension ${raw.key} contains an invalid dependency key`)
      }
      if (seenRequires.has(required)) {
        throw new Error(`admin extension ${raw.key} contains duplicate dependency ${required}`)
      }
      seenRequires.add(required)
    }

    const fingerprint = fingerprintSource(sourceRoot)
    if (raw.fingerprint !== fingerprint) {
      throw new Error(
        `admin extension ${raw.key} fingerprint mismatch: manifest ${raw.fingerprint}, computed ${fingerprint}`,
      )
    }

    return {
      key: raw.key,
      apiVersion: raw.apiVersion,
      fingerprint,
      requires: requires.sort(),
      sourceRoot,
    }
  })
}

const legacyEntry = (legacyRoot) => {
  const sourceRoot = path.join(legacyRoot, 'src/js')
  if (!fs.statSync(sourceRoot, { throwIfNoEntry: false })?.isDirectory()) {
    return null
  }

  return {
    key: 'host',
    apiVersion: CLIENT_EXTENSION_API_VERSION,
    fingerprint: fingerprintSource(sourceRoot),
    requires: [],
    sourceRoot,
  }
}

const collectEntries = ({ manifestPaths, legacyRoot, includeLegacy }) => {
  const entries = manifestPaths.flatMap(parseManifest)
  if (includeLegacy || entries.length === 0) {
    const legacy = legacyEntry(legacyRoot)
    if (legacy) {
      entries.push(legacy)
    }
  }
  entries.sort((a, b) => a.key.localeCompare(b.key))

  const byKey = new Map()
  for (const entry of entries) {
    if (byKey.has(entry.key)) {
      throw new Error(`duplicate admin extension key: ${entry.key}`)
    }
    byKey.set(entry.key, entry)
  }
  for (const entry of entries) {
    for (const required of entry.requires) {
      if (required !== CORE_EXTENSION_KEY && !byKey.has(required)) {
        throw new Error(`admin extension ${entry.key} requires missing module ${required}`)
      }
    }
  }

  const visiting = new Set()
  const visited = new Set()
  const visit = (key, path) => {
    if (visiting.has(key)) {
      throw new Error(`admin extension dependency cycle: ${[...path, key].join(' -> ')}`)
    }
    if (visited.has(key)) return

    visiting.add(key)
    const entry = byKey.get(key)
    for (const required of entry.requires) {
      if (required !== CORE_EXTENSION_KEY) {
        visit(required, [...path, key])
      }
    }
    visiting.delete(key)
    visited.add(key)
  }
  for (const entry of entries) {
    visit(entry.key, [])
  }

  return entries
}

const buildGeneratedModule = (entries, generatedRoot) => {
  const imports = []
  const pages = []
  const modules = []
  const pageOwners = new Map()
  let importIndex = 0

  for (const entry of entries) {
    const linkedRoot = path.join(generatedRoot, 'roots', entry.key)
    fs.symlinkSync(entry.sourceRoot, linkedRoot, process.platform === 'win32' ? 'junction' : 'dir')

    const pageRoot = path.join(entry.sourceRoot, 'Pages')
    for (const file of listFiles(pageRoot).filter((value) => value.endsWith('.vue'))) {
      const pageKey = toPosix(path.relative(pageRoot, file)).replace(/\.vue$/, '')
      if (pageOwners.has(pageKey)) {
        throw new Error(
          `duplicate admin page key ${pageKey}: ${pageOwners.get(pageKey)} and ${entry.key}`,
        )
      }
      pageOwners.set(pageKey, entry.key)

      const specifier = `./roots/${entry.key}/Pages/${toPosix(path.relative(pageRoot, file))}`
      pages.push(`  ${JSON.stringify(pageKey)}: () => import(${JSON.stringify(specifier)}),`)
    }

    const extensionsRoot = path.join(entry.sourceRoot, 'extensions')
    for (const file of listFiles(extensionsRoot).filter(
      (value) => value.endsWith('.ts') && !value.endsWith('.d.ts'),
    )) {
      const moduleKey = `${entry.key}/${toPosix(path.relative(extensionsRoot, file)).replace(/\.ts$/, '')}`
      const identifier = `extension${importIndex++}`
      const specifier = `./roots/${entry.key}/extensions/${toPosix(path.relative(extensionsRoot, file))}`
      imports.push(`import * as ${identifier} from ${JSON.stringify(specifier)}`)
      modules.push(`  ${JSON.stringify(moduleKey)}: ${identifier},`)
    }
  }

  const bundleManifest = {
    formatVersion: CLIENT_BUNDLE_FORMAT_VERSION,
    extensions: entries.map(({ key, apiVersion, fingerprint, requires }) => ({
      key,
      apiVersion,
      fingerprint,
      ...(requires.length > 0 ? { requires } : {}),
    })),
  }
  const source = [
    '// Code generated by scripts/materialize-admin-extensions.cjs. DO NOT EDIT.',
    ...imports,
    '',
    'export const extensionPages = {',
    ...pages,
    '} as const',
    '',
    'export const extensionModules = {',
    ...modules,
    '} as const',
    '',
    `export const clientBundleManifest = ${JSON.stringify(bundleManifest, null, 2)} as const`,
    '',
  ].join('\n')

  return { bundleManifest, source }
}

const materializeAdminExtensions = ({
  generatedRoot,
  manifestPaths = [],
  legacyRoot,
  includeLegacy = false,
}) => {
  const entries = collectEntries({ manifestPaths, legacyRoot, includeLegacy })

  fs.rmSync(generatedRoot, { force: true, recursive: true })
  fs.mkdirSync(path.join(generatedRoot, 'roots'), { recursive: true })

  const { bundleManifest, source } = buildGeneratedModule(entries, generatedRoot)
  const generatedModulePath = path.join(generatedRoot, 'generated.ts')
  const bundleManifestPath = path.join(generatedRoot, 'admin-extensions.manifest.json')
  const tailwindSourcesPath = path.join(generatedRoot, 'tailwind-sources.css')
  fs.writeFileSync(generatedModulePath, source)
  fs.writeFileSync(bundleManifestPath, `${JSON.stringify(bundleManifest, null, 2)}\n`)
  fs.writeFileSync(
    tailwindSourcesPath,
    `${entries.map((entry) => `@source ${JSON.stringify(toPosix(entry.sourceRoot))};`).join('\n')}\n`,
  )

  return {
    bundleManifest,
    bundleManifestPath,
    generatedModulePath,
    generatedRoot,
    sourceRoots: entries.map((entry) => entry.sourceRoot),
    tailwindSourcesPath,
  }
}

if (require.main === module) {
  const {
    adminExtRoot,
    adminExtRootConfigured,
    adminExtensionManifestPaths,
    adminGeneratedRoot,
  } = require('./admin-paths.cjs')
  const result = materializeAdminExtensions({
    generatedRoot: adminGeneratedRoot,
    manifestPaths: adminExtensionManifestPaths,
    legacyRoot: adminExtRoot,
    includeLegacy: adminExtRootConfigured,
  })
  console.log(
    `admin extensions materialized: ${result.bundleManifest.extensions.length} -> ${result.generatedRoot}`,
  )
}

module.exports = {
  CLIENT_BUNDLE_FORMAT_VERSION,
  CLIENT_EXTENSION_API_VERSION,
  fingerprintSource,
  materializeAdminExtensions,
}
