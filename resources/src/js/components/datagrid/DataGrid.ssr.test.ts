// @vitest-environment node
import { describe, expect, it, vi } from 'vitest'
import { createSSRApp } from 'vue'
import { renderToString } from '@vue/server-renderer'
import DataGrid from './DataGrid.vue'

describe('server-only DataGrid rendering without browser globals', () => {
  it('renders initial rows without starting transport', async () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    try {
      const html = await renderToString(
        createSSRApp(DataGrid, {
          apiUrl: '/users',
          initialData: {
            data: [{ item: { id: 1, name: 'Server-only row' }, actions: [] }],
            config: { columns: [{ key: 'name', label: 'Name', title: 'Name' }] },
          },
        }),
      )
      expect(html).toContain('Server-only row')
      expect(fetch).not.toHaveBeenCalled()
    } finally {
      vi.unstubAllGlobals()
    }
  })
  it('does not fetch on the server when initial data is unavailable', async () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    try {
      await renderToString(createSSRApp(DataGrid, { apiUrl: '/users' }))
      expect(fetch).not.toHaveBeenCalled()
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
