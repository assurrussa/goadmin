import { describe, expect, it } from 'vitest'
import { createApp, nextTick } from 'vue'
import { performance } from 'node:perf_hooks'
import os from 'node:os'
import { writeFileSync } from 'node:fs'
import DataGrid from '../../src/js/components/datagrid/DataGrid.vue'
import { MockGridHost, makeRows } from './fixture'

describe('bounded JSDOM mount benchmark (not browser paint or API latency)', () => {
  it('records 100 and 1000 fully rendered row cases', async () => {
    const samples = []
    for (const count of [100, 1000]) {
      for (let run = 0; run < 3; run++) {
        const host = new MockGridHost()
        host.rows = makeRows(count)
        host.pageSize = count
        const response = host.response()
        const root = document.createElement('div')
        document.body.append(root)
        const app = createApp(DataGrid, {
          apiUrl: '/audit',
          initialData: response,
          syncWithUrl: false,
        })
        const warnings: string[] = []
        app.config.warnHandler = (message) => warnings.push(message)
        console.log('benchmark.start', count, run)
        const start = performance.now()
        app.mount(root)
        await nextTick()
        const milliseconds = performance.now() - start
        console.log('benchmark.mounted', count, run, milliseconds)
        const rows = root.querySelectorAll('tbody tr').length
        const elements = root.querySelectorAll('*').length
        expect(rows).toBe(count)
        if (run > 0)
          samples.push({
            count,
            run,
            milliseconds: Math.round(milliseconds * 100) / 100,
            rows,
            elements,
            vueWarnings: warnings.length,
          })
        app.unmount()
        root.remove()
      }
    }
    const result = {
      scope:
        'Vue createApp mount + nextTick using real DataGrid and full 9-column fixture; JSDOM; no layout, paint, network or database; first sample per size excluded as warmup; virtualized shared runner results are noisy',
      node: process.version,
      vue: 'package-lock.json',
      platform: process.platform,
      arch: process.arch,
      cpu: os.cpus()[0]?.model,
      cpuCount: os.cpus().length,
      samples,
    }
    writeFileSync('../audit/datagrid/dom-benchmark.json', JSON.stringify(result, null, 2) + '\n')
    console.log(JSON.stringify(result))
  })
})
