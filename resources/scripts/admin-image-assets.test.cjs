const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const http = require('node:http')
const test = require('node:test')
const assert = require('node:assert/strict')
const { adminImageAssets, collectImageAssets } = require('./admin-image-assets.cjs')

const write = (file, content) => {
  fs.mkdirSync(path.dirname(file), { recursive: true })
  fs.writeFileSync(file, content)
}
const fixture = (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'goadmin-image-assets-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  write(path.join(root, 'src/images/favicon.ico'), Buffer.from([0, 1, 2, 255]))
  return root
}
const image = (root, name) => path.join(root, 'src/images', name)

test('discovers hidden, nested and flattened legacy aliases without changing bytes', (t) => {
  const root = fixture(t)
  write(image(root, 'one/two/deep.png'), 'deep')
  write(image(root, '.hidden.svg'), '<svg/>')
  const assets = collectImageAssets(root)
  assert.deepEqual([...assets.keys()].sort(), [
    'favicon.ico',
    'images/.hidden.svg',
    'images/deep.png',
    'images/favicon.ico',
    'images/one/two/deep.png',
    'images/two/deep.png',
  ])
  for (const alias of ['images/one/two/deep.png', 'images/two/deep.png', 'images/deep.png']) {
    assert.equal(fs.readFileSync(assets.get(alias), 'utf8'), 'deep')
  }
  assert.deepEqual(fs.readFileSync(assets.get('favicon.ico')), Buffer.from([0, 1, 2, 255]))
})

test('rejects collisions, missing favicon, symlinks and unsafe path names', (t) => {
  const root = fixture(t)
  write(image(root, 'a/logo.png'), 'first')
  write(image(root, 'b/logo.png'), 'second')
  assert.throws(() => collectImageAssets(root), /ambiguous admin image output/)
  fs.rmSync(image(root, 'b'), { recursive: true })
  for (const target of [image(root, 'favicon.ico'), path.join(root, 'secret.txt')]) {
    write(target, 'private')
    fs.symlinkSync(target, image(root, 'link.png'))
    assert.throws(() => collectImageAssets(root), /unsupported admin image path/)
    fs.unlinkSync(image(root, 'link.png'))
  }
  fs.symlinkSync(path.join(root, 'src'), image(root, 'linked-directory'))
  assert.throws(() => collectImageAssets(root), /unsupported admin image path/)
  fs.unlinkSync(image(root, 'linked-directory'))
  write(image(root, 'bad\\name.png'), 'bad')
  assert.throws(() => collectImageAssets(root), /unsupported admin image path/)
  fs.unlinkSync(image(root, 'bad\\name.png'))
  fs.unlinkSync(image(root, 'favicon.ico'))
  assert.throws(() => collectImageAssets(root), /require src\/images\/favicon.ico/)
})

test('rejects a symlinked image root or parent instead of walking another tree', (t) => {
  const root = fixture(t)
  fs.renameSync(path.join(root, 'src'), path.join(root, 'outside'))
  fs.symlinkSync(path.join(root, 'outside'), path.join(root, 'src'))
  assert.throws(() => collectImageAssets(root), /must not use symlinks/)
})

test('Vite emits legacy aliases into its configured output root', async (t) => {
  const { build } = await import('vite')
  const root = fixture(t)
  write(image(root, 'one/two/deep.png'), 'deep')
  write(path.join(root, 'entry.js'), 'export const marker = 1')
  const outDir = path.join(root, 'custom-output/dist')
  await build({
    configFile: false,
    root,
    logLevel: 'silent',
    publicDir: false,
    plugins: [adminImageAssets(root)],
    build: { outDir, rollupOptions: { input: path.join(root, 'entry.js') } },
  })
  for (const [name, source] of collectImageAssets(root)) {
    assert.deepEqual(fs.readFileSync(path.join(outDir, name)), fs.readFileSync(source))
  }
})

test('build refuses output collisions with another Vite asset', (t) => {
  const root = fixture(t)
  const plugin = adminImageAssets(root)
  assert.throws(
    () => plugin.generateBundle.call({ emitFile() {} }, {}, { 'images/favicon.ico': {} }),
    /ambiguous admin image output/,
  )
})

const request = (server, requestPath, method = 'GET', headers = {}) =>
  new Promise((resolve, reject) => {
    const req = http.request(
      {
        host: '127.0.0.1',
        port: server.httpServer.address().port,
        path: requestPath,
        method,
        headers,
      },
      (res) => {
        const chunks = []
        res.on('data', (chunk) => chunks.push(chunk))
        res.on('end', () =>
          resolve({ status: res.statusCode, headers: res.headers, body: Buffer.concat(chunks) }),
        )
      },
    )
    req.on('error', reject)
    req.end()
  })

test('Vite serves GET/HEAD, query strings, additions, changes and deletions without stale maps', async (t) => {
  const { createServer } = await import('vite')
  const root = fixture(t)
  const server = await createServer({
    configFile: false,
    root,
    logLevel: 'silent',
    publicDir: false,
    plugins: [adminImageAssets(root)],
    server: { host: '127.0.0.1', port: 0 },
  })
  t.after(() => server.close())
  await server.listen()
  for (const name of ['/favicon.ico', '/images/favicon.ico']) {
    const get = await request(server, `${name}?v=1`)
    assert.equal(get.status, 200)
    assert.deepEqual(get.body, fs.readFileSync(image(root, 'favicon.ico')))
    assert.match(get.headers['content-type'], /image/)
    const head = await request(server, name, 'HEAD')
    assert.equal(head.status, 200)
    assert.equal(head.body.length, 0)
    assert.equal(head.headers['content-length'], get.headers['content-length'])
    assert.equal(
      (await request(server, name, 'GET', { 'if-none-match': get.headers.etag })).status,
      304,
    )
  }
  write(image(root, 'nested/новое image.png'), 'first')
  const alias = '/images/' + encodeURIComponent('новое image.png')
  assert.equal((await request(server, alias)).body.toString(), 'first')
  write(image(root, 'nested/новое image.png'), 'changed content')
  assert.equal((await request(server, alias)).body.toString(), 'changed content')
  fs.unlinkSync(image(root, 'nested/новое image.png'))
  assert.equal((await request(server, alias)).status, 404)
  fs.symlinkSync(path.join(root, 'private.txt'), image(root, 'bad.png'))
  assert.equal((await request(server, '/favicon.ico')).status, 500)
  fs.unlinkSync(image(root, 'bad.png'))
  assert.equal((await request(server, '/favicon.ico')).status, 200)
  write(image(root, 'duplicate/favicon.ico'), 'collision')
  assert.equal((await request(server, '/favicon.ico')).status, 500)
  fs.rmSync(image(root, 'duplicate'), { recursive: true })
  assert.equal((await request(server, '/favicon.ico')).status, 200)
})

test('image aliases cannot traverse roots or bypass Vite filesystem deny rules', async (t) => {
  const { createServer } = await import('vite')
  const root = fixture(t)
  write(path.join(root, 'secret.txt'), 'never-return-this-secret')
  write(image(root, '.env'), 'never-return-this-secret')
  const server = await createServer({
    configFile: false,
    root,
    logLevel: 'silent',
    publicDir: false,
    plugins: [adminImageAssets(root)],
    server: { host: '127.0.0.1', port: 0, fs: { allow: [root] } },
  })
  t.after(() => server.close())
  await server.listen()
  for (const url of [
    '/images/../secret.txt',
    '/images/%2e%2e/secret.txt',
    '/images/%2e%2e%2fsecret.txt',
    '/images/..%5csecret.txt',
    '/images/%252e%252e/secret.txt',
    '/images/%00/favicon.ico',
    '/images/.env',
  ]) {
    const response = await request(server, url)
    assert(response.status >= 400, `${url}: ${response.status}`)
    assert(!response.body.toString().includes('never-return-this-secret'), url)
  }
  assert.equal((await request(server, '/images/unknown.png')).status, 404)
})


test('trusted symlinked checkout ancestors preserve collection, build and dev aliases', async (t) => {
  const { build, createServer } = await import('vite')
  const root = fixture(t)
  const holder = fs.mkdtempSync(path.join(os.tmpdir(), 'goadmin-image-alias-'))
  t.after(() => fs.rmSync(holder, { recursive: true, force: true }))
  const ancestor = path.join(holder, 'ancestor')
  fs.symlinkSync(path.dirname(root), ancestor, 'dir')
  const aliasRoot = path.join(ancestor, path.basename(root))
  assert.deepEqual(collectImageAssets(aliasRoot), collectImageAssets(root))
  write(path.join(root, 'entry.js'), 'export const marker = 1')
  const outDir = path.join(root, 'alias-output')
  await build({
    configFile: false,
    root: aliasRoot,
    logLevel: 'silent',
    publicDir: false,
    plugins: [adminImageAssets(aliasRoot)],
    build: { outDir, rollupOptions: { input: path.join(aliasRoot, 'entry.js') } },
  })
  for (const [name, source] of collectImageAssets(aliasRoot)) {
    assert.deepEqual(fs.readFileSync(path.join(outDir, name)), fs.readFileSync(source))
  }
  const server = await createServer({
    configFile: false,
    root: aliasRoot,
    logLevel: 'silent',
    publicDir: false,
    plugins: [adminImageAssets(aliasRoot)],
    server: { host: '127.0.0.1', port: 0, fs: { allow: [aliasRoot] } },
  })
  t.after(() => server.close())
  await server.listen()
  for (const name of ['/favicon.ico', '/images/favicon.ico']) {
    const get = await request(server, `${name}?version=alias`)
    assert.equal(get.status, 200)
    assert.deepEqual(get.body, fs.readFileSync(image(root, 'favicon.ico')))
    const head = await request(server, name, 'HEAD')
    assert.equal(head.status, 200)
    assert.equal(head.body.length, 0)
    assert.equal(head.headers['content-length'], get.headers['content-length'])
  }
  const privateFile = path.join(root, 'private.txt')
  write(privateFile, 'never-return-this-secret')
  fs.symlinkSync(privateFile, image(root, 'in-tree-link.png'))
  assert.throws(() => collectImageAssets(aliasRoot), /unsupported admin image path/)
  const denied = await request(server, '/favicon.ico')
  assert.equal(denied.status, 500)
  assert(!denied.body.toString().includes('never-return-this-secret'))
  fs.unlinkSync(image(root, 'in-tree-link.png'))
  assert.equal((await request(server, '/favicon.ico')).status, 200)
})
