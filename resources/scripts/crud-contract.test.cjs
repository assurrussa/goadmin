const fs = require('node:fs')
const path = require('node:path')
const os = require('node:os')
const assert = require('node:assert/strict')
const test = require('node:test')
const { crudFingerprint } = require('./crud-contract.cjs')
test('CRUD renderer fingerprint is deterministic and matches the Go requirement when server source is present', () => {
 const fingerprint=crudFingerprint()
 assert.match(fingerprint,/^sha256:[a-f0-9]{64}$/)
 assert.equal(fingerprint,crudFingerprint())
 const source=path.join(__dirname,'../../features/crud/client_generated.go')
 if(fs.existsSync(source)) assert.ok(fs.readFileSync(source,'utf8').includes(JSON.stringify(fingerprint)))
})
test('shared CRUD renderer changes invalidate the fingerprint while tests do not', (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'goadmin-crud-contract-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  const sources = [
    'src/js/crud/index.ts',
    'src/js/Pages/crud/Index.vue',
    'src/js/components/date/LocalizedDatePicker.vue',
    'src/js/components/confirmation/confirmation.ts',
    'src/js/components/datagrid/DataGridTable.vue',
    'src/js/composables/useDataGrid.ts',
    'src/js/composables/useDataGridWorkflow.ts',
    'src/js/composables/useRecordNavigation.ts',
    'src/js/lib/recordFieldValue.ts',
  ]
  for (const source of sources) {
    const file = path.join(root, source)
    fs.mkdirSync(path.dirname(file), { recursive: true })
    fs.writeFileSync(file, 'v1')
  }
  const initial = crudFingerprint(root)
  fs.writeFileSync(path.join(root, 'src/js/components/datagrid/DataGrid.test.ts'), 'test')
  assert.equal(crudFingerprint(root), initial)
  for (const source of sources) {
    const file = path.join(root, source)
    fs.writeFileSync(file, 'v2')
    assert.notEqual(crudFingerprint(root), initial, source)
    fs.writeFileSync(file, 'v1')
  }
})
