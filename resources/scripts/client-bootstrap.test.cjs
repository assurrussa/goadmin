const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const test = require('node:test')

const appEntry = fs.readFileSync(path.join(__dirname, '..', 'src', 'js', 'app.ts'), 'utf8')

test('keeps the browser Inertia entrypoint on the client render path', () => {
  assert.match(appEntry, /createInertiaApp\(\{/)
  assert.match(appEntry, /app\.mount\(el\)/)
  assert.match(appEntry, /failed to bootstrap admin client/)
  assert.doesNotMatch(appEntry, /@vue\/server-renderer/)
  assert.doesNotMatch(appEntry, /\brender\s*:\s*renderToString\b/)
})

test('uses an explicit slot render function for the persistent admin layout', () => {
  assert.match(appEntry, /render\(AdminLayout, null, \{ default: \(\) => page \}\)/)
  assert.match(appEntry, /mod\.default\.layout = renderAdminLayout/)
  assert.doesNotMatch(appEntry, /mod\.default\.layout = AdminLayout as unknown as LayoutComponent/)
})
