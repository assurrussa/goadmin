const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const test = require('node:test')
const assert = require('node:assert/strict')

const { fingerprintSource, materializeAdminExtensions } = require('./materialize-admin-extensions.cjs')

const write = (file, content) => {
  fs.mkdirSync(path.dirname(file), { recursive: true })
  fs.writeFileSync(file, content)
}

const createManifest = (root, key, sourceRoot, options = {}) => {
  const manifestPath = path.join(root, `${key}.manifest.json`)
  write(
    manifestPath,
    `${JSON.stringify(
      {
        formatVersion: 1,
        extensions: [
          {
            key,
            apiVersion: 1,
            sourceRoot: path.relative(root, sourceRoot),
            fingerprint: options.fingerprint ?? fingerprintSource(sourceRoot),
            requires: options.requires ?? [],
            ...(options.themeModule !== undefined ? { themeModule: options.themeModule } : {}),
          },
        ],
      },
      null,
      2,
    )}\n`,
  )
  return manifestPath
}

const fixture = (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'goadmin-client-manifest-'))
  t.after(() => fs.rmSync(root, { force: true, recursive: true }))
  return root
}

test('materializes deterministic multi-root module and path-free bundle manifest', (t) => {
  const root = fixture(t)
  const contentRoot = path.join(root, 'content')
  const hostRoot = path.join(root, 'host')
  write(
    path.join(contentRoot, 'Pages/content/Index.vue'),
    '<script setup lang="ts">defineProps<{ apiBase: string }>()</script>\n<template>content</template>\n',
  )
  write(path.join(contentRoot, 'extensions/install.ts'), 'export default {}\n')
  write(path.join(hostRoot, 'Pages/settings/Index.vue'), '<template>settings</template>\n')

  const result = materializeAdminExtensions({
    generatedRoot: path.join(root, 'generated'),
    legacyRoot: path.join(root, 'missing-legacy'),
    manifestPaths: [
      createManifest(root, 'gocms', contentRoot),
      createManifest(root, 'host.settings', hostRoot, { requires: ['gocms'] }),
    ],
  })

  const generated = fs.readFileSync(result.generatedModulePath, 'utf8')
  assert.match(generated, /"content\/Index": \(\) => import\("\.\/roots\/gocms\/Pages\/content\/Index\.vue"\)/)
  assert.match(generated, /"settings\/Index": \(\) => import\("\.\/roots\/host\.settings\/Pages\/settings\/Index\.vue"\)/)
  assert.match(generated, /"gocms\/install": extension/)
  assert.match(generated, /export const selectedThemeModuleKey: string \| null = null/)
  assert.deepEqual(
    result.bundleManifest.extensions.map((extension) => extension.key),
    ['gocms', 'host.settings'],
  )

  const emitted = fs.readFileSync(result.bundleManifestPath, 'utf8')
  assert.equal(emitted.includes(root), false)

  const tailwindSources = fs.readFileSync(result.tailwindSourcesPath, 'utf8')
  assert.equal(
    tailwindSources,
    `@source ${JSON.stringify(contentRoot.split(path.sep).join('/'))};\n` +
      `@source ${JSON.stringify(hostRoot.split(path.sep).join('/'))};\n`,
  )
})

test('selects one existing theme module independently of manifest order without duplicate imports', (t) => {
  const root = fixture(t)
  const firstRoot = path.join(root, 'first')
  const themeRoot = path.join(root, 'theme')
  write(path.join(firstRoot, 'extensions/ordinary.ts'), 'export default {}\n')
  write(path.join(themeRoot, 'extensions/nested/theme.ts'), 'export default { theme: {} }\n')
  write(path.join(themeRoot, 'extensions/other.ts'), 'export default {}\n')
  const manifests = [
    createManifest(root, 'aaa', firstRoot),
    createManifest(root, 'zzz.theme', themeRoot, { themeModule: 'extensions/nested/theme.ts' }),
  ]
  const materialize = (manifestPaths, name) => materializeAdminExtensions({
    generatedRoot: path.join(root, name),
    legacyRoot: path.join(root, 'missing'),
    manifestPaths,
  })
  const first = materialize(manifests, 'generated-first')
  const second = materialize([...manifests].reverse(), 'generated-second')
  const generated = fs.readFileSync(first.generatedModulePath, 'utf8')
  assert.equal(generated, fs.readFileSync(second.generatedModulePath, 'utf8'))
  assert.match(generated, /export const selectedThemeModuleKey: string \| null = "zzz\.theme\/nested\/theme"/)
  assert.equal(generated.match(/import \* as .*extensions\/nested\/theme\.ts/g).length, 1)
  assert.match(generated, /"zzz\.theme\/nested\/theme": extension/)
  assert.deepEqual(first.bundleManifest, second.bundleManifest)
  assert.equal(JSON.stringify(first.bundleManifest).includes('themeModule'), false)
})

test('rejects multiple selected theme modules before replacing generated output', (t) => {
  const root = fixture(t)
  const sourceRoot = path.join(root, 'source')
  write(path.join(sourceRoot, 'extensions/theme.ts'), 'export default {}\n')
  const generatedRoot = path.join(root, 'generated')
  write(path.join(generatedRoot, 'sentinel'), 'preserved')
  assert.throws(() => materializeAdminExtensions({
    generatedRoot,
    legacyRoot: path.join(root, 'missing'),
    manifestPaths: ['first', 'second'].map((key) => createManifest(root, key, sourceRoot, {
      themeModule: 'extensions/theme.ts',
    })),
  }), /multiple admin theme modules selected: first, second/)
  assert.equal(fs.readFileSync(path.join(generatedRoot, 'sentinel'), 'utf8'), 'preserved')
})

test('rejects unsafe or unsupported theme module paths and missing modules', (t) => {
  const root = fixture(t)
  const sourceRoot = path.join(root, 'source')
  write(path.join(sourceRoot, 'extensions/theme.ts'), 'export default {}\n')
  for (const themeModule of [
    null, 42, '', '/extensions/theme.ts', 'C:\\extensions\\theme.ts',
    '../extensions/theme.ts', 'extensions/../theme.ts', 'extensions/./theme.ts',
    'extensions//theme.ts', 'extensions\\theme.ts', 'theme.ts',
    'extensions/theme.js', 'extensions/theme.d.ts', 'extensions/theme\0.ts',
  ]) {
    assert.throws(() => materializeAdminExtensions({
      generatedRoot: path.join(root, 'generated'),
      legacyRoot: path.join(root, 'missing'),
      manifestPaths: [createManifest(root, 'theme', sourceRoot, { themeModule })],
    }), /themeModule must be a relative .ts module under extensions\//, String(themeModule))
  }
  for (const themeModule of ['extensions/missing.ts', 'extensions/directory.ts']) {
    fs.mkdirSync(path.join(sourceRoot, 'extensions/directory.ts'), { recursive: true })
    assert.throws(() => materializeAdminExtensions({
      generatedRoot: path.join(root, 'generated'),
      legacyRoot: path.join(root, 'missing'),
      manifestPaths: [createManifest(root, 'theme', sourceRoot, { themeModule })],
    }), /themeModule does not exist or is not a file/)
  }
})

test('rejects theme module symlinks including a symlinked ancestor', (t) => {
  const root = fixture(t)
  const sourceRoot = path.join(root, 'source')
  write(path.join(sourceRoot, 'extensions/real/theme.ts'), 'export default {}\n')
  fs.symlinkSync('real/theme.ts', path.join(sourceRoot, 'extensions/alias.ts'))
  fs.symlinkSync('real', path.join(sourceRoot, 'extensions/alias-dir'))
  for (const themeModule of ['extensions/alias.ts', 'extensions/alias-dir/theme.ts']) {
    assert.throws(() => materializeAdminExtensions({
      generatedRoot: path.join(root, 'generated'),
      legacyRoot: path.join(root, 'missing'),
      manifestPaths: [createManifest(root, 'theme', sourceRoot, {
        themeModule,
        fingerprint: `sha256:${'0'.repeat(64)}`,
      })],
    }), /themeModule must not contain symlinks/)
  }
})

test('legacy extension modules keep working without selecting a theme', (t) => {
  const root = fixture(t)
  write(path.join(root, 'src/js/extensions/theme.ts'), 'export default {}\n')
  const result = materializeAdminExtensions({ generatedRoot: path.join(root, 'generated'), legacyRoot: root })
  const generated = fs.readFileSync(result.generatedModulePath, 'utf8')
  assert.match(generated, /"host\/theme": extension/)
  assert.match(generated, /export const selectedThemeModuleKey: string \| null = null/)
  assert.equal(result.bundleManifest.extensions[0].key, 'host')
})

test('rejects duplicate extension and page keys before Vite starts', (t) => {
  const root = fixture(t)
  const firstRoot = path.join(root, 'first')
  const secondRoot = path.join(root, 'second')
  write(path.join(firstRoot, 'Pages/content/Index.vue'), '<template>first</template>\n')
  write(path.join(secondRoot, 'Pages/content/Index.vue'), '<template>second</template>\n')

  const first = createManifest(root, 'gocms', firstRoot)
  const duplicateKey = createManifest(root, 'gocms-copy', secondRoot)
  const duplicateKeyBody = JSON.parse(fs.readFileSync(duplicateKey, 'utf8'))
  duplicateKeyBody.extensions[0].key = 'gocms'
  write(duplicateKey, `${JSON.stringify(duplicateKeyBody, null, 2)}\n`)

  assert.throws(
    () =>
      materializeAdminExtensions({
        generatedRoot: path.join(root, 'duplicate-key'),
        legacyRoot: path.join(root, 'missing-legacy'),
        manifestPaths: [first, duplicateKey],
      }),
    /duplicate admin extension key/,
  )

  assert.throws(
    () =>
      materializeAdminExtensions({
        generatedRoot: path.join(root, 'duplicate-page'),
        legacyRoot: path.join(root, 'missing-legacy'),
        manifestPaths: [first, createManifest(root, 'host.content', secondRoot)],
      }),
    /duplicate admin page key/,
  )
})

test('rejects invalid dependency graphs and fingerprint drift', (t) => {
  const root = fixture(t)
  const sourceRoot = path.join(root, 'content')
  write(path.join(sourceRoot, 'Pages/content/Index.vue'), '<template>content</template>\n')

  assert.throws(
    () =>
      materializeAdminExtensions({
        generatedRoot: path.join(root, 'missing-required'),
        legacyRoot: path.join(root, 'missing-legacy'),
        manifestPaths: [createManifest(root, 'gocms', sourceRoot, { requires: ['host.missing'] })],
      }),
    /requires missing module/,
  )

  assert.throws(
    () =>
      materializeAdminExtensions({
        generatedRoot: path.join(root, 'cycle'),
        legacyRoot: path.join(root, 'missing-legacy'),
        manifestPaths: [
          createManifest(root, 'gocms', sourceRoot, { requires: ['host.cycle'] }),
          createManifest(root, 'host.cycle', sourceRoot, { requires: ['gocms'] }),
        ],
      }),
    /dependency cycle/,
  )

  assert.throws(
    () =>
      materializeAdminExtensions({
        generatedRoot: path.join(root, 'duplicate-dependency'),
        legacyRoot: path.join(root, 'missing-legacy'),
        manifestPaths: [
          createManifest(root, 'gocms', sourceRoot, {
            requires: ['goadmin.core', 'goadmin.core'],
          }),
        ],
      }),
    /duplicate dependency/,
  )

  assert.throws(
    () =>
      materializeAdminExtensions({
        generatedRoot: path.join(root, 'drift'),
        legacyRoot: path.join(root, 'missing-legacy'),
        manifestPaths: [
          createManifest(root, 'gocms-drift', sourceRoot, {
            fingerprint: `sha256:${'0'.repeat(64)}`,
          }),
        ],
      }),
    /fingerprint mismatch/,
  )
})

test('materializes the opt-in CSS-only shell theme with its verified fingerprint', (t) => {
  const generatedRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'goadmin-shell-theme-'))
  t.after(() => fs.rmSync(generatedRoot, { recursive: true, force: true }))
  const exampleRoot = path.resolve(__dirname, '../examples/theme-shell')
  const manifestPath = path.join(exampleRoot, 'extension.json')
  const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8')).extensions[0]
  assert.equal(manifest.fingerprint, fingerprintSource(path.join(exampleRoot, 'src')))
  const result = materializeAdminExtensions({ generatedRoot, manifestPaths: [manifestPath] })
  assert.equal(result.bundleManifest.extensions.length, 1)
  assert.equal(result.bundleManifest.extensions[0].key, 'example.shell-theme')
  assert.match(fs.readFileSync(result.generatedModulePath, 'utf8'), /import \* as extension0.*extensions\/theme\.ts/)
  const source = fs.readFileSync(path.join(exampleRoot, 'src/extensions/theme.ts'), 'utf8')
  assert.match(source, /import '\.\.\/theme\.css'/)
  assert.match(source, /export default \{\}/)
})
