import { expect, it } from 'vitest'
import { createApp, nextTick } from 'vue'
import { performance } from 'node:perf_hooks'
import { appendFileSync } from 'node:fs'
import os from 'node:os'
import DataGrid from '../../src/js/components/datagrid/DataGrid.vue'
import { MockGridHost, makeRows } from './fixture'
it('records one isolated cold mount (not a browser performance claim)', async () => {
  const count = Number(process.env.AUDIT_ROWS || 100)
  const host = new MockGridHost()
  host.rows = makeRows(count)
  host.pageSize = count
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp(DataGrid, {
    apiUrl: '/audit',
    initialData: host.response(),
    syncWithUrl: false,
  })
  let warnings = 0
  app.config.warnHandler = () => {
    warnings++
  }
  const start = performance.now()
  app.mount(root)
  await nextTick()
  const milliseconds = performance.now() - start
  const rows = root.querySelectorAll('tbody tr').length
  const result = {
    scope:
      'single cold Vue mount + nextTick, JSDOM, full nine-column mock fixture; excludes transform/setup/layout/paint/network; shared VM noisy',
    count,
    milliseconds: Math.round(milliseconds * 100) / 100,
    rows,
    elements: root.querySelectorAll('*').length,
    warnings,
    node: process.version,
    arch: process.arch,
    cpu: os.cpus()[0]?.model,
  }
  expect(rows).toBe(count)
  appendFileSync('../audit/datagrid/dom-single-benchmark.jsonl', JSON.stringify(result) + '\n')
  console.log(JSON.stringify(result))
  app.unmount()
  root.remove()
})
