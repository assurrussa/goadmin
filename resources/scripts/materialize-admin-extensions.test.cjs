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
