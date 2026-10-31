const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const test = require('node:test')
const vm = require('node:vm')

const template = fs.readFileSync(path.join(__dirname, '../../views/app.gohtml'), 'utf8')
const script = [...template.matchAll(/<script>([\s\S]*?)<\/script>/g)]
  .map(match => match[1]).find(source => source.includes('goadmin.appearance.device.v1'))
assert.ok(script, 'real template must contain the early initializer')
function paint(props, device, systemDark = false) {
  const attrs = {}, classes = new Set()
  const root = { setAttribute: (k, v) => { attrs[k] = v },
    classList: { add: c => classes.add(c), remove: c => classes.delete(c) } }
  const reads = []
  vm.runInNewContext(script.replace('{{ marshal .page }}', JSON.stringify({ props })), {
    document: { documentElement: root }, window: { matchMedia: () => ({ matches: systemDark }) },
    localStorage: { getItem: key => { reads.push(key); return key === 'goadmin.appearance.device.v1' ? device : 'dark' } },
    console: { warn() {} },
  })
  return { attrs, classes, lang: root.lang, reads }
}
test('verified account preferences paint before mount without reading device choices', () => {
  const result = paint({ authuser: { id: 7 }, appearance: { owner: '7', payload: {
    theme: 'light', locale: 'en', density: 'compact' } } }, '{"theme":"dark"}', true)
  assert.equal(result.attrs['data-theme-mode'], 'light')
  assert.equal(result.attrs['data-density'], 'compact')
  assert.equal(result.lang, 'en')
  assert.deepEqual(result.reads, [])
})
test('missing, failed or wrong-owner private reads cannot import another browser choice', () => {
  for (const appearance of [null, { owner: '7', unavailable: true },
    { owner: '8', payload: { theme: 'dark', locale: 'en', density: 'compact' } }]) {
    const result = paint({ authuser: { id: 7 }, appearance }, '{"theme":"dark"}')
    assert.equal(result.attrs['data-theme-mode'], 'light')
    assert.equal(result.attrs['data-density'], 'default')
    assert.equal(result.lang, 'ru')
    assert.deepEqual(result.reads, [])
  }
})
test('anonymous mode uses only the separate device document and follows the system', () => {
  const result = paint({}, '{"theme":"system","locale":"en","density":"comfortable"}', true)
  assert.equal(result.attrs['data-theme-mode'], 'dark')
  assert.ok(result.classes.has('dark'))
  assert.equal(result.lang, 'en')
  assert.deepEqual(result.reads, ['goadmin.appearance.device.v1'])
})
test('malformed and unsupported device values retain safe initial defaults', () => {
  for (const raw of ['broken', '{"theme":"evil","density":"huge","locale":"other"}']) {
    const result = paint({}, raw)
    assert.equal(result.attrs['data-theme-mode'], 'light')
    assert.equal(result.attrs['data-density'], 'default')
    assert.equal(result.lang, 'ru')
  }
})
