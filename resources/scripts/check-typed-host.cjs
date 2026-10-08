const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { spawnSync } = require('node:child_process')

// Exercise the actual generated extension registry and the complete resource
// build graph, including app.ts. Never narrow the host's type-check include list.
const resources = path.resolve(__dirname, '..')
const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'goadmin-typed-host-'))
const fixture = path.join(temporary, 'resources')
const host = path.join(temporary, 'host')
const pages = path.join(host, 'src/js/Pages')
const run = (args) => {
  const result = spawnSync(process.execPath, args, {
    cwd: fixture,
    encoding: 'utf8',
    env: {
      ...process.env,
      VITE_ADMIN_EXT_ROOT: host,
      VITE_ADMIN_EXTENSION_MANIFESTS: '',
      VITE_ADMIN_TSCONFIG_PATH: '',
    },
  })
  assert.ifError(result.error)
  return { status: result.status, output: `${result.stdout}${result.stderr}` }
}
const check = () =>
  run([
    path.join(resources, 'node_modules/vue-tsc/bin/vue-tsc.js'),
    '--build',
    '--force',
    '--pretty',
    'false',
  ])

try {
  fs.cpSync(resources, fixture, {
    recursive: true,
    filter: (source) =>
      !['node_modules', '.admin-extensions', 'dist', 'coverage'].includes(path.basename(source)),
  })
  fs.symlinkSync(
    path.join(resources, 'node_modules'),
    path.join(fixture, 'node_modules'),
    process.platform === 'win32' ? 'junction' : 'dir',
  )
  // Keep compiler state local even though installed dependencies are shared.
  for (const name of ['paths', 'node', 'vitest']) {
    const configPath = path.join(fixture, `tsconfig.${name}.json`)
    const config = JSON.parse(fs.readFileSync(configPath, 'utf8'))
    config.compilerOptions.tsBuildInfoFile = `./.typecheck-cache/${name}.tsbuildinfo`
    fs.writeFileSync(configPath, JSON.stringify(config, null, 2))
  }
  fs.mkdirSync(pages, { recursive: true })
  // Keep the positive registry to ONE required-prop page: a prop-free union
  // member can make the original tuple assertion silently accept this mismatch.
  fs.writeFileSync(
    path.join(pages, 'TypedHostPage.vue'),
    `<script setup lang="ts">\ndefineProps<{ title: string }>()\n</script>\n<template><h1>{{ title.toUpperCase() }}</h1></template>\n`,
  )
  const consumer = path.join(pages, 'TypedHostUsage.vue')
  const usage = (template) =>
    `<script setup lang="ts">\nimport TypedHostPage from './TypedHostPage.vue'\n</script>\n<template>${template}</template>\n`

  const generated = run([path.join(fixture, 'scripts/generate-tsconfig-paths.cjs')])
  assert.equal(generated.status, 0, generated.output)
  const registry = fs.readFileSync(path.join(fixture, '.admin-extensions/generated.ts'), 'utf8')
  assert.match(registry, /"TypedHostPage": \(\) => import/)

  const positive = check()
  assert.equal(positive.status, 0, `Full typed-host resource check failed:\n${positive.output}`)

  // These errors must remain visible in the generated host, without ts-ignore,
  // expected-error directives, weakened Vue shims, or erased prop declarations.
  fs.writeFileSync(
    consumer,
    usage('<TypedHostPage title="Typed host" /><TypedHostPage :title="42" /><TypedHostPage />'),
  )
  fs.writeFileSync(
    path.join(fixture, 'src/js/invalid-host-page.ts'),
    `import type { PageLoader } from './inertiaPage'\nexport const invalidPage: PageLoader = async () => ({ default: 42 })\n`,
  )
  const regenerated = run([path.join(fixture, 'scripts/generate-tsconfig-paths.cjs')])
  assert.equal(regenerated.status, 0, regenerated.output)
  const negative = check()
  assert.notEqual(
    negative.status,
    0,
    'Incorrect and missing required host props unexpectedly passed',
  )
  assert.match(
    negative.output,
    /TypedHostUsage\.vue.*error TS2322: Type 'number' is not assignable to type 'string'/,
  )
  assert.match(negative.output, /Property 'title' is missing/)
  assert.match(negative.output, /invalid-host-page\.ts.*error TS2322/)
  const diagnostics = negative.output.split('\n').filter((line) => /error TS\d+:/.test(line))
  assert.equal(diagnostics.length, 3, negative.output)
  assert.doesNotMatch(negative.output, /src\/js\/app\.ts.*error TS/)
  console.log(
    'Typed-host full resource graph passes; wrong and missing required props are rejected.',
  )
} finally {
  fs.rmSync(temporary, { recursive: true, force: true })
}
